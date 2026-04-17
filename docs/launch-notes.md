# Launch notes — internal reference

This file is **not user-facing**. It's a working document for the v1.0
public announcement: draft copy, suggested channels, and canned answers
for expected questions. Delete or rename after launch.

## Three pitch angles

Pick one per channel. Don't cross-post verbatim.

### Angle A — "replaces my 9 zsh aliases"

For developer-focused audiences (r/commandline, r/golang, dev.to).

> I kept accumulating `tm-*` aliases for tmux — nine of them, plus an
> INI file that was supposed to let me restore layouts after reboots
> but never actually worked. Rewrote the whole thing as a single Go
> binary: declarative YAML for the layout, drift detection when the
> config and live tmux diverge, undo for destructive actions, and a
> quick fuzzy picker when you just want to attach.
>
> It's MIT-licensed and intentionally boring in scope — no plugin
> system, no GUI, no telemetry. Tmux does the actual multiplexing;
> tmh is just the glue and the verification layer on top.

### Angle B — "declarative tmux sessions in YAML"

For tooling-focused audiences (HN, lobste.rs).

> tmh is a single Go binary that makes tmux sessions declarative —
> one `config.yml` with sessions, windows, panes, templates,
> profiles, and lifecycle hooks. The angle that makes it different
> from tmuxinator / tmuxp is the verification direction: instead of
> treating the config as ground truth and bulldozing the live state,
> `tmh diff` tells you what drifted and you decide whether to push
> (live ← config) or pull (config ← live) per-entry.
>
> Secondary features: SQLite-backed undo/history/snapshots, a TUI
> dashboard with command palette and fuzzy filter, `tmh freeze` to
> capture live state into YAML (inverse of push), hardened file
> perms, and goreleaser-signed multi-arch binaries.

### Angle C — "sharing tmux setups with your team"

For team-lead audiences (dev.to, Twitter/Mastodon threads).

> Onboarding a teammate to a complex monorepo = 45 minutes of
> "which terminal do I open which command in?". With tmh, you
> commit a `config.yml` to the repo, they run `tmh import
> team.yml` + `tmh init`, and their tmux comes up identical to
> yours — minus the secrets, which `tmh export --minimal` strips
> automatically.
>
> Schema-validated YAML with editor autocomplete, profiles per
> environment, optional lifecycle hooks with a trust prompt. No
> plugin system — the hooks are just shell commands you approve on
> first run.

## Channels

Primary — pick two, not all:

- **[Hacker News](https://news.ycombinator.com/submit)** — Show HN
  post, link to the repo + demo GIF. Angle B. Wednesday or Thursday
  morning US time.
- **[lobste.rs](https://lobste.rs/)** — tag `go`, `unix`. Angle B.
- **[r/tmux](https://www.reddit.com/r/tmux/)** — Angle A or C. These
  folks already know the pain.
- **[r/golang](https://www.reddit.com/r/golang/)** — Angle A.
- **dev.to** — long-form writeup. Angle C.

Skip unless you already have reach: Twitter/X, Mastodon, LinkedIn.
They reward ongoing presence, not launch splashes.

## Pinned "First 48 hours" issue

Open this as issue #1 immediately after the tag is live and pin it:

> **Title:** v1.0 first-48h — known items & planned follow-ups
>
> Thanks for looking at tmh! This issue tracks everything that didn't
> quite make v1.0 and known rough edges, so you can see if your
> concern is already on the radar before opening a new issue.
>
> ### Known gaps
>
> - [ ] No Windows support (tmux itself doesn't target Windows).
> - [ ] No auto-session-resurrection of pane *contents* — use
>       tmux-resurrect for that; tmh tracks structure, not scrollback.
> - [ ] `tmh freeze` does non-destructive merge only; dir conflicts
>       are reported but not auto-applied. Use `tmh sync --pull --all`
>       for destructive overwrite.
> - [ ] UI subcomponents (confirm, settings, theme, toast) have
>       minimal unit tests — PRs welcome.
> - [ ] No Docker image. Running tmux inside a container is a
>       distraction from tmh's purpose.
>
> ### Planned v1.1
>
> - [ ] Fully wire scoped hooks into CreateSession (data model is
>       already in 1.0; execution is pending wire-up).
> - [ ] Variable substitution `${VAR}` inside `command:` fields.
> - [ ] Colorblind-safe theme variant + `REDUCE_MOTION` honor.
> - [ ] `tmh doctor --fix` interactive remediation.
>
> Please open individual issues for concrete bugs or feature
> requests — this thread is just a triage hint so we don't duplicate.

## Canned responses for expected questions

Copy-paste these when they come up (tweak the intro to match context).

### "Why not just use tmuxinator?"

> Good question — tmuxinator is mature and stable. The two
> differences that drove tmh: (1) Go binary, no Ruby toolchain on
> every machine, and (2) drift-as-verification. tmuxinator assumes
> the config is ground truth and rebuilds the live state to match;
> tmh shows you the diff and lets you push or pull per entry. If
> you already like tmuxinator, you probably don't need tmh — this
> is for folks who want a safety net between YAML and live tmux,
> not a bulldozer.

### "How is this different from sesh?"

> sesh is brilliant at one thing — zero-config fuzzy session
> discovery via zoxide. tmh's bare-`tmh` picker is directly inspired
> by it (A3 in our internal plan) and we borrow the `discover:`
> concept. The difference is scope: sesh is a session switcher, tmh
> is a session switcher + declarative config + drift detection +
> hooks + TUI dashboard. If you only need the switcher, sesh is
> lighter and better at it. If you also want the config-as-code
> layer, tmh is built around that.

### "Does it work with zellij?"

> No. tmh drives tmux specifically — zellij has its own session
> manager built in. Different tool for a different multiplexer.

### "Does it support Windows?"

> No, and we have no plans to. tmux doesn't target Windows, and the
> wrapping-a-different-multiplexer path would be a rewrite.

### "Is there a plugin system?"

> No, by design. The common plugin use cases (custom commands,
> custom pickers, custom status strings) are covered by the
> command palette, `hooks:`, and `tmh status` respectively. A
> plugin ABI is a commitment we're not ready to make for v1.0;
> revisit in v2.x if real use cases accumulate.

### "Can I run tmh inside a Docker container?"

> The binary itself is a `CGO_ENABLED=0` static binary so it runs
> in any distroless image, but tmux needs to be on the host — you
> don't gain anything by containerising the wrapper. Most users
> who ask this really want tmux-in-container, which is outside
> tmh's scope.

### "Why self-hosted git for dev? Why not GitHub-only?"

> The canonical repo is GitHub. Internal development mirrors to
> self-hosted for uptime-independence; releases publish to both.
> You only interact with GitHub — the mirror is for the maintainer.

## Post-launch monitoring

- Watch for issues on GitHub for 48h actively.
- Check HN / Reddit submissions every 2–3h on launch day.
- Aggregate common questions → this file (so the next release has
  better FAQ coverage).
- If a CRITICAL security issue surfaces, follow SECURITY.md — do not
  triage publicly.

## Don't

- Don't tag @company or @person. Organic reach only.
- Don't reply to HN commenters in their subthreads with corrections;
  just one comment at the top after the first few replies, if needed.
- Don't engage with bad-faith critiques. Move on.
- Don't ship a v1.0.1 patch release in the first 48h unless it's a
  data-loss or security bug — usability tweaks can wait a week.
