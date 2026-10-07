package production

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
)

type screenRunner struct {
	output string
	calls  int
}

func (r *screenRunner) Run(context.Context, ...string) ([]byte, error) {
	r.calls++
	return []byte(r.output), nil
}

func TestNormalizeScreenStripsANSIAndRedactsSecrets(t *testing.T) {
	got := normalizeScreen("\x1b[32m$ export API_KEY=abc123\x1b[m\r\ntoken: xyz\n", true)
	want := []string{"$ export API_KEY=[REDACTED]", "token: [REDACTED]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized screen = %#v, want %#v", got, want)
	}
}

func TestSampledRecentlyBoundsRevisionChurn(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	catalog := &domain.Catalog{Now: now, History: []*domain.HistoryChunk{{
		Scope: domain.HistoryScope{Kind: domain.KindTerminal, ID: "terminal-1"},
		At:    now.Add(-4 * time.Second), Kind: "screen",
	}}}
	if !sampledRecently(catalog, "terminal-1") {
		t.Fatal("sample inside interval was not suppressed")
	}
	catalog.Now = now.Add(2 * time.Second)
	if sampledRecently(catalog, "terminal-1") {
		t.Fatal("sample after interval remained suppressed")
	}
}

func TestCaptureScreensRedactsDeduplicatesAndSequences(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	graph := testGraph("base")
	catalog, err := ProjectRuntime(nil, graph, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	runner := &screenRunner{output: "\x1b[32mtoken=abc123\x1b[m\nready"}
	client := &Client{runner: runner}
	client.captureScreens(context.Background(), catalog, graph)
	if len(catalog.History) != 1 {
		t.Fatalf("history after first capture = %d", len(catalog.History))
	}
	if got := catalog.History[0].Lines; !reflect.DeepEqual(got, []string{"token=[REDACTED]", "ready"}) {
		t.Fatalf("captured lines = %#v", got)
	}

	catalog.Now = now.Add(6 * time.Second)
	client.captureScreens(context.Background(), catalog, graph)
	if len(catalog.History) != 1 {
		t.Fatalf("identical screen was duplicated: %d chunks", len(catalog.History))
	}

	runner.output = "ready\nchanged"
	catalog.Now = now.Add(12 * time.Second)
	client.captureScreens(context.Background(), catalog, graph)
	if len(catalog.History) != 2 || catalog.History[1].Seq != 2 ||
		catalog.History[1].ID != "history-"+graph.Terminals[0].ID+"-00000002" {
		t.Fatalf("sequenced history = %+v", catalog.History)
	}
	if runner.calls != 3 {
		t.Fatalf("dump-screen calls = %d, want 3", runner.calls)
	}
}
