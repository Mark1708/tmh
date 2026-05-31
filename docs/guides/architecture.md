# Architecture

High-level map of the tmh codebase for contributors and reviewers. Kept
intentionally short — the code itself is the source of truth and files
referenced here are all under 800 lines.

## Layering

```
cmd/tmh              Cobra CLI entry points — thin wrappers, no business logic
cmd/tmh-gen          Build-time generator (schema, man pages, completions)

internal/actions     Stateful side effects: attach, sync, reload, freeze, init,
                     kill, import, export, snapshot, undo, hooks, tmux_audit
internal/config      YAML parsing + validation + resolver (profiles, templates,
                     inheritance), JSON schema, comment-preserving writer,
                     discover rules (glob + optional zoxide)
internal/tmux        Runner interface + CLI implementation (spawning `tmux`).
                     `tmuxtest` is a pure-Go MockRunner used across tests.
internal/state       SQLite store: history, snapshots, marks, trust-hashes,
                     reload queue. Pure-Go driver (modernc.org/sqlite) — no CGO.
internal/ui          Bubble Tea TUI: dashboard, palette, picker, settings,
                     diff/confirm/history screens, theming, i18n strings.
internal/i18n        go-i18n v2 bundle + English + Russian locales.
internal/errors      Typed sentinel errors (tmux-layer, config-layer, hooks).
internal/shell       $SHELL → rc-file resolution.
internal/xdg         XDG config/state/cache paths.
```

## Data flow: `tmh sync --push` (apply config to tmux)

```
                 ┌──────────────┐
cmd/tmh/cmd/sync │  newSyncCmd  │
                 └───────┬──────┘
                         │ loadConfig() → *config.Config
                         ▼
         actions.Push(ctx, Runner, cfg, opts)
                         │
                         ├── config.Resolve(cfg, profile)   // inherit defaults,
                         │                                  // apply templates
                         │
                         ├── for each ResolvedSession:
                         │     Runner.HasSession / NewSession / NewWindow
                         │     applyWindowLayout
                         │
                         └── SyncReport (created / updated / skipped)
```

## Data flow: `tmh sync --pull` / `tmh freeze` (live → config)

```
                     ┌────────────────┐
                     │ Runner.ListSessions, ListPanes, ListWindows
                     └────────┬───────┘
                              ▼
                   config.LiveSnapshot (in-memory)
                              │
                              ▼
           config.Diff(resolved, live)  →  []config.Drift
                              │
                   ┌──────────┼──────────┐
                   ▼                      ▼
              sync --pull              tmh freeze
              (destructive,            (non-destructive,
              applies via              applies via
              PathSet/PathDelete)      PathSet only)
                   │                      │
                   └──────────┬───────────┘
                              ▼
                    config.Write(cfg, path, opts)
                              │
                        yaml.Node tree → file
                        (comments preserved)
```

## State DB schema

See `internal/state/db.go` for the authoritative DDL. Tables:

| Table              | Purpose                                              |
|--------------------|------------------------------------------------------|
| `events`           | Append-only audit log keyed by unix-ts               |
| `snapshots`        | Serialised session trees for `tmh undo`              |
| `trust`            | `(config_path, config_hash)` — hook trust decisions  |
| `reload_queue`     | Panes queued for shell re-source while busy          |
| `marks`            | Named target bookmarks for TUI                       |

Both CLI and TUI open the same DB. WAL + `busy_timeout=5s` + `foreign_keys=on`
are applied on every connect.

## Key design rules

1. **Non-destructive by default.** Writes through `config.PathSet` /
   `config.PathDelete` keep the yaml.Node tree intact — comments and
   template references never rot. Destructive changes require explicit
   `--all` / `--apply-all`.

2. **Runner interface is the only tmux seam.** Production uses
   `tmux.CLIRunner`; tests use `tmux.tmuxtest.MockRunner`. No package
   outside `internal/tmux` forks `tmux` directly.

3. **`config.Config` is parsed once, passed by reference.** Mutations go
   through `PathSet`/`PathDelete`; callers write back via `config.Write`.

4. **TUI sub-models receive messages, own their state, and return
   updated copies** per the Bubble Tea contract. The root model routes
   messages to the active screen only.

5. **Hooks are imperative — excluded from drift.** `config/diff.go`
   compares dirs, commands, and layouts; Hooks fields are ignored.

6. **i18n JSON output is English-stable.** `--json` on ls/diff/audit
   emits English `ReasonCode` strings for script consumers regardless of
   active locale.

## Adding a new subcommand

1. Create `cmd/tmh/cmd/<name>.go` with a `newFooCmd() *cobra.Command`.
2. Register it inside `root.AddCommand(...)` in `cmd/tmh/cmd/root.go`.
3. Put side-effect logic in `internal/actions/<name>.go` so the CLI
   stays thin and the logic is callable from the TUI.
4. Add i18n keys for `cli.<name>.short` and any flag descriptions to
   both `internal/i18n/locales/en.json` and `ru.json`.
5. Run `make docs` to refresh man pages, completions, and JSON schema.
6. Add tests in `internal/actions/<name>_test.go` — use MockRunner.

## Adding a new config field

1. Add the field to the relevant struct in `internal/config/types.go`
   with `yaml:"..."` tag (used by both parser and schema reflector).
2. If the field participates in inheritance/resolution, update
   `internal/config/resolver.go` to propagate it into the Resolved
   types.
3. If the field affects drift semantics, update `internal/config/diff.go`.
4. Run `make docs` — the JSON schema regenerates from tags.
5. Add tests.
