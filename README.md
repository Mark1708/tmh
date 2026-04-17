# tmh

> Declarative tmux sessions in YAML, with live-vs-config drift detection,
> a full-featured TUI dashboard, and a sesh-style fuzzy picker. One Go
> binary, no plugins, no telemetry.

[![CI](https://github.com/mark1708/tmh/actions/workflows/ci.yml/badge.svg)](https://github.com/mark1708/tmh/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/mark1708/tmh.svg)](https://pkg.go.dev/github.com/mark1708/tmh)
[![Go Report Card](https://goreportcard.com/badge/github.com/mark1708/tmh)](https://goreportcard.com/report/github.com/mark1708/tmh)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

Русская версия — [README.ru.md](./README.ru.md).

<p align="center">
  <img src="./docs/demo-picker.gif" alt="tmh picker" width="760">
</p>
<p align="center">
  <sub><code>docs/demo-tour.gif</code> — full dashboard walkthrough · <code>docs/demo-workflow.gif</code> — drift detection and resolution</sub>
</p>

tmh exists because the zsh aliases around `tmux` stop scaling around
session #5. nine `tm-*` aliases, one ini file, no diff, no undo, no
sharing. tmh is the single-binary replacement: one `config.yml`, one
tool, and a `tmh diff` that tells you what drifted so you don't find
out by accident.

---

## Table of contents

- [Install](#install)
- [First run](#first-run)
- [Quick start](#quick-start)
- [UI language](#ui-language)
- [Config — `config.yml`](#config--configyml)
- [CLI reference](#cli-reference)
- [TUI dashboard](#tui-dashboard)
- [Picker (bare `tmh`)](#picker-bare-tmh)
- [Process visibility](#process-visibility)
- [Marks and last-location](#marks-and-last-location)
- [tmux integration](#tmux-integration)
- [Hooks and trust](#hooks-and-trust)
- [Snapshots and undo](#snapshots-and-undo)
- [Sharing with a teammate](#sharing-with-a-teammate)
- [Discovery rules (glob + zoxide)](#discovery-rules-glob--zoxide)
- [`tmh freeze` — capture live into YAML](#tmh-freeze--capture-live-into-yaml)
- [JSON schema and editor integration](#json-schema-and-editor-integration)
- [Security model](#security-model)
- [Troubleshooting](#troubleshooting)
- [Architecture](#architecture)
- [License](#license)

---

## Install

### `go install`

```sh
go install github.com/mark1708/tmh/cmd/tmh@latest
```

Requires Go 1.25+ (the embedded `modernc.org/sqlite` driver raises the
floor).

### Homebrew

```sh
brew install mark1708/tap/tmh
```

The formula installs the binary, man pages, and bash/zsh/fish
completions.

### From source

```sh
git clone https://github.com/mark1708/tmh.git
cd tmh
go build -o ~/.local/bin/tmh ./cmd/tmh
```

### Verify

```sh
tmh version
tmh doctor
```

`doctor` checks:

- tmux ≥ 3.2, `$SHELL`, `config.yml` (existence + schema);
- tmux-server reachability, optional `fd`, `terminal-notifier` (macOS
  only);
- a separate **tmux integration** block audits server options
  (`default-terminal`, `mouse`, `escape-time`, `extended-keys`,
  `base-index`, `pane-base-index`, `renumber-windows`), conflicting
  hooks (`after-new-window`, `automatic-rename=on`), and the presence
  of `#(tmh status)` in `status-right`. Every ⚠/✗ finding prints a
  ready-to-paste line for `~/.tmux.conf`.

Binary-release downloads include a GPG-signed `checksums.txt`; verify
via `gpg --verify checksums.txt.sig checksums.txt` — full guide in
[docs/verify.md](./docs/verify.md).

---

## First run

If `~/.config/tmh/config.yml` is missing, `tmh` offers four options
interactively (when stdin is a TTY):

1. **start empty** — minimal config with `version: 1` and empty
   sections.
2. **import from live tmux** — runs `sync --pull --bootstrap`,
   auto-derives `roots:` via a longest-common-prefix scan of every
   session's first-pane cwd, then imports every live session under
   `sessions:`.
3. **import from file / URL** — read a teammate's YAML into the new
   config location.
4. **quit**.

Option 2 is the path of least resistance if tmux is already running —
you get a honest YAML with every window captured:

```sh
tmh sync --bootstrap
```

After that, `cat ~/.config/tmh/config.yml` shows inferred `roots:` and
`sessions:` keyed by name, each window expressed as `{root: <key>,
path: <relative>}` when it fits a root.

In non-TTY mode (pipe, CI) an empty config is created silently and tmh
keeps working — `ls`, `attach`, `kill`, `reload --shell`, `popup`,
`scratch`, and `window` all tolerate a missing config (pass-through
mode).

Every tmh write prepends a `# yaml-language-server: $schema=…` header
so any editor with the YAML language server (VS Code, Helix, Neovim)
picks up autocomplete and inline validation automatically — see
[JSON schema and editor integration](#json-schema-and-editor-integration).

---

## Quick start

```sh
# Import live tmux into config.yml.
tmh sync --bootstrap

# After a reboot — bring everything back with one command.
tmh init

# What's declared and what drifted?
tmh ls
tmh diff

# Switch windows.
tmh attach epcp:lk             # outside tmux → attach-session
                               # inside tmux  → switch-client

# Sync dotfiles into live sessions (a killer feature).
tmh reload --shell             # source the right rc file in every idle pane
tmh reload --shell --busy      # …and queue busy panes for later
tmh reload --tmux              # tmux source-file ~/.tmux.conf
tmh reload --all               # both at once

# Capture a live layout you built by hand into the YAML.
tmh freeze

# Bare command — quick picker (TTY + tmux running); --dashboard for the full TUI.
tmh
tmh --dashboard
```

`tmh reload --shell` picks the right rc file automatically — bash →
`~/.bashrc`, fish → `~/.config/fish/config.fish`, zsh → `~/.zshrc`.
The `--rc <path>` flag overrides the auto-detection.

---

## UI language

English (default) and Russian are bundled. Unsupported locales
(`de_DE`, `ja_JP`, …) silently fall back to English — users never see
raw i18n keys.

Resolution order (highest wins):

1. `--lang en|ru` — a global flag that overrides everything. Affects
   runtime output (toasts, errors, `fmt.Print*` lines). Cobra help text
   is bound at startup and is **not** retranslated by `--lang` — that's
   a cobra limitation.
2. `defaults.lang: ru` in `config.yml`.
3. Environment variables `TMH_LANG`, `LC_ALL`, `LC_MESSAGES`, `LANG`
   (the prefix before `_`/`.` is consulted).
4. Fallback — English.

Live switching from the TUI: `S` (settings) → **Appearance** section →
`↑↓`. The change applies immediately and persists as `defaults.lang`.

JSON outputs (`tmh ls --json`, `tmh diff --json`, `tmh tmux audit
--json`) stay English regardless of locale — they're a stable scripting
contract. `Drift` exposes a stable `ReasonCode` field (e.g.
`session_gone`) that the TUI resolves to a localised string at
render-time.

---

## Config — `config.yml`

Lives at `~/.config/tmh/config.yml` (or wherever `$TMH_CONFIG` or
`--config` points). YAML with structural references — no Mustache, no
templating DSL.

### A complete example

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mark1708/tmh/main/schemas/tmh.schema.json

version: 1

# Named root directories so long prefixes aren't repeated.
roots:
  work: ~/work/orgA
  home: ~/work/personal
  kb:   ~/work/personal/kb/bases

# Global fallbacks applied when a deeper level leaves a field unset.
defaults:
  layout: 3-pane
  shell:  zsh
  lang:   en                         # en | ru; empty → auto-detect from env
  popup:  {width: 80%, height: 60%}
  env:
    EDITOR: nvim

# Reusable window templates. `extends:` only references templates —
# chains are rejected at validation (ErrTemplateChain).
templates:
  kb_base:
    layout: 2-pane
    command: nvim .

# Custom tmux layout hashes for experimental layouts.
# Capture your own: arrange the window, then `tmh layout save <name>`.
layouts:
  my-ide:
    hash: "5c3b,239x56,0,0{119x56,0,0,0,..."
    description: "editor left 50%, stacked panes right"

# Profiles — filter sessions by group + optional env/defaults overlay.
profiles:
  work:
    groups: [work, orgA]
    env: {AWS_REGION: eu-central-1}
  personal:
    groups: [home, kb]

# Discovery — auto-generate candidate sessions from the filesystem
# (and optionally zoxide). Entries appear in `tmh ls` and the picker;
# they become real sessions on attach.
discover:
  - path: ~/work/orgA/services/*
    template: go_service
    zoxide: true
    zoxide_limit: 15

# Declared sessions.
sessions:
  epcp:
    group: [work, orgA]
    root:  work
    path:  products/epcp/repos
    env:
      KUBE_CONTEXT: epcp-dev
      AWS_PROFILE:  epcp
    on_attach:
      - mise use
    windows:
      # shorthand: bare string = {dir: <value>}, relative to root
      lk:      lk-mosru-epcp
      mdr:     mdr
      filings: filings
      # full form with template + command
      kb:
        extends: kb_base
        root:    kb
        path:    epcp
        # window-scoped hooks fire in addition to session-scoped ones
        on_create:
          - make deps
```

### Window schema

```yaml
windows:
  <name>:
    dir:      string           # absolute or relative
    root:     string            # key from roots.<...>
    path:     string            # alternative to dir when rooted
    layout:   string            # 1-pane | 2-pane | 3-pane | <layouts.<key>>
    command:  string            # command for the main pane
    extends:  string            # key from templates.<...>
    env:      {KEY: VALUE}      # env overrides
    focus:    bool              # active window after init
    hooks:                      # window-scoped hooks (see Hooks section)
      on_create:  [...]
      on_attach:  [...]
      on_destroy: [...]
    panes:                      # explicit pane layout
      - dir: ...
        command: ...
        env: {}
        focus: true
        hooks: {...}            # pane-scoped hooks
```

Short form `name: "string"` is equivalent to `name: {dir: "string"}`.

### Path resolution

1. Absolute `dir:` → used as-is.
2. `root:` set → `roots[root] / (path || dir)`.
3. `session.root` set + `dir:` relative → `roots[session.root] /
   session.path / dir`.
4. Otherwise → `$PWD / dir`.

Optional shorthand: a string starting with `$key/...` expands to
`{root: key, path: ...}`. `$$` is a literal `$`. The shorthand is
normalised in-memory on load (`config.Normalize`); there's no CLI
wrapper yet for persisting a normalised version to disk.

### Env merge

Deeper levels override:

```
defaults.env
  → profiles[active].env
    → sessions[x].env
      → sessions[x].windows[y].env
        → sessions[x].windows[y].panes[z].env
```

Maps are merged key-by-key, not replaced wholesale.

### Validation

`tmh doctor` validates the schema and prints
`config.yml schema: <err>` if anything is off. Checked:

- every `root:` resolves to a declared `roots.<key>` (`ErrUnknownRoot`);
- every `extends:` resolves to `templates.<key>` (`ErrUnknownTemplate`);
- `extends` depth is exactly 1 (`ErrTemplateChain`);
- every `layout:` is a built-in or a declared `layouts.<key>`
  (`ErrUnknownLayout`);
- `panes[]` count is compatible with built-in layouts (`ErrLayoutMismatch`).

---

## CLI reference

Global flags available on every subcommand:

```
--config string      path to config.yml (overrides $TMH_CONFIG and defaults)
--profile string     profile name from config.yml
--lang en|ru         UI language; overrides config and env
```

```
tmh                          open the TUI dashboard (or the picker — see below)
tmh --dashboard              force the full TUI, bypassing the picker
tmh version                  print the version
tmh doctor                   environment + tmux-integration audit
tmh completion {zsh|bash|fish}   completion script
```

### Sessions

```
tmh attach [name|name:window]   attach (outside tmux) / switch-client (inside)
tmh new [--name] [--dir] [--layout] [--group] [--save] [--attach]
                                 without flags — interactive wizard (huh form)
tmh init [--only a,b]            bring up everything from config (skip existing)
tmh kill <pattern>               kill sessions matching a substring
tmh ls [--json]                  sessions/windows tree
tmh window [--dir]               new ad-hoc window in the current session
tmh scratch [--dir]              ephemeral session
```

### Process inspector

```
tmh ps                          table of every pane: session/window/pane/cmd/pid/cwd
tmh ps --session <name>         restrict to one session
tmh ps --format json|tsv        machine-readable output (json is pipe-friendly natively)
```

Example:

```
SESSION   WINDOW   PANE  CMD      PID    CWD
work      editor   0     nvim     12345  ~/work/myproject/src
work      server   0     go       12346  ~/work/myproject
kb        main     0     zsh      -      ~/kb
```

### Sync, diff, freeze

```
tmh sync --push                live ← config (create missing sessions/windows)
tmh sync --pull [--all]        config ← live (add new, update drift)
tmh sync --bootstrap           import every live session into an empty config
tmh sync --dry-run             print planned changes without applying them
tmh diff [--json]              print drift entries
tmh freeze [--session <name>] [--dry-run]
                               non-destructive live → YAML capture; see
                               "tmh freeze" section below
```

Drift statuses:

| Status  | Meaning |
|---------|---------|
| `ok`    | window is identical in live and config (root/dir match) |
| `drift` | first pane's `pane_current_path` ≠ resolved dir |
| `new`   | window appeared live inside a tracked session, absent in config |
| `gone`  | window declared in config but not running |

### Dotfile sync

```
tmh reload                     (default --all) shell + tmux
tmh reload --shell             source the right rc file in idle shell panes
tmh reload --tmux              tmux source-file ~/.tmux.conf
tmh reload --busy              non-idle panes are queued; sourced when they free up
tmh reload --status            show the deferred queue
tmh reload --rc <path>         override the rc path (otherwise derived from $SHELL)
tmh reload --tmux-conf <path>  override the tmux conf path
tmh watch [--auto]             fsnotify watcher on the dotfiles
tmh status                     single-glyph segment for tmux status-right
```

### Snapshots / undo / export / import

```
tmh snapshot save <name>       named checkpoint of live state
tmh snapshot list
tmh snapshot restore <name>
tmh snapshot delete <name>
tmh undo                       revert the last destructive action
tmh export [--minimal] [--only <name>]   YAML to stdout; --minimal redacts secrets
tmh import <path> --merge|--replace
```

### Layouts, popup, tmux integration

```
tmh layout save <name> [--description]   save the active window layout
tmh popup <cmd> [--width] [--height] [--no-env] [--no-cwd] [--session] [--window]
                                          command in a tmux popup with env/cwd from config
tmh tmux audit [--json]                   print audit findings for the tmux server
tmh tmux setup [--append]                 snippet for ~/.tmux.conf; --append adds a managed block
```

---

## TUI dashboard

```
tmh --dashboard                 explicit full dashboard
tmh                             picker first; dashboard on fall-through
```

### Layout

```
┌─ tmh · ~/.config/tmh/config.yml ──── ⚠ drift:2 ──────────────────┐
│  SESSIONS                   │  DETAIL                             │
│  ▼ ● epcp   7w   ok         │  session: epcp                      │
│    ├─ ● lk   3p   ok        │  live     ✓                         │
│    ├─   mdr  3p   ok        │  attached ✓                         │
│    ├─ ! jr   3p   drift     │  windows  7                         │
│    └─ …                     │  status   ok                        │
│  ▼ ● kb     8w              │                                     │
│                             │  preview                            │
│                             │  $ mise use                         │
│                             │  $ git status                       │
├──────────────────────────────────────────────────────────────────┤
│ a · n · d · R · s · S · : · ^L · ? · q          [ OK reload done ]│
└──────────────────────────────────────────────────────────────────┘
```

Layout features:

- Boolean detail fields (`live`, `attached`) render as ✓/✗.
- Below the detail fields — an async **preview** (`tmux capture-pane`
  of the focused session/window's first pane). Refreshed on cursor
  change; cache keyed by target.
- **Inline toasts** attach to the right side of the footer for 4–5 s
  (errors 5 s, action-done 4 s). Every toast also enters the history
  ring (last 30), accessible via `Ctrl+L`.

### Keymap

**Navigation**

| Key | Action |
|---|---|
| `j` / `k` / `↑↓` | up / down |
| `h` / `l` | collapse / expand session |
| `Tab` on a window row | toggle inline pane rows |
| `ShiftTab` on a window row | cycle preview between panes |
| `/` | inline tree filter (Enter keeps, Esc clears) |
| `g` / `G` | top / bottom |
| `PgUp` / `PgDn` | page |

**Actions**

| Key | Action |
|---|---|
| `enter` / `a` | attach (tmux takes the TTY; return via `prefix d`) |
| `n` | new session via the wizard |
| `d` | kill session / window / pane (context-aware, confirmed) |
| `u` | undo the last destructive action |
| `m<a>` | set mark `a` on the current position |
| `'<a>` | jump to mark `a` |
| `''` | return to the previous position (last-location) |

**Sync / reload**

| Key | Action |
|---|---|
| `r` | refresh the TUI tree |
| `R` | `source <rc>` + `tmux source-file` |
| `s` | `sync --push` (create missing entries) |
| `D` | drift screen |

**Other**

| Key | Action |
|---|---|
| `:` / `Ctrl+P` | command palette (fuzzy + parametric actions) |
| `S` | settings |
| `Ctrl+L` | action history with OK/ERR badges |
| `Ctrl+T` | cycle the theme |
| `?` | contextual help (screen-specific) |
| `q` / `Ctrl+C` | quit |

### Settings screen

Seven categories in a master-detail layout (left column — categories,
right column — fields):

| Category | What it tunes |
|---|---|
| Appearance | theme (Catppuccin variants), language (en/ru) |
| Display | show processes in tree, footer heatmap, default preview pane |
| History | retention (7d/30d/90d/forever), max entries, clear |
| Marks | persist_across_sessions, reset all marks |
| Tmux | escape-time, mouse, base-index — writes to `~/.config/tmh/tmux.conf` |
| Behaviour | auto-refresh interval, dry_run_default, confirm_on_kill |
| Keybindings | read-only quick reference |

Live-apply: theme, language, display fields apply instantly. Tmux
fields need `Ctrl+S` to save.

### Command palette

`:` or `Ctrl+P`. Fuzzy search + parametric actions:

| Action | Description |
|---|---|
| `mark: set mark` | set a named mark (prompts for a letter) |
| `goto: jump to process` | jump to the first pane with the given command (prompts for a name) |
| `attach <session>` | one entry per live session |
| data refresh, sync, init, diff, snapshot, undo, doctor, … | standard actions |

Parametric actions surface an extra input field before execution. `Esc`
cancels back to the selection list.

### Confirm dialog

On `d` (kill):

- `y` / `Enter` — execute
- `n` / `Esc` — cancel
- `t` — dry-run: show what would be killed without touching anything

---

## Picker (bare `tmh`)

When stdin and stdout are both TTYs and a tmux server is reachable,
bare `tmh` routes to a compact fuzzy picker instead of the full TUI
dashboard. This is the sesh-style muscle memory: type a few letters,
Enter attaches.

```
┌ tmh — pick a session ─────────────────────────────────────┐
│                                                            │
│  api            attached                                   │
│  web            live                                       │
│  infra          configured                                 │
│  scratch        discovered   ~/work/scratch                │
│  notes          discovered   ~/work/personal/notes         │
│                                                            │
└────────────────────────────────────────────────────────────┘
 ↑/↓ move · / filter · enter attach · d dashboard · esc cancel
```

Keys:

| Key | Action |
|---|---|
| `↑` / `↓` / `j` / `k` | move |
| type letters | fuzzy filter |
| `Enter` | attach (outside tmux) or switch-client (inside); discovered candidates are created first |
| `d` / `?` | fall through to the dashboard |
| `Esc` / `Ctrl+C` | cancel without attaching |

The picker falls through to the dashboard automatically if:

- `--dashboard` is passed explicitly;
- stdin or stdout is not a TTY;
- no tmux server is running (so there's nothing to pick from);
- the listing is empty even counting discovered candidates.

The status column shows one of `attached`, `live`, `configured`,
`discovered`. Discovered entries come from `discover:` rules and are
materialised with `tmux new-session` on first attach.

---

## Process visibility

The TUI refreshes `pane_current_command` for every pane every 2 s
(tunable via `defaults.behaviour.auto_refresh_interval`).

**In the session tree** — the session row appends unique non-idle
process names: `claude vim`. The window row shows the first non-shell
pane's process.

**In the detail panel** — for each window pane: a marker on the
current preview pane, the index, the command, and the cwd.

**Command drift:** if `config.yml` declares `command: nvim` and the
live pane runs `zsh`, the detail panel prints

```
drift   nvim ≠ expected: zsh
```

Example:

```yaml
sessions:
  work:
    windows:
      editor:
        dir: src
        command: nvim    # expected process
```

**Inline filter `/`:** press `/` and type a fragment of a session /
window / process name. The footer counter shows `3/42`. `Enter` keeps
the filter (navigation still works); `Esc` clears.

---

## Marks and last-location

Marks are vim-style bookmarks so you can jump between frequently-used
sessions or windows.

### Set a mark

```
m<letter>    set a mark on the current position
             example: ma → mark 'a' on the focused window
```

Via palette: `:` → `mark: set mark` → enter a letter.

### Jump to a mark

```
'<letter>    jump to the mark and push the current position into the
             last-location ring
             example: 'a
```

### Last-location

```
''           return to the previous position (pop from the ring)
```

Every jump (`'<letter>`, `attach`, `''`) pushes the current position
into a 10-slot ring. Repeated `''` cycles through it.

When the ring is non-empty, the footer shows `'' ← prev`.

### Persistence

Marks and the ring live in `~/.local/state/tmh/marks.json`. Killing a
session/window/pane automatically invalidates marks that pointed at the
gone target.

Turn persistence off via `defaults.marks.persist_across_sessions: false`
or Settings → Marks.

---

## tmux integration

For tmh to deliver a good UX (truecolor, fast escape, extended keys,
inline status segment) the tmux server needs a minimal set of options.
Check the current state and get a ready-to-paste snippet:

```sh
tmh tmux audit          # ✓/⚠/✗ per option + hint on how to fix
tmh tmux audit --json   # same, machine-readable
tmh tmux setup          # snippet for ~/.tmux.conf (stdout)
tmh tmux setup --append # append a managed block to ~/.tmux.conf (idempotent)
```

The audit covers:

- **baseline** (required for tmh to work well): `default-terminal
  tmux-256color` + RGB, `mouse on`, `escape-time 0`, `extended-keys
  on`;
- **recommended** (UX niceties): `base-index 1`, `pane-base-index 1`,
  `renumber-windows on`;
- **conflicts**: hook `after-new-window` (races with tmh's
  window-creation path), `automatic-rename=on` (clobbers window
  names);
- **integration**: the `#(tmh status)` segment in `status-right` —
  without it, drift/reload badges don't show up in the status bar.

Recommended bind for `~/.tmux.conf`:

```tmux
bind R run-shell "tmh reload --all"          # prefix R → dotfiles reload
set -ag status-right ' #(tmh status)'        # drift/reload badges
```

---

## Hooks and trust

`on_create`, `on_attach`, `on_destroy` are lists of shell commands run
at lifecycle events. Each runs under `sh -c`, inheriting `env` and
`cwd` from the resolved config.

Hooks live at **three scopes**:

| Scope    | YAML path                                    |
|----------|----------------------------------------------|
| Session  | `sessions.<name>.hooks.*` / `profiles.<name>.hooks.*` |
| Window   | `sessions.<s>.windows.<w>.hooks.*`          |
| Pane     | `sessions.<s>.windows.<w>.panes[].hooks.*`  |

Profile hooks concatenate **before** session hooks at the session
scope; template hooks concatenate **before** window-specific ones when
a window `extends:` a template.

### First run of a config with hooks

```
⚠  config.yml contains shell hooks:
    sessions.epcp.on_attach: mise use
    sessions.epcp.windows.db.on_create: docker compose up -d

Trust and run? [y/N]
```

After `y`, the file's SHA-256 is stored in `~/.local/state/tmh/state.db`.
No more prompts until the file changes. Any edit re-triggers the
prompt.

For programmatic bypass (CI, audit) the internal
`actions.HookOptions.NoHooks=true` skips execution — exposed only via
code today, no CLI flag.

---

## Snapshots and undo

**Snapshots** — named restore points for the structure of every live
session (windows + cwd + layout). Pane contents are not preserved — a
hint records which process was running.

```sh
tmh snapshot save pre-demo
# ... break things ...
tmh snapshot restore pre-demo
```

**Undo** — a short history of the last destructive action (currently
only `kill_session`). Before a kill, tmh stores a session snapshot in
the `events` table; `tmh undo` restores from that payload.

---

## Sharing with a teammate

Export a sanitised YAML:

```sh
tmh export --minimal > team.yml
```

`--minimal` does two things:

- redacts env keys ending in `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD`,
  `_PWD`, `_API_KEY` → `<redacted>`;
- rewrites absolute `dir:` values into `{root, path}` pairs when the
  prefix matches a declared root, removing user-specific absolute
  paths.

Your teammate:

```sh
go install github.com/mark1708/tmh/cmd/tmh@latest
tmh import team.yml --merge
tmh init
```

`--merge` — overlay onto the existing config (incoming side wins on
conflicts). `--replace` — full replacement.

---

## Discovery rules (glob + zoxide)

Declared sessions are the authoritative list — but enumerating every
project in a monorepo or scratch workspace gets tedious fast. The
`discover:` block auto-generates **candidate** sessions from the
filesystem (and optionally zoxide); they show up in `tmh ls` and the
picker, but aren't drift-checked.

```yaml
discover:
  - path: ~/work/orgA/services/*    # tilde + filepath.Glob (no **)
    template: go_service             # seed each discovered session with this template
    zoxide: true                     # additionally pull from `zoxide query --list`
    zoxide_limit: 15                 # cap the number of zoxide entries (default 20)
```

Resolution order:

1. Directories matching `path:` glob (directories only; files and
   broken symlinks are skipped).
2. Top-N zoxide paths when `zoxide:` is true and the binary exists.
3. **Declared sessions always win** — a session named in
   `sessions:` suppresses the corresponding discovered candidate.

Discovered entries:

- appear in `tmh ls` with a `discovered` status;
- appear in the picker with their absolute directory;
- become real sessions via `tmux new-session` on first attach;
- are ignored by `tmh diff` — they're candidates, not drift targets.

Without a `discover:` block nothing extra happens — everything in this
section is opt-in.

---

## `tmh freeze` — capture live into YAML

The authoring complement to `tmh diff`. When you build a layout by
hand (start a session, rename windows, arrange panes) and want to keep
it, `tmh freeze` writes it back into `~/.config/tmh/config.yml`
**without** clobbering comments, templates, profiles, or existing
entries.

```sh
tmh freeze --dry-run            # preview planned changes
tmh freeze                      # actually write
tmh freeze --session api        # restrict to one session
```

Semantics:

| Class of change | What freeze does |
|---|---|
| session not in config | add it with an inferred root |
| window not in config  | add it (inferred root or absolute dir) |
| window matches config | mark as **unchanged** (no write) |
| window dir differs    | report as **conflict** — do not overwrite |

Conflicts are left for you to resolve explicitly:

- `tmh sync --pull --all` — destructive overwrite (config ← live);
- manual edit in YAML;
- manual `tmux` rearrange so things match again.

Freeze and drift detection together turn into a closed loop: build
live → freeze → edit config → `tmh diff` shows you exactly what moved
since you froze.

---

## JSON schema and editor integration

tmh ships a JSON Schema document generated from `config/types.go`.
Every `config.Write` call prepends a modeline:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mark1708/tmh/main/schemas/tmh.schema.json
version: 1
...
```

If your editor runs [yaml-language-server][yls] (default in VS Code's
YAML extension, Helix's `yaml` LSP, Neovim's `lsp-zero` /
`nvim-lspconfig`), autocompletion and inline validation light up
automatically — no manual configuration.

Regenerate the schema yourself when you fork or modify types:

```sh
make schema                     # fast — schema only
make docs                       # schema + man pages + completions
```

The schema lives at `schemas/tmh.schema.json` and is committed so
release tarballs + Homebrew installs include it.

Writers that round-trip configs (`tmh sync`, `tmh import`) keep the
modeline on every edit. The auto-insertion can be opted out
per-writer via `config.WriteOptions.NoSchemaHeader: true` — useful
when writing machine-consumed YAML that shouldn't carry comments.

[yls]: https://github.com/redhat-developer/yaml-language-server

---

## Security model

### File permissions

All tmh-written files are `0600`:

- `~/.config/tmh/config.yml` (may contain env secrets)
- `~/.local/state/tmh/history.jsonl`
- `~/.local/state/tmh/state.db` (trust hashes, marks, snapshots)
- `~/.local/state/tmh/marks.json`

Directories are `0755`. Any existing file with wider perms gets
tightened on next write.

### Hook trust model

Shell commands inside `config.yml` never run unprompted. The first
time tmh sees hooks (or sees a config whose SHA-256 has changed) it
prints the full command inventory across **every scope** and asks for
explicit `y`. The decision is stored in the SQLite `trust` table keyed
by `(path, sha256)`.

Lost trust decisions are not a security issue — they cause an extra
prompt, nothing more. The table persists across upgrades.

### Secret handling

- `tmh export --minimal` redacts env keys matching
  `*_{TOKEN,KEY,SECRET,PASSWORD,PWD,API_KEY}` before printing YAML.
- `env:` values in config.yml are **not** encrypted at rest. tmh
  relies on file perms (`0600`) and your filesystem's access control;
  use secret managers + environment variable interpolation from the
  shell for anything sensitive.
- Vulnerability disclosure policy — [SECURITY.md](./SECURITY.md).

---

## Troubleshooting

**`tmh` seems to hang after `attach`**
`prefix d` inside tmux detaches and returns you to the TUI. If it's
genuinely stuck — `Ctrl+\` (SIGQUIT) or `pkill -INT tmh` from another
terminal.

**`state.db` is corrupt**
`internal/state` exposes `FixState(path)` which renames the broken
file to `state.db.broken.<ts>` and starts fresh. No CLI wrapper yet —
do it by hand:

```sh
mv ~/.local/state/tmh/state.db ~/.local/state/tmh/state.db.broken.$(date +%s)
```

Expected loss: snapshots / undo / trust decisions.

**Ad-hoc session isn't flagged as drift**
By design — sessions not in config are ignored. Add them via
`tmh sync --pull` (or `tmh freeze`).

**Hooks don't run**
If `config.yml` changed, the trust prompt re-fires. Either answer `y`
again or inspect `~/.local/state/tmh/state.db` (table `trust`).

**`go install` fails with 410 Gone / unknown revision**
Ensure Go ≥ 1.25 and the full path
`github.com/mark1708/tmh/cmd/tmh@latest`. If the module is unreachable
via `proxy.golang.org`, set `GOPROXY=direct`.

**Enable structured logging for debugging**

```sh
TMH_LOG=debug tmh
```

Supported levels: `debug`, `info`, `warn`, `error`. Logs go to
`~/.local/state/tmh/tmh.log` in JSON, rotated at 5 MB × 3 files.
Without `TMH_LOG` the log is entirely silent.

**Drift/reload badge missing from tmux status-right**
Run `tmh tmux audit` — likely the `#(tmh status)` segment is absent.
Fix with `tmh tmux setup --append` or add it by hand to
`~/.tmux.conf`.

**Picker doesn't appear with bare `tmh`**
Picker only activates with a TTY + a running tmux server. Otherwise
tmh routes to the dashboard (which can still bootstrap everything).
Force explicit: `tmh --dashboard`.

**`tmh reload --shell` doesn't source my rc file**
tmh picks the rc file by `$SHELL` — bash → `~/.bashrc`, fish →
`~/.config/fish/config.fish`, zsh → `~/.zshrc`, anything else →
`~/.profile`. Override with `--rc <path>`.

---

## Architecture

```
cmd/tmh/              cobra entry + subcommands
cmd/tmh-gen/          build-time generator: JSON schema, man pages, completions

internal/
  config/             parser / resolver / validator / atomic writer, diff
                      (+ReasonCode), discover rules, JSON schema reflector
  tmux/               Runner interface (CLIRunner) — the only tmux seam
  tmux/tmuxtest/      MockRunner for tests (never imported by production code)
  actions/            side-effect API; CLI + TUI are thin frontends
                      (includes AuditTmuxConfig, Setup, snapshots, hooks, freeze)
  state/              SQLite WAL + busy_timeout: events / snapshots / trust /
                      reload_queue + JSONL history + marks + last-location ring
  slogx/              global slog logger, rotating writer, TMH_LOG env
  errors/             typed sentinels (en-only, stable API for errors.Is)
  i18n/               go-i18n v2, embedded locales/{en,ru}.json, DetectLang
  shell/              $SHELL → rc-file resolution (bash/zsh/fish/profile)
  ui/                 bubbletea: dashboard, picker, palette, settings, diff,
                      confirm, help, history, errrender
    ui/pane/          Provider — pane_current_command cache (TTL, FindByCommand)
    ui/refresh/       Refresher — periodic batch fetch with seq-based debounce
    ui/toast/         Kind enum + TTL
    ui/picker/        bare-tmh fuzzy picker (bubbles.list + textinput)
  xdg/                XDG paths (Config, State, Backups, Log, History, Marks,
                      TmuxConf, Schemas)
```

Design rules:

- All side effects live in `internal/actions`; CLI and TUI just call them.
- `internal/tmux.Runner` is the **only** contact with `tmux`. Tests
  use `tmuxtest.MockRunner`; nothing outside `internal/tmux` forks
  `tmux` directly.
- `config.yml` mutations go through `config.PathSet/Delete/Rename` +
  `config.Write` with comments preserved via `yaml.Node`.
- Errors are typed sentinels in `internal/errors` — **English-only** and
  stable for `errors.Is` and external tests. UI localisation happens at
  the boundary via `internal/ui/errrender`.
- JSON outputs are never localised: `Drift.Reason` (en) +
  `Drift.ReasonCode` (stable) — the TUI resolves the code into
  localised text via `i18n.T("drift.reason." + code)`.

Deeper notes — [CONTRIBUTING.md](./CONTRIBUTING.md) +
[docs/architecture.md](./docs/architecture.md) + [docs/](./docs/).

---

## License

MIT — see [LICENSE](./LICENSE).
