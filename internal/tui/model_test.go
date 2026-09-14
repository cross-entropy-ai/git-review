package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func localModel(c *gitdiff.Comparison, opts gitdiff.Options, color, persist bool, theme Theme) *Model {
	source := backend.NewLocal(opts, persist)
	return New(source.Restore(c), source, color, theme)
}

func sampleModel(color bool) *Model {
	c := &gitdiff.Comparison{Base: "main", Head: "feature", MergeBase: "0123456789", Added: 2, Deleted: 1}
	for _, path := range []string{"main.go", "docs/中文.md", "last.txt"} {
		f := gitdiff.File{Path: path, Status: "M", Added: 1, Deleted: 1}
		f.Hunks = []gitdiff.Hunk{{Header: "@@ -1,20 +1,20 @@"}}
		for i := 1; i <= 20; i++ {
			f.Hunks[0].Lines = append(f.Hunks[0].Lines, gitdiff.Line{Kind: ' ', Text: "package main // content", Old: i, New: i})
		}
		c.Files = append(c.Files, f)
	}
	return localModel(c, gitdiff.Options{}, color, false, DarkTheme)
}

func press(m *Model, key string) {
	switch key {
	case "tab":
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
	case "esc":
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	case "enter":
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "left":
		m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	case "right":
		m.Update(tea.KeyMsg{Type: tea.KeyRight})
	case "up":
		m.Update(tea.KeyMsg{Type: tea.KeyUp})
	case "down":
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	case " ":
		m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	default:
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
}

func TestFoldViewedAndNavigation(t *testing.T) {
	m := sampleModel(false)
	expanded := len(m.rows)
	press(m, " ")
	if !m.collapsed["main.go"] || len(m.rows) >= expanded {
		t.Fatal("space did not collapse the current file")
	}
	press(m, "enter")
	if m.collapsed["main.go"] || len(m.rows) != expanded {
		t.Fatal("enter did not expand the current file")
	}
	press(m, "v")
	if !m.viewed["main.go"] || !m.collapsed["main.go"] || m.selected != 1 {
		t.Fatal("viewed did not fold and advance")
	}
	press(m, "p")
	press(m, "v")
	if m.viewed["main.go"] || m.collapsed["main.go"] {
		t.Fatal("unviewed did not expand")
	}
	press(m, "z")
	if len(m.rows) != 6 {
		t.Fatalf("collapse all left %d rows", len(m.rows))
	}
	press(m, "z")
	if len(m.rows) != expanded {
		t.Fatal("expand all failed")
	}
	press(m, "tab")
	press(m, "j")
	if m.selected != 1 {
		t.Fatal("file-focus navigation failed")
	}
	press(m, "tab")
	offset := m.offset
	press(m, "j")
	if m.offset != offset+1 {
		t.Fatal("diff-focus scrolling failed")
	}
}

func TestPickerUnicodeEmptyAndCancel(t *testing.T) {
	m := sampleModel(false)
	press(m, "f中文")
	if len(m.fileMatches) != 1 || m.fileMatches[0] != 1 || m.selected != 0 {
		t.Fatalf("Unicode picker failed: %v", m.fileMatches)
	}
	press(m, "enter")
	if m.picking || m.selected != 1 || len(m.visible) != 3 {
		t.Fatal("file picker did not select without filtering the review")
	}
	press(m, "fmissing")
	if !strings.Contains(m.View(), "No files match") {
		t.Fatal("empty picker message missing")
	}
	press(m, "enter")
	if !m.picking || m.selected != 1 {
		t.Fatal("empty picker changed selection")
	}
	press(m, "esc")
	if m.picking || len(m.visible) != 3 || m.selected != 1 {
		t.Fatal("cancel changed the review")
	}
}

func TestViewDimensionsColorAndTerminalSafety(t *testing.T) {
	for _, theme := range []Theme{DarkTheme, LightTheme} {
		for _, color := range []bool{false, true} {
			m := sampleModel(color)
			m.palette = paletteFor(theme)
			m.comparison.Files[0].Path = "bad\x1b[2J\nfile.go"
			m.comparison.Files[0].Hunks[0].Lines[0].Text = "var secret = \"hello\"\x1b]52;c;evil\a"
			for _, size := range [][2]int{{120, 35}, {90, 24}, {60, 18}, {45, 12}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for _, help := range []bool{false, true} {
					m.help = help
					view := m.View()
					lines := strings.Split(view, "\n")
					if len(lines) != size[1] {
						t.Errorf("size %v help %v: got %d lines", size, help, len(lines))
					}
					for i, line := range lines {
						if width := ansi.StringWidth(line); width != size[0] {
							t.Errorf("size %v row %d has width %d", size, i, width)
						}
					}
					if strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\x1b]52") {
						t.Fatal("repository content injected terminal controls")
					}
					if !color && strings.Contains(view, "\x1b") {
						t.Fatal("no-color output has ANSI escapes")
					}
				}
			}
		}
	}
}

