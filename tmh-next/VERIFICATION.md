# VERIFICATION.md — tmh-next implementation evidence

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
| `zellij --version` | zellij 0.45.1 (Homebrew arm64 bottle) |
| terminal (captures) | ghostty 1.3.1, macOS darwin 24.6.0, arm64 |

Commands run from the repository `tmh-next/` directory unless noted.

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
navigation; verified frame-by-frame (animated attach/terminal-creation help →
agents prompt accepted → snapshots plan → reconcile apply+undo → tmux probe →
machines → category-scoped Settings form → search jump → clean quit to shell).

### PTY smoke launches (real pseudo-terminals, `internal/app/ptysmoke_test.go`)

Test-driver findings fixed along the way: PTY Esc merges with the next byte
into an Alt-sequence (settled `sendEsc`), stale renderer diffs pollute plain
buffers (full-repaint `snapshot`/`expectScreen` via size nudge at the current
size class), and the palette apply/choose Enter race (race-tolerant `paletteGo`).

**Launch A — default** (`./bin/tmh-next --live=false --scenario default`): all
checklist steps PASS — picker opens, Esc → Dashboard, palette → Agents,
needs_input agent prompted (running + timeline/event/history + MOCK success),
snapshots → plan → Reconcile include/exclude selector → apply confirm →
field-level undo, Settings → History saves 24h retention through a focused Huh
form (reopen+Esc discards while keeping the category and route), search
`ledger` jumps to the owning agent, context selector survives 70×18 too-small
with selection/revision unchanged, `q` exits 0. Transcript:
`artifacts/pty-launch-a.txt`.

**Launch B — degraded** (`--scenario degraded --page backends --resource
backend:native`): initial stack `[dashboard, backends]`; native probe shows
`Backend probe timed out` with revision unchanged; input remains live (tmux
probe succeeds); machines build-01 connect shows `Machine connect unavailable`
without mutation; Esc×2 → Dashboard; `q` exits 0. Transcript:
`artifacts/pty-launch-b.txt`.

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


## Production implementation (2026-10-07)

The production path now includes:

- normalized Zellij discovery with stable product IDs and adapter-only native
  identifiers;
- a reconciliation daemon over a permission-bounded Unix HTTP/JSON socket;
- SQLite catalog, audit-event and idempotency-result persistence;
- desired-state loading from `~/.config/tmh/config.yml`;
- revision-checked mutations, runtime rediscovery and watch-stream updates;
- bounded, ANSI-stripped, duplicate-suppressed and secret-redacted pane
  capture;
- TUI-side interactive attach after Bubble Tea releases the terminal;
- an explicit `--demo` switch preserving the original deterministic backend.

Focused regression coverage includes graph referential integrity, malformed
Zellij JSON, minimum-version rejection, session-local native ID collisions,
stdout/stderr separation, ownership gates, command argument safety, timeout
handling, atomic catalog/result commits, persisted idempotency results,
history capture and redaction, production-aware confirmation copy, daemon API
round trips and TUI initialization through the remote client.

### Final automated gate

| Command | Result |
|---|---|
| `gofmt -w ./cmd ./internal` | 0 |
| `go mod tidy` | 0 |
| `go vet ./...` | 0 |
| `go test -count=1 -race -coverpkg=./internal/... -coverprofile=coverage.out ./...` | **0** — 17 packages ok, 2 command packages with no tests |
| `go tool cover -func=coverage.out \| sed -n '$p'` | total **81.7%** ≥ 80.0 — PASS |
| `go build -trimpath -o ./bin/tmhd ./cmd/tmhd` | 0 |
| `go build -trimpath -o ./bin/tmh-next ./cmd/tmh-next` | 0 |
| `vhs ./demo.tape` | 0 |
| `stat artifacts/tmh-next-tour.gif` | 3,536,714 bytes |

### Runtime-feedback and usability revision

The requested review items are covered by focused regressions and real terminal
journeys:

- `TestProjectRuntimeBuildsValidatedLiveCatalog` proves every production
  snapshot exposes the local performance service, target SLOs, trace storage
  and benchmark storage. Persisted stale/unavailable performance state is
  repaired during live projection.
- Typed backend failures now render operation-specific messages without
  duplicating the machine error code. Conflict reload failures use a dedicated
  `ReloadFailedMsg`, release the serialized mutation lane and re-arm refresh.
  Repeated identical notifications coalesce instead of flooding the screen.
- Settings is a vertical General/Runtime/History/Security browser. Each category
  opens a focused Huh form; source, current values and safety descriptions stay
  visible. The UI shows success only after backend commit.
- Workspace and terminal details explain Zellij ownership. `n` creates a new
  terminal pane beside a live managed terminal; `enter` attaches and returns
  after detach. Exited terminals are rejected locally with actionable copy.
- `?` opens four auto-advancing walkthroughs: attach a session, create a
  terminal, recover workspace drift and change settings. Playback, manual
  stepping, use-case switching, restart and destination launch are covered by
  `TestGuidedHelpAnimationAndUseCaseLaunch`.
- The revised VHS tour renders the animated help and focused Settings path.
  Both real PTY demo launches pass after the copy and interaction changes.

An isolated production smoke used `tmhd` with an empty temporary YAML config,
its own Unix socket and SQLite state. `tmh-next` rendered `LIVE`, animated and
switched help use cases, launched the empty Workspaces destination, opened the
available Performance page, completed `recorded live discovery trace`, opened
Settings → History, discarded the draft with `Settings changes discarded`,
and exited with code 0. The daemon and temporary state were removed afterward.

### Live Zellij and daemon evidence

Live-adapter verification used disposable Zellij sessions under the short
socket root `/tmp/zellij-tmh-final`; all were deleted afterward.

1. `tmhd` started against Zellij 0.45.1 and the real user config. `/v1/snapshot`
   reported scenario `production`, backend `healthy`, and an observed managed
   workspace with a live terminal.
2. A pane containing `history-secret token=abc123` was captured as bounded
   history with `token=[REDACTED]`; repeated screen content was not duplicated.
3. Sending `visible` through `/v1/execute` wrote to the live pane. Repeating
   the exact action with the same idempotency key returned the stored result
   and did not append a second `visible`.
4. A confirmed close transitioned the real pane to `exited`; reconciliation
   retained its historical terminal record while the live surface binding was
   removed.
5. With a Zellij client focused on `tmh-shell-fresh`, a production split moved
   revision 7 → 8 and the returned catalog contained two live shell terminals.
   A detached-session probe that Zellij accepted but did not retain was
   rejected as `invalid_state` and did not commit a catalog revision.
6. `./bin/tmh-next --socket /tmp/tmhd-final.LixHhO/tmhd.sock --dashboard`
   rendered the production `LIVE` Dashboard, opened the command palette via
   `ctrl+p`, and exited 0 via `ctrl+c`.

The opt-in integration test also passed against disposable session
`tmh-integration-20261007-0958`:

```bash
ZELLIJ_SOCKET_DIR=/tmp/zellij-tmh \
TMH_ZELLIJ_INTEGRATION=1 \
TMH_ZELLIJ_SESSION=tmh-integration-20261007-0958 \
  go test -tags=integration ./internal/backend/zellij \
    -run TestLiveDiscoveryOfManagedSession -count=1 -v
```

Result: PASS. On macOS, a short `ZELLIJ_SOCKET_DIR` is required because the
default `$TMPDIR` can exceed Zellij's Unix-socket path limit.