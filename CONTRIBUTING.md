# Contributing

Thanks for the interest. tmh is a single-binary Go project; the codebase is
small enough that you can read the whole thing in an afternoon.

## Layout

```
cmd/tmh/        cobra entrypoint + subcommands
internal/
  config/       YAML parser, resolver, validator, atomic writer, diff
  tmux/         Runner interface + CLIRunner (production)
  tmux/tmuxtest MockRunner (tests only — never imported by production)
  actions/      side-effect API shared by CLI and TUI frontends
  state/        SQLite (events, snapshots, trust, reload_queue)
  errors/       typed sentinels
  xdg/          XDG paths
```

## Rules

- All side-effects live in `internal/actions`. Keep CLI and (future) TUI
  thin — they only translate flags into `actions.*` calls.
- `internal/tmux.Runner` is the single seam for tmux. Tests use
  `tmuxtest.MockRunner`. Never call `exec.Command("tmux", ...)` directly
  outside `internal/tmux`.
- Mutations to `config.yml` go through `config.PathSet/Delete/Rename`
  + `config.Write`. The yaml.Node tree is preserved so comments survive.
- New errors get a typed sentinel in `internal/errors` and are wrapped with
  `fmt.Errorf("...: %w", ErrX)`.

## Workflow

```sh
make test       # go test ./...
make test-race  # mandatory before pushing
make lint       # golangci-lint (when added to your toolchain)
```

CI runs `go test -race -coverprofile`. Coverage threshold is 60% (raise as
the project matures).

## Commit messages

Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`,
`chore:`. Body should explain *why*, not *what*.

## Running against your real tmux

```sh
go build -o /tmp/tmh ./cmd/tmh
/tmp/tmh ls
/tmp/tmh --config /tmp/scratch-config.yml sync --bootstrap
```

Don't point `--config` at your real `~/.config/tmh/config.yml` while
hacking — use a scratch path until your change is reviewed.
