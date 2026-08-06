# tmh docs

This directory separates hand-written guides, generated reference files,
and recorded demos.

## Hand-written guides

- [Architecture](./guides/architecture.md)
- [Active session (optional)](./guides/active-session.md) — real tmux session with linked windows
- [Release verification](./guides/verify.md)
- [Versioning policy](./guides/versioning.md)

## Generated reference

`docs/generated/` is produced by `make docs` via `cmd/tmh-gen`:

- `docs/generated/man/*.1` — man pages generated from the Cobra command tree.
- `docs/generated/completions/{bash,zsh,fish}/tmh` — shell completions.

Do not hand-edit generated files. Update the command tree or generator,
then run `make docs`.

## Demos

Demo tapes and rendered GIFs live in [demos](./demos/). Regenerate them
with `make demo` when the terminal UI changes.
