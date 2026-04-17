# tmh

> Declarative tmux sessions in YAML, with live-vs-config drift detection
> and a full-featured TUI dashboard. A single Go binary.

[![CI](https://github.com/mark1708/tmh/actions/workflows/ci.yml/badge.svg)](https://github.com/mark1708/tmh/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/mark1708/tmh.svg)](https://pkg.go.dev/github.com/mark1708/tmh)
[![Go Report Card](https://goreportcard.com/badge/github.com/mark1708/tmh)](https://goreportcard.com/report/github.com/mark1708/tmh)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

Русская версия: [README.ru.md](./README.ru.md).

<p align="center">
  <img src="./docs/demo-picker.gif" alt="tmh picker flow" width="720">
</p>

<sub>Three scripted demos live in <code>docs/*.tape</code> — render with <code>make demo</code> (requires <a href="https://github.com/charmbracelet/vhs"><code>vhs</code></a>).</sub>

## Why tmh?

Shell aliases around `tmux` stop scaling around session #5. tmh replaces the
usual pile of `tm-attach`, `tm-new`, `tm-reload` aliases with **one**
declarative config and a tool that:

- Lists sessions/windows/panes with a tree view, process hints, and drift
  badges (`tmh` or `tmh ls`).
- Detects drift between your **config** and the **live** tmux state — `tmh
  diff` shows what's missing, extra, or different. Drift detection is
  verification, not enforcement: you decide whether to push or pull.
- Persists history, snapshots, marks, and hook-trust hashes in a local
  SQLite DB so you can undo destructive actions.
- Runs lifecycle hooks (`on_create`, `on_attach`, `on_destroy`) with a
  trust prompt on first use and on hash change.
- Ships a TUI dashboard with command palette, fuzzy filter, theming, and
  inline pane previews.

## Install

```sh
# Go 1.25+
go install github.com/mark1708/tmh/cmd/tmh@latest
```

```sh
# Homebrew (macOS / Linux)
brew install mark1708/tap/tmh
```

```sh
# From source
git clone https://github.com/mark1708/tmh.git
cd tmh && go build -o ~/.local/bin/tmh ./cmd/tmh
```

tmh expects **tmux ≥ 3.2** and a POSIX shell (bash, zsh, or fish).

## 30-second tour

```sh
# Bootstrap config from the live tmux server
tmh sync --bootstrap

# See what you've got
tmh ls

# Open the TUI dashboard
tmh

# Check environment and tmux integration
tmh doctor
```

`tmh sync --bootstrap` introspects the running tmux server, infers common
root directories, and writes a minimal `~/.config/tmh/config.yml` you can
edit and commit to git.

## Config in 10 lines

```yaml
version: 1

roots:
  work: ~/work/orgA

sessions:
  api:
    root: work
    windows:
      server: {path: services/api, command: go run ./cmd/api}
      logs:   {path: services/api, command: tail -f /tmp/api.log}
```

Everything else (templates, profiles, hooks, env, layouts, pane trees) is
optional. See [`examples/`](./examples) for realistic setups and
[README.ru.md](./README.ru.md) for the full field-by-field reference.

## Documentation

- [README.ru.md](./README.ru.md) — full reference (Russian, comprehensive).
- [CHANGELOG.md](./CHANGELOG.md) — release notes.
- [CONTRIBUTING.md](./CONTRIBUTING.md) — how to build, test, contribute.
- [SECURITY.md](./SECURITY.md) — disclosure policy.
- [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) — community expectations.
- [`examples/`](./examples) — minimal, monorepo, polyglot, devops configs.

## Feature map

| Area           | Capabilities                                                             |
|----------------|--------------------------------------------------------------------------|
| CLI            | `attach new init kill ls ps sync diff reload watch status doctor`        |
| State          | `snapshot undo export import scratch popup window layout tmux audit`     |
| TUI            | Tree dashboard, palette, fuzzy filter, marks, toast, theming, i18n       |
| Persistence    | SQLite state DB, JSONL history, comment-preserving YAML writer           |
| Drift          | `tmh diff` / `tmh watch` / footer badge via `tmh status` in tmux         |
| i18n           | English + Russian bundled (`--lang`, `$LANG`, `defaults.lang`)           |

## Security

tmh writes `config.yml`, the state DB, and history with `0600` perms and
gates hook execution behind a per-hash trust prompt. See
[SECURITY.md](./SECURITY.md) for the full policy and the private disclosure
channel.

## Contributing

Pull requests welcome. Before opening one, please:

1. Run `go test -race ./...` and `go vet ./...`.
2. Keep files focused (<800 lines); extract when they grow.
3. Update the CHANGELOG under `[Unreleased]`.
4. See [CONTRIBUTING.md](./CONTRIBUTING.md) for the layout tour and
   commit-message conventions.

Bug reports and feature requests use the issue forms under
[`.github/ISSUE_TEMPLATE/`](./.github/ISSUE_TEMPLATE). Please run `tmh
doctor` first — it answers most environment questions.

## License

[MIT](./LICENSE) © 2026 Mark Guriev
