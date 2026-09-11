package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/imwithye/git-review/internal/gitdiff"
)

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
	return New(c, gitdiff.Options{}, color, false, DarkTheme)
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
	press(m, "C")
	if len(m.rows) != 6 {
		t.Fatalf("collapse all left %d rows", len(m.rows))
	}
	press(m, "E")
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

func TestFilterUnicodeEmptyAndClear(t *testing.T) {
	m := sampleModel(false)
	press(m, "/")
	press(m, "中文")
	if len(m.visible) != 1 || m.selected != 1 {
		t.Fatalf("Unicode filter failed: %v", m.visible)
	}
	press(m, "enter")
	press(m, "C")
	if m.collapsed["main.go"] {
		t.Fatal("filtered collapse changed a hidden file")
	}
	press(m, "esc")
	if len(m.visible) != 3 {
		t.Fatal("filter did not clear")
	}
	press(m, "/")
	press(m, "missing")
	press(m, "enter")
	for _, key := range []string{"v", " ", "n", "p", "j", "C", "E", "]", "["} {
		press(m, key)
	}
	if !strings.Contains(m.View(), "No files match") {
		t.Fatal("empty filter message missing")
	}
	press(m, "esc")
	if len(m.visible) != 3 || m.offset < 0 {
		t.Fatal("failed to recover from empty filter")
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
	m.Update(loadedMsg{comparison: m.comparison})
	if !m.viewed["main.go"] {
		t.Fatal("same-snapshot refresh lost in-memory progress")
	}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 12})
	press(m, "?")
	for i := 0; i < len(m.helpLines()); i++ {
		press(m, "j")
	}
	if !strings.Contains(m.View(), "Working tree") {
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

func TestScrollingAtBoundaryKeepsFileSelection(t *testing.T) {
	m := sampleModel(false)
	press(m, "C")
	press(m, "n")
	selected := m.selected
	press(m, "j")
	if m.selected != selected {
		t.Fatal("a scroll that cannot move reset the selected file")
	}
}

func TestBatchedKeystrokesAndBracketedPaste(t *testing.T) {
	m := sampleModel(false)
	press(m, "/docs")
	if !m.filtering || m.filter != "docs" || len(m.visible) != 1 {
		t.Fatalf("batched filter input was lost: filter=%q visible=%v", m.filter, m.visible)
	}
	press(m, "esc")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("vq"), Paste: true})
	if cmd != nil || m.viewedCount() != 0 {
		t.Fatal("pasted text executed review shortcuts")
	}
	press(m, "/")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("docs/中文"), Paste: true})
	if m.filter != "docs/中文" || len(m.visible) != 1 {
		t.Fatal("bracketed paste did not populate the filter")
	}
}
