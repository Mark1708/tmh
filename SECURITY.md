# Security Policy

## Supported Versions

tmh follows semantic versioning. The latest minor release (`1.x.y`) is
actively supported with security fixes. Older minor versions may receive
patches on a best-effort basis.

| Version | Supported |
| ------- | --------- |
| latest  | ✅ Yes    |
| < latest| ⚠️ Best effort |

## Reporting a Vulnerability

If you believe you have found a security vulnerability in tmh, please report
it **privately** — do not open a public GitHub issue, discussion, or pull
request.

Send a detailed report to **mark1708.work@gmail.com** with the subject line
`[tmh security]`. Include:

- a description of the vulnerability and its impact,
- reproduction steps or a proof-of-concept,
- the version / commit hash where it was observed,
- optionally, a suggested mitigation.

You will receive an acknowledgement within **72 hours**. We aim to provide a
fix or a mitigation plan within **90 days** of confirmation. Coordinated
disclosure is strongly preferred.

## Out of Scope

The following are explicitly **out of scope** for security reports:

- Vulnerabilities in upstream dependencies — please report those to the
  upstream project. We will track them via `govulncheck` and update promptly.
- Vulnerabilities in tmux itself — report to the tmux project.
- Issues that require physical access to the user's machine or shell session.
- Social-engineering scenarios that assume the attacker already controls the
  user's terminal or `$SHELL`.

## Scanning

We run [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
in CI on every push to `main`. Findings are triaged within a week.

## Hardening Best Practices for Users

- Keep `config.yml` and the state DB at restrictive file permissions
  (`0600`). tmh writes them with `0600` by default — verify after upgrades.
- Treat `hooks` (`on_create`, `on_attach`, etc.) as trusted code — tmh
  prompts when a hook changes, but you are responsible for auditing the
  command.
- Never share your `config.yml` publicly if it contains secrets in `env:` —
  prefer environment variables resolved at runtime.

## Credits

Responsible disclosures will be credited in the release notes (unless the
reporter prefers to remain anonymous).
