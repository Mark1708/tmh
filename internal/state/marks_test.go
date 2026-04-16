package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"git.mark1708.ru/me/tmh/internal/state"
)

// newTempStore creates a MarksStore backed by a temp file.
func newTempStore(t *testing.T) *state.MarksStore {
	t.Helper()
	dir := t.TempDir()
	return state.NewMarksStoreAt(filepath.Join(dir, "marks.json"))
}

// TestSetGetMark verifies basic set/get round-trip.
func TestSetGetMark(t *testing.T) {
	ms := newTempStore(t)
	ms.SetMark('a', "work:editor", 3)

	m, ok := ms.GetMark('a')
	if !ok {
		t.Fatal("expected mark 'a' to exist")
	}
	if m.Target != "work:editor" || m.CursorIdx != 3 {
		t.Fatalf("unexpected mark: %+v", m)
	}
}

// TestMarkPersistence verifies that marks survive a reload.
func TestMarkPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marks.json")

	ms1 := state.NewMarksStoreAt(path)
	ms1.SetMark('b', "dev:server", 7)

	// Open a second store pointing at the same file.
	ms2 := state.NewMarksStoreAt(path)
	m, ok := ms2.GetMark('b')
	if !ok {
		t.Fatal("mark 'b' not persisted")
	}
	if m.Target != "dev:server" || m.CursorIdx != 7 {
		t.Fatalf("unexpected loaded mark: %+v", m)
	}
}

// TestInvalidateMark checks that invalidation makes the mark inaccessible.
func TestInvalidateMark(t *testing.T) {
	ms := newTempStore(t)
	ms.SetMark('c', "logs:tail", 0)
	ms.InvalidateMark("logs:tail")

	_, ok := ms.GetMark('c')
	if ok {
		t.Fatal("invalidated mark should not be returned")
	}
}

// TestAllMarksFiltersInvalid ensures AllMarks only returns valid marks.
func TestAllMarksFiltersInvalid(t *testing.T) {
	ms := newTempStore(t)
	ms.SetMark('x', "s1:w1", 0)
	ms.SetMark('y', "s2:w2", 1)
	ms.InvalidateMark("s1:w1")

	marks := ms.AllMarks()
	if len(marks) != 1 || marks[0].Letter != 'y' {
		t.Fatalf("expected only mark 'y', got %v", marks)
	}
}

// TestClearMarks verifies ClearMarks wipes everything.
func TestClearMarks(t *testing.T) {
	ms := newTempStore(t)
	ms.SetMark('p', "proj:main", 0)
	ms.ClearMarks()

	if len(ms.AllMarks()) != 0 {
		t.Fatal("expected zero marks after clear")
	}
}

// TestLocationRingCap verifies the ring stays at most lastLocationRingSize entries.
func TestLocationRingCap(t *testing.T) {
	ms := newTempStore(t)
	for i := 0; i < 15; i++ {
		ms.PushLocation("s:w", i)
	}

	// Pop all and count.
	count := 0
	for ms.HasLastLocation() {
		_, ok := ms.PopLocation()
		if !ok {
			break
		}
		count++
	}
	if count != 10 {
		t.Fatalf("expected ring cap of 10, got %d", count)
	}
}

// TestLocationRingNewestFirst verifies the ring is ordered newest-first.
func TestLocationRingNewestFirst(t *testing.T) {
	ms := newTempStore(t)
	ms.PushLocation("old:w", 0)
	ms.PushLocation("new:w", 1)

	loc, ok := ms.PopLocation()
	if !ok {
		t.Fatal("expected location")
	}
	if loc.Target != "new:w" {
		t.Fatalf("expected newest entry first, got %s", loc.Target)
	}
}

// TestPopLocationEmpty returns false when ring is empty.
func TestPopLocationEmpty(t *testing.T) {
	ms := newTempStore(t)
	_, ok := ms.PopLocation()
	if ok {
		t.Fatal("PopLocation on empty ring should return false")
	}
}

// TestCorruptFileFallback verifies that a corrupt file starts fresh without panicking.
func TestCorruptFileFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marks.json")

	// Write garbage.
	if err := os.WriteFile(path, []byte("!!!not json{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	ms := state.NewMarksStoreAt(path)
	// Should start fresh — no marks, no locations.
	if len(ms.AllMarks()) != 0 {
		t.Fatal("expected fresh state after corrupt file")
	}
	if ms.HasLastLocation() {
		t.Fatal("expected empty location ring after corrupt file")
	}
}

// TestAtomicWrite verifies the temp+rename pattern doesn't leave a .tmp file.
func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marks.json")
	ms := state.NewMarksStoreAt(path)
	ms.SetMark('z', "s:w", 0)

	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("temp file should not exist after atomic write")
	}

	// Verify the main file is valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("marks file not written: %v", err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("marks file is not valid JSON: %v", err)
	}
}
