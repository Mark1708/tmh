# VERIFICATION.md — tmh-next demo evidence

## Environment

| Item | Value |
|---|---|
| `go version` | go1.26.0 darwin/arm64 (local go1.22.5 + `GOTOOLCHAIN=auto` → go1.26.0) |
| `go env GOTOOLCHAIN` | auto |
| bubbletea | charm.land/bubbletea/v2 v2.0.10 (sum h1:oolvo20VBpI0PfqE7iFjkZ1bx0WpmXGfnKz5Yldjq5o=) |
| bubbles | charm.land/bubbles/v2 v2.2.1 (sum h1:Fq1+qm5hV6GkvzLQDhCBpXXE5tLgvh1PRriCLwSvIQU=) |
| lipgloss | charm.land/lipgloss/v2 v2.0.6 (sum h1:EaGKeuA8FvF+v2BT5VmZd2LoYLaMZJXA5n34th8nCIQ=) |
| huh | charm.land/huh/v2 v2.0.3 (sum h1:2cJsMqEPwSywGHvdlKsJyQKPtSJLVnFKyFbsYZTlLkU=) |
| `vhs --version` | vhs version 0.11.0 |
| terminal (captures) | ghostty 1.3.1, macOS darwin 24.6.0, arm64 |

Commands run from `/Users/mark/tmp/tmh-next` unless noted.

## Phase 2 — Charm v2 compatibility gate

Command: `go test ./internal/... -run 'TestCharmV2ViewLifecycle|TestLipGlossOverlayComposition|TestBubblesSelectorFilterEscape|TestHuhDraftSaveDiscardAndCompactLayout' -v`

First run (red — assumptions corrected against pinned sources, per plan §2.5):

| Failure | Root cause found in pinned source | Correction |
|---|---|---|
| `rendered output does not contain frame text` | non-TTY output renders no frame bytes (renderer skips painting without a real terminal size); the *model* view is still authoritative | assert `out.Len() > 0` and assert content via `View()` |
| `overlay text at column 12, want 10` | `strings.Index` is a **byte** offset; `│` is 3 bytes in UTF-8 | new `testutil.ColumnOf` display-column helper (`ansi.StringWidth` of prefix) |
| `visible items after 'ag': [agents events dashboard]` | filtering is async: list returns `tea.BatchMsg`; matches apply only when `FilterMatchesMsg` is fed back; cursor-blink cmds must not be pumped (they sleep 530 ms) | `expandBatch` + `testutil.RunCmdFast` bounded pump |
| `valid submit state = 0` / `escape state = 0` | huh completion flows through returned `nextGroupMsg` cmd (discarded by naive driver); huh `KeyMap.Quit` is **ctrl+c only** — Esc is root-owned by design | pump returned cmds; Esc contract = root consumes before huh (locked in subtest `root-consumed esc leaves form untouched`) |

Final run: **ok** github.com/mark1708/tmh-next/internal/app — all 4 gate tests PASS
(full `go test ./internal/...` output captured below as phases complete).

Gate conclusion: pinned v2 APIs behave as recorded in `RESEARCH.md` §2; no v1
fallback needed.

## Phase 3 — Domain catalog and mock backend

Command: `go test -race ./internal/mock/`

Red run (implementation vs test disagreement — four genuine defects found):

| Finding | Kind | Fix |
|---|---|---|
| `split` added the new window/surface to observed only → `validation: workspace base states differ but drift is empty` | backend bug | split records user intent in **both** desired and observed |
| `backend-toggle` disabled zellij **without** confirmation (`confirmation_required` expected) | backend bug | confirm condition corrected to fire when the toggle *disables* |
| event counter expectation `evt-0012` | test error | ticks add no events; 8 fixture + 3 action events → `evt-0011` |
| second restore plan applied `pull` on drift-free workspace | test error | base has no drift → second plan applies `freeze` (always has one op) |

