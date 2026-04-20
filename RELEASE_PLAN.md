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
2. Owner: **mark1708** (or your chosen GitHub org — if different, **stop
   and come back to this plan after** updating `go.mod`,
   `homebrew/tmh.rb`, and README install snippets).
3. Repository name: **tmh**.
4. Visibility: **Public**.
5. **Do NOT** initialise with README, .gitignore, or LICENSE — the local
   repo already has them and an initial commit on `main`.
6. Create the repository.

### 1.2 Push the local `main` once

```sh
cd /Users/mark/Documents/Projects/me/products/terminal/repos/tmh
git remote add github git@github.com:mark1708/tmh.git
git push github main
git push github --tags   # no tags yet, harmless
```

### 1.3 Configure the self-hosted mirror to keep pushing to GitHub

Two options; pick one.

**Option A — Gitea/Forgejo native mirror UI (recommended):**

1. Open your self-hosted repo settings at
   `https://github.com/Mark1708/tmh/settings`.
2. Enable "Push mirror" → add `https://github.com/mark1708/tmh.git`,
   sync every 8h, push on every event.
3. Auth: create a GitHub personal access token with `repo` scope and
   paste it as the mirror's HTTPS password. Username can be anything.

**Option B — a local git hook:**

```sh
cat > .git/hooks/post-push <<'EOF'
#!/bin/sh
git push github "$@"
EOF
chmod +x .git/hooks/post-push
```

Option A survives laptop reimage; Option B needs to be re-set on every
clone. Prefer A.

### Verify

```sh
git ls-remote https://github.com/mark1708/tmh HEAD
# should print a commit hash matching `git rev-parse main` locally
```

---

## Step 2 — Homebrew distribution

**Why:** the README's second install snippet is `brew install
mark1708/tap/tmh`. That command only resolves if a tap repo with the
name `homebrew-tap` exists under your GitHub account and contains
`Formula/tmh.rb`.

### 2.1 Create the tap repository

1. Open <https://github.com/new>.
2. Owner: **mark1708**.
3. Repository name: **homebrew-tap** (the `homebrew-` prefix is
   mandatory — that's how `brew install owner/tap/formula` maps to a
   GitHub URL).
4. Visibility: **Public**.
5. Initialise with a README (optional — a placeholder is fine).

### 2.2 Copy the formula into the tap

```sh
cd /tmp
git clone git@github.com:mark1708/homebrew-tap.git
cd homebrew-tap
mkdir -p Formula
cp /Users/mark/Documents/Projects/me/products/terminal/repos/tmh/homebrew/tmh.rb Formula/tmh.rb
git add Formula/tmh.rb
git commit -m "add tmh formula"
git push
```

After step 5 below produces a release with real checksums, you will come
back and update `Formula/tmh.rb` in this tap — see **Step 6** near the
end of this document.

### Verify

```sh
brew tap mark1708/tap
brew info mark1708/tap/tmh
# prints the formula version + sha256 placeholders (they'll be real
# after step 6)
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
Users who follow `docs/verify.md` depend on this signature.

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

Push the public half to a keyserver so `docs/verify.md` users can fetch
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
in `docs/verify.md`.

### 4.3 Export the key for GitHub Actions

```sh
gpg --armor --export-secret-keys <LONG_KEYID> > /tmp/tmh-signing-key.asc
```

Keep this file on disk for the next five minutes only. You're about to
paste it into a GitHub secret and then delete it locally.

### 4.4 Add secrets to the GitHub repo

1. Open <https://github.com/mark1708/tmh/settings/secrets/actions>.
2. Click **New repository secret**.
3. Add two secrets, each a single value:
   - Name `GPG_PRIVATE_KEY` — paste the **entire content** of
     `/tmp/tmh-signing-key.asc` including the BEGIN/END lines.
   - Name `GPG_FINGERPRINT` — paste the 40-char fingerprint (no spaces).

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
grep -E "GPG_PRIVATE_KEY|GPG_FINGERPRINT" \
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

### 5.2 Create the annotated tag

Annotated (`-a`) so the tag itself carries a message:

```sh
git tag -a v1.0.0 -m "tmh 1.0.0 — first public release"
```

Sign it if you want (optional; the release artefacts are already GPG-
signed via goreleaser):

```sh
git tag -s v1.0.0 -m "tmh 1.0.0 — first public release"
```

### 5.3 Push the tag to both remotes

```sh
git push origin v1.0.0     # self-hosted — triggers .gitea workflow
git push github v1.0.0     # public — triggers .github workflow
```

If you set up the push-mirror in 1.3 Option A, pushing to `origin` alone
is enough — but pushing explicitly to `github` is safer for the first
release.

### 5.4 Watch the release workflow

Open <https://github.com/mark1708/tmh/actions>. The **release**
workflow kicks off immediately. It takes 2–4 minutes.

If it fails:

- **`gpg: signing failed: No secret key`** → Step 4.4 secrets aren't
  set or `GPG_FINGERPRINT` doesn't match the imported key.
- **`goreleaser: config must be in...`** → `.goreleaser.yml` has a
  typo; fix on `main`, delete the tag with
  `git tag -d v1.0.0 && git push origin :refs/tags/v1.0.0`, re-tag.
- **`go: module not found`** → module path mismatch; the tag points at
  a commit where `go.mod` still says `github.com/Mark1708/tmh`. Re-tag after
  fixing.

### 5.5 Publish the release

The workflow creates a **draft** release (see `.goreleaser.yml`:
`release.draft: true`). Open
<https://github.com/mark1708/tmh/releases>, click the draft, paste the
GPG **fingerprint** from step 4.2 into the notes body, and click
**Publish release**.

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
gh release download v1.0.0 --repo mark1708/tmh
gpg --verify checksums.txt.sig checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
```

---

## Appendix — after the tag is live

The following are handled (or prepared) by Claude Code in this PR:

- **GIF demos:** `make demo` was run locally; `docs/demo-*.gif` are
  committed and referenced from README. No action needed.
- **Homebrew formula sha256:** the post-release update is scripted in
  `scripts/update-formula-sha256.sh`. Run it once the GitHub release
  has assets:
  ```sh
  bash scripts/update-formula-sha256.sh v1.0.0
  ```
  This fetches `checksums.txt` from the release, rewrites the
  `REPLACE_WITH_*_SHA256` placeholders in `homebrew/tmh.rb`, and
  prints a diff to review before you commit to both the main repo and
  the `homebrew-tap` repo.
- **Launch posts:** ready-to-paste drafts live in
  `docs/launch-posts.md`. Posting them to HN / r/tmux / r/golang /
  lobste.rs is an action you take from your own accounts — Claude
  Code can't authenticate as you, and shouldn't.

When all of Steps 1–5 are green, Step 6 runs in two minutes, and you're
free to post whenever you're ready.
