# Example configurations

Four worked examples covering the typical setups. Each file is a
self-contained `config.yml` — copy into `~/.config/tmh/` and run
`tmh init` to materialise it.

| File                 | When to pick it                                              |
|----------------------|--------------------------------------------------------------|
| [minimal.yml](./minimal.yml)   | You just want one session, one window. Smallest possible config. |
| [monorepo.yml](./monorepo.yml) | Multiple services in one repo. Uses `roots` to share a prefix. |
| [polyglot.yml](./polyglot.yml) | Multiple repos + toolchains. Uses `templates` + `profiles` to avoid duplication. |
| [devops.yml](./devops.yml)     | K8s / Docker / observability workflows. Shows `discover` + hooks. |

Cross-references:

- **Templates vs profiles** — templates are reusable window shapes,
  profiles scope which sessions apply + override env/defaults. See
  [`polyglot.yml`](./polyglot.yml) for both used together.
- **`discover` rules** — auto-generate session candidates from a glob
  or zoxide. See [`devops.yml`](./devops.yml).
- **Hooks** — pre/post lifecycle shell commands with a trust prompt.
  See [`devops.yml`](./devops.yml).

## Tip

When copying, keep the first line — that's the
`# yaml-language-server: $schema=…` modeline which enables live
autocompletion and validation in VS Code, Helix, and Neovim with any
standard yaml-language-server setup.

If your editor doesn't support schema modelines, regenerate the schema
at any time with `make schema` and point your editor config at
`schemas/tmh.schema.json` manually.
