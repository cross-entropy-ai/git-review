package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
	"github.com/cross-entropy-ai/git-review/internal/tui"
	"github.com/muesli/termenv"
)

const usage = `Usage: git review [options] [base [head]]

Review committed changes from merge-base(base, head) to head in a TUI.
Default base: main, origin/main, master, then origin/master. Default head: HEAD.

Options:
  --base REF       Base branch, tag, or commit
  --head REF       Head branch, tag, or commit (default HEAD)
  --context N      Context lines per hunk, 0–100 (default 3)
  -C DIR           Run in this repository (default current directory)
  --stat           Print file and total statistics without opening the TUI
  --theme MODE     Color theme: auto, light, or dark (default auto)
  --no-color       Disable color and syntax highlighting (also NO_COLOR)
  --no-state       Keep viewed progress in memory only
  --no-mouse       Disable mouse reporting for native terminal text selection
  --version        Print version
  -h, --help       Show this help

Keys: Tab focus · t tree/list · j/k scroll · n/p file · Space fold · v viewed · ? help · q quit
Mouse: click files, fold arrows, viewed boxes, and toolbar; scroll or drag rails.
Only committed branch changes are included; the worktree and index are untouched.
`

func Run(args []string, stdout, stderr io.Writer, version string) int {
	flags := flag.NewFlagSet("git-review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts gitdiff.Options
	var stat, noColor, noState, noMouse, showVersion bool
	var theme string
	flags.StringVar(&opts.Base, "base", "", "base ref")
	flags.StringVar(&opts.Head, "head", "HEAD", "head ref")
	flags.StringVar(&opts.Dir, "C", ".", "repository directory")
	flags.IntVar(&opts.Context, "context", 3, "context lines")
	flags.BoolVar(&stat, "stat", false, "print statistics")
	flags.StringVar(&theme, "theme", "auto", "color theme: auto, light, or dark")
	flags.BoolVar(&noColor, "no-color", false, "disable colors")
	flags.BoolVar(&noState, "no-state", false, "disable progress persistence")
	flags.BoolVar(&noMouse, "no-mouse", false, "disable mouse reporting")
	flags.BoolVar(&showVersion, "version", false, "show version")
	flags.Usage = func() { fmt.Fprint(stdout, usage) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if showVersion {
		fmt.Fprintln(stdout, "git-review "+version)
		return 0
	}
	if flags.NArg() > 2 {
		fmt.Fprintln(stderr, "git-review: expected at most two refs; place options before positional refs")
		return 2
	}
	baseSet, headSet := false, false
	flags.Visit(func(f *flag.Flag) {
		baseSet = baseSet || f.Name == "base"
		headSet = headSet || f.Name == "head"
	})
	if flags.NArg() > 0 {
		if baseSet {
			fmt.Fprintln(stderr, "git-review: choose --base or a positional base, not both")
			return 2
		}
		opts.Base = flags.Arg(0)
	}
	if flags.NArg() > 1 {
		if headSet {
			fmt.Fprintln(stderr, "git-review: choose --head or a positional head, not both")
			return 2
		}
		opts.Head = flags.Arg(1)
	}
	if opts.Context < 0 || opts.Context > 100 {
		fmt.Fprintln(stderr, "git-review: --context must be between 0 and 100")
		return 2
	}
	if theme != "auto" && theme != "light" && theme != "dark" {
		fmt.Fprintln(stderr, "git-review: --theme must be auto, light, or dark")
		return 2
	}
	if !stat && (!term.IsTerminal(os.Stdin.Fd()) || !isTerminal(stdout)) {
		fmt.Fprintln(stderr, "git-review: the TUI needs an interactive terminal; use --stat for redirected output")
		return 1
	}
	c, err := gitdiff.Load(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(stderr, "git-review:", err)
		return 1
	}
	if stat {
		printStats(stdout, c)
		return 0
	}
	_, envNoColor := os.LookupEnv("NO_COLOR")
	color := !noColor && !envNoColor && os.Getenv("TERM") != "dumb"
	// Query before Bubble Tea starts reading terminal input. Explicit themes and
	// monochrome output do not need a terminal query.
	resolvedTheme := resolveTheme(theme, color, func() bool {
		return termenv.NewOutput(stdout).HasDarkBackground()
	})
	model := tui.New(c, opts, color, !noState, resolvedTheme)
	programOptions := []tea.ProgramOption{tea.WithOutput(stdout), tea.WithAltScreen()}
	if !noMouse {
		programOptions = append(programOptions, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(model, programOptions...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "git-review:", err)
		return 1
	}
	return 0
}

func resolveTheme(name string, color bool, hasDarkBackground func() bool) tui.Theme {
	if name == "light" {
		return tui.LightTheme
	}
	if name == "auto" && color && !hasDarkBackground() {
		return tui.LightTheme
	}
	return tui.DarkTheme
}

func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(file.Fd())
}

func displayPath(path string) string {
	// Quoting also neutralizes embedded control characters in non-TUI output.
	return strconv.QuoteToGraphic(path)
}

func printStats(out io.Writer, c *gitdiff.Comparison) {
	fmt.Fprintf(out, "%s...%s\n\n", displayPath(c.Base), displayPath(c.Head))
	for _, f := range c.Files {
		path := displayPath(f.Path)
		if f.OldPath != "" {
			path = displayPath(f.OldPath) + " -> " + path
		}
		stats := fmt.Sprintf("+%d -%d", f.Added, f.Deleted)
		if f.Binary {
			stats = "binary"
		}
		fmt.Fprintf(out, "%-4s %-12s %s\n", f.Status, stats, path)
	}
	fmt.Fprintf(out, "\n%d files changed, %d insertions(+), %d deletions(-)\n", len(c.Files), c.Added, c.Deleted)
	if len(c.Files) == 0 {
		fmt.Fprintln(out, "No committed changes to review.")
	}
}
