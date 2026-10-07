# tmh-next — terminal-native control plane (Charm v2)

A fullscreen Go TUI for inspecting and controlling local Zellij workspaces,
tabs, panes and agent processes. The default executable uses a local `tmhd`
daemon, a permission-bounded Unix socket and a durable SQLite catalog. The
original deterministic 15-page demo remains available with `--demo`.

Production mode currently targets Zellij **0.45.1 or newer**. It discovers the
live runtime, reconciles desired workspaces from `~/.config/tmh/config.yml`,
captures bounded/redacted pane history, persists snapshots and audit events,
and revision-checks every command while deduplicating committed results.
Interactive attach runs in the TUI process after Bubble Tea releases the
terminal; the daemon never owns the user's PTY.

## Build and run

```bash
go build -trimpath -o ./bin/tmhd ./cmd/tmhd
go build -trimpath -o ./bin/tmh-next ./cmd/tmh-next
./bin/tmh-next                             # picker-first, production mode
./bin/tmh-next --dashboard                 # open the live Dashboard directly
./bin/tmh-next --page agents               # [dashboard, agents]
```

`tmh-next` connects to the default Unix socket and, when necessary, starts the
sibling `tmhd` binary automatically. Runtime state defaults to
`~/.local/state/tmh/state.db`; daemon logs go to
`~/.local/state/tmh/tmhd.log`. To supervise the daemon yourself:

```bash
./bin/tmhd --poll 2s
./bin/tmh-next --dashboard
```

Deterministic demo mode is explicit and performs no external side effects:

```bash
./bin/tmh-next --demo
./bin/tmh-next --demo --scenario degraded \
  --page backends --resource backend:native
./bin/tmh-next --demo --live=false
```

## Flags

| Flag | Meaning |
|---|---|
| `--dashboard` | open `[dashboard]` (mutually exclusive with `--page`) |
| `--page <route>` | startup route; non-dashboard pages stack over Dashboard |
| `--resource <kind>:<id>` | explicit primary/context resource for the page |
| `--socket <path>` | production `tmhd` Unix socket |
| `--demo` | use the deterministic in-memory backend |
| `--scenario default\|empty\|degraded` | demo fixture set |
| `--live=false` | disable periodic ticks in demo mode |

Invalid combinations (`--dashboard` + `--page`, `--resource` without
`--page`, unknown route/kind/scenario, resource on aggregate pages, wrong
kind for a route) exit with code **2** before the alternate screen opens.
An unknown but well-formed resource id renders an in-app *Not found* state
with disabled actions — it is never silently replaced.

## Global keys

| Key | Action |
|---|---|
| `ctrl+p` | command palette (navigate + page commands) |
| `ctrl+k` | quick switch (workspaces · views · terminals · agents) |
| `space` | current page's contextual command selector |
| `?` | animated use-case help (attach, create terminal, recover drift, settings) |
| `esc` | close overlay / exit filter / pop route (Dashboard is the bottom) |
| `q`, `ctrl+c` | quit |
| `/` | filter on list pages |
| `tab`/`shift+tab` | compact master↔detail region · reconcile mode cycle |

Page keys are listed in each page's footer help (for example, Agents:
`enter` focus, `p` prompt, `w` wait, `c` claim). Destructive production
actions render a `LIVE ACTION` confirmation; demo confirmations render
`MOCK ONLY`.

The help overlay auto-advances through each workflow. Use `←`/`→` (or
`h`/`l`) to switch use cases, `j`/`k` to inspect steps, `space` to
pause/resume, and `enter` to open the relevant page.

## Routes

`dashboard` · `workspaces` · `workspace` · `active` · `terminals` · `agents`
· `history` · `search` · `snapshots` · `reconcile` · `backends` · `machines`
· `performance` · `events` · `config`

Layout: wide (≥110 cols) master/detail 40/60; compact (72–109) single region
with `tab`; below 72×20 a dedicated too-small view (only `q`/`ctrl+c`).
Colors are Catppuccin Mocha/Latte, switched after the terminal answers the
background-color query; dark is the default until then.

