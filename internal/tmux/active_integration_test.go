//go:build tmuxintegration

// Package tmux integration tests verify the real tmux server invariants that
// the production code relies on. They are gated behind the tmuxintegration
// build tag and require a tmux >= 3.2 binary on PATH. Each test owns an
// isolated socket so parallel runs never collide, and no test mutates
// production behaviour — these are pure observers of tmux semantics.
package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	harnessCmdTimeout = 10 * time.Second
	hookPollInterval  = 5 * time.Millisecond
	hookPollTimeout   = 3 * time.Second
)

// testHarness owns an isolated tmux server reachable through a named socket.
type testHarness struct {
	socket string
	t      *testing.T
}

// socketCounter guarantees a unique socket per harness even when several tests
// in the same process share a PID and identical sanitised names.
var socketCounter atomic.Uint64

// newHarness starts a fresh harness with a collision-proof socket name.
func newHarness(t *testing.T) *testHarness {
	t.Helper()
	return newHarnessWithSocket(t, socketName(t))
}

// newHarnessWithSocket binds the harness to an explicit socket name. It is the
// restart seam: a killed server can be revived on the same socket to prove the
// socket path is stable while the server identity changes.
func newHarnessWithSocket(t *testing.T, socket string) *testHarness {
	t.Helper()
	h := &testHarness{socket: socket, t: t}
	t.Cleanup(func() { _ = h.close() })
	return h
}

// socketName builds "tmh-<pid>-<sanitised-test-name>-<monotonic-counter>".
// PID + counter make collisions impossible within one process and extremely
// unlikely across processes; sanitisation keeps the name socket-safe.
func socketName(t *testing.T) string {
	n := socketCounter.Add(1)
	return fmt.Sprintf("tmh-%d-%s-%d", os.Getpid(), sanitizeForSocket(t.Name()), n)
}

func sanitizeForSocket(s string) string {
	out := strings.NewReplacer("/", "_", " ", "_", ".", "_", ":", "_").Replace(s)
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// close terminates the owned tmux server. tmux leaves the UNIX socket file on
// disk after kill-server (verified locally on tmux 3.6b), so the exact socket
// path is queried from the live server first and unlinked afterwards. Errors
// are ignored: the server may already be gone (e.g. after an explicit
// kill-server in a restart test) and a second close() is a safe no-op.
func (h *testHarness) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), harnessCmdTimeout)
	defer cancel()
	path := h.trySocketPath()
	_ = exec.CommandContext(ctx, "tmux", "-L", h.socket, "kill-server").Run()
	if path != "" {
		_ = os.Remove(path)
	}
	return nil
}

// trySocketPath returns the real on-disk socket path from the live server, or
// "" if the server is not reachable (already killed). Used by close() to unlink
// the stale socket file tmux leaves behind.
func (h *testHarness) trySocketPath() string {
	out, err := h.run("display-message", "-p", "#{socket_path}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// run executes a tmux command against the harness socket and returns combined
// output with any error.
func (h *testHarness) run(args ...string) ([]byte, error) {
	full := append([]string{"-L", h.socket, "-f", "/dev/null"}, args...)
	return exec.Command("tmux", full...).CombinedOutput()
}

// runMust executes a tmux command and fails the test on any error.
func (h *testHarness) runMust(args ...string) []byte {
	out, err := h.run(args...)
	if err != nil {
		h.t.Fatalf("tmux %v: %v\n%s", args, err, out)
	}
	return out
}

// winRow is one row from list-windows with index, name and stable @N id.
type winRow struct {
	index string
	name  string
	id    string
}

// parseWindows turns list-windows formatted output into structured rows.
func parseWindows(out []byte) []winRow {
	var rows []winRow
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 {
			rows = append(rows, winRow{index: f[0], name: f[1], id: f[2]})
		}
	}
	return rows
}

// listWindows returns structured window rows for a target session/window.
func (h *testHarness) listWindows(target string) []winRow {
	out := h.runMust("list-windows", "-t", target, "-F", "#{window_index} #{window_name} #{window_id}")
	return parseWindows(out)
}

// windowID resolves the stable @N id of a named window in a session.
func (h *testHarness) windowID(session, name string) string {
	for _, r := range h.listWindows(session) {
		if r.name == name {
			return r.id
		}
	}
	h.t.Fatalf("window %q not found in session %q", name, session)
	return ""
}

// newWindowAfter creates a detached window named newName immediately after the
// window identified by prev (a "session:window" target). Using -a (append)
// gives deterministic indices and avoids the renumber race that plain
// "new-window -d" hits once a detached session's active pointer drifts.
func (h *testHarness) newWindowAfter(prevTarget, newName string) {
	h.runMust("new-window", "-a", "-d", "-t", prevTarget, "-n", newName)
}

// linkTo links an existing window id into a session, failing the test on error.
func (h *testHarness) linkTo(windowID, destSession string) {
	h.runMust("link-window", "-d", "-s", windowID, "-t", destSession)
}

// windowPresent reports whether a window with the given stable id is still
// linked into the session.
func windowPresent(rows []winRow, id string) bool {
	for _, r := range rows {
		if r.id == id {
			return true
		}
	}
	return false
}

// assertWindowAt fails the test unless the window with the given stable id is
// linked into the session at exactly wantIndex.
func assertWindowAt(t *testing.T, rows []winRow, id, wantIndex string) {
	t.Helper()
	for _, r := range rows {
		if r.id == id {
			if r.index != wantIndex {
				t.Errorf("window %s at index %s, want %s", id, r.index, wantIndex)
			}
			return
		}
	}
	t.Errorf("window %s not linked into session (want index %s)", id, wantIndex)
}

// serverEpoch is the (socket_path, start_time, pid) triple that uniquely
// identifies a tmux server instance for a given socket.
type serverEpoch struct {
	socketPath string
	startTime  string
	pid        string
}

// queryEpoch reads the server identity triple via a single display-message.
func (h *testHarness) queryEpoch() serverEpoch {
	format := strings.Join([]string{"#{socket_path}", "#{start_time}", "#{pid}"}, epochTransportSep)
	out := h.runMust("display-message", "-p", format)
	epoch, err := parseServerEpoch(out)
	if err != nil {
		h.t.Fatalf("parse epoch output %q: %v", out, err)
	}
	parts := strings.Split(epoch.Value, sep)
	if len(parts) != 3 {
		h.t.Fatalf("unexpected canonical epoch %q: want 3 fields", epoch.Value)
	}
	return serverEpoch{socketPath: parts[0], startTime: parts[1], pid: parts[2]}
}

// assertEpochValid fails the test if any epoch field is empty or the numeric
// fields are not parseable as integers.
func assertEpochValid(t *testing.T, e serverEpoch) {
	t.Helper()
	if e.socketPath == "" {
		t.Error("epoch socket_path is empty")
	}
	if e.startTime == "" {
		t.Error("epoch start_time is empty")
	}
	if _, err := strconv.ParseInt(e.startTime, 10, 64); err != nil {
		t.Errorf("epoch start_time %q not an int: %v", e.startTime, err)
	}
	if e.pid == "" {
		t.Error("epoch pid is empty")
	}
	if _, err := strconv.Atoi(e.pid); err != nil {
		t.Errorf("epoch pid %q not an int: %v", e.pid, err)
	}
}

// waitForLineCount polls a file until it contains at least want non-empty
// lines, returning exactly the first want lines. run-shell (used by tmux
// hooks) is asynchronous, so a hook that writes to a marker file needs bounded
// polling rather than a fixed sleep. The ticker is a poll mechanism, not a
// synchronisation delay.
func waitForLineCount(t *testing.T, path string, want int) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), hookPollTimeout)
	defer cancel()
	ticker := time.NewTicker(hookPollInterval)
	defer ticker.Stop()
	for {
		if got := readNLines(path, want); got != nil {
			return got
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %d lines in %s", want, path)
		case <-ticker.C:
		}
	}
}

// readNLines returns the first want non-empty lines of path, or nil if fewer
// are currently available.
func readNLines(path string, want int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if s := strings.TrimSpace(l); s != "" {
			lines = append(lines, s)
			if len(lines) == want {
				return lines
			}
		}
	}
	return nil
}

// TestActiveTmux32SameWindowID verifies that linked windows share the same
// stable window_id across sessions.
func TestActiveTmux32SameWindowID(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "source", "-n", "main")
	h.newWindowAfter("source:main", "target")

	sourceID := h.windowID("source", "target")

	h.runMust("new-session", "-d", "-s", "dest")
	h.linkTo(sourceID, "dest")

	destID := h.windowID("dest", "target")
	if sourceID != destID {
		t.Errorf("window_id mismatch across links: source=%s dest=%s", sourceID, destID)
	}
}

