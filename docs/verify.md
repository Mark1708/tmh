# Verifying a tmh release

Every tagged tmh release on GitHub ships with:

- `tmh_<version>_<os>_<arch>.tar.gz` binaries
- `checksums.txt` — SHA-256 sums for every archive
- `checksums.txt.sig` — detached GPG signature of `checksums.txt`

Use the signature to verify you're running the binary the maintainer
actually built.

## One-time: import the signing key

The tmh signing key lives at [keys.openpgp.org][openpgp]:

```sh
gpg --keyserver keys.openpgp.org \
    --recv-keys <FINGERPRINT>
```

Replace `<FINGERPRINT>` with the value published in the release notes
of the version you're verifying. Do **not** fetch a key from a random
URL you find in a README — always cross-reference the fingerprint
against a release note rendered on GitHub (or wherever the authoritative
mirror lives).

[openpgp]: https://keys.openpgp.org

## Verifying a downloaded release

Assuming you've downloaded `tmh_1.0.0_darwin_arm64.tar.gz`,
`checksums.txt`, and `checksums.txt.sig` into the same directory:

```sh
# 1. signature → confirms checksums.txt came from the maintainer
gpg --verify checksums.txt.sig checksums.txt

# 2. the tarball's digest matches the trusted checksums file
sha256sum -c checksums.txt --ignore-missing 2>/dev/null || \
  shasum -a 256 -c checksums.txt --ignore-missing
```

If both steps pass, the archive is byte-identical to what CI produced
from the tagged commit.

Extract and install:

```sh
tar xzf tmh_1.0.0_darwin_arm64.tar.gz
install -m 0755 tmh ~/.local/bin/tmh
tmh version
```

## Homebrew users

Homebrew computes its own SHA-256 on download and refuses to install if
the digest doesn't match the formula's expected value. You can still
manually re-verify the signed checksums file from the release page if
you'd like belt-and-braces assurance.

## If verification fails

1. Re-download from the official release page — mirrors, browser cache,
   and incomplete downloads are the most common culprits.
2. Confirm the fingerprint you imported matches the release-note
   fingerprint exactly (copy-paste only, never retype).
3. Open a SECURITY advisory (see [SECURITY.md](../SECURITY.md)) — do
   **not** discuss the discrepancy in a public issue until we've
   confirmed whether it's a compromise or a build-pipeline bug.
