# Versioning policy

tmh follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)
from `1.0.0` onwards.

## Public API surface

The following are covered by the versioning contract. Breaking changes
to any of them require a **major** release bump and a CHANGELOG entry
under `[Breaking changes]`.

### 1. CLI surface

- The set of subcommands (`tmh attach`, `tmh ls`, …).
- The set of flags and their names (long form only — short forms may
  change between minors with a deprecation notice).
- The meaning of exit codes:
  - `0` — success.
  - `1` — user-visible failure (missing config, tmux errors, denied
    hooks, validation).
  - `2` — invalid invocation (bad flag, unknown subcommand, etc. —
    cobra's default for usage errors).
- The shape of `--json` output (keys, types, nesting). `--json` on
  `tmh ls`, `tmh diff`, `tmh tmux audit` emits **English-stable** strings
  regardless of locale. New fields are additive (minor bump); removed
  fields are breaking (major bump).

### 2. Config schema (`config.yml`)

- The top-level fields (`version`, `roots`, `defaults`, `templates`,
  `layouts`, `profiles`, `sessions`, `discover`).
- Every documented field within those blocks (see
  `schemas/tmh.schema.json`).
- YAML shorthand forms (scalar strings coerced to `{Dir: s}` for
  windows, `$key/path` coerced to `{root, path}`).

The config parser is **lenient** about unknown fields at minor versions
(a forward-compat window), but `tmh doctor` will warn. Unknown fields at
major boundaries become hard errors.

### 3. On-disk state layout

- `~/.config/tmh/config.yml`
- `~/.local/state/tmh/{state.db, history.jsonl, snapshots/, marks.db}`

These paths are stable. The DB schema inside `state.db` is internal — it
migrates automatically and is not part of the versioning contract.

## NOT part of the public API

- Any Go package under `internal/` — importers from outside the module
  have no stability guarantee. Fork if you depend on them.
- The TUI layout, keybindings, color palette — these may change freely
  between minors based on UX feedback. Custom keybindings via the
  settings file remain supported.
- Error **messages** (wording). `errors.Is` against the sentinel
  constants in `internal/errors` is part of the internal contract only.
- Generated artefacts (`docs/man/*.1`, `docs/completions/*`, the JSON
  schema file) — these are generated from the API surface and will
  re-generate automatically per release. Don't hand-edit.

## Deprecations

A feature marked `deprecated` in a minor release stays functional until
the next **major** release. CHANGELOG lists both the deprecation entry
point and the removal target (e.g., "deprecated in 1.3, removed in 2.0").

## Release cadence

- **Patch** (`1.x.y`): bug fixes, security fixes, doc updates. No user
  action required.
- **Minor** (`1.y.0`): additive features, new config fields, new
  subcommands, new TUI screens. `tmh --help` and `tmh doctor` surface
  what's new.
- **Major** (`x.0.0`): breaking changes. A migration guide lives under
  `docs/migrate-<oldver>-to-<newver>.md` and is referenced from the
  CHANGELOG entry.

There is no fixed schedule — releases happen when a coherent batch of
changes has baked on `main`.

## Version string

`tmh version` prints `<semver> (commit <short-sha>, built <date>)`. The
commit hash and build date are injected via goreleaser `-ldflags`; a
`go build` without ldflags prints `dev`.
