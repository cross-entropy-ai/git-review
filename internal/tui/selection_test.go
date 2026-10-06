package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

func selectionModel(t *testing.T, split, wrap bool, lines ...diff.Line) *Model {
	t.Helper()
	m := sampleModel(true)
	t.Cleanup(m.Close)
	m.comparison.Files = m.comparison.Files[:1]
	m.comparison.Files[0].Hunks = []diff.Hunk{{Header: "@@ -1 +1 @@", Lines: lines}}
	m.width, m.height = 120, 35
	m.splitMode, m.wrapLines = split, wrap
	m.rebuild()
	return m
}

func codePosition(t *testing.T, m *Model, needle string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(m.View(), "\n") {
		index := m.offset + y - contentTop
		if y < contentTop || y >= contentTop+m.bodyHeight() || index >= len(m.rows) || (m.rows[index].kind != 'l' && m.rows[index].kind != 'd') {
			continue
		}
		plain := ansi.Strip(line)
		if i := strings.Index(plain, needle); i >= 0 {
			return ansi.StringWidth(plain[:i]), y
		}
	}
	t.Fatalf("code %q not visible", needle)
	return 0, 0
}

func dragCode(m *Model, x1, y1, x2, y2 int) tea.Cmd {
	m.Update(tea.MouseMsg{X: x1, Y: y1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: x2, Y: y2, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	_, cmd := m.Update(tea.MouseMsg{X: x2, Y: y2, Action: tea.MouseActionRelease})
	return cmd
}

func runCopy(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("release did not dispatch a clipboard write")
	}
	m.Update(cmd())
}

func TestDragCodeCopiesOnlySelectedSource(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			m := selectionModel(t, false, false,
				diff.Line{Kind: '-', Text: "before alpha", Old: 1},
				diff.Line{Kind: '+', Text: "beta after", New: 1})
			var copied []string
			m.SetClipboard(func(text string) error { copied = append(copied, text); return nil })
			x1, y1 := codePosition(t, m, "alpha")
			x2, y2 := codePosition(t, m, "beta")
			x2 += 3
			if reverse {
				x1, y1, x2, y2 = x2, y2, x1, y1
			}
			m.Update(tea.MouseMsg{X: x1, Y: y1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			m.Update(tea.MouseMsg{X: x2, Y: y2, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
			if len(copied) != 0 || m.selectedText() != "alpha\nbeta" {
				t.Fatalf("drag copied early or selected wrong text: %q", m.selectedText())
			}
			plain := ansi.Strip(m.View())
			line := strings.Split(m.View(), "\n")[y1]
			if !strings.Contains(line, colorCode(48, m.palette.accent)) {
				t.Fatal("selection is not highlighted")
			}
			_, cmd := m.Update(tea.MouseMsg{X: x2, Y: y2, Action: tea.MouseActionRelease})
			runCopy(t, m, cmd)
			m.View()
			m.View()
			if len(copied) != 1 || copied[0] != "alpha\nbeta" || m.dragging != "" {
				t.Fatalf("copied=%q dragging=%q", copied, m.dragging)
			}
			if !strings.Contains(plain, "before alpha") || !strings.Contains(plain, "beta after") {
				t.Fatal("highlight changed the displayed source")
			}
			press(m, "esc")
			if m.textSelection != nil {
				t.Fatal("Escape did not clear the selection")
			}
		})
	}
}

func TestDragCodeSplitKeepsStartingSide(t *testing.T) {
	for _, newSide := range []bool{false, true} {
		m := selectionModel(t, true, false,
			diff.Line{Kind: '-', Text: "old alpha", Old: 1},
			diff.Line{Kind: '-', Text: "old beta", Old: 2},
			diff.Line{Kind: '+', Text: "new gamma", New: 1},
			diff.Line{Kind: '+', Text: "new delta", New: 2})
		start, end, want := "old alpha", "old beta", "old alpha\nold beta"
		if newSide {
			start, end, want = "new gamma", "new delta", "new gamma\nnew delta"
		}
		x1, y1 := codePosition(t, m, start)
		x2, y2 := codePosition(t, m, end)
		var copied string
		m.SetClipboard(func(text string) error { copied = text; return nil })
		runCopy(t, m, dragCode(m, x1, y1, x2+len(end)-1, y2))
		if copied != want {
			t.Fatalf("newSide=%v copied=%q, want %q", newSide, copied, want)
		}
	}
	m := selectionModel(t, true, false,
		diff.Line{Kind: '-', Text: "left", Old: 1},
		diff.Line{Kind: '+', Text: "right", New: 1})
	x, y := codePosition(t, m, "left")
	m.SetClipboard(func(text string) error {
		if text != "left" {
			t.Fatalf("crossing split copied %q", text)
		}
		return nil
	})
	runCopy(t, m, dragCode(m, x, y, m.width-2, y))
}

func TestDragCodeUnicodeTabsAndWrap(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, wrap := range []bool{false, true} {
			m := selectionModel(t, split, wrap, diff.Line{Kind: '+', Text: "prefix\t中文e\u0301suffix", New: 123456})
			x, y := codePosition(t, m, "中文")
			var copied string
			m.SetClipboard(func(text string) error { copied = text; return nil })
			runCopy(t, m, dragCode(m, x+1, y, x+4, y))
			if copied != "中文e\u0301" {
				t.Fatalf("split=%v wrap=%v: copied=%q", split, wrap, copied)
			}
			for _, line := range strings.Split(m.View(), "\n") {
				if ansi.StringWidth(line) != m.width {
					t.Fatal("selection changed rendered width")
				}
			}
		}
	}
	for _, split := range []bool{false, true} {
		text := "start\t" + strings.Repeat("中a ", 20) + "finish"
		m := selectionModel(t, split, true, diff.Line{Kind: '+', Text: text, New: 1})
		x1, y1 := codePosition(t, m, "start")
		x2, y2 := codePosition(t, m, "finish")
		m.SetClipboard(func(copied string) error {
			if copied != text {
				t.Fatalf("wrapped source was changed: %q", copied)
			}
			return nil
		})
		runCopy(t, m, dragCode(m, x1, y1, x2+5, y2))
	}
}

func TestDragCodeHorizontalScrollAndCancellation(t *testing.T) {
	m := selectionModel(t, false, false, diff.Line{Kind: '+', Text: strings.Repeat("x", 100) + "target", New: 1})
	m.xOffset = 100
	x, y := codePosition(t, m, "target")
	copies := 0
	m.SetClipboard(func(text string) error {
		copies++
		if text != "target" {
			t.Fatalf("copied %q", text)
		}
		return nil
	})
	clickAt(m, x, y)
	if copies != 0 || m.textSelection != nil {
		t.Fatal("ordinary click copied text")
	}
	runCopy(t, m, dragCode(m, x, y, x+5, y))
	m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: x + 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 25})
	_, cmd := m.Update(tea.MouseMsg{X: x + 3, Y: y, Action: tea.MouseActionRelease})
	if cmd != nil || m.textSelection != nil || copies != 1 {
		t.Fatal("resize did not cancel the active drag")
	}
	m.SetClipboard(func(string) error { return errors.New("clipboard blocked") })
	x, y = codePosition(t, m, "target")
	runCopy(t, m, dragCode(m, x, y, x+5, y))
	if !strings.Contains(m.message, "Copy failed: clipboard blocked") {
		t.Fatalf("copy error not shown: %s", m.message)
	}
	m.install(m.snapshot)
	if m.textSelection != nil {
		t.Fatal("refresh retained stale selection")
	}
}

