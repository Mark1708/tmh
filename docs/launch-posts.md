# Launch posts — ready-to-paste drafts

Finalised copy for each channel. Each post is self-contained — paste
as-is, edit only if you want to tweak emphasis. Links assume
`github.com/mark1708/tmh` is live; swap if the owner is different.

Do **not** cross-post verbatim across channels — HN and lobste.rs both
index Show HN submissions and will silently rank duplicates lower.

---

## Channel 1 — Hacker News (Show HN)

**Submit at:** <https://news.ycombinator.com/submit>

**Title (80-char limit, no emoji, no trailing punctuation):**

```
Show HN: Tmh – declarative tmux sessions in YAML with drift detection
```

**URL:** `https://github.com/mark1708/tmh`

**Text (leave blank — HN prefers URL-only submissions for tools):**

If the link submission gets no traction in the first 30 min, post one
comment (not a reply) with the body below. Don't edit after submitting.

```
Author here. I kept accumulating `tm-*` aliases around tmux — nine of
them, plus an INI file that was supposed to let me restore layouts
after reboots but never actually worked. Tmh is the one-binary
replacement.

The angle that's different from tmuxinator / tmuxp: drift detection as
verification, not enforcement. `tmh diff` shows what drifted between
the YAML and the live tmux state; you decide per-entry whether to push
(live ← config) or pull (config ← live). No "reconcile by bulldozer"
workflow.

Other things that shipped in v1.0: SQLite-backed undo/history/marks,
a TUI dashboard with command palette + fuzzy filter, `tmh freeze`
(capture live → YAML, preserving comments), optional zoxide-style
discovery (`discover:` rules), lifecycle hooks with a trust prompt,
hardened file perms (0600), i18n (English + Russian bundled),
goreleaser multi-arch signed tarballs.

Intentionally boring in scope: no plugin system, no GUI, no telemetry.
Tmux stays the multiplexer; tmh is just the glue + verification layer.

MIT license. Feedback welcome.
```

---

## Channel 2 — r/tmux

**Submit at:** <https://www.reddit.com/r/tmux/submit>

**Title:**

```
[OC] Tmh 1.0 — a Go binary that replaces my pile of `tm-*` aliases with declarative YAML + drift detection
```

**Flair:** Question / Project (pick "Project" if available)

**Body:**

