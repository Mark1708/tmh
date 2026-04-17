# Migrating from zsh tmux aliases to tmh

If you have the usual pile of `tm-*` aliases accumulated over the years,
this is what they map to in tmh. Nothing about the underlying tmux
behaviour changes — tmh just gives you one declarative config and one
binary instead of nine shell aliases and one INI file.

## Before — typical zsh setup

```zsh
# ~/.zshrc excerpt — 9 aliases, one ad-hoc INI file
alias tm='tmux attach || tmux new-session -s main'
alias tm-new='tmux new-session -d -s'
alias tm-ls='tmux list-sessions'
alias tm-kill='tmux kill-session -t'
alias tm-reload='tmux source ~/.tmux.conf'

tm-bootstrap() {
  # reads ~/.config/tm-sessions.ini and calls `tmux new-session` N times
  while IFS='=' read -r name dir; do
    [[ $name == '#'* ]] && continue
    tmux has-session -t "$name" 2>/dev/null && continue
    tmux new-session -d -s "$name" -c "$dir"
  done < ~/.config/tm-sessions.ini
}

tm-sync() { echo "TODO"; }   # never actually implemented
tm-save() { echo "TODO"; }
tm-undo() { echo "TODO"; }
```

```ini
# ~/.config/tm-sessions.ini — flat, no templating, no drift detection
api=/Users/you/work/orgA/api
web=/Users/you/work/orgA/web
notes=/Users/you/work/personal/notes
```

The pain points this hits:

- **No drift detection.** If tmux rolls back a window because your
  laptop rebooted, nothing tells you.
- **Flat INI.** Can't share a baseline with a teammate without exposing
  your home directory; no env, no hooks, no templates.
- **No undo.** Accidental `tm-kill api` = lost layout.
- **No ambient discovery.** New project? Edit the INI or one-shot tmux.

## After — tmh

Install once:

```sh
go install github.com/mark1708/tmh/cmd/tmh@latest
# or: brew install mark1708/tap/tmh
```

Bootstrap from whatever tmux already has running:

```sh
tmh sync --bootstrap
```

That writes `~/.config/tmh/config.yml` with inferred roots and every
live session enumerated. From there, the alias → command translation:

| zsh alias                | tmh equivalent                       |
|--------------------------|--------------------------------------|
| `tm`                     | `tmh` (interactive picker)           |
| `tm` (in a tmux pane)    | `tmh` (still picker; Enter switches) |
| `tm-new <name>`          | `tmh new <name>` or edit YAML + `tmh init` |
| `tm-ls`                  | `tmh ls`                             |
| `tm-kill <name>`         | `tmh kill <name>`  (undoable)        |
| `tm-reload`              | `tmh reload --tmux`                  |
| `tm-bootstrap`           | `tmh init`                           |
| `tm-sync` (was TODO)     | `tmh sync --push` / `--pull`         |
| `tm-save` (was TODO)     | `tmh snapshot save`                  |
| `tm-undo` (was TODO)     | `tmh undo`                           |

## Example config after `tmh sync --bootstrap`

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mark1708/tmh/main/schemas/tmh.schema.json

version: 1

roots:
  work: ~/work/orgA
  home: ~/work/personal

sessions:
  api:
    root: work
    windows:
      server: {path: api, command: go run ./cmd/api}
      logs:   {path: api, command: tail -f /tmp/api.log}
  notes:
    root: home
    windows:
      edit: {path: notes, command: nvim}
```

## What you gain

1. **Drift detection.** `tmh diff` tells you what moved since you last
   init'd — rebooted laptop, flaky tmux-continuum restore, teammate
   edits to the config.
2. **Undo.** Destructive actions (kill, layout clobber) are snapshotted.
3. **Sharing.** Commit the YAML; use `tmh export --minimal` to share
   sanitised versions (env secrets redacted, paths normalised to
   `{root, path}` pairs).
4. **Ambient discovery.** Add a `discover:` rule and every project
   directory under `~/work/*` shows up in the picker without enumeration.
5. **Hooks.** `on_create`, `on_attach`, `on_destroy` at session,
   window, or pane level — with a trust prompt so shell commands in a
   YAML file never run behind your back.
6. **First-class i18n.** English + Russian ship in the binary.

## If you want to keep the alias muscle memory

```zsh
alias tm='tmh'               # picker-first behaviour matches old `tm`
alias tm-ls='tmh ls'
alias tm-kill='tmh kill'
alias tm-reload='tmh reload --all'
alias tm-bootstrap='tmh sync --bootstrap'
```

`tm-new` intentionally has no direct alias — prefer editing the YAML
and running `tmh init`, or use `tmh new --name foo --dir ~/work/foo`
for genuine ad-hoc cases.

## What doesn't change

- tmux itself, its keybindings, its config file (`~/.tmux.conf`).
- Your shell, your prompt, your editor.
- Existing plugins (`tmux-resurrect`, `tmux-continuum`) — tmh is
  orthogonal; they save pane content, tmh tracks layout declaratively.

If something here doesn't cover your old alias, open an issue — we'd
rather add a first-class subcommand than send you back to raw `tmux`
commands for routine workflows.
