# tmh-next — terminal-native demo cockpit (Charm v2)

A fullscreen Go TUI that demonstrates the full tmh-next product surface on
**deterministic mock data**: picker-first startup, dashboard, 15 routes,
typed navigation stack, serialized mock mutations with busy/error/success
states, root-owned Huh config form, command palette, quick switch and a
reproducible VHS tour.

Everything that looks like a side effect (`attach`, `send`, `kill`,
`restore`, `probe`, `connect`, `save`, …) mutates only an **in-memory
catalog** and carries a visible `MOCK` badge. Nothing touches tmux, Zellij,
processes, files, sockets or `~/.config/tmh`. Restarting resets to the
fixture.

## Run

```bash
go run ./cmd/tmh-next                       # picker-first: dashboard + quick switch
go run ./cmd/tmh-next --dashboard           # dashboard only
go run ./cmd/tmh-next --page agents \
    --resource agent:agent-0002             # [dashboard, agents] deep link
go run ./cmd/tmh-next --scenario degraded \
    --page backends --resource backend:native
go run ./cmd/tmh-next --live=false          # deterministic capture (no ticks)
```

Build: `go build -trimpath -o ./bin/tmh-next ./cmd/tmh-next`.

## Flags

| Flag | Meaning |
|---|---|
| `--dashboard` | open `[dashboard]` (mutually exclusive with `--page`) |
| `--page <route>` | startup route; non-dashboard pages stack over Dashboard so `esc` returns home |
| `--resource <kind>:<id>` | explicit primary/context resource for the page |
| `--scenario default\|empty\|degraded` | deterministic fixture set |
| `--live=false` | disable the 1s periodic mock ticks (user actions stay async) |

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
| `?` | help overlay |
| `esc` | close overlay / exit filter / pop route (Dashboard is the bottom) |
| `q`, `ctrl+c` | quit |
| `/` | filter on list pages |
| `tab`/`shift+tab` | compact master↔detail region · reconcile mode cycle |

Page keys are listed in each page's footer help (e.g. Agents: `enter` focus,
`p` prompt, `w` wait, `c` claim; destructive actions always confirm with a
`MOCK ONLY` dialog first).

## Routes

`dashboard` · `workspaces` · `workspace` · `active` · `terminals` · `agents`
· `history` · `search` · `snapshots` · `reconcile` · `backends` · `machines`
· `performance` · `events` · `config`

Layout: wide (≥110 cols) master/detail 40/60; compact (72–109) single region
with `tab`; below 72×20 a dedicated too-small view (only `q`/`ctrl+c`).
Colors are Catppuccin Mocha/Latte, switched after the terminal answers the
background-color query; dark is the default until then.

## Architecture

```
cmd/tmh-next              wiring: flags → demo client → resident pages → tea
internal/domain           current UI catalog, actions and typed errors
internal/control          runtime snapshot/client/watch contracts
internal/runtimegraph     normalized machine/backend/workspace/view/surface graph
internal/backend/zellij   read-only Zellij 0.45.1 discovery adapter
internal/mock             deterministic demo provider and logical transitions
internal/app              root shell: route stack, mutation lane, overlays, search
internal/ui               shared route/page/command contracts, theme and layout
internal/ui/pages         the fifteen resident pages
```

The shell depends on `control.Client`
(`Snapshot`/`Search`/`Execute`). Mock-only time progression is the separate
`control.DemoTicker` extension, so it cannot leak into a production
transport. `mock.Client` preserves the existing deterministic demo.

The production model is `runtimegraph.Graph`: Machine → BackendInstance →
Workspace → View → Surface, with Terminal attached through a Surface and
Agent optionally attached to a Terminal or View. Native Zellij ids are
runtime bindings, not product ids. `backend/zellij.Adapter` currently
implements strict, read-only discovery using argv-safe `zellij 0.45.1` CLI
calls. It is deliberately not wired into the default demo executable yet;
no mutation command is enabled before daemon-side ownership, capability,
revision and idempotency gates exist.

Mutation discipline: at most one `Execute` **or** `Tick` in flight (one
armed 1s timer, a single pending-tick bit, identity+revision-checked
results); search runs as a separate concurrent read lane that only accepts
the latest sequence at the current revision. Failures never mutate the
catalog, revision, logical clock or audit trail.

## Development

```bash
go fmt ./... && go vet ./...
go test -race -coverpkg=./internal/... -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1     # ≥80% gate
go build -trimpath -o ./bin/tmh-next ./cmd/tmh-next
vhs ./demo.tape                                 # → artifacts/tmh-next-tour.gif
```

Opt-in live Zellij discovery test (use only a disposable managed session):

```bash
export ZELLIJ_SOCKET_DIR=/tmp/zellij-tmh
zellij attach --create-background tmh-integration-check -- sleep 300
TMH_ZELLIJ_INTEGRATION=1 TMH_ZELLIJ_SESSION=tmh-integration-check \
  go test -tags=integration ./internal/backend/zellij \
    -run TestLiveDiscoveryOfManagedSession -count=1 -v
zellij kill-session tmh-integration-check
```

The short socket directory avoids macOS `$TMPDIR` exceeding Zellij's Unix
socket path limit.

See `RESEARCH.md` (pinned Charm v2 evidence) and `VERIFICATION.md` (gates,
red/green history, PTY transcripts).

This demo is not a promise of production runtime durability or PTY
semantics; it is the interactive product-surface specification.
