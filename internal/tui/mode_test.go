package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func TestModeControlVisibleAndClickable(t *testing.T) {
	for _, width := range []int{45, 60, 70, 90, 120} {
		for _, test := range []struct {
			mode  gitdiff.Mode
			local bool
			label string
		}{
			{gitdiff.ModeAuto, false, "Committed"},
			{gitdiff.ModeAuto, true, "Working tree"},
			{gitdiff.ModeWorkingTree, true, "Working tree"},
			{gitdiff.ModeCommitted, false, "Committed"},
		} {
			m := sampleModel(true)
			m.options.Mode, m.comparison.WorkingTree = test.mode, test.local
			m = New(m.comparison, m.options, true, false, DarkTheme)
			m.width = width
			resolved := gitdiff.ModeCommitted
			if test.local {
				resolved = gitdiff.ModeWorkingTree
			}
			if m.options.Mode != resolved || strings.Contains(m.View(), "Auto") {
				t.Fatal("startup did not resolve auto to an actual review mode")
			}
			m.comparison.Root = "/a/very-long-repository-name-that-must-not-hide-the-mode"
			lines := strings.Split(m.View(), "\n")
			header := ansi.Strip(lines[0])
			needle := "m Mode: " + test.label
			index := strings.Index(header, needle)
			if index < 0 || !strings.Contains(header, "git review") {
				t.Fatalf("width %d: mode or title is hidden: %s", width, header)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != width {
					t.Fatalf("width %d: overflowing row: %q", width, line)
				}
			}
			_, cmd := m.Update(tea.MouseMsg{X: ansi.StringWidth(header[:index]) + 3, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if cmd == nil || !m.loading {
				t.Fatal("clicking the visible mode control did not start loading")
			}
			if m.options.Mode != resolved {
				t.Fatal("mode changed before the comparison finished loading")
			}
			if _, again := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")}); again != nil {
				t.Fatal("repeated toggle started an overlapping load")
			}
		}
	}
}

func TestModeSwitchLoadsDiffAndPreservesRefs(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", os.DevNull)
	git("commit", "--allow-empty", "-qm", "base")
	git("tag", "base-tag")
	git("switch", "-qc", "feature")
	write("committed.txt")
	git("add", ".")
	git("commit", "-qm", "feature")
	head := git("rev-parse", "HEAD")
	git("switch", "-q", "main")
	write("local.txt")
	opts := gitdiff.Options{Dir: dir, Base: "base-tag", Head: head, Mode: gitdiff.ModeAuto, Context: 3}
	c, err := gitdiff.Load(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	m := New(c, opts, false, false, DarkTheme)
	load := func(key string) {
		t.Helper()
		cmd := m.activate(key)
		if cmd == nil {
			t.Fatalf("%s did not start a load", key)
		}
		m.Update(cmd())
		if m.loading || strings.Contains(m.message, "failed") {
			t.Fatalf("load failed: %s", m.message)
		}
		if m.options.Base != "base-tag" || m.options.Head != head {
			t.Fatalf("switch lost explicit refs: %+v", m.options)
		}
	}
	check := func(mode gitdiff.Mode, path string) {
		t.Helper()
		if m.options.Mode != mode || len(m.comparison.Files) != 1 || m.comparison.Files[0].Path != path {
			t.Fatalf("want %s / %s, got %s / %+v", mode, path, m.options.Mode, m.comparison.Files)
		}
	}
	check(gitdiff.ModeWorkingTree, "local.txt")
	if err := os.Remove(filepath.Join(dir, "local.txt")); err != nil {
		t.Fatal(err)
	}
	load("r")
	if m.options.Mode != gitdiff.ModeWorkingTree || !m.comparison.WorkingTree || len(m.comparison.Files) != 0 {
		t.Fatal("refresh changed scope after local changes disappeared")
	}

	// A fresh auto session on a clean checkout starts and stays committed.
	c, err = gitdiff.Load(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	m = New(c, opts, false, false, DarkTheme)
	check(gitdiff.ModeCommitted, "committed.txt")
	write("local.txt")
	load("r")
	check(gitdiff.ModeCommitted, "committed.txt")
	load("m")
	check(gitdiff.ModeWorkingTree, "local.txt")
	load("r")
	check(gitdiff.ModeWorkingTree, "local.txt")
	load("m")
	check(gitdiff.ModeCommitted, "committed.txt")

	// A failed switch must leave the current diff and mode consistent.
	load("m")
	m.options.Base = "missing-ref"
	previous := m.comparison
	cmd := m.activate("m")
	m.Update(cmd())
	if m.options.Mode != gitdiff.ModeWorkingTree || m.comparison != previous || m.loading || !strings.Contains(m.message, "failed") {
		t.Fatalf("failed switch corrupted the current review: %+v, %s", m.options, m.message)
	}
}
