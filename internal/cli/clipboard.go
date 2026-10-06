package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Clipboard escapes and renderer frames must be written atomically to the same
// terminal. Preserve Fd so Bubble Tea can still detect and resize the terminal.
type terminalOutput struct {
	io.Writer
	mu sync.Mutex
}

func (o *terminalOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.Writer.Write(p)
}

// Bubble Tea's terminal detection requires the full ReadWriteCloser interface,
// even for its output stream.
func (o *terminalOutput) Read(p []byte) (int, error) {
	if r, ok := o.Writer.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, io.ErrClosedPipe
}

func (o *terminalOutput) Close() error {
	if c, ok := o.Writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (o *terminalOutput) Fd() uintptr {
	if f, ok := o.Writer.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}

func (o *terminalOutput) copySelection(text string) error {
	// On SSH, copy through the terminal to the user's clipboard rather than
	// changing a clipboard on the remote machine.
	if os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_TTY") == "" {
		var commands [][]string
		if runtime.GOOS == "darwin" {
			commands = append(commands, []string{"pbcopy"})
		}
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			commands = append(commands, []string{"wl-copy"})
		}
		if os.Getenv("DISPLAY") != "" {
			commands = append(commands, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
		}
		for _, args := range commands {
			path, err := exec.LookPath(args[0])
			if err != nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, path, args[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("%s: %w", args[0], err)
			}
			return nil
		}
	}
	seq := ansi.SetSystemClipboard(text)
	if os.Getenv("TMUX") != "" {
		seq = ansi.TmuxPassthrough(seq)
	}
	n, err := o.Write([]byte(seq))
	if err == nil && n != len(seq) {
		return io.ErrShortWrite
	}
	return err
}