Green: `ok github.com/mark1708/tmh-next/internal/mock` — TestFixturesHaveReferentialIntegrity, TestScenarioIsolation, TestLogicalClockAndIDs, TestEveryActionTransition (all 31 action kinds: success + every listed failure code), TestFailuresDoNotMutate, TestReconcileUndoInvalidation, TestSearchRevision, TestScenarioTickContracts.

`go vet ./internal/...` clean; `go test -race ./internal/...` all ok.

## Phase 4 — Root shell, mutation lane, routing, input ownership

Command: `go test -race ./internal/app/`

Red-run findings (genuine defects + test-design corrections):

| Finding | Kind | Fix |
|---|---|---|
| `--resource` without `--page` and `--page dashboard --resource …` were accepted | CLI gap | explicit rejection with exit-2 message |
| quick-switch/palette/context overlays shared one kind, tests asserted distinct kinds | design | overlays carry their kind; selector routing keyed on `ov.sel` |
| gated-backend tests deadlocked: `fire` runs commands synchronously, a closed gate blocks the harness | test design | `dispatchAsync`/`drainAsync` — gated commands run on goroutines, results fed back after release |
| search-lane test fed results synchronously, so no stale interleaving was possible | test design | hold query-1 result until query-2 supersedes it |
| `tooSmall` classification of the pre-size 0×0 root swallowed ctrl+p | test setup | harness `boot` installs a 120×40 size |

Green: `ok github.com/mark1708/tmh-next/internal/app` — TestInitialStacksAndResourcePolicies, TestRoutePushReplaceBack, TestExplicitResourceDoesNotFallback, TestOverlayOwnsEscape, TestFilterOwnsEscapeAndQ, TestTooSmallSuspendsOverlay, TestSpaceAlwaysOpensContextActions, TestCtrlCAlwaysQuits, TestMutatorsNeverOverlap, TestSingleTimerToken, TestActionDuringTickIsRejected, TestStaleTimerAndMutationTokensAreIgnored, TestActionCompletionDoesNotDuplicateArmedTimer, TestConflictReloadsWithoutRetry, TestSearchSeqAndRevision, TestTickPendingCoalesces, TestToastSequence.

## Phases 5–7 — Pages, data-plane, system pages and root-owned Config form

Commands: `go test ./internal/ui/pages/ ./internal/ui/components/ ./internal/app/`

Red-run findings (genuine defects):

| Finding | Kind | Fix |
|---|---|---|
| page views overflowed compact widths (dashboard 128 cols at 109) | page bug | every page clips its output ANSI-aware to the page width (`base.clip`) |
| `setCatalog` replaced an explicit vanished Location ID with a real one — "Not found" never rendered | page bug | explicit Location IDs (from `Enter`) never fall back; UI selections use the same-ordinal rule |
| not-found computed only in `SetCatalog` — stale after navigation | page bug | computed lazily in every page `View` |
| agent keys (p/w/c) dispatched intents on invalid states | page bug | guards emit recoverable toasts instead |
| events page disabled follow *after* cursor handling — cursor keys returned early | page bug | follow-disable runs before cursor dispatch |
| snapshots `r` navigated without creating the plan | page bug | `r` batches snapshot-plan + Reconcile push |
| reconcile include/exclude mutated the read-only catalog | contract bug | page-local exclusion map; apply carries `Action.Selection` of included op IDs; backend honors Selection |
| root: async `FilterMatchesMsg` never reached open selector overlays (filter never applied in-app) | root bug | non-key messages route to open selector overlays |
| root: huh group-transition messages went to the active page instead of the open form | root bug | non-key messages route to the open config form via shared completion check |
| cross-field rules in the retention field blocked form navigation after an invalid submit | design | cross-field rules run solely in the final Confirm.Validate per plan; `No`-completion = root-owned discard |
| test double `RecordingBackend` never committed revisions — second action always looked stale and stuck the lane | test bug | double commits its catalog like the real backend |
| selector rows wrapped each in a border and pagination hid rows (bubbles list subtracts its filter-title row) | component bug | single bordered panel, delegate spacing aligned with `updatePagination` budget |

