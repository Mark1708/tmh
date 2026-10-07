package domain

import (
	"errors"
	"fmt"
	"time"
)

// ActionKind enumerates every mock action. The transition table lives in the
// mock backend; the domain only carries identity.
type ActionKind string

const (
	ActionAttach         ActionKind = "attach"
	ActionFreeze         ActionKind = "freeze"
	ActionSplit          ActionKind = "split"
	ActionLink           ActionKind = "link"
	ActionPin            ActionKind = "pin"
	ActionUnpin          ActionKind = "unpin"
	ActionTouch          ActionKind = "touch"
	ActionPrune          ActionKind = "prune"
	ActionSend           ActionKind = "send"
	ActionKill           ActionKind = "kill"
	ActionAgentFocus     ActionKind = "agent-focus"
	ActionAgentPrompt    ActionKind = "agent-prompt"
	ActionAgentWait      ActionKind = "agent-wait"
	ActionAgentClaim     ActionKind = "agent-claim"
	ActionHistoryExport  ActionKind = "history-export"
	ActionSnapshotCreate ActionKind = "snapshot-create"
	ActionSnapshotTag    ActionKind = "snapshot-tag"
	ActionSnapshotDelete ActionKind = "snapshot-delete"
	ActionSnapshotPlan   ActionKind = "snapshot-plan"
	ActionRestoreApply   ActionKind = "restore-apply"
	ActionReconcileUndo  ActionKind = "reconcile-undo"
	ActionBackendProbe   ActionKind = "backend-probe"
	ActionBackendDoctor  ActionKind = "backend-doctor"
	ActionBackendToggle  ActionKind = "backend-toggle"
	ActionMachineConnect ActionKind = "machine-connect"
	ActionMachinePing    ActionKind = "machine-ping"
	ActionMachineDoctor  ActionKind = "machine-doctor"
	ActionPerfRecord     ActionKind = "perf-record"
	ActionPerfBenchmark  ActionKind = "perf-benchmark"
	ActionConfigValidate ActionKind = "config-validate"
	ActionConfigSave     ActionKind = "config-save"
	ActionConfigReload   ActionKind = "config-reload"
)

// AllActionKinds returns every action kind (used by exhaustive tests).
func AllActionKinds() []ActionKind {
	return []ActionKind{
		ActionAttach, ActionFreeze, ActionSplit, ActionLink, ActionPin, ActionUnpin,
		ActionTouch, ActionPrune, ActionSend, ActionKill, ActionAgentFocus,
		ActionAgentPrompt, ActionAgentWait, ActionAgentClaim, ActionHistoryExport,
		ActionSnapshotCreate, ActionSnapshotTag, ActionSnapshotDelete, ActionSnapshotPlan,
		ActionRestoreApply, ActionReconcileUndo, ActionBackendProbe, ActionBackendDoctor,
		ActionBackendToggle, ActionMachineConnect, ActionMachinePing, ActionMachineDoctor,
		ActionPerfRecord, ActionPerfBenchmark, ActionConfigValidate, ActionConfigSave,
		ActionConfigReload,
	}
}

// Action is a user mutation intent. Confirmed must be set by the UI before
// destructive actions reach the backend; the backend re-checks.
type Action struct {
	Kind             ActionKind  `json:"kind"`
	Target           ResourceRef `json:"target"`
	Value            string      `json:"value,omitempty"`
	Selection        []string    `json:"selection,omitempty"`
	Confirmed        bool        `json:"confirmed"`
	ExpectedRevision uint64      `json:"expected_revision"`
}

// Identity returns (kind, target) identity for mutation acceptance checks.
func (a Action) Identity() string {
	return fmt.Sprintf("%s/%s:%s", a.Kind, a.Target.Kind, a.Target.ID)
}

// MutationResult is the backend response for Execute and Tick.
type MutationResult struct {
	BaseRevision       uint64   `json:"base_revision"`
	NewRevision        uint64   `json:"new_revision"`
	Catalog            *Catalog `json:"catalog"`
	Action             Action   `json:"action"`
	IsTick             bool     `json:"is_tick"`
	TickToken          uint64   `json:"tick_token,omitempty"`
	Message            string   `json:"message"`
	InteractiveCommand []string `json:"interactive_command,omitempty"`
}

// SearchQuery is a concurrent read query.
type SearchQuery struct {
	Text  string `json:"text"`
	Scope string `json:"scope"` // all|history|events|agents
	Limit int    `json:"limit"`
}

// SearchHit is one grouped search result row.
type SearchHit struct {
	Ref     ResourceRef `json:"ref"`
	Title   string      `json:"title"`
	Snippet string      `json:"snippet"`
	Scope   string      `json:"scope"` // history|events|agents|workspaces|terminals
}

// SearchResult carries hits with the catalog revision they were read at.
type SearchResult struct {
	Revision uint64      `json:"revision"`
	Hits     []SearchHit `json:"hits"`
}

// ErrCode is a stable machine-readable failure code.
type ErrCode string

const (
	CodeNotFound           ErrCode = "not_found"
	CodeNotLive            ErrCode = "not_live"
	CodeBadTarget          ErrCode = "bad_target"
	CodeInvalidState       ErrCode = "invalid_state"
	CodeConfirmationNeeded ErrCode = "confirmation_required"
	CodeProtected          ErrCode = "protected"
	CodeEmptyValue         ErrCode = "empty_value"
	CodeEmptySelection     ErrCode = "empty_selection"
	CodeStalePlan          ErrCode = "stale_plan"
	CodeUndoUnavailable    ErrCode = "undo_unavailable"
	CodeUndoStale          ErrCode = "undo_stale"
	CodeProbeTimeout       ErrCode = "probe_timeout"
	CodeUnreachable        ErrCode = "unreachable"
	CodeBusy               ErrCode = "busy"
	CodeValidation         ErrCode = "validation"
	CodeAlreadyClaimed     ErrCode = "already_claimed"
	CodeRevisionConflict   ErrCode = "revision_conflict"
	CodeMalformedResult    ErrCode = "malformed_result"
)

// ErrRevisionConflict signals an expected-revision mismatch; the caller may
// reload the catalog and surface a recoverable conflict.
var ErrRevisionConflict = errors.New("revision conflict")

// IsRevisionConflict reports whether err is a revision conflict.
func IsRevisionConflict(err error) bool { return errors.Is(err, ErrRevisionConflict) }

// Error is a typed action failure. Failures never mutate state.
type Error struct {
	Code ErrCode `json:"code"`
	Msg  string  `json:"msg"`
}

// Error implements error.
func (e *Error) Error() string { return string(e.Code) + ": " + e.Msg }

// Fail builds a typed action error.
func Fail(code ErrCode, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// ErrCodeOf extracts the typed code, defaulting to unknown.
func ErrCodeOf(err error) ErrCode {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return "unknown"
}

// ErrorMessage returns the human-readable detail without repeating a typed
// error code. Transport and UI adapters use it when they render the code
// separately.
func ErrorMessage(err error) string {
	var actionErr *Error
	if errors.As(err, &actionErr) {
		return actionErr.Msg
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// Clock abstracts logical time for the backend (mock-only implementations).
type Clock interface {
	Now() time.Time
}
