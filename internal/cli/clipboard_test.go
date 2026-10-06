package cli

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

var _ term.File = (*terminalOutput)(nil)

func TestTerminalClipboardEscapesAndConcurrentRendering(t *testing.T) {
	// SSH must use the client's terminal, even if the server has GUI helpers.
	t.Setenv("SSH_CONNECTION", "test")
	t.Setenv("TMUX", "")
	var buffer bytes.Buffer
	output := &terminalOutput{Writer: &buffer}
	text := "hello\t中文\n\x1b]52;c;unsafe\a"
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = output.Write([]byte("frame\n")) }()
		go func() {
			defer wg.Done()
			if err := output.copySelection(text); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
	if strings.Count(buffer.String(), seq) != 20 || strings.Count(buffer.String(), "frame\n") != 20 {
		t.Fatal("clipboard writes were interleaved with renderer frames")
	}
	if strings.Contains(buffer.String(), "unsafe") {
		t.Fatal("raw control characters reached the terminal")
	}
	t.Setenv("TMUX", "test")
	buffer.Reset()
	if err := output.copySelection("hello"); err != nil {
		t.Fatal(err)
	}
	if buffer.String() != ansi.TmuxPassthrough(ansi.SetSystemClipboard("hello")) {
		t.Fatal("tmux passthrough missing")
	}
}

type failingClipboardWriter struct{}

func (failingClipboardWriter) Write([]byte) (int, error) { return 0, errors.New("closed terminal") }

type shortClipboardWriter struct{}

func (shortClipboardWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestTerminalClipboardErrorsAndFileDescriptor(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "test")
	for _, writer := range []io.Writer{failingClipboardWriter{}, shortClipboardWriter{}} {
		if err := (&terminalOutput{Writer: writer}).copySelection("hello"); err == nil {
			t.Fatal("write error was lost")
		}
	}
	f, err := os.CreateTemp(t.TempDir(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if (&terminalOutput{Writer: f}).Fd() != f.Fd() {
		t.Fatal("terminal descriptor was not preserved")
	}
}

func TestLocalClipboardHelperAndFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("WAYLAND_DISPLAY", "test")
	t.Setenv("PATH", dir)
	name := "wl-copy"
	if runtime.GOOS == "darwin" {
		name = "pbcopy"
	}
	dest := filepath.Join(dir, "copied")
	t.Setenv("CLIPBOARD_TEST_FILE", dest)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n/bin/cat > \"$CLIPBOARD_TEST_FILE\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var terminal bytes.Buffer
	out := &terminalOutput{Writer: &terminal}
	text := "$(echo not-a-command)\t中文\n"
	if err := out.copySelection(text); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(dest)
	if err != nil || string(copied) != text || terminal.Len() != 0 {
		t.Fatalf("helper copied %q: %v", copied, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := out.copySelection(text); err == nil {
		t.Fatal("native clipboard failure was hidden")
	}
}
