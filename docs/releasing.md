# Publishing a release

A stable tag such as `v0.1.0` triggers `.github/workflows/release.yml`. The workflow runs checks, builds four archives, publishes a GitHub Release, and commits the matching formula to `cross-entropy-ai/homebrew-tap`.

## One-time setup

1. Push the workflow changes to `cross-entropy-ai/git-review` and the initial formula to the tap's `main` branch.
2. Create a fine-grained personal access token scoped to **only** `cross-entropy-ai/homebrew-tap`, with **Contents: read and write**. Complete organization approval if required. The token's user must be allowed to push to the tap's `main` branch.
3. Add it as the **`HOMEBREW_TAP_TOKEN`** Actions secret in the **git-review** repository. For example, run `gh secret set HOMEBREW_TAP_TOKEN --repo cross-entropy-ai/git-review` and paste it at the hidden prompt.

The workflow's own `GITHUB_TOKEN` publishes releases. A [separate credential is required for another repository](https://github.com/actions/checkout#checkout-multiple-repos-private). No local SSH key is uploaded or reused. Renew the tap token before it expires.

Before the first release, the checked-in formula supports `brew install --HEAD cross-entropy-ai/tap/git-review` and builds from source. The first successful release replaces it with a formula using the four published binary archives and their actual SHA-256 checksums. No placeholder release URLs or checksums are installed in the tap.

## Cut a release

From a clean, reviewed commit on `main`:

```sh
just check
# Use the next version; the tag supplies the binary's version.
git tag -a v0.1.0 -m 'Release v0.1.0'
git push origin main
git push origin v0.1.0
```

Only stable `vMAJOR.MINOR.PATCH` tags are supported. Prerelease tags are rejected so they cannot replace the stable Homebrew formula.

The release contains:

- `git-review_VERSION_darwin_amd64.tar.gz`
- `git-review_VERSION_darwin_arm64.tar.gz`
- `git-review_VERSION_linux_amd64.tar.gz`
- `git-review_VERSION_linux_arm64.tar.gz`
- `checksums.txt` and the generated `git-review.rb`

Each archive contains `git-review` and `README.md`. Builds disable CGO; Git remains a runtime dependency. macOS binaries are not Apple Developer ID signed or notarized.

After both jobs finish, verify the [release downloads](https://github.com/cross-entropy-ai/git-review/releases) and install with `brew install cross-entropy-ai/tap/git-review`. Existing users run `brew update && brew upgrade git-review`.

## Verify locally

```sh
just dist v0.1.0
# macOS (use sha256sum --check on Linux):
(cd dist/v0.1.0 && shasum -a 256 --check checksums.txt)
```

This produces the exact archive layout and formula without publishing anything. Outputs live under the ignored `dist/v0.1.0/` directory. CI also runs checks on macOS and Linux and builds all four targets as downloadable preview artifacts.

## Recover a failed run

If publishing fails, fix the cause and rerun the failed job. Assets are uploaded while the release is a draft, then published together. A rerun never overwrites published assets: it compares their checksum manifest with the rebuilt manifest instead. Do not move published tags; release a new version for changes.

If the tap update fails, the GitHub Release remains available. Add or fix `HOMEBREW_TAP_TOKEN`, then rerun **only the failed jobs**. The tap update fetches the latest stable release, verifies the formula checksum, and commits only when it changes. This prevents an older workflow retry from downgrading Homebrew. Branch protection must permit the token's user to push; a concurrent change to the same formula may require another retry.

You can also run the workflow manually for an existing tag with `gh workflow run release.yml --ref v0.1.0`; selecting a branch is rejected.