The `config` route is labeled **Settings** in the UI. It uses a vertical
General/Runtime/History/Security browser and opens a focused Huh editor for
only the selected category. Save success is shown only after the backend
commits; `Esc` discards without contacting it.

## Production behavior

Real command paths include interactive session attach, pane input, new terminal
pane creation (`n` from a workspace), confirmed pane close, durable snapshots,
restore-plan push/pull/freeze, history export, backend probes, machine health
checks and performance probes. Attach suspends Bubble Tea while Zellij owns the
terminal; detaching resumes tmh-next and refreshes the catalog.
Raw Zellij identifiers remain adapter-only bindings; product IDs are stable
hashes scoped by backend and session.

Mutation safety:

- commands are argv-safe; no shell command strings are constructed;
- every mutation carries the catalog revision and is committed atomically with
  its idempotency result;
- runtime-changing commands are rediscovered before the result is published;
- pane input, split, close and restore-push are allowed only for `tmh-*`
  sessions or sessions declared in `~/.config/tmh/config.yml`;
- destructive operations require explicit confirmation;
- socket directories and sockets are mode `0700`/`0600`;
- pane captures are bounded, duplicate-suppressed and redact secret-like
  `token`, `password`, `api_key` and `secret` assignments.

Observed unmanaged sessions remain visible and attachable, but mutating them
returns `protected`. Zellij 0.45.1 can accept an interactive split while no
client is focused and then discard the shell pane; tmhd detects that missing
runtime effect, returns `invalid_state`, and commits nothing. Native pane
re-parenting and restore undo are rejected because Zellij cannot provide those
operations safely. The production backend is local-only; tmux, SSH and
remote-machine execution are not enabled.

## Architecture

```text
cmd/tmh-next              CLI, daemon auto-start, production/demo wiring
cmd/tmhd                  Unix-socket daemon and reconciliation loop
internal/control          snapshot/search/execute/watch contracts
internal/runtimegraph     machine → backend → workspace → view → surface graph
internal/backend/zellij   strict Zellij 0.45.1 discovery adapter
internal/production       projection, commands, history and idempotency
internal/store            SQLite catalog + command-result transactions
internal/daemonapi        versioned local HTTP/JSON API
internal/remote           Unix-socket TUI client
internal/mock             opt-in deterministic demo backend
internal/app              route stack, mutation lane, overlays and search
internal/ui/pages         fifteen resident pages
```

The TUI depends only on `control.Client`; production and demo providers
implement the same snapshot/search/execute contract. Watch streaming is a
separate extension. Demo time progression is isolated behind
`control.DemoTicker` and is never available through the production transport.

Production reconciliation loads the previous SQLite catalog, current tmh YAML
desired state and a fresh normalized Zellij graph. A revision is written only
when observable state changes; unchanged polls update no durable revision or
audit event.

## Development

```bash
gofmt -w ./cmd ./internal
go mod tidy
go vet ./...
go test -count=1 -race -coverpkg=./internal/... \
  -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1     # ≥80% gate
go build -trimpath -o ./bin/tmhd ./cmd/tmhd
go build -trimpath -o ./bin/tmh-next ./cmd/tmh-next
vhs ./demo.tape                                # deterministic demo tour
```

Opt-in live Zellij discovery test (use only a disposable `tmh-*` session):

```bash
export ZELLIJ_SOCKET_DIR=/tmp/zellij-tmh
zellij attach --create-background tmh-integration-check -- sleep 300
TMH_ZELLIJ_INTEGRATION=1 TMH_ZELLIJ_SESSION=tmh-integration-check \
  go test -tags=integration ./internal/backend/zellij \
    -run TestLiveDiscoveryOfManagedSession -count=1 -v
zellij kill-session tmh-integration-check
```

The short socket directory avoids macOS `$TMPDIR` exceeding Zellij's Unix
socket path limit. See `RESEARCH.md` for pinned Charm v2 evidence and
`VERIFICATION.md` for automated gates, live daemon evidence and PTY results.