```
Hey r/tmux — releasing v1.0 of a tool I've been using personally for a
while.

**The problem it solves:** I had 9 zsh aliases and an INI file that
tried to describe my tmux sessions, and none of it handled the "tmux
rolled back after a reboot and I didn't notice" case. Also: no undo,
no sharing with teammates, no drift detection.

**What tmh does:**

- `~/.config/tmh/config.yml` declares sessions / windows / panes,
  templates, profiles, hooks. Comments are preserved on edits.
- `tmh diff` — shows drift between YAML and live tmux. No push
  behaviour by default; you decide whether to push or pull per entry.
- `tmh freeze` — capture live state into the YAML non-destructively
  (inverse direction from tmuxinator).
- `tmh` (bare) — fuzzy picker with sesh-style ergonomics. `--dashboard`
  opens the full TUI.
- `tmh undo` — revert the last destructive action (kill, layout
  clobber).
- Optional `discover:` rules — glob-match `~/work/*` or pull from
  zoxide so you don't have to enumerate every project.

**What it isn't:**

- No plugin system. Hooks + command palette + `tmh status` cover the
  typical plugin use cases without an ABI commitment.
- Not a session-persistence tool. tmux-resurrect does pane contents;
  tmh tracks layout declaratively.
- No Windows support. tmux itself doesn't target Windows.

Install: `brew install mark1708/tap/tmh` or
`go install github.com/mark1708/tmh/cmd/tmh@latest`.

Repo + demo GIFs: https://github.com/mark1708/tmh

MIT-licensed, English + Russian UI, tests green with `-race`. Happy to
answer questions here.
```

---

## Channel 3 — r/golang

**Submit at:** <https://www.reddit.com/r/golang/submit>

**Title:**

```
Tmh: a Go TUI for tmux — declarative YAML sessions, drift detection, built on Bubble Tea
```

**Body:**

```
I released v1.0 of tmh, a tmux session manager written in Go. Posting
here for Go-focused feedback on the architecture and libraries chosen.

**Why Go:** the other tools in this space are Ruby (tmuxinator) or
Python (tmuxp), both of which force a language runtime on every
machine. Single static Go binary beats both.

**Stack (all dependencies intentional, no surprise vendoring):**

- `spf13/cobra` — CLI (27+ subcommands, cobra handles man pages +
  completions via `cmd/tmh-gen`).
- `charmbracelet/bubbletea` + `bubbles` + `lipgloss` — TUI dashboard
  and picker.
- `gopkg.in/yaml.v3` — config parser, plus a comment-preserving
  writer built on yaml.Node so edits don't rot the user's notes.
- `modernc.org/sqlite` — pure-Go SQLite for history/snapshots/marks
  /hook-trust. No CGO anywhere in the project.
- `invopop/jsonschema` — auto-generates a JSON Schema from the config
  struct tags, emitted as a yaml-language-server modeline header on
  every config write.

**Design decisions worth mentioning:**

- `tmux.Runner` is a 50-method interface; production uses a
  `CLIRunner` that spawns `tmux`, tests use a pure-Go `MockRunner`.
  No package outside `internal/tmux` forks `tmux` directly.
- Error wrapping is consistent (94 `fmt.Errorf("...: %w", ...)`
  instances) with typed sentinels in `internal/errors` for classification.
- File permissions are 0600 for anything that could contain env
  secrets (config, history, SQLite DB).
- `-race` + goroutine-boundary discipline — the refresh loop in
  `internal/ui/refresh` uses a seq counter so stale results from
  superseded fetches are silently dropped instead of racing.

Repo: https://github.com/mark1708/tmh
Architecture notes: https://github.com/mark1708/tmh/blob/main/docs/architecture.md

PRs welcome — `go test -race ./...` stays green with a 60% coverage
floor enforced in CI.
```

---

## Channel 4 — lobste.rs

**Submit at:** <https://lobste.rs/stories/new>

**URL:** `https://github.com/mark1708/tmh`

**Title:**

```
Tmh: declarative tmux sessions in YAML with drift detection (Go, MIT)
```

**Tags (pick 2–3 from the allowed list):** `go`, `unix`, `release`

**Description:**

```
Open-source TUI hub for tmux: one `config.yml` with sessions /
windows / panes / templates / profiles / hooks; `tmh diff` flags
drift between the config and live tmux without bulldozing either
direction; SQLite-backed undo/history/marks; a Bubble Tea dashboard
with palette + fuzzy picker; optional zoxide-style `discover:`
rules. Single static Go binary, multi-arch signed tarballs. No
telemetry, no plugin system.

This is the write-up of a v1.0 cut over a tool I've been using for
~6 months to replace nine zsh aliases and one never-finished INI
file. Architecture notes and a migration guide (zsh aliases → YAML)
are in `docs/`.
```

---

## Channel 5 — dev.to long-form (angle: team sharing)

**Submit at:** <https://dev.to/new>

**Title:**

```
How I stopped onboarding teammates with "which terminal do I open which command in?" — committing tmux sessions to git with tmh
```

**Tags:** `tmux`, `productivity`, `go`, `showdev`

**Cover image:** use `docs/demo-picker.gif` as the cover or embed it
below the intro.

**Body (markdown — rendered by dev.to):**

```markdown
Onboarding a teammate to a moderately complex monorepo takes me ~45
minutes every time. Half of it is *"open a terminal, `cd services/api`,
run `go run ./cmd/api`. Now in another terminal, `cd services/api` and
`tail -f /tmp/api.log`. Now another terminal for the db migration
console..."* — verbatim tmux invocations a human parrots.

The rest is me mistyping a path and sending them off in a broken
state.

I fixed my half of the problem with **tmh** — a Go binary that makes
tmux sessions declarative. One `config.yml` in the repo, one `tmh
init`, and my teammate's tmux comes up identical to mine. Minus the
secrets, which `tmh export --minimal` strips automatically.

## The shape of a tmh config

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/mark1708/tmh/main/schemas/tmh.schema.json

version: 1

roots:
  work: ~/work/orgA

templates:
  go-service:
    layout: 3-pane
    panes:
      - {command: go run ./cmd/$service}
      - {command: go test ./...}
      - {command: zsh}

sessions:
  api:
    root: work
    windows:
      dev: {path: services/api, extends: go-service, env: {service: api}}
      db:  {path: services/api, command: psql myapp_dev}
```

Templates eliminate the "I need the same 3-pane layout across 8
services" tax. Env vars let one template parameterise per-service.
Profiles let one YAML serve dev/staging/prod without duplication.

## The feature that changed my workflow

`tmh diff` is the headline I talk about least but use most. It
compares the YAML with the live tmux state and tells me what drifted.
When my laptop reboots and tmux-continuum restores something wrong, I
see it immediately. When a teammate edits the config and opens a PR,
the diff is a structural list I can review instead of a wall of YAML.

Drift isn't enforced. tmh doesn't "reconcile" your tmux behind your
back. You decide whether to push (config → live) or pull (live →
config), per entry.

## What I stopped doing

- My nine `tm-*` zsh aliases are gone.
- `tmux-continuum` is still there, but I trust it less — tmh
  structural drift detection is my safety net, pane-content
  resurrection is a bonus.
- The Notion doc with "here's how to open the project" is gone.

Full feature list, architecture notes, migration guide for zsh
aliases:

https://github.com/mark1708/tmh

Install: `brew install mark1708/tap/tmh` or
`go install github.com/mark1708/tmh/cmd/tmh@latest`.

MIT-licensed, tests green with `-race`, no telemetry.

---

Feedback welcome — especially if you've hit the same "which terminal
does what" onboarding problem and solved it differently.
```

---

## Post-launch — watch and respond

- Refresh the HN submission every 15 min for the first 2h; the HN
  front-page window is tight.
- Reddit moderators are slow on weekends — if r/tmux posts don't
  appear, message modmail. Don't re-post.
- lobste.rs users love a well-edited description; read the top-voted
  replies before responding.
- If a comment pattern appears across channels (e.g., "why not tmuxp
  freeze?"), add it to `docs/launch-notes.md` under **Canned
  responses** so next time you don't have to re-write the answer.

**Do not post a v1.0.1 patch in the first 48 hours** unless it's a
data-loss or security bug. Usability tweaks can wait a week — batching
them into v1.0.1 reads better than six patches in a day.
