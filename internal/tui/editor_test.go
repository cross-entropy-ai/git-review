package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEditorSettingsAndLiteralFilePath(t *testing.T) {
	for _, setting := range []string{"GIT_EDITOR", "core.editor", "VISUAL", "EDITOR"} {
		t.Run(setting, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("TERM", "xterm")
			for _, key := range []string{"GIT_EDITOR", "VISUAL", "EDITOR"} {
				t.Setenv(key, "")
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = root
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
			}
			git("init", "-q")
			editor := `printf '%s\n' 'chosen editor'`
			switch setting {
			case "GIT_EDITOR":
				git("config", "core.editor", "false")
				t.Setenv("GIT_EDITOR", editor)
				t.Setenv("VISUAL", "false")
				t.Setenv("EDITOR", "false")
			case "core.editor":
				git("config", "core.editor", editor)
				t.Setenv("VISUAL", "false")
				t.Setenv("EDITOR", "false")
			case "VISUAL":
				t.Setenv("VISUAL", editor)
				t.Setenv("EDITOR", "false")
			case "EDITOR":
				t.Setenv("EDITOR", editor)
			}
			path := "-中文 ' $(touch injected); `touch injected`.txt"
			filename := filepath.Join(root, path)
			if err := os.WriteFile(filename, []byte("content"), 0600); err != nil {
				t.Fatal(err)
			}
			cmd, err := editorCommand(context.Background(), root, path)
			if err != nil {
				t.Fatal(err)
			}
			out, err := cmd.Output()
			if err != nil || string(out) != "chosen editor\n"+filename+"\n" {
				t.Fatalf("editor arguments: %q, %v", out, err)
			}
			if _, err := os.Stat(filepath.Join(root, "injected")); !os.IsNotExist(err) {
				t.Fatal("file path was interpreted as shell code")
			}
			// Exported reports live outside the checkout but use the same editor.
			report := filepath.Join(t.TempDir(), path)
			if err := os.WriteFile(report, []byte("report"), 0600); err != nil {
				t.Fatal(err)
			}
			cmd, err = fileEditorCommand(context.Background(), root, report)
			if err != nil {
				t.Fatal(err)
			}
			out, err = cmd.Output()
			if err != nil || string(out) != "chosen editor\n"+report+"\n" {
				t.Fatalf("export editor arguments: %q, %v", out, err)
			}
		})
	}
}

func TestOpenEditorSelectionAndInputModes(t *testing.T) {
	t.Setenv("GIT_EDITOR", "true")
	m := sampleModel(false)
	t.Cleanup(m.Close)
	m.comparison.Root = t.TempDir()
	// Only the second file exists: opening the first selection would fail.
	filename := filepath.Join(m.comparison.Root, m.comparison.Files[1].Path)
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, nil, 0600); err != nil {
		t.Fatal(err)
	}
	press(m, "n")
	if cmd := m.activate("e"); cmd == nil {
		t.Fatalf("selected file did not open: %s", m.message)
	}
	press(m, "t")
	press(m, "tab")
	if cmd := m.activate("e"); cmd == nil {
		t.Fatalf("tree file did not open: %s", m.message)
	}
	press(m, "left")
	if cmd := m.activate("e"); cmd != nil || !strings.Contains(m.message, "Select a file") {
		t.Fatal("directory selection opened a file")
	}
	press(m, "f")
	if cmd := m.activate("e"); cmd != nil || m.fileQuery != "e" {
		t.Fatal("picker input triggered the editor")
	}
	press(m, "missing")
	press(m, "enter")
	if cmd := m.activate("e"); cmd != nil {
		t.Fatal("empty file list triggered the editor")
	}
	press(m, "esc")
	press(m, "?")
	if cmd := m.activate("e"); cmd != nil {
		t.Fatal("help triggered the editor")
	}
	press(m, "?")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}, Paste: true}); cmd != nil {
		t.Fatal("paste triggered the editor")
	}
}

func TestEditorUnavailableFilesAndCompletion(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct{ root, path string }{
		{"", "file.txt"}, {root, "missing.txt"}, {root, "."},
		{root, "../outside.txt"}, {root, filepath.Join(root, "absolute.txt")},
	} {
		if _, err := editorCommand(context.Background(), test.root, test.path); err == nil {
			t.Errorf("accepted unavailable file: %+v", test)
		}
	}
	m := sampleModel(false)
	t.Cleanup(m.Close)
	if cmd := m.activate("e"); cmd != nil || !strings.Contains(m.message, "no local working tree") {
		t.Fatal("comparison without a working tree started an editor")
	}
	m.Update(editorFinishedMsg{err: errors.New("failed\x1b[2J")})
	if !strings.Contains(m.message, "Editor failed") || strings.Contains(m.message, "\x1b") {
		t.Fatalf("unsafe or missing error message: %q", m.message)
	}
	m.Update(editorFinishedMsg{})
	if !strings.Contains(m.message, "press r") {
		t.Fatalf("missing refresh hint: %q", m.message)
	}
}
