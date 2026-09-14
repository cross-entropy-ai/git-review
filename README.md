# git review

**Review your branch. Keep your place.**

A pull-request-style review experience, right in your terminal. Browse the whole branch, read highlighted diffs, and check off files as you go.

[![Watch git review in action](docs/demo.gif)](docs/demo.gif)

[View the full-size demo](docs/demo.gif) · [Download the recording](docs/demo.cast)

*Compare inline and split diffs, browse a file tree, resume viewed progress, and switch between committed and local changes.*

## Make your next review easier

- **See the whole change.** Get branch-wide and per-file additions and deletions, then jump straight to the code that matters.
- **Make progress you can see.** Mark a file viewed to fold it and move on. Come back later and pick up the same review where you left off.
- **Find your way through large diffs.** Switch between a file list and a directory tree, find files with fuzzy search, search diff text, and collapse files you've already read.
- **Review before you commit.** `git review -w` includes staged edits, unstaged edits, and new files Git hasn't tracked yet.
- **Work the way you like.** Use the keyboard or mouse, with syntax highlighting and automatic light and dark themes.

Local review needs only Git. GitHub PR review also requires an installed, logged-in [GitHub CLI (`gh`)](https://cli.github.com).

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

## Review a GitHub pull request

Install [GitHub CLI](https://cli.github.com) and log in before using remote review:

```sh
gh auth login

git review https://github.com/owner/repo/pull/918
git review '#918'                         # PR in the repository identified by origin
git review 918                            # Local ref first; otherwise origin PR #918
git review https://github.com/owner/repo 918
```

Quote `'#918'` so the shell passes it as an argument. A full PR URL works outside a Git checkout. Number-only targets use `git remote get-url origin`, including HTTPS and SSH origins. If origin points to a fork, its PR numbers are used; a full PR URL can select the upstream repository instead. Explicit `--base 918` always treats `918` as a local ref. A local comparison error never silently changes the target to a PR.

GitHub review calls `gh api` to read PR metadata, paginated file diffs, and Viewed states. It never clones, fetches, or checks out a repository. Missing `gh` or an inactive login produces installation/login instructions. Private repositories require access through the active `gh` account. For GitHub Enterprise, log in to the URL's host with `gh auth login --hostname HOST`.

The header shows **GitHub PR** and the repository/PR number. Inline/split views, the file tree, file and text search, and folding work as in local review. The PR scope stays fixed; `m` does not switch to local changes.

- Press `v` or click Viewed to update the file's state on GitHub. `[~]` means the update is still saving; a failure restores the previous state and shows the error.
- Press `r` to reload both the diff and GitHub's Viewed states. `[!]` means GitHub reports new changes since the file was viewed.
- `--no-state` keeps progress in memory and disables GitHub Viewed reads and writes. `--stat` also performs no Viewed synchronization.
- While Viewed updates are pending, `r` and `q` ask you to wait and try again after saving. `Ctrl+C` can exit immediately; a request already received by GitHub may still complete.

GitHub supplies fixed patch context, so PR targets do not support custom `--context`, local scope flags, or `--base`/`--head`. PRs exceeding the files API's 3000-file limit are rejected. Binary files, metadata-only changes, and omitted/incomplete patches have an explicit notice; unavailable text is never shown as a complete diff.

## A few keys go a long way

| Key | What it does |
| --- | --- |
| `↑` / `↓` or `j` / `k` | Scroll and navigate files |
| `n` / `p` | Jump to the next / previous file, or text match when search is active |
| `Space` | Fold or unfold the selected file |
| `t` | Toggle Tree/List |
| `s` | Toggle inline / split diff (old on the left, new on the right) |
| `e` | Open the selected local file in the default editor |
| `v` | Mark viewed, fold, and move on |
| `f` | Open the fuzzy file finder popup |
| `/` | Search text across the current comparison's diff |
| `Tab` | Switch between the sidebar and diff |
| `m` | Toggle Working tree / Committed |
| `r` | Refresh the review |
| `?` / `q` | Floating help / quit |

Prefer the mouse? Click a file to jump to it, click its checkbox to mark it viewed, and scroll either pane. In the tree, click a directory to expand or collapse it.

Press `f` for an embedded fzf-style file finder; no external `fzf` installation is needed. Type parts of a filename or path to narrow the list, including rename source paths. Use arrows, Tab/Shift+Tab, or Ctrl+N/P to select, Enter to open, and Esc to cancel. Choosing a file keeps the full review list intact.

Press `/` to search added, removed, and context lines in all loaded diff hunks. Search is literal and case-insensitive, with live highlights and a match counter. Matches in folded files expand automatically; inline and split views are supported. Press Enter to confirm, then `n` / `p` to navigate matches with wraparound. Esc clears the search and restores `n` / `p` file navigation. While typing a query, `n` and `p` remain ordinary text. This searches the displayed comparison, not unchanged repository content outside its hunks. Press `?` for a scrollable help popup; Esc or `q` closes it and returns to the same review position.

Press `e` to edit the selected file in your local working tree, including when reviewing committed changes. The editor follows Git's settings: `GIT_EDITOR`, `core.editor`, `VISUAL`, then `EDITOR`, with Git's fallback (usually `vi`). Editor arguments such as `code --wait` are supported. After closing the editor, press `r` to refresh. Files missing locally and remote PR comparisons without a local working tree cannot be opened.

## Good to know

- Auto mode checks staged, unstaged, and untracked changes, respecting Git ignore rules. `-w` / `--working-tree` shows current on-disk changes against `HEAD`; a clean worktree stays in this mode with an empty diff. `-c` / `--committed` always shows committed changes. Refreshing with `r` keeps the current scope, even if local changes appear or disappear.
- Reviewing leaves your files and staging area unchanged; `e` lets you edit files explicitly. Local review progress is saved under `.git/git-review/`; use `--no-state` for a session without saved progress.
- Local progress belongs to an exact comparison. Refreshing changed content starts a fresh review for the whole comparison; unchanged content keeps its viewed marks.
- Use a terminal at least 90 columns wide for the sidebar. `--no-mouse` restores native terminal text selection; `--no-color` disables colors.

For the full option list, run `git review --help`. To work on the project, run `just` for available tasks or `just check` to verify a change.

Maintainers: see [Publishing a release](docs/releasing.md) for release automation.
