# RELEASE_PLAN.md — manual steps for tmh v1.0

This document is the **maintainer runbook** for turning the `main`
branch (as of commit `784f397`) into a public v1.0 release. Everything
here is outside the scope of what Claude Code can automate because each
step needs an account, a key, or a GitHub UI click.

Do the steps **in order**. Each section ends with a one-line "verify"
command so you can confirm the step actually took effect before moving
on.

---

## Step 1 — Create the public GitHub repository and wire the mirror

**Why:** until there's a public GitHub repo, `go install
github.com/mark1708/tmh/cmd/tmh@latest` returns 410 Gone, every install
instruction in the README is false, and Homebrew can't fetch release
tarballs. Everything else depends on this.

### 1.1 Create the repo

1. Open <https://github.com/new>.
2. Owner: **Mark1708** (or your chosen GitHub org — if different, **stop
   and come back to this plan after** updating `go.mod`, README install
   snippets, and `.goreleaser.yml` Homebrew settings).
3. Repository name: **tmh**.
4. Visibility: **Public**.
5. **Do NOT** initialise with README, .gitignore, or LICENSE — the local
   repo already has them and an initial commit on `main`.
6. Create the repository.

### 1.2 Push the local `main` once

```sh
cd /Users/mark/Documents/Projects/me/products/terminal/repos/tmh
git remote add github git@github.com:Mark1708/tmh.git
git push github main
git push github --tags   # no tags yet, harmless
```

### 1.3 Keep both remotes explicit

Do not rely on push mirrors or local hooks for release tags. The release
contract is explicit: `make release TAG=vX.Y.Z` creates one annotated tag and
pushes it to both `origin` and `github`. That keeps the self-hosted remote and
the public GitHub release workflow in sync without hidden automation.

### Verify

```sh
git ls-remote https://github.com/mark1708/tmh HEAD
# should print a commit hash matching `git rev-parse main` locally
```

---

## Step 2 — Homebrew distribution

**Why:** the README's second install snippet is `brew install --cask
mark1708/tap/tmh`. That command resolves when GoReleaser can publish the
cask to `Mark1708/homebrew-tap` during the tagged GitHub release.

### 2.1 Confirm the tap repository

1. Open <https://github.com/new>.
2. Owner: **Mark1708**.
3. Repository name: **homebrew-tap** (the `homebrew-` prefix is
   mandatory — that's how `brew install --cask owner/tap/cask` maps to a
   GitHub URL).
4. Visibility: **Public**.
5. Initialise with a README (optional — a placeholder is fine).

### 2.2 Confirm GoReleaser Homebrew settings

```sh
grep -A30 '^homebrew_casks:' /Users/mark/Documents/Projects/me/products/terminal/repos/tmh/.goreleaser.yml
```

The block must target `owner: mark1708`, `name: homebrew-tap`, and use
`{{ .Env.HOMEBREW_TAP_TOKEN }}`. Do not copy or hand-edit a cask from this
repository; GoReleaser owns the cask contents and checksums.

### Verify

```sh
brew tap mark1708/tap
brew info --cask mark1708/tap/tmh
# prints the latest cask after the first GoReleaser-published release
```

### 2.3 Homebrew core (deferred)

**Not a v1.0 blocker.** Submitting tmh to the central `homebrew-core`
repo requires > 50 GitHub stars plus a month of stable releases (see
<https://docs.brew.sh/Acceptable-Formulae>). Revisit for v1.1+.

---

## Step 4 — GPG signing for release artefacts

**Why:** `.goreleaser.yml` has a `signs:` block that invokes `gpg
--detach-sign` on `checksums.txt`. If the signing key isn't available to
the CI runner, the goreleaser step fails and no release is published.
Users who follow `docs/guides/verify.md` depend on this signature.

### 4.1 Ensure a signing key exists locally

```sh
gpg --list-secret-keys --keyid-format=long
```

- If you already have a key you're happy to sign tmh releases with,
  note its long keyid (the 16-hex-char string after `sec rsa…/`) and
  skip to 4.2.
- Otherwise generate one:
  ```sh
  gpg --full-generate-key
  ```
  - Kind: `(1) RSA and RSA`
  - Size: `4096`
  - Expiry: `2y` (renewable)
  - Real name: your name as you want it in release notes
  - Email: `mark1708.work@gmail.com`
  - Passphrase: use a **strong** one; you'll paste this into a secret

### 4.2 Publish the public key

Push the public half to a keyserver so `docs/guides/verify.md` users can fetch
it:

```sh
gpg --send-keys --keyserver keys.openpgp.org <LONG_KEYID>
```

Note the **fingerprint**:

```sh
gpg --fingerprint <LONG_KEYID>
# copy the 40-hex-char fingerprint (last line, groups of 4)
```

Paste the fingerprint into the first release note — users reference it
in `docs/guides/verify.md`.

### 4.3 Export the key for GitHub Actions

```sh
gpg --armor --export-secret-keys <LONG_KEYID> > /tmp/tmh-signing-key.asc
```

Keep this file on disk for the next five minutes only. You're about to
paste it into a GitHub secret and then delete it locally.

### 4.4 Add secrets to the GitHub repo

1. Open <https://github.com/mark1708/tmh/settings/secrets/actions>.
2. Click **New repository secret**.
3. Add four secrets, each a single value:
   - Name `GPG_PRIVATE_KEY` — paste the **entire content** of
     `/tmp/tmh-signing-key.asc` including the BEGIN/END lines.
   - Name `GPG_FINGERPRINT` — paste the 40-char fingerprint (no spaces).
   - Name `GPG_PASSPHRASE` — paste the passphrase for the exported signing key.
   - Name `HOMEBREW_TAP_TOKEN` — GitHub token with write access to
     `Mark1708/homebrew-tap`.

4. Delete the armored key from disk:
   ```sh
   shred -u /tmp/tmh-signing-key.asc   # or: rm -P on macOS
   ```

### Verify

```sh
# The secrets should both show up in:
open "https://github.com/mark1708/tmh/settings/secrets/actions"
# You can't read them back; you can only confirm they exist.
```

Also confirm the workflow references them correctly:

```sh
grep -E "GPG_PRIVATE_KEY|GPG_FINGERPRINT|GPG_PASSPHRASE|HOMEBREW_TAP_TOKEN" \
  /Users/mark/Documents/Projects/me/products/terminal/repos/tmh/.github/workflows/release.yml
