# Publishing a release

Push a stable tag such as `v0.1.0`. GitHub Actions runs checks, builds macOS/Linux binaries for amd64/arm64, and publishes them with an installer, a Homebrew formula, and SHA-256 checksums. The workflow uses GitHub's automatic `GITHUB_TOKEN`; no additional secrets are required. Updating the Homebrew tap is a manual step after publishing.

## Cut a release

Push the workflow to `main` first. From a clean, reviewed commit:

```sh
just check
git push origin main
# Use the next version; the tag supplies the binary's version.
git tag -a v0.1.0 -m 'Release v0.1.0'
git push origin v0.1.0
```

Only stable `vMAJOR.MINOR.PATCH` tags are supported. After the workflow succeeds, users can install or upgrade using the README's one-line installer.

The release contains:

- `git-review_darwin_amd64.tar.gz`
- `git-review_darwin_arm64.tar.gz`
- `git-review_linux_amd64.tar.gz`
- `git-review_linux_arm64.tar.gz`
- `checksums.txt`, `install.sh`, and `git-review.rb`

Asset names stay the same across versions so [GitHub's latest download links](https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases) keep working. The installer resolves the latest release once and downloads its archive and checksum from that exact tag.

Each archive contains `git-review` and `README.md`. Builds disable CGO; Git remains a runtime dependency. macOS binaries are not Apple Developer ID signed or notarized.

## Update Homebrew manually

After the release is published:

1. Download `git-review.rb` and `checksums.txt` from that specific GitHub Release. Verify the formula with `shasum -a 256 git-review.rb` (or `sha256sum` on Linux) against its entry in `checksums.txt`.
2. Replace `Formula/git-review.rb` in your `cross-entropy-ai/homebrew-tap` checkout with the downloaded formula.
3. Review the diff, then commit and push the tap yourself.

The generated formula pins the release tag, version, and actual SHA-256 checksums for all four binary archives. Copy the published formula as-is; there is no need to rebuild or calculate archive checksums locally. The release workflow never writes to the tap.

Once the tap is updated, users can run `brew install cross-entropy-ai/tap/git-review` or `brew update && brew upgrade git-review`. Until the first release is added, the initial formula supports source installation with `brew install --HEAD cross-entropy-ai/tap/git-review`.

## Verify locally

```sh
just dist v0.1.0
# macOS (use sha256sum --check on Linux):
(cd dist/v0.1.0 && shasum -a 256 --check checksums.txt)
just test-installer
```

Outputs live under the ignored `dist/v0.1.0/` directory. CI runs checks on macOS and Linux, tests the installer, and builds all four targets as downloadable preview artifacts.

## Recover a failed run

Fix the cause and rerun the failed job. Assets are uploaded while the release is a draft, then published together. A rerun never overwrites published assets: it compares their checksum manifest with the rebuilt manifest instead. Do not move published tags; release a new version for changes.

You can also run the workflow manually for an existing tag with `gh workflow run release.yml --ref v0.1.0`; selecting a branch is rejected.
