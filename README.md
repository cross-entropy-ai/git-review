# git review

A local, pull-request-style review TUI written in Go. Browse all committed changes on a branch with a file sidebar, total and per-file additions/deletions, line numbers, syntax highlighting, folding, and saved viewed progress. Use the keyboard or mouse in an interface with bordered panes, review progress, and clickable controls.

The selected file is outlined in blue in the diff pane, with a bold blue file name. File headers have a contrasting neutral background; the selected sidebar entry and diff header use a blue background. Code and other areas preserve your terminal background, using colored line numbers and signs for additions and deletions while retaining syntax highlighting. The outline follows keyboard and mouse selection and remains visible when the file is folded or its header has scrolled out of view.

```text
  git review  /  .review-fixture                                                          r Refresh   ? Help
  main → feature/health-check  │  8 files  +26 −8                                       ━━━━━━━━━━ 0/8 viewed
  / server  (1 matches)                                                                C Collapse   E Expand
╭─ FILES · 1 ──────────────────────╮ ╭─ DIFF · internal/server/server.go ────────────────────────────────────╮
│›▾ [ ] server.go                  │ │ ▾ internal/server/server.go                      M +14 −3  [ ] Viewed ┃
│       internal/server     +14 −3 │ │ @@ -1,11 +1,22 @@                                                     ┃
│                                  │ │   1    1   │ package server                                           ┃
│                                  │ │   2    2   │                                                          ┃
│                                  │ │   3    3   │ import (                                                 │
│                                  │ │   4      - │     "fmt"                                                │
│                                  │ │        4 + │     "encoding/json"                                      │
│                                  │ │   5    5   │     "net/http"                                           │
│                                  │ │        6 + │     "time"                                               │
│                                  │ │   6    7   │ )                                                        │
│                                  │ │   7    8   │                                                          │
╰──────────────────────────────────╯ ╰───────────────────────────────────────────────────────────────────────╯
  internal/server/server.go                                                                               39%
  Tab Focus   Space Fold   v Viewed   / Filter   t Tree                                      ? Help   q Quit
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
./git-review --theme light         # Override automatic terminal theme detection
./git-review --theme dark
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
| `t` | Toggle the flat file list and directory tree |
| `j` / `k`, `↑` / `↓` | Scroll the diff and follow file selection; at a scroll boundary, select the previous / next file. With sidebar focus, navigate its entries |
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

## Mouse controls

Mouse support is enabled by default in terminals that support mouse reporting.

| Action | Behavior |
| --- | --- |
| Click a file in the sidebar | Select it and jump to its diff |
| Click a sidebar fold arrow | Fold / unfold that file |
| Click a sidebar checkbox | Toggle viewed and fold / unfold, keeping the file selected |
| Click a file header in the diff | Fold / unfold that file |
| Click a file header's Viewed box | Toggle viewed without jumping to another file |
| Scroll over the sidebar | Browse the file list independently of the diff |
| Scroll over the diff | Scroll vertically; at a scroll boundary, select the previous / next file |
| Shift + wheel or horizontal wheel | Scroll code horizontally |
| Click or drag a scrollbar | Jump through that pane |
| Click the filter field or toolbar | Filter, collapse, expand, refresh, or open help |
| Click a footer shortcut | Run the displayed action |
| Click a directory in tree mode | Expand / collapse its children |

Use `--no-mouse` to disable mouse reporting and use native terminal text selection. Keyboard shortcuts remain available with or without mouse support. Below 90 columns the sidebar is hidden; `n` / `p` and `Tab` navigation remain available. Minimum terminal size: 45 × 12. A terminal with at least 100 columns is recommended.

## File list and tree

The default flat list preserves Git's diff output order, normally ordered by full path; Git ordering configuration such as `diff.orderFile` can affect it. The sidebar shows basenames with directories on the next line, so this is not a basename sort.

Press `t` (or click `t Tree` / `t List` in the footer) to switch views. The tree groups changed files by directory, sorting directories before files and names in case-sensitive lexical order at each level. Directory counts include all matching files below them. Renames appear under their destination; deleted files remain available even if they no longer exist on disk.

With tree focus (`Tab`), use `j` / `k` or `↑` / `↓` to navigate entries. `←` / `h` closes a directory or moves to its parent; `→` / `l` opens a directory or enters its first child, and expands a folded file. `Space` / `Enter` toggles the focused directory or file. Clicking a directory toggles it; file arrows and viewed checkboxes work at every indentation level. Mark files viewed individually with `v` or their checkbox.

Directory folding only affects the sidebar. Switching views preserves the current diff position, file folds, filter, and viewed progress; directory expansion is kept for the current session. `n` / `p` continue navigating files in diff order and reveal their ancestors automatically. Filtering shows matching paths and their ancestors with directories expanded; clear it with `Esc` to fold directories again.

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

`--theme auto` (the default) detects the terminal background at startup using [termenv](https://github.com/muesli/termenv), with `COLORFGBG` as a fallback. If detection is unavailable, the theme defaults to dark. Use `--theme light` or `--theme dark` to override detection, including in terminal multiplexers. Restart after changing the terminal background. Both palettes cover headers, selection, borders, line numbers, change statistics, and syntax highlighting; code rows keep the terminal's background.

`NO_COLOR` or `--no-color` disables colors and syntax highlighting, including header fills, and skips background detection. Highlighting uses each hunk's available context, so multiline strings or comments crossing omitted lines may not retain complete lexer state. Hunks larger than 256 KiB use plain code rendering while remaining fully browsable. A Git command's output is limited to 64 MiB; larger comparisons return an explicit error and can be narrowed by choosing a smaller commit range.
