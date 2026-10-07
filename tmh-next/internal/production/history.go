package production

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/mark1708/tmh-next/internal/domain"
	"github.com/mark1708/tmh-next/internal/runtimegraph"
)

const (
	maxScreenLines       = 240
	maxScreenColumns     = 4096
	maxHistoryChunks     = 2000
	screenSampleInterval = 5 * time.Second
)

var (
	secretAssignment = regexp.MustCompile(`(?i)(token|password|secret|api[_-]?key)(\s*[=:]\s*)\S+`)
	ansiEscape       = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

// captureScreens is a bounded, best-effort Zellij fallback. It records changed
// viewports and marks the first sample as a gap because dump-screen cannot
// promise lossless PTY history.
func (c *Client) captureScreens(ctx context.Context, catalog *domain.Catalog, graph *runtimegraph.Graph) {
	if c.runner == nil {
		return
	}
	for _, terminal := range catalog.Terminals {
		if terminal.State != domain.TerminalLive {
			continue
		}
		if sampledRecently(catalog, terminal.ID) {
			continue
		}
		_, session, pane, err := resolveTerminal(graph, catalog, domain.Ref(domain.KindTerminal, terminal.ID))
		if err != nil {
			continue
		}
		raw, err := c.runner.Run(ctx, "--session", session, "action", "dump-screen", "--pane-id", pane)
		if err != nil {
			continue
		}
		lines := normalizeScreen(string(raw), catalog.Config.RedactSecrets)
		if len(lines) == 0 || sameLastScreen(catalog, terminal.ID, lines) {
			continue
		}
		seq := nextHistorySequence(catalog, terminal.ID)
		catalog.History = append(catalog.History, &domain.HistoryChunk{
			ID:    "history-" + terminal.ID + "-" + formatSequence(seq),
			Scope: domain.HistoryScope{Kind: domain.KindTerminal, ID: terminal.ID},
			Seq:   seq, At: catalog.Now, Lines: lines, Kind: "screen", Gap: seq == 1,
		})
	}
	pruneHistory(catalog)
}

func normalizeScreen(screen string, redact bool) []string {
	screen = strings.ReplaceAll(screen, "\r\n", "\n")
	screen = ansiEscape.ReplaceAllString(screen, "")
	screen = strings.TrimRight(screen, "\n")
	if screen == "" {
		return nil
	}
	lines := strings.Split(screen, "\n")
	if len(lines) > maxScreenLines {
		lines = lines[len(lines)-maxScreenLines:]
	}
	for i, line := range lines {
		if len(line) > maxScreenColumns {
			line = line[:maxScreenColumns]
		}
		if redact {
			line = secretAssignment.ReplaceAllString(line, "$1$2[REDACTED]")
		}
		lines[i] = line
	}
	return lines
}

func sampledRecently(catalog *domain.Catalog, terminalID string) bool {
	for index := len(catalog.History) - 1; index >= 0; index-- {
		chunk := catalog.History[index]
		if chunk.Scope.Kind == domain.KindTerminal && chunk.Scope.ID == terminalID && chunk.Kind == "screen" {
			return catalog.Now.Sub(chunk.At) < screenSampleInterval
		}
	}
	return false
}

func sameLastScreen(catalog *domain.Catalog, terminalID string, lines []string) bool {
	for i := len(catalog.History) - 1; i >= 0; i-- {
		chunk := catalog.History[i]
		if chunk.Scope.Kind == domain.KindTerminal && chunk.Scope.ID == terminalID && chunk.Kind == "screen" {
			return reflect.DeepEqual(chunk.Lines, lines)
		}
	}
	return false
}

func nextHistorySequence(catalog *domain.Catalog, terminalID string) int {
	sequence := 0
	for _, chunk := range catalog.History {
		if chunk.Scope.Kind == domain.KindTerminal && chunk.Scope.ID == terminalID && chunk.Seq > sequence {
			sequence = chunk.Seq
		}
	}
	return sequence + 1
}

func formatSequence(sequence int) string {
	return fmt.Sprintf("%08d", sequence)
}

func pruneHistory(catalog *domain.Catalog) {
	cutoff := time.Time{}
	if catalog.Config.RetentionHours > 0 {
		cutoff = catalog.Now.Add(-time.Duration(catalog.Config.RetentionHours) * time.Hour)
	}
	kept := catalog.History[:0]
	for _, chunk := range catalog.History {
		if cutoff.IsZero() || !chunk.At.Before(cutoff) {
			kept = append(kept, chunk)
		}
	}
	if len(kept) > maxHistoryChunks {
		kept = kept[len(kept)-maxHistoryChunks:]
	}
	catalog.History = kept
}
