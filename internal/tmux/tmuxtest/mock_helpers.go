package tmuxtest

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	errs "github.com/mark1708/tmh/internal/errors"
)

type mockWindow struct {
	id      string
	index   int
	name    string
	layout  string
	panes   []*mockPane
	active  bool
	autoRen bool
}

type mockPane struct {
	id      string
	command string
	path    string
	active  bool
}

type mockPhysicalWindow struct {
	id    string
	links map[string]*mockWindowLink
}

type mockWindowLink struct {
	session string
	index   int
	name    string
	active  bool
}

type mockSession struct {
	name     string
	attached bool
	windows  []*mockWindow
}

func (m *MockRunner) nextPaneID() string {
	m.paneSerial++
	return fmt.Sprintf("%%%d", m.paneSerial)
}

func (m *MockRunner) nextWindowID() string {
	m.winSerial++
	return fmt.Sprintf("@%d", m.winSerial)
}

func (m *MockRunner) findWindow(target string) (*mockWindow, error) {
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 || parts[0] == "" {
		return nil, fmt.Errorf("%w: %s", errs.ErrWindowNotFound, target)
	}
	s, ok := m.sessions[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errs.ErrSessionNotFound, parts[0])
	}
	suffix := parts[1]
	if dot := strings.IndexByte(suffix, '.'); dot >= 0 {
		suffix = suffix[:dot]
	}
	if idx, err := strconv.Atoi(suffix); err == nil {
		for _, w := range s.windows {
			if w.index == idx {
				return w, nil
			}
		}
	}
	for _, w := range s.windows {
		if w.id == suffix {
			return w, nil
		}
	}
	for _, w := range s.windows {
		if w.name == suffix {
			return w, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", errs.ErrWindowNotFound, target)
}

func parseLinkDestination(destination string) (string, *int, error) {
	parts := strings.SplitN(destination, ":", 2)
	if parts[0] == "" {
		return "", nil, fmt.Errorf("invalid destination %q", destination)
	}
	if len(parts) == 1 {
		return parts[0], nil, nil
	}
	index, err := strconv.Atoi(parts[1])
	if err != nil || index < 0 {
		return "", nil, fmt.Errorf("invalid destination %q", destination)
	}
	return parts[0], &index, nil
}

func sortedMockWindows(windows []*mockWindow) []*mockWindow {
	out := append([]*mockWindow(nil), windows...)
	sort.Slice(out, func(i, j int) bool { return out[i].index < out[j].index })
	return out
}

func parseWindowTarget(target string) (session string, index int, err error) {
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid target %q", target)
	}
	suffix := parts[1]
	if dot := strings.IndexByte(suffix, '.'); dot >= 0 {
		suffix = suffix[:dot]
	}
	idx, err := strconv.Atoi(suffix)
	if err != nil {
		return parts[0], 0, err
	}
	return parts[0], idx, nil
}
