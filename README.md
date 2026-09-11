# git review

A local, pull-request-style review TUI written in Go. Browse all committed changes on a branch with a file sidebar, total and per-file additions/deletions, line numbers, syntax highlighting, folding, and saved viewed progress.

```text
 git review │ main → feature/health-check
 8 files  +26 -8   2/8 viewed   merge-base 6c97a421 · committed changes
 ─────────────────────────────────────────────────────────────────────────────
 FILES                         │ ● DIFF  internal/server/server.go
  ▸ ✓ assets/logo.png          │ ▾ internal/server/server.go        M +14 -3
      M  binary                │ @@ -1,12 +1,23 @@
 ›▾ ○ internal/server/server.go│     1     1   package server
      M  +14 -3                │     2     2
  ▾ ○ config.json              │     3     3   import (
      M  +4 -3                 │     4       -     "fmt"
                               │           4 +     "encoding/json"
 ? help  q quit  Tab focus  n/p file  Space fold  v viewed  / filter
```

## Build and run

Requires [Go](https://go.dev/) (see `go.mod` for the required version), [just](https://github.com/casey/just), and Git. All development tasks run through `just`.

```sh
just build                         # Build ./git-review with CGO disabled
./git-review -C /path/to/repo       # Default: main...HEAD
./git-review --base develop
./git-review --base main --head feature/login
./git-review main feature/login    # Put options before positional refs
./git-review --context 8
./git-review --stat                # Noninteractive output for scripts/pipes
```

Install the binary on your `PATH` to use it as a Git subcommand:

```sh
just install                      # Defaults to ~/.local/bin/git-review
# Ensure ~/.local/bin is on PATH
git review
git review --base origin/main

# Or choose a destination
just install /your/bin
```

The output is a single binary. It needs no Go runtime, Node, browser, or background service; system Git is required at runtime. The default base is the first available ref in this order: `main`, `origin/main`, `master`, `origin/master`. The tool does not fetch; remote-tracking refs reflect your local repository.

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `Tab` | Switch focus between the file list and diff |
| `j` / `k`, `↑` / `↓` | Scroll the diff, or select files when the file list has focus |
| `n` / `p` | Next / previous file |
| `Space` / `Enter` | Fold / unfold the selected file without changing viewed status |
| `v` | Toggle viewed; marking viewed folds the file and advances to the next unviewed file |
| `C` / `E` | Collapse / expand all files matching the current filter |
| `/` | Filter paths, case insensitive, with Unicode support |
| `Enter` / `Esc` | Apply / clear the filter |
| `Ctrl+D` / `Ctrl+U`, `PgDn` / `PgUp` | Scroll down / up half a page |
| `g` / `G`, `Home` / `End` | Top / bottom |
| `[` / `]` | Previous / next hunk |
| `h` / `l`, `←` / `→` | Scroll code horizontally; `0` resets |
| `r` | Reload branches and diff |
| `?` | Open help; `j` / `k` scroll help on small terminals |
| `q` / `Ctrl+C` | Quit; in help, `q` closes help first |

Mouse-wheel scrolling is supported. Below 90 columns the sidebar is hidden; `n` / `p` and `Tab` navigation remain available. Minimum terminal size: 45 × 12. A dark terminal with at least 100 columns is recommended.

## Comparison and progress semantics

- Compares `merge-base(base, head)` to `head`, matching `git diff base...head`. Commits added only to the base branch do not appear as deletions in the reviewed branch.
- **Only committed changes are included.** Working-tree changes, staged changes, and untracked files are excluded. Source files, the index, branches, and commits remain untouched.
- Refs are resolved to commit IDs before metadata, statistics, and patches are read, keeping each loaded comparison consistent if a branch moves during loading.
- Viewed progress is saved under the current worktree's Git directory at `git-review/<snapshot>.json`, keyed by merge-base and head commit. Reopening the same comparison restores progress; changing either snapshot endpoint starts a fresh review so new changes are not marked viewed accidentally.
- Ordinary folding lasts for the session. `--no-state` disables progress-file reads and writes, including for read-only repositories. Save failures appear in the status line and do not prevent browsing.
- Supports additions, deletions, modifications, renames, mode changes, symlinks, submodule pointers, and binary files. Binary files display `binary` and do not contribute to text-line totals.
- NUL-delimited Git metadata preserves filenames containing spaces, tabs, newlines, and Unicode. Terminal control characters are escaped for display. External diff and textconv helpers are disabled.

## Development and sample repository

```sh
just                 # List tasks
just deps            # Download and reconcile Go dependencies
just fmt             # go fmt
just lint            # Formatting check and go vet
just test            # Unit tests and real-Git integration tests
just test-race       # Race detector; requires a local C toolchain
just check           # lint + test + build
just fixture         # Create ignored .review-fixture/
just demo            # Build and open the sample review
just run --base main
```

The fixture includes Go, TypeScript, JSON, and Markdown changes, deletions, a rename, a binary, Unicode filenames, and a missing final newline. `just fixture` preserves an existing fixture directory. Automated tests create independent temporary repositories and do not modify this project's history.

```text
cmd/                  CLI entry point and injected version
internal/cli/         Arguments, terminal detection, and --stat
internal/gitdiff/     Git commands, NUL metadata, and unified patch parsing
internal/review/      Atomic local progress storage
internal/tui/         Bubble Tea model, layout, and Chroma highlighting
scripts/              Sample repository generator
justfile              Build and development tasks
```

## MVP scope

This MVP provides local review with a single-column unified diff. GitHub authentication, remote PR fetching, comments, submitting reviews, editing files, and side-by-side diffs are outside its scope.

`NO_COLOR` or `--no-color` disables colors and syntax highlighting. Highlighting uses each hunk's available context, so multiline strings or comments crossing omitted lines may not retain complete lexer state. Hunks larger than 256 KiB use plain code rendering while remaining fully browsable. A Git command's output is limited to 64 MiB; larger comparisons return an explicit error and can be narrowed by choosing a smaller commit range.
