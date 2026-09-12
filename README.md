# git review

**Review your branch. Keep your place.**

A pull-request-style review experience, right in your terminal. Browse the whole branch, read highlighted diffs, and check off files as you go.

[![Watch git review in action](docs/demo.gif)](docs/demo.gif)

[View the full-size demo](docs/demo.gif) · [Download the recording](docs/demo.cast)

*Compare inline and split diffs, browse a file tree, resume viewed progress, and switch between committed and local changes.*

## Make your next review easier

- **See the whole change.** Get branch-wide and per-file additions and deletions, then jump straight to the code that matters.
- **Make progress you can see.** Mark a file viewed to fold it and move on. Come back later and pick up the same review where you left off.
- **Find your way through large diffs.** Switch between a file list and a directory tree, filter by path, and collapse files you've already read.
- **Review before you commit.** `git review -w` includes staged edits, unstaged edits, and new files Git hasn't tracked yet.
- **Work the way you like.** Use the keyboard or mouse, with syntax highlighting and automatic light and dark themes.

One binary. Runs locally. No GitHub account or browser needed.

## Get started

### Homebrew

```sh
brew install cross-entropy-ai/tap/git-review
```

Upgrade with `brew update && brew upgrade git-review`. The formula installs the binary for your macOS or Linux machine, on amd64 or arm64.

Before the first release is added to the tap, use `brew install --HEAD cross-entropy-ai/tap/git-review` to build from source. Homebrew installs Go for this initial source build.

### Install directly

Install the latest release on macOS or Linux with one command:

```sh
curl -fsSL https://github.com/cross-entropy-ai/git-review/releases/latest/download/install.sh | sh
```

The installer detects your platform, verifies the download's SHA-256 checksum, and installs to `~/.local/bin`. Run the same command to upgrade. Git is required; no Go toolchain or package manager is needed.

Add `export PATH="$HOME/.local/bin:$PATH"` to your shell configuration if needed, then run `git review` inside any Git repository. Set `INSTALL_DIR` to choose another location: `curl -fsSL https://github.com/cross-entropy-ai/git-review/releases/latest/download/install.sh | INSTALL_DIR=/your/bin sh`.

### Download manually

Prefer to install it yourself? These links always download the latest release:

| Platform | Download |
| --- | --- |
| macOS · Apple Silicon | [darwin_arm64](https://github.com/cross-entropy-ai/git-review/releases/latest/download/git-review_darwin_arm64.tar.gz) |
| macOS · Intel | [darwin_amd64](https://github.com/cross-entropy-ai/git-review/releases/latest/download/git-review_darwin_amd64.tar.gz) |
| Linux · x86-64 | [linux_amd64](https://github.com/cross-entropy-ai/git-review/releases/latest/download/git-review_linux_amd64.tar.gz) |
| Linux · ARM64 | [linux_arm64](https://github.com/cross-entropy-ai/git-review/releases/latest/download/git-review_linux_arm64.tar.gz) |

Download [checksums.txt](https://github.com/cross-entropy-ai/git-review/releases/latest/download/checksums.txt) to verify your archive. Extract it and put `git-review` on your `PATH`, for example on Apple Silicon:

```sh
tar -xzf git-review_darwin_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 755 git-review ~/.local/bin/git-review
```

Downloads and the installer become available after the first release. You can build from source in the meantime.

### Build from source

Prefer to build it yourself? Install [Go](https://go.dev/) (version in [go.mod](go.mod)) and [just](https://github.com/casey/just):

```sh
git clone https://github.com/cross-entropy-ai/git-review.git
cd git-review
just install
```

This installs to `~/.local/bin`. Run `just demo` from the checkout for a ready-made sample review.

## Choose what to review

**Automatic by default** — review local changes when present; otherwise review committed changes since the common ancestor with your base:

```sh
git review                   # Same as git review --auto
```

Force a scope when needed:

```sh
git review -w                # --working-tree: staged, unstaged, and untracked changes
git review -c                # --committed: committed changes, even with local edits
```

Committed review compares `merge-base(base, head)` to `head`. Both refs can be branches, tags, or commit IDs. Passing refs selects committed review; add `--auto` to use them only as the fallback when there are no local changes. The three mode flags are mutually exclusive. Auto only selects the initial scope at startup. The highlighted Mode control shows Working tree or Committed. Click it or press `m` to toggle between them; your base/head refs are preserved.

A few useful variations:

```sh
git review --base develop     # Review committed changes against a different base
git review v1.0 HEAD          # Compare a tag and a commit ref
git review v1.1.0...v1.2.0    # Shorthand for git review v1.1.0 v1.2.0
git review -C /path/to/repo    # Review another repository
git review --theme light      # Choose light or dark manually
git review --stat             # Print a quick change summary
```

The `base...head` shorthand requires both refs and cannot be combined with another positional ref, `--base`, or `--head`. It uses the same merge-base comparison as the two-ref form.

## A few keys go a long way

| Key | What it does |
| --- | --- |
| `↑` / `↓` or `j` / `k` | Scroll and navigate files |
| `n` / `p` | Jump to the next / previous file |
| `Space` | Fold or unfold the selected file |
| `t` | Toggle Tree/List |
| `s` | Toggle inline / split diff (old on the left, new on the right) |
| `v` | Mark viewed, fold, and move on |
| `/` | Find files by path |
| `Tab` | Switch between the sidebar and diff |
| `m` | Toggle Working tree / Committed |
| `r` | Refresh the review |
| `?` / `q` | Help / quit |

Prefer the mouse? Click a file to jump to it, click its checkbox to mark it viewed, and scroll either pane. In the tree, click a directory to expand or collapse it.

## Good to know

- Auto mode checks staged, unstaged, and untracked changes, respecting Git ignore rules. `-w` / `--working-tree` shows current on-disk changes against `HEAD`; a clean worktree stays in this mode with an empty diff. `-c` / `--committed` always shows committed changes. Refreshing with `r` keeps the current scope, even if local changes appear or disappear.
- Your files and staging area stay as they are. Viewed progress is saved locally under `.git/git-review/`; use `--no-state` for a session without saved progress.
- Progress belongs to an exact comparison. Refreshing changed content starts a fresh review for the whole comparison; unchanged content keeps its viewed marks.
- Use a terminal at least 90 columns wide for the sidebar. `--no-mouse` restores native terminal text selection; `--no-color` disables colors.

For the full option list, run `git review --help`. To work on the project, run `just` for available tasks or `just check` to verify a change.

Maintainers: see [Publishing a release](docs/releasing.md) for release automation.