func TestDragCodeDoesNotInterceptCommentModeOrBlankSplitCells(t *testing.T) {
	m := selectionModel(t, true, false, diff.Line{Kind: '+', Text: "added", New: 1})
	m.SetClipboard(func(string) error { t.Fatal("unexpected copy"); return nil })
	x, y := codePosition(t, m, "added")
	if cmd := dragCode(m, m.layout().diffX+15, y, x-1, y); cmd != nil || m.textSelection != nil {
		t.Fatal("blank split cell started a selection")
	}
	press(m, "c")
	if !m.lineSelecting {
		t.Fatal("comment mode did not open")
	}
	if cmd := dragCode(m, x, y, x+4, y); cmd != nil || m.textSelection != nil || !m.lineSelecting {
		t.Fatal("text dragging intercepted comment selection")
	}
}

func TestDragWrappedTabAndEmptyLines(t *testing.T) {
	m := selectionModel(t, false, true, diff.Line{Kind: '+', Text: "placeholder", New: 1})
	cell, ok := m.selectionCell(2, false)
	if !ok {
		t.Fatal("missing code cell")
	}
	text := strings.Repeat("x", cell.width-2) + "\tend"
	m.comparison.Files[0].Hunks[0].Lines[0].Text = text
	m.rebuild()
	x, y := codePosition(t, m, strings.Repeat("x", cell.width-2))
	// Select only the first visual row, ending inside a tab split by wrapping.
	m.SetClipboard(func(copied string) error {
		if copied != strings.Repeat("x", cell.width-2)+"\t" {
			t.Fatalf("partial wrapped tab: %q", copied)
		}
		return nil
	})
	runCopy(t, m, dragCode(m, x, y, x+cell.width-1, y))
	m = selectionModel(t, false, false,
		diff.Line{Kind: ' ', Text: "first", Old: 1, New: 1},
		diff.Line{Kind: ' ', Text: "", Old: 2, New: 2},
		diff.Line{Kind: ' ', Text: "last", Old: 3, New: 3})
	x1, y1 := codePosition(t, m, "first")
	x2, y2 := codePosition(t, m, "last")
	m.SetClipboard(func(copied string) error {
		if copied != "first\n\nlast" {
			t.Fatalf("blank source line lost: %q", copied)
		}
		return nil
	})
	runCopy(t, m, dragCode(m, x1, y1, x2+3, y2))
}

func TestDragCodePastBlankSplitRowsAndViewport(t *testing.T) {
	m := selectionModel(t, true, false,
		diff.Line{Kind: '-', Text: "old", Old: 1},
		diff.Line{Kind: '+', Text: "new", New: 1},
		diff.Line{Kind: '+', Text: "extra", New: 2})
	x, y := codePosition(t, m, "old")
	m.SetClipboard(func(copied string) error {
		if copied != "old" {
			t.Fatalf("blank split row copied %q", copied)
		}
		return nil
	})
	runCopy(t, m, dragCode(m, x, y, x, y+1))
	var lines []diff.Line
	for i := 1; i <= 60; i++ {
		lines = append(lines, diff.Line{Kind: '+', Text: fmt.Sprintf("line%02d", i), New: i})
	}
	m = selectionModel(t, false, false, lines...)
	x, y = codePosition(t, m, "line01")
	m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	for i := 0; i < 5; i++ {
		m.Update(tea.MouseMsg{X: x + 5, Y: contentTop + m.bodyHeight(), Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	}
	if m.offset != 5 || !strings.HasPrefix(m.selectedText(), "line01\nline02") {
		t.Fatal("dragging beyond viewport did not scroll and extend")
	}
	press(m, "esc")
	_, cmd := m.Update(tea.MouseMsg{X: x + 5, Y: contentTop + m.bodyHeight(), Action: tea.MouseActionRelease})
	if cmd != nil || m.dragging != "" {
		t.Fatal("cancelled drag still copied")
	}
}