func TestHighlightUsesSyntaxColors(t *testing.T) {
	f := gitdiff.File{Path: "main.go", Hunks: []gitdiff.Hunk{{Lines: []gitdiff.Line{
		{Kind: '-', Text: "var message = \"old\""},
		{Kind: '+', Text: "var message = \"new\""},
	}}}}
	var previous string
	for _, style := range []string{"github-dark", "github"} {
		colored := highlightFile(f, true, style)
		if !strings.Contains(colored[0][1], "\x1b[38;2;") {
			t.Fatal("Go code is not syntax highlighted")
		}
		if ansi.Strip(colored[0][1]) != f.Hunks[0].Lines[1].Text {
			t.Fatal("highlighting changed source text")
		}
		if colored[0][1] == previous {
			t.Fatal("light and dark themes use identical syntax colors")
		}
		previous = colored[0][1]
		plain := highlightFile(f, false, style)
		if plain[0][0] != f.Hunks[0].Lines[0].Text {
			t.Fatal("no-color output changed text")
		}
	}
}

func TestRefreshAndHelp(t *testing.T) {
	m := sampleModel(false)
	press(m, "v")
	m.Update(loadedMsg{snapshot: m.snapshot})
	if !m.viewed["main.go"] {
		t.Fatal("same-snapshot refresh lost in-memory progress")
	}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 12})
	press(m, "?")
	for i := 0; i < len(m.helpContent()); i++ {
		press(m, "j")
	}
	if !strings.Contains(m.View(), "excluded.") {
		t.Fatal("help cannot scroll to the end")
	}
	press(m, "q")
	if m.help {
		t.Fatal("q did not close help")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q did not quit")
	}
}

func TestArrowNavigationWhenDiffFits(t *testing.T) {
	for _, folded := range []bool{false, true} {
		m := sampleModel(false)
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 150})
		if folded {
			press(m, "z")
		}
		for _, want := range []int{1, 2, 2} {
			press(m, "down")
			if m.selected != want || m.offset != 0 {
				t.Fatalf("folded=%v: down selected=%d offset=%d, want %d/0", folded, m.selected, m.offset, want)
			}
		}
		for _, want := range []int{1, 0, 0} {
			press(m, "up")
			if m.selected != want || m.offset != 0 {
				t.Fatalf("folded=%v: up selected=%d offset=%d, want %d/0", folded, m.selected, m.offset, want)
			}
		}
		press(m, "j")
		press(m, " ")
		if m.selected != 1 || m.collapsed["docs/中文.md"] == folded {
			t.Fatal("fold acted on a different file after arrow navigation")
		}
	}
}

func TestScrollBoundaryReachesFilesBelowViewportTop(t *testing.T) {
	m := sampleModel(false)
	m.collapsed["docs/中文.md"], m.collapsed["last.txt"] = true, true
	m.rebuild()
	for i := 0; i < len(m.rows)+3; i++ {
		press(m, "down")
	}
	if m.selected != 2 || m.offset != len(m.rows)-m.bodyHeight() {
		t.Fatal("scrolling to the bottom did not reach the last folded file")
	}
	press(m, " ")
	if m.collapsed["last.txt"] {
		t.Fatal("last file could not be unfolded")
	}
}

func TestScrollDirectionAfterSelectingVisibleFile(t *testing.T) {
	m := sampleModel(false)
	press(m, "z")
	m.collapsed["last.txt"] = false
	m.rebuild()
	// All headers are visible, but the last file extends below the viewport.
	m.selected = 2
	press(m, "up")
	if m.selected != 1 || m.offset != 0 {
		t.Fatal("up at the top moved the viewport downward")
	}
	press(m, "down")
	if m.selected != 1 || m.offset != 1 {
		t.Fatal("downward scrolling moved selection to an earlier file")
	}
}

func TestBatchedKeystrokesAndBracketedPaste(t *testing.T) {
	m := sampleModel(false)
	press(m, "fdocs")
	if !m.picking || m.fileQuery != "docs" || len(m.fileMatches) != 1 {
		t.Fatalf("batched picker input was lost: query=%q matches=%v", m.fileQuery, m.fileMatches)
	}
	press(m, "esc")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("vq"), Paste: true})
	if cmd != nil || m.viewedCount() != 0 {
		t.Fatal("pasted text executed review shortcuts")
	}
	press(m, "f")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("docs/中文"), Paste: true})
	if m.fileQuery != "docs/中文" || len(m.fileMatches) != 1 {
		t.Fatal("bracketed paste did not populate the picker")
	}
}

func TestWorkingTreeRefreshInvalidatesViewedSnapshot(t *testing.T) {
	for _, persist := range []bool{false, true} {
		m := sampleModel(false)
		source := m.source.(*backend.Local)
		source.Persist = persist
		first := *m.comparison
		first.WorkingTree, first.GitDir = true, t.TempDir()
		first.Base, first.Head = "HEAD", "working tree"
		first.MergeBase, first.HeadOID = "head-commit", "first-content-tree"
		m.install(source.Restore(&first))
		if cmd := m.activate("v"); cmd != nil {
			m.Update(cmd())
		}
		m.Update(loadedMsg{snapshot: source.Restore(&first)})
		if !m.viewed["main.go"] {
			t.Fatal("refreshing unchanged working-tree content lost progress")
		}
		second := first
		second.HeadOID = "edited-content-tree"
		m.Update(loadedMsg{snapshot: source.Restore(&second)})
		if m.viewedCount() != 0 || m.collapsed["main.go"] {
			t.Fatal("changed working-tree snapshot retained stale viewed progress")
		}
		if persist {
			m.Update(loadedMsg{snapshot: source.Restore(&first)})
			if !m.viewed["main.go"] {
				t.Fatal("reopening the exact working-tree snapshot did not restore progress")
			}
		}
	}
}
