# tmh

Single-binary tmux hub: declarative YAML sessions, live ↔ config sync, dotfile reload, shareable team setup.

## Why

Replaces a pile of zsh-functions and a flat INI file with one Go binary that:

- starts a fully-laid-out workspace from one command (`tmh init`)
- imports an existing tmux session graph into YAML (`tmh sync --bootstrap`)
- shows drift between the live world and the declared one (`tmh diff`)
- re-sources `~/.zshrc` in idle shell panes — and queues busy ones (`tmh reload --shell --busy`)
- snapshots and restores live state (`tmh snapshot save / undo`)
- exports a shareable, secret-stripped slice for teammates (`tmh export --minimal`)

## Install

```sh
# self-hosted (recommended)
GOPRIVATE=git.mark1708.ru/* go install git.mark1708.ru/me/tmh/cmd/tmh@latest

# direct Homebrew formula (no tap)
brew install https://git.mark1708.ru/me/tmh/raw/branch/main/homebrew/tmh.rb
```

`tmh doctor` checks tmux ≥ 3.2, the config schema, the state DB, and missing optional tools.

## Quick start

```sh
# adopt your current tmux setup into config.yml
tmh sync --bootstrap

# from now on, after a reboot:
tmh init

# pick something
tmh attach epcp:lk           # outside tmux: attaches; inside: switches client
tmh ls                       # tree of configured + live + ad-hoc

# kept your dotfiles in sync
tmh reload --shell           # source ~/.zshrc in idle panes
tmh reload --shell --busy    # also queue non-idle ones for later
tmh watch                    # foreground fsnotify; events fire on save
```

## Config

YAML, structurally referenced — no Mustache. See `examples/`:

- `minimal.yml` — one window, one command
- `monorepo.yml` — every service is a window, env per session, `on_attach` hook
- `polyglot.yml` — profiles + per-stack env
- `devops.yml` — kubectl context per cluster

```yaml
version: 1
roots:
  acme: ~/Code/acme
defaults:
  layout: 3-pane
sessions:
  acme:
    root: acme
    env: { ENV: dev }
    windows:
      gateway: services/gateway
      web:
        path: web
        layout: 2-pane
        command: pnpm dev
```

## Commands

```
attach    new       init      kill       ls          sync
diff      reload    watch     status     scratch     popup
window    layout    snapshot  undo       export      import
config    doctor    version   completion
```

`tmh --help` for full flags. `tmh completion zsh|bash|fish` for shell completion.

## Sharing with a teammate

```sh
# you
tmh export --minimal > team.yml
# strips *_TOKEN/_KEY/_SECRET env values and rewrites absolute paths via roots

# them
brew install https://git.mark1708.ru/me/tmh/raw/branch/main/homebrew/tmh.rb
tmh import team.yml --merge
tmh init
```

## License

MIT — see [LICENSE](LICENSE).
