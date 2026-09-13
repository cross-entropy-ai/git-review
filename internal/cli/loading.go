package cli

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/cross-entropy-ai/git-review/internal/backend"
)

// startPRLoading owns one terminal line until stop is called. Stop waits for
// rendering to finish, so neither errors nor the TUI race the last frame.
func startPRLoading(out io.Writer, pr backend.PullRequest, color bool) func() {
	label := fmt.Sprintf("%s/%s #%d", pr.Owner, pr.Repo, pr.Number)
	if !isTerminal(out) || os.Getenv("TERM") == "dumb" {
		fmt.Fprintf(out, "  GitHub PR · %s · Loading review…\n", label)
		return func() {}
	}
	file := out.(*os.File)
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	draw := func(frame int) {
		spinner, title, hint := string(frames[frame%len(frames)]), label, "Loading review…"
		if color {
			spinner = "\x1b[36m" + spinner + "\x1b[0m"
			title = "\x1b[1m" + title + "\x1b[0m"
			hint = "\x1b[2m" + hint + "\x1b[0m"
		}
		line := "  " + spinner + "  " + title + " · " + hint
		width, _, err := term.GetSize(file.Fd())
		if err != nil || width < 1 {
			width = 80
		}
		// Leave the final column free to avoid wrapping and stale spinner lines.
		fmt.Fprint(out, "\r\x1b[2K", ansi.Truncate(line, width-1, "…"))
	}
	draw(0)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for frame := 1; ; frame++ {
			select {
			case <-stop:
				fmt.Fprint(out, "\r\x1b[2K")
				return
			case <-ticker.C:
				draw(frame)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}
