# git review

**Review your branch. Keep your place.**

A pull-request-style review experience, right in your terminal. Browse the whole branch, read highlighted diffs, and check off files as you go.

[![Watch git review in action](docs/demo.gif)](https://asciinema.org/a/1265196)

[Watch the 37-second demo](https://asciinema.org/a/1265196) · [Download the recording](docs/demo.cast)

*Explore a branch, switch to a file tree, mark files viewed, and catch untracked files before committing.*

## Make your next review easier

- **See the whole change.** Get branch-wide and per-file additions and deletions, then jump straight to the code that matters.
- **Make progress you can see.** Mark a file viewed to fold it and move on. Come back later and pick up the same review where you left off.
- **Find your way through large diffs.** Switch between a file list and a directory tree, filter by path, and collapse files you've already read.
- **Review before you commit.** `git review -w` includes staged edits, unstaged edits, and new files Git hasn't tracked yet.
- **Work the way you like.** Use the keyboard or mouse, with syntax highlighting and automatic light and dark themes.

One binary. Runs locally. No GitHub account or browser needed.

## Get started

Git is required. Choose Homebrew or download a single binary—no Go toolchain needed for a released version.

### Homebrew

```sh
brew install cross-entropy-ai/tap/git-review
```

Upgrade later with `brew update && brew upgrade git-review`.

**Before the first release:** use `brew install --HEAD cross-entropy-ai/tap/git-review` to build the latest source. Homebrew installs Go for this source build. The standard command above becomes available after the first release updates the tap.

### Download a binary

Open [Releases](https://github.com/cross-entropy-ai/git-review/releases/latest) and download the archive for your machine:

| Platform | Archive suffix |
| --- | --- |
| macOS · Apple Silicon | `darwin_arm64.tar.gz` |
| macOS · Intel | `darwin_amd64.tar.gz` |
| Linux · x86-64 | `linux_amd64.tar.gz` |
| Linux · ARM64 | `linux_arm64.tar.gz` |

Release downloads become available with the first version tag. Each release includes `checksums.txt` for SHA-256 verification. Extract your downloaded archive and put `git-review` on your `PATH`, for example:

```sh
# Replace the filename with your downloaded archive.
tar -xzf git-review_0.1.0_darwin_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 755 git-review ~/.local/bin/git-review
```

Add `export PATH="$HOME/.local/bin:$PATH"` to your shell configuration if needed. Then, inside any Git repository:

```sh
git review
```

### Build from source

Prefer to build it yourself? Install [Go](https://go.dev/) (version in [go.mod](go.mod)) and [just](https://github.com/casey/just):

```sh
git clone https://github.com/cross-entropy-ai/git-review.git
cd git-review
just install
```

This installs to `~/.local/bin`. Run `just demo` from the checkout for a ready-made sample review.

## Two ways to review

**Before opening a PR** — review your branch's committed changes against `main`:

```sh
git review
```

**Before committing** — review everything pending against `HEAD`, including new files:

```sh
git review -w
```

A few useful variations:

```sh
git review --base develop     # Choose a different base branch
git review -C /path/to/repo    # Review another repository
git review --theme light      # Choose light or dark manually
git review --stat             # Print a quick change summary
```

## A few keys go a long way

| Key | What it does |
| --- | --- |
| `↑` / `↓` or `j` / `k` | Scroll and navigate files |
| `n` / `p` | Jump to the next / previous file |
| `Space` | Fold or unfold the selected file |
| `t` | Toggle Tree/List |
| `v` | Mark viewed, fold, and move on |
| `/` | Find files by path |
| `Tab` | Switch between the sidebar and diff |
| `r` | Refresh the review |
| `?` / `q` | Help / quit |

Prefer the mouse? Click a file to jump to it, click its checkbox to mark it viewed, and scroll either pane. In the tree, click a directory to expand or collapse it.

## Good to know

- The default review shows committed branch changes since the common ancestor with your base. `-w` / `--working-tree` shows current on-disk changes against `HEAD`, respecting Git ignore rules.
- Your files and staging area stay as they are. Viewed progress is saved locally under `.git/git-review/`; use `--no-state` for a session without saved progress.
- Progress belongs to an exact comparison. Refreshing changed content starts a fresh review for the whole comparison; unchanged content keeps its viewed marks.
- Use a terminal at least 90 columns wide for the sidebar. `--no-mouse` restores native terminal text selection; `--no-color` disables colors.

For the full option list, run `git review --help`. To work on the project, run `just` for available tasks or `just check` to verify a change.

Maintainers: see [Publishing a release](docs/releasing.md) for release automation and Homebrew setup.