Green: all package tests PASS under `go test -race ./internal/...`.
Coverage after phases 5–7: **82.9%** of `./internal/...` statements (gate ≥80%).

## Phases 8 + acceptance — executable, tour, PTY smoke, final gate

### VHS tour findings (red → green)

First recording drifted off-script: typing into the palette without `/` never engaged
the built-in filter, so palette Enters chose index 0 and cascaded into wrong
routes; pushing Dashboard mid-stack also stacked duplicate breadcrumbs. Fixes:
**selector auto-enters filtering on printable keys** (navigation keys j/k and `/`
itself excluded; the auto path now returns BOTH the transition and filter
commands — dropping the filter command kept matches from ever computing in a
real terminal), **navigating home collapses the stack**, and a
history-search hit now carries its typed scope (`terminal:term-0001`) so jumps
land on the right context. Final tape uses settled single-Enter palette
navigation; verified frame-by-frame (agents prompt accepted → snapshots plan →
reconcile apply+undo → tmux probe → machines → Huh config walk → search jump →
clean quit to shell).

### PTY smoke launches (real pseudo-terminals, `internal/app/ptysmoke_test.go`)

Test-driver findings fixed along the way: PTY Esc merges with the next byte
into an Alt-sequence (settled `sendEsc`), stale renderer diffs pollute plain
buffers (full-repaint `snapshot`/`expectScreen` via size nudge at the current
size class), and the palette apply/choose Enter race (race-tolerant `paletteGo`).

**Launch A — default** (`./bin/tmh-next --live=false --scenario default`): all 8
checklist steps PASS — picker opens, Esc → Dashboard, palette → Agents,
needs_input agent prompted (running + timeline/event/history + MOCK success),
snapshots → plan → Reconcile include/exclude selector → apply confirm →
field-level undo, config draft saved (24h retention visible; reopen+Esc
discards, route stays Config), search `ledger` jumps to owning agent, context
selector survives 70×18 too-small with selection/revision unchanged, `q`
exits 0. Transcript: `artifacts/pty-launch-a.txt`.

**Launch B — degraded** (`--scenario degraded --page backends --resource
backend:native`): initial stack `[dashboard, backends]`; native probe fails
inline `probe_timeout` with revision unchanged; input still live (tmux probe
ok); machines build-01 connect → `unreachable` without mutation; Esc×2 →
Dashboard; `q` exits 0. Transcript: `artifacts/pty-launch-b.txt`.

### Final automated gate (project root, 2026-10-07)

| Command | Result |
|---|---|
| `go fmt ./...` | 0 (clean) |
| `go mod tidy` | 0 |
| `go vet ./...` | 0 |
| `go test -race -coverpkg=./internal/... -coverprofile=coverage.out ./...` | **0** — 9 packages ok |
| `go tool cover -func=coverage.out \| tee coverage.txt` + awk ≥80 gate | total **86.8%** ≥ 80.0 — PASS |
| `go build -trimpath -o ./bin/tmh-next ./cmd/tmh-next` | 0 |
| `vhs ./demo.tape` | 0 |
| `test -s ./artifacts/tmh-next-tour.gif` | 0 (2.6 MB) |

Toolchain note: Go 1.26 toolchains ship without a standalone `covdata`; the
first multi-package coverage run exits 1 while tests pass. Packages without
test files trigger the internal covdata path, so every internal package now
carries at least a smoke test — the exact gate command then returns 0 with no
warnings.

| Item | Value |
|---|---|
| `go version` | go1.26.0 darwin/arm64 (GOTOOLCHAIN=auto over local go1.22.5) |
| `vhs --version` | vhs version 0.11.0 |
| capture terminal | 1600×1000, font size 14 (per demo.tape) |
| coverage | 86.8% of `./internal/...` statements |