// TestActiveTmux32ListAllLinks verifies that list-windows -a reports every
// linked instance of a window across sessions.
func TestActiveTmux32ListAllLinks(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "source", "-n", "main")
	h.newWindowAfter("source:main", "shared")
	sharedID := h.windowID("source", "shared")

	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("dest%d", i)
		h.runMust("new-session", "-d", "-s", name)
		h.linkTo(sharedID, name)
	}

	out := h.runMust("list-windows", "-a", "-F", "#{session_name}:#{window_index} #{window_id} #{window_name}")
	linkCount := 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[1] == sharedID && f[2] == "shared" {
			linkCount++
		}
	}
	// source + 3 destinations = 4 links.
	if linkCount != 4 {
		t.Errorf("expected 4 links of %s, found %d", sharedID, linkCount)
	}
}

// TestActiveTmux32ExactUnlink verifies that unlink-window removes only one
// alias when targeted by session:@ID, leaving all other links intact. Using
// the stable @ID (not a window name) avoids any ambiguity from duplicate names.
func TestActiveTmux32ExactUnlink(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "source", "-n", "main")
	h.newWindowAfter("source:main", "shared")
	sharedID := h.windowID("source", "shared")

	h.runMust("new-session", "-d", "-s", "dest1")
	h.linkTo(sharedID, "dest1")
	h.runMust("new-session", "-d", "-s", "dest2")
	h.linkTo(sharedID, "dest2")

	if out, err := h.run("unlink-window", "-t", "dest1:"+sharedID); err != nil {
		t.Fatalf("unlink-window dest1:%s failed: %v\n%s", sharedID, err, out)
	}

	if windowPresent(h.listWindows("dest1"), sharedID) {
		t.Error("dest1 still has shared window after exact unlink")
	}
	if !windowPresent(h.listWindows("dest2"), sharedID) {
		t.Error("dest2 lost shared window when dest1 was unlinked")
	}
	if !windowPresent(h.listWindows("source"), sharedID) {
		t.Error("source lost shared window when dest1 was unlinked")
	}
}

// TestActiveTmux32RefuseUnlinkLastLink verifies that unlink-window refuses to
// unlink the last remaining link without -k, targeted by session:@ID.
func TestActiveTmux32RefuseUnlinkLastLink(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "source", "-n", "solo")
	soloID := h.windowID("source", "solo")

	out, err := h.run("unlink-window", "-t", "source:"+soloID)
	if err == nil {
		t.Fatalf("unlink-window succeeded on last link - expected refusal\n%s", out)
	}

	if !windowPresent(h.listWindows("source"), soloID) {
		t.Error("last link was destroyed - window disappeared")
	}
}

// TestActiveTmux32RenumberStability verifies that with renumber-windows on,
// removing an interior window compacts surviving indexes while every surviving
// window_id stays stable. Command completion (kill-window, list-windows) is
// the only synchronisation boundary: renumber-windows is synchronous on close.
func TestActiveTmux32RenumberStability(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "test", "-n", "main")
	h.newWindowAfter("test:main", "one")
	h.newWindowAfter("test:one", "two")
	h.newWindowAfter("test:two", "three")
	h.runMust("set-option", "-t", "test", "renumber-windows", "on")

	before := indexByID(h.listWindows("test"))
	oneID := h.windowID("test", "one")

	h.runMust("kill-window", "-t", "test:one")

	after := indexByID(h.listWindows("test"))

	if _, ok := after[oneID]; ok {
		t.Error("killed window one still present after kill-window")
	}
	for id, beforeIdx := range before {
		if id == oneID {
			continue
		}
		afterIdx, ok := after[id]
		if !ok {
			t.Errorf("surviving window %s disappeared after renumber", id)
			continue
		}
		if afterIdx > beforeIdx {
			t.Errorf("index for %s grew instead of compacting: %s -> %s", id, beforeIdx, afterIdx)
		}
	}
	if gaps := indexGaps(after); len(gaps) > 0 {
		t.Errorf("indexes not compact after renumber: gaps %v", gaps)
	}
}

// indexByID maps each stable window id to its current index string.
func indexByID(rows []winRow) map[string]string {
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.id] = r.index
	}
	return m
}

