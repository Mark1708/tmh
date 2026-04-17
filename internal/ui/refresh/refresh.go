// Package refresh provides a cadenced pane-command batch fetcher for the
// tmh TUI.
//
// Design contract:
//   - One tea.Every tick drives the cadence; missing a tick reschedules it.
//   - Each tick increments a sequence counter. Stale results (seq mismatch)
//     are silently dropped, so rapid scrolling or resize doesn't cause a flood.
//   - The fetch is a single `tmux list-panes -a` call regardless of the
//     number of panes (O(1) tmux invocations per tick).
//   - While a text-input widget is focused (palette, filter, etc.) the tick
//     fires but the fetch is skipped — the Tick is rescheduled normally so
//     the cadence never dies.
package refresh

import (
	"context"
	"fmt"
	"time"

	"github.com/mark1708/tmh/internal/tmux"
	"github.com/mark1708/tmh/internal/ui/pane"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultInterval is the suggested pane-command refresh cadence.
const DefaultInterval = 2 * time.Second

// TickMsg is sent by the periodic timer managed by Refresher.
type TickMsg struct{ Seq uint64 }

// PaneDataMsg carries the result of a successful batch pane fetch.
type PaneDataMsg struct {
	Data map[string]pane.Info
	Seq  uint64
}

// Refresher manages the periodic pane-data fetch loop.
// It is owned by the root model and stored by value.
type Refresher struct {
	interval time.Duration
	seq      uint64
}

// New creates a Refresher with the given cadence interval.
func New(interval time.Duration) *Refresher {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Refresher{interval: interval}
}

// SetInterval changes the cadence. Takes effect on the next Tick call.
func (r *Refresher) SetInterval(d time.Duration) {
	if d > 0 {
		r.interval = d
	}
}

// Tick returns a tea.Cmd that fires TickMsg after one interval.
// Always returns a non-nil Cmd; the loop never dies.
func (r *Refresher) Tick() tea.Cmd {
	d := r.interval
	return tea.Tick(d, func(time.Time) tea.Msg {
		return TickMsg{Seq: r.seq}
	})
}

// Fetch returns a tea.Cmd that calls tmux list-panes -a and returns
// PaneDataMsg. seq is the current sequence number; on arrival the root model
// must check seq == r.Seq() before applying the result.
//
// The caller is responsible for not calling Fetch when a text-input is
// focused (see the input-pause rule in the package doc).
func (r *Refresher) Fetch(runner tmux.Runner, seq uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		panes, err := runner.ListPanes(ctx, "")
		if err != nil {
			// Non-fatal: return empty data, not an error message.
			return PaneDataMsg{Data: map[string]pane.Info{}, Seq: seq}
		}
		data := make(map[string]pane.Info, len(panes))
		for _, p := range panes {
			key := fmt.Sprintf("%s:%d.%d", p.Session, p.Window, p.Index)
			data[key] = pane.Info{
				Command: p.Command,
				Path:    p.Path,
				Active:  p.Active,
			}
		}
		return PaneDataMsg{Data: data, Seq: seq}
	}
}

// BumpSeq increments and returns the sequence counter.
// Call this when issuing a new Fetch so that concurrent in-flight results
// from a prior fetch are discarded.
func (r *Refresher) BumpSeq() uint64 {
	r.seq++
	return r.seq
}

// Seq returns the current sequence counter without modifying it.
func (r *Refresher) Seq() uint64 {
	return r.seq
}

// Interval returns the current cadence.
func (r *Refresher) Interval() time.Duration {
	return r.interval
}
