package actions

import (
	"time"

	"github.com/mark1708/tmh/internal/state"
)

type activeTransition string

const (
	activeKeep         activeTransition = "keep"
	activeExpireSafe   activeTransition = "expire_safe"
	activeExpireOrphan activeTransition = "expire_orphan"
	activeStaleRow     activeTransition = "stale_row"
	activeGoneWindow   activeTransition = "gone_window"
)

type classifiedActiveWindow struct {
	row        state.ActiveWindow
	links      activeLinkSet
	transition activeTransition
}

func classifyActiveWindow(row state.ActiveWindow, links activeLinkSet, now time.Time) classifiedActiveWindow {
	var transition activeTransition
	switch {
	case !links.hasActive && len(links.sources) > 0:
		transition = activeStaleRow
	case !links.hasActive:
		transition = activeGoneWindow
	case row.ExpiresAt.After(now):
		transition = activeKeep
	case len(links.sources) > 0:
		transition = activeExpireSafe
	default:
		transition = activeExpireOrphan
	}
	return classifiedActiveWindow{row: row, links: links, transition: transition}
}

func classifyActiveRows(rows []state.ActiveWindow, snapshot activeSnapshot, now time.Time) []classifiedActiveWindow {
	classified := make([]classifiedActiveWindow, 0, len(rows))
	for _, row := range rows {
		classified = append(classified, classifyActiveWindow(row, snapshot.forWindow(row.WindowID), now))
	}
	return classified
}