// indexGaps returns the sorted indexes that are not a contiguous 0..n-1 range.
func indexGaps(idToIndex map[string]string) []string {
	max := -1
	for _, idx := range idToIndex {
		n, err := strconv.Atoi(idx)
		if err == nil && n > max {
			max = n
		}
	}
	var gaps []string
	for i := 0; i <= max; i++ {
		want := strconv.Itoa(i)
		found := false
		for _, idx := range idToIndex {
			if idx == want {
				found = true
				break
			}
		}
		if !found {
			gaps = append(gaps, want)
		}
	}
	return gaps
}

// TestActiveTmux32FreeIndexBase0 verifies that link-window picks the lowest
// free index (1 after killing index 1) under the default base-index 0.
func TestActiveTmux32FreeIndexBase0(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "source", "-n", "w0")
	h.newWindowAfter("source:w0", "w1")
	h.newWindowAfter("source:w1", "w2")
	h.runMust("kill-window", "-t", "source:1")

	w2ID := h.windowID("source", "w2")
	h.runMust("new-session", "-d", "-s", "dest")
	h.linkTo(w2ID, "dest")

	assertWindowAt(t, h.listWindows("dest"), w2ID, "1")
}

// TestActiveTmux32FreeIndexBase1 verifies free-index behaviour with a global
// base-index of 1. set-option -g is synchronous, so no propagation delay is
// needed: the next new-session observes the new global default immediately.
func TestActiveTmux32FreeIndexBase1(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "permanent")
	h.runMust("set-option", "-g", "base-index", "1")

	baseOut := h.runMust("show-options", "-g", "base-index")
	if !strings.Contains(string(baseOut), "1") {
		t.Fatalf("global base-index not set to 1: %s", baseOut)
	}

	h.runMust("new-session", "-d", "-s", "source", "-n", "w1")
	h.runMust("new-window", "-d", "-t", "source", "-n", "w2")
	h.runMust("kill-window", "-t", "source:2")

	w1ID := h.windowID("source", "w1")
	h.runMust("new-session", "-d", "-s", "dest", "-n", "placeholder")
	h.linkTo(w1ID, "dest")

	assertWindowAt(t, h.listWindows("dest"), w1ID, "2")
}

// TestActiveTmux32ConflictNoReplacement verifies that link-window refuses to
// replace an existing window at an occupied target index without -k.
func TestActiveTmux32ConflictNoReplacement(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "source", "-n", "target")
	targetID := h.windowID("source", "target")

	h.runMust("new-session", "-d", "-s", "dest", "-n", "existing")

	out, err := h.run("link-window", "-d", "-s", targetID, "-t", "dest:0")
	if err == nil {
		t.Fatalf("link-window succeeded at occupied index 0 - expected conflict\n%s", out)
	}

	for _, r := range h.listWindows("dest") {
		if r.name != "existing" {
			t.Errorf("original window replaced by conflict: found %q", r.name)
		}
	}
}

// TestActiveTmux32HookComposition verifies that two independently indexed
// session-window-changed hooks coexist without overwriting each other. The
// real production hook name is session-window-changed; slot 1708 is the
// canonical tmh slot and a second slot must remain independently configurable.
func TestActiveTmux32HookComposition(t *testing.T) {
	h := newHarness(t)
	h.runMust("new-session", "-d", "-s", "test")

	h.runMust("set-hook", "-t", "test", "session-window-changed[1708]", "run-shell 'echo 1708 >/dev/null'")
	h.runMust("set-hook", "-t", "test", "session-window-changed[1709]", "run-shell 'echo 1709 >/dev/null'")

	hooks := string(h.runMust("show-hooks", "-t", "test"))
	if !strings.Contains(hooks, "session-window-changed[1708]") {
		t.Error("hook slot 1708 missing from show-hooks output")
	}
	if !strings.Contains(hooks, "session-window-changed[1709]") {
		t.Error("hook slot 1709 missing from show-hooks output")
	}
}

