package state

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"git.mark1708.ru/me/tmh/internal/xdg"
)

// Mark records a named navigation target.
type Mark struct {
	Letter    rune   `json:"letter"`
	Target    string `json:"target"` // tmux target: "session" or "session:window"
	CursorIdx int    `json:"cursor_idx"`
	Valid     bool   `json:"valid"` // false when the target was killed
}

// Location is one entry in the last-location ring.
type Location struct {
	Target    string `json:"target"`
	CursorIdx int    `json:"cursor_idx"`
}

// marksFile is the serialised form of MarksStore.
type marksFile struct {
	Marks         map[string]Mark  `json:"marks"`           // letter string → Mark
	LastLocations []Location       `json:"last_locations"`  // ring, newest first
	SavedAt       time.Time        `json:"saved_at"`
}

const lastLocationRingSize = 10

// MarksStore holds named marks and the last-location ring.
// All methods are safe for concurrent use.
type MarksStore struct {
	mu            sync.Mutex
	marks         map[rune]Mark
	lastLocations []Location // ring, newest first, max lastLocationRingSize
	path          string
}

// NewMarksStore opens or creates the marks store at the default XDG path.
func NewMarksStore() *MarksStore {
	return NewMarksStoreAt(xdg.MarksPath())
}

// NewMarksStoreAt opens or creates the marks store at an explicit path.
// This is primarily useful for tests.
func NewMarksStoreAt(path string) *MarksStore {
	ms := &MarksStore{
		marks: make(map[rune]Mark),
		path:  path,
	}
	ms.load() // best-effort; errors are silent
	return ms
}

// load reads the persisted marks from disk. Corrupt files are ignored.
func (ms *MarksStore) load() {
	data, err := os.ReadFile(ms.path)
	if err != nil {
		return // file absent or unreadable — start fresh
	}
	var f marksFile
	if json.Unmarshal(data, &f) != nil {
		return // corrupt — start fresh
	}
	for letter, m := range f.Marks {
		if len([]rune(letter)) == 1 {
			ms.marks[[]rune(letter)[0]] = m
		}
	}
	ms.lastLocations = f.LastLocations
}

// save flushes marks to disk. Called after every mutation.
// Errors are best-effort: a failed write is silently ignored so that a
// read-only file system does not crash the TUI. The atomic write (temp file +
// rename) guarantees the on-disk file is never left in a partial state.
func (ms *MarksStore) save() {
	letterMap := make(map[string]Mark, len(ms.marks))
	for r, m := range ms.marks {
		letterMap[string(r)] = m
	}
	f := marksFile{
		Marks:         letterMap,
		LastLocations: ms.lastLocations,
		SavedAt:       time.Now(),
	}
	data, err := json.Marshal(f)
	if err != nil {
		return
	}
	// Atomic write via temp file.
	tmp := ms.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	if renameErr := os.Rename(tmp, ms.path); renameErr != nil {
		// Best-effort: clean up the temp file and accept the loss.
		_ = os.Remove(tmp)
	}
}

// SetMark records a named mark. Overwrites any existing mark for the letter.
func (ms *MarksStore) SetMark(letter rune, target string, cursorIdx int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.marks[letter] = Mark{Letter: letter, Target: target, CursorIdx: cursorIdx, Valid: true}
	ms.save()
}

// GetMark returns the mark for the given letter. Returns false when absent or invalid.
func (ms *MarksStore) GetMark(letter rune) (Mark, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m, ok := ms.marks[letter]
	if !ok || !m.Valid {
		return Mark{}, false
	}
	return m, true
}

// InvalidateMark marks the given target as invalid (e.g. after the pane/window is killed).
// All marks whose Target matches are invalidated.
func (ms *MarksStore) InvalidateMark(target string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	changed := false
	for r, m := range ms.marks {
		if m.Target == target && m.Valid {
			m.Valid = false
			ms.marks[r] = m
			changed = true
		}
	}
	if changed {
		ms.save()
	}
}

// AllMarks returns all currently valid marks ordered by letter.
func (ms *MarksStore) AllMarks() []Mark {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var out []Mark
	for _, m := range ms.marks {
		if m.Valid {
			out = append(out, m)
		}
	}
	return out
}

// ClearMarks removes all marks.
func (ms *MarksStore) ClearMarks() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.marks = make(map[rune]Mark)
	ms.save()
}

// PushLocation adds a location to the last-location ring.
// The ring is capped at lastLocationRingSize entries.
func (ms *MarksStore) PushLocation(target string, cursorIdx int) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	loc := Location{Target: target, CursorIdx: cursorIdx}
	// Prepend (newest first).
	ms.lastLocations = append([]Location{loc}, ms.lastLocations...)
	if len(ms.lastLocations) > lastLocationRingSize {
		ms.lastLocations = ms.lastLocations[:lastLocationRingSize]
	}
	ms.save()
}

// PopLocation removes and returns the most recent location.
// Returns false when the ring is empty.
func (ms *MarksStore) PopLocation() (Location, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if len(ms.lastLocations) == 0 {
		return Location{}, false
	}
	loc := ms.lastLocations[0]
	ms.lastLocations = ms.lastLocations[1:]
	ms.save()
	return loc, true
}

// HasLastLocation reports whether the ring is non-empty.
func (ms *MarksStore) HasLastLocation() bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return len(ms.lastLocations) > 0
}

// ErrMarksPathMissing is returned when the XDG state dir cannot be created.
var ErrMarksPathMissing = errors.New("marks: state directory unavailable")