```

---

## Step 5 — Cut the v1.0.0 tag

**Why:** goreleaser runs **only** on tag pushes matching `v*`. Tag
creation is a one-way commitment — treat this as the release moment.

### 5.1 Pre-tag sanity check

```sh
cd /Users/mark/Documents/Projects/me/products/terminal/repos/tmh
git status                 # must be clean; no staged or untracked files
git log --oneline -5       # last commit should be the CHANGELOG finalisation
make test-race             # all packages green
make docs                  # regenerate man/completions/schema to be safe
git diff --stat            # if `make docs` produced anything, commit it
```

### 5.2 Prepare the annotated tag

The Makefile release target creates an annotated (`-a`) tag so the tag
itself carries a message, then pushes that tag to both remotes.

If you need a signed tag instead of the Makefile's annotated tag, do it
manually before pushing (optional; the release artefacts are already GPG-
signed via goreleaser):

```sh
git tag -s v1.0.0 -m "tmh 1.0.0 — first public release"
```

### 5.3 Push the tag to both remotes

```sh
make release TAG=v1.0.0    # annotated tag + pushes to origin and github
```

Do not push only one remote for release tags. Both remotes must receive the
same tag so the internal repository and the public GitHub release stay aligned.

### 5.4 Watch the release workflow

Open <https://github.com/mark1708/tmh/actions>. The **release**
workflow kicks off immediately. It takes 2–4 minutes.

If it fails:

- **`gpg: signing failed: No secret key`** → Step 4.4 secrets aren't
  set or `GPG_FINGERPRINT` doesn't match the imported key.
- **`gpg: signing failed: Inappropriate ioctl for device`** →
  `GPG_PASSPHRASE` is missing or does not match the imported key.
- **`goreleaser: config must be in...`** → `.goreleaser.yml` has a
  typo; fix on `main`, delete the tag with
  `git tag -d v1.0.0 && git push origin :refs/tags/v1.0.0`, re-tag.
- **`go: module not found`** → module path mismatch; the tag points at
  a commit where `go.mod` does not say `github.com/mark1708/tmh`. Re-tag
  after fixing.

### 5.5 Verify the published release

The workflow publishes the GitHub release directly. Open
<https://github.com/mark1708/tmh/releases> and confirm the assets,
`checksums.txt`, `checksums.txt.sig`, and release notes are present.
GoReleaser also opens or updates the cask in `Mark1708/homebrew-tap`.

### Verify

```sh
# Public install path now works:
go install github.com/mark1708/tmh/cmd/tmh@v1.0.0
~/go/bin/tmh version
# should print: 1.0.0 (commit <sha>, built <date>)
```

Also download and verify a binary tarball as a user would:

```sh
cd /tmp
gh release download v1.0.0 --repo Mark1708/tmh
gpg --verify checksums.txt.sig checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
```

---

## Appendix — after the tag is live

The following are handled (or prepared) by Claude Code in this PR:

- **GIF demos:** `make demo` was run locally; `docs/demos/demo-*.gif` are
  committed and referenced from README. No action needed.
- **Homebrew cask:** GoReleaser publishes it to
  `Mark1708/homebrew-tap` during the release workflow. Verify with
  `brew update && brew reinstall --cask mark1708/tap/tmh`; do not run a manual
  checksum script or edit a cask in this repository.
- **Launch posts:** Posting to HN / r/tmux / r/golang / lobste.rs is an
  action you take from your own accounts — Claude Code can't authenticate
  as you, and shouldn't.

When all of Steps 1–5 are green, Step 6 runs in two minutes, and you're
free to post whenever you're ready.