// TestActiveTmux32HookWindowID verifies the real session-window-changed hook
// semantics: the hook exposes the selected #{window_id} (the stable @N id of
// the now-active window), and fires on select-window, next-window,
// previous-window and last-window. The hook body writes #{window_id} to a
// marker file via run-shell; the test polls that file (run-shell is
// asynchronous) and compares recorded ids against the expected selection.
func TestActiveTmux32HookWindowID(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "test", "-n", "main")
	h.newWindowAfter("test:main", "other")
	// Establish a known active window before arming the hook so the first
	// selection change is guaranteed to fire it.
	h.runMust("select-window", "-t", "test:main")

	mainID := h.windowID("test", "main")
	otherID := h.windowID("test", "other")
	marker := filepath.Join(t.TempDir(), "hook.out")

	body := fmt.Sprintf("run-shell 'echo #{window_id} >> %s'", marker)
	h.runMust("set-hook", "-t", "test", "session-window-changed[1708]", body)

	ops := []struct {
		name   string
		run    func()
		wantID string
	}{
		{"select-window other", func() { h.runMust("select-window", "-t", "test:other") }, otherID},
		{"select-window main", func() { h.runMust("select-window", "-t", "test:main") }, mainID},
		{"next-window", func() { h.runMust("next-window", "-t", "test") }, otherID},
		{"previous-window", func() { h.runMust("previous-window", "-t", "test") }, mainID},
		{"last-window", func() { h.runMust("last-window", "-t", "test") }, otherID},
	}

	var recorded []string
	for i, op := range ops {
		op.run()
		lines := waitForLineCount(t, marker, i+1)
		recorded = lines
		last := lines[len(lines)-1]
		if last != op.wantID {
			t.Errorf("%s: hook recorded %q, want %q", op.name, last, op.wantID)
		}
	}

	want := []string{otherID, mainID, otherID, mainID, otherID}
	if len(recorded) != len(want) {
		t.Fatalf("recorded %d hook lines, want %d", len(recorded), len(want))
	}
	for i, w := range want {
		if recorded[i] != w {
			t.Errorf("hook line %d: got %q, want %q", i, recorded[i], w)
		}
	}
}

// TestActiveTmux32ServerEpochInputs verifies that socket_path, start_time and
// pid are all available via display-message, parse as expected, and are stable
// within a single server instance (across additional sessions).
func TestActiveTmux32ServerEpochInputs(t *testing.T) {
	h := newHarness(t)
	h.runMust("new-session", "-d", "-s", "first")

	first := h.queryEpoch()
	assertEpochValid(t, first)

	h.runMust("new-session", "-d", "-s", "second")
	second := h.queryEpoch()
	assertEpochValid(t, second)

	if first != second {
		t.Errorf("server epoch changed within one server:\n first=%+v\n second=%+v", first, second)
	}
}

// TestActiveTmux32ServerEpochChange verifies that reviving a server on the same
// socket keeps the socket path stable while the (start_time, pid) identity
// tuple changes — proving a restart is observable and unambiguous.
func TestActiveTmux32ServerEpochChange(t *testing.T) {
	h := newHarness(t)
	h.runMust("new-session", "-d", "-s", "first")
	before := h.queryEpoch()
	assertEpochValid(t, before)

	socket := h.socket
	_ = h.close()

	// Revive on the same socket via the restart seam.
	h2 := newHarnessWithSocket(t, socket)
	h2.runMust("new-session", "-d", "-s", "second")
	after := h2.queryEpoch()
	assertEpochValid(t, after)

	if after.socketPath != before.socketPath {
		t.Errorf("socket_path changed across restart:\n before=%q\n after=%q", before.socketPath, after.socketPath)
	}
	if after.startTime == before.startTime && after.pid == before.pid {
		t.Errorf("server identity unchanged across restart:\n before=%+v\n after=%+v", before, after)
	}
}

// TestActiveTmux32SessionUserOptions verifies that session-scoped user options
// can be set and retrieved, and do not leak across sessions.
func TestActiveTmux32SessionUserOptions(t *testing.T) {
	h := newHarness(t)

	h.runMust("new-session", "-d", "-s", "test")
	h.runMust("set-option", "-t", "test", "@tmh-active-owner", "tmh/v1")

	value := strings.TrimSpace(string(h.runMust("show-options", "-t", "test", "-v", "@tmh-active-owner")))
	if value != "tmh/v1" {
		t.Errorf("user option mismatch: got %q, want %q", value, "tmh/v1")
	}

	h.runMust("new-session", "-d", "-s", "other")
	out, err := h.run("show-options", "-t", "other", "-v", "@tmh-active-owner")
	if err == nil {
		if v := strings.TrimSpace(string(out)); v == "tmh/v1" {
			t.Error("session-local user option leaked into another session")
		}
	}
}
