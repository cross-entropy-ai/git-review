package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func searchModel() *Model {
	c := &gitdiff.Comparison{Files: []gitdiff.File{
		{Path: "a.go", Hunks: []gitdiff.Hunk{{Lines: []gitdiff.Line{
			{Kind: ' ', Text: "package main", Old: 1, New: 1},
			{Kind: '-', Text: "old NEEDLE needle", Old: 2},
			{Kind: '+', Text: "new needle", New: 2},
		}}}},
		{Path: "目录/b.go", Hunks: []gitdiff.Hunk{{Lines: []gitdiff.Line{
			{Kind: '+', Text: strings.Repeat("长", 50) + "\tneedle", New: 1},
		}}}},
	}}
	return localModel(c, gitdiff.Options{}, true, false, DarkTheme)
}

func TestSearchOccurrencesNavigationAndFoldedFiles(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := searchModel()
		t.Cleanup(m.Close)
		if split {
			press(m, "s")
		}
		press(m, "C")
		m.viewed["目录/b.go"] = true
		press(m, "/needle")
		if !m.searching || len(m.matches) != 4 || m.matchIndex != 0 || m.collapsed["a.go"] {
			t.Fatalf("live search failed: matches=%v index=%d", m.matches, m.matchIndex)
		}
		press(m, "enter")
		if m.searching || m.search != "needle" {
			t.Fatal("enter discarded the search")
		}
		for i := 1; i < 4; i++ {
			press(m, "n")
			if m.matchIndex != i {
				t.Fatal("next match skipped an occurrence")
			}
		}
		if m.selected != 1 || m.collapsed["目录/b.go"] || !m.viewed["目录/b.go"] || m.xOffset == 0 {
			t.Fatal("search did not reveal offscreen folded content while preserving viewed state")
		}
		if !strings.Contains(ansi.Strip(m.View()), "needle") {
			t.Fatal("current match is not visible after horizontal reveal")
		}
		press(m, "n")
		if m.matchIndex != 0 || m.selected != 0 || m.xOffset != 0 {
			t.Fatal("next match did not wrap and restore horizontal position")
		}
		press(m, "p")
		if m.matchIndex != 3 {
			t.Fatal("previous match did not wrap")
		}
		press(m, "s")
		press(m, "n")
		if m.selected != 0 || !m.currentMatch(0, 0, 1) {
			t.Fatal("layout toggle invalidated search locations")
		}
		press(m, "esc")
		if m.search != "" || len(m.matches) != 0 || len(m.visible) != 2 {
			t.Fatal("clearing search changed the file list")
		}
	}
}

func TestSearchUnicodeLiteralInputAndRefresh(t *testing.T) {
	spans := matchColumns("\t中Ä [x] ä", "ä")
	if len(spans) != 2 || spans[0] != [2]int{6, 7} || spans[1] != [2]int{12, 13} {
		t.Fatalf("wrong Unicode display columns: %v", spans)
	}
	if matches := matchColumns("a [x] b", "[x]"); len(matches) != 1 {
		t.Fatal("search interpreted a literal query as a regular expression")
	}
	m := searchModel()
	t.Cleanup(m.Close)
	press(m, "/")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("NEEDLE"), Paste: true})
	if len(m.matches) != 4 || m.splitMode || m.viewedCount() != 0 {
		t.Fatal("paste was interpreted as shortcuts or lost case-insensitive matches")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	press(m, "目录/b.go")
	if len(m.matches) != 0 || len(m.visible) != 2 || !strings.Contains(m.View(), "No matches") {
		t.Fatal("content search matched a filename or hid the review")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	press(m, "中文")
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.search != "中" {
		t.Fatal("backspace split a Unicode character")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	press(m, "needle")
	press(m, "enter")
	m.comparison.Files = nil
	m.install(m.snapshot)
	press(m, "n")
	if len(m.matches) != 0 || len(m.rows) != 0 {
		t.Fatal("refresh left stale search locations")
	}
}

func TestSearchHighlightPreservesContentAndNoColor(t *testing.T) {
	m := searchModel()
	t.Cleanup(m.Close)
	press(m, "/needle")
	code := "old NEEDLE needle"
	highlighted := m.highlightSearch(code, 0, 0, 1)
	if ansi.Strip(highlighted) != code || !strings.Contains(highlighted, colorCode(48, m.palette.accent)) {
		t.Fatal("active match was not highlighted or changed the code")
	}
	m.color = false
	if m.highlightSearch(code, 0, 0, 1) != code || strings.Contains(m.View(), "\x1b") {
		t.Fatal("no-color search emitted terminal styling")
	}
	if !strings.Contains(m.renderRow(row{kind: 'l', file: 0, hunk: 0, line: 1}, 80), "›") {
		t.Fatal("no-color search has no current-line marker")
	}
}

func TestSearchRevealsMatchAtFrameEdge(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := searchModel()
		t.Cleanup(m.Close)
		m.width = 90
		m.splitMode = split
		available := m.layout().diffWidth - 2*fileFrameInset - 14
		if split {
			available = (m.layout().diffWidth-2*fileFrameInset-1)/2 - 9
		}
		m.comparison.Files[0].Hunks[0].Lines[0].Text = strings.Repeat(" ", available-1) + "hit"
		m.rebuild()
		press(m, "/hit")
		if m.xOffset == 0 || !strings.Contains(ansi.Strip(m.View()), "hit") {
			t.Fatal("file frame clipped the current match")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) != m.width {
				t.Fatal("search highlighting changed the rendered width")
			}
		}
	}
}

func TestSearchNPInputAndFileNavigation(t *testing.T) {
	m := searchModel()
	t.Cleanup(m.Close)
	press(m, "/np")
	if m.search != "np" || !m.searching || m.selected != 0 {
		t.Fatal("n/p were not treated as text while entering the search")
	}
	press(m, "enter")
	press(m, "n")
	press(m, "p")
	if m.selected != 0 || len(m.matches) != 0 {
		t.Fatal("n/p navigated files while a search with no matches was active")
	}
	press(m, "esc")
	press(m, "n")
	if m.selected != 1 {
		t.Fatal("clearing search did not restore next-file navigation")
	}
	press(m, "p")
	if m.selected != 0 {
		t.Fatal("clearing search did not restore previous-file navigation")
	}
}

func TestRefreshRevealsActiveSearchMatch(t *testing.T) {
	m := searchModel()
	t.Cleanup(m.Close)
	press(m, "/needle")
	press(m, "enter")
	// A refreshed comparison contains only the long, horizontally hidden match.
	snapshot, comparison := *m.snapshot, *m.comparison
	comparison.Files = comparison.Files[1:]
	snapshot.Comparison = &comparison
	m.install(&snapshot)
	if len(m.matches) != 1 || m.matchIndex != 0 || m.xOffset == 0 || !strings.Contains(ansi.Strip(m.View()), "needle") {
		t.Fatal("refresh left the active match counter disconnected from the visible diff")
	}
}
