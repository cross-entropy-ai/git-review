package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func clickAt(m *Model, x, y int) {
	m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
}

// Locate controls in the actual rendered text, so tests also catch drift
// between drawing coordinates and mouse hit regions.
func clickText(t *testing.T, m *Model, needle string) {
	t.Helper()
	for y, line := range strings.Split(m.View(), "\n") {
		plain := ansi.Strip(line)
		if index := strings.Index(plain, needle); index >= 0 {
			clickAt(m, ansi.StringWidth(plain[:index])+1, y)
			return
		}
	}
	t.Fatalf("control %q is not visible:\n%s", needle, ansi.Strip(m.View()))
}

func TestMouseSidebarSelectFoldAndViewed(t *testing.T) {
	m := sampleModel(true)
	clickText(t, m, "中文.md")
	if m.selected != 1 || !m.fileFocus {
		t.Fatal("clicking a Unicode filename did not select and focus it")
	}
	clickAt(m, 2, contentTop+2)
	if !m.collapsed["docs/中文.md"] || m.viewed["docs/中文.md"] {
		t.Fatal("fold arrow changed viewed status or failed to fold")
	}
	clickAt(m, 2, contentTop+2)
	if m.collapsed["docs/中文.md"] {
		t.Fatal("second click did not unfold")
	}
	clickAt(m, 5, contentTop+2)
	if !m.viewed["docs/中文.md"] || !m.collapsed["docs/中文.md"] || m.selected != 1 {
		t.Fatal("checkbox did not mark viewed while keeping the clicked file selected")
	}
	clickAt(m, 5, contentTop+2)
	if m.viewed["docs/中文.md"] || m.collapsed["docs/中文.md"] {
		t.Fatal("checkbox did not clear viewed and unfold")
	}
}

func TestMouseFileCardAndToolbar(t *testing.T) {
	m := sampleModel(true)
	clickText(t, m, "f Files")
	if !m.picking {
		t.Fatal("file finder did not open")
	}
	press(m, "docs")
	clickText(t, m, "Enter Open")
	if m.picking || m.selected != 1 || len(m.visible) != 3 {
		t.Fatal("apply button failed")
	}
	clickText(t, m, "▾ docs/中文.md")
	if !m.collapsed["docs/中文.md"] || m.fileFocus {
		t.Fatal("file card click did not fold and focus diff")
	}
	clickText(t, m, "▸ docs/中文.md")
	if m.collapsed["docs/中文.md"] {
		t.Fatal("file card click did not unfold")
	}
	clickText(t, m, "Viewed")
	if !m.viewed["docs/中文.md"] {
		t.Fatal("file card viewed button failed")
	}
	press(m, "esc")
	clickText(t, m, "z Collapse all")
	if len(m.rows) != 6 {
		t.Fatal("collapse toolbar action failed")
	}
	clickText(t, m, "z Expand all")
	if len(m.rows) <= 6 {
		t.Fatal("expand toolbar action failed")
	}
	clickText(t, m, "? Help")
	if !m.help {
		t.Fatal("help toolbar action failed")
	}
	clickText(t, m, "Esc Close help")
	if m.help {
		t.Fatal("close help action failed")
	}
}

func TestMouseWheelAndScrollbarUsePointerPane(t *testing.T) {
	m := sampleModel(false)
	for i := 0; i < 30; i++ {
		file := m.comparison.Files[0]
		file.Path = fmt.Sprintf("extra/%02d.go", i)
		m.comparison.Files = append(m.comparison.Files, file)
	}
	m.rebuild()
	m.Update(tea.MouseMsg{X: 12, Y: contentTop + 2, Button: tea.MouseButtonWheelDown})
	if m.sideOffset == 0 || m.offset != 0 || m.selected != 0 {
		t.Fatal("sidebar wheel scrolled the diff or changed selection")
	}
	firstVisible := m.visible[m.sideOffset]
	clickAt(m, 12, contentTop)
	if m.selected != firstVisible {
		t.Fatal("scrolled sidebar hit the wrong file")
	}
	g := m.layout()
	previousOffset := m.offset
	m.Update(tea.MouseMsg{X: g.diffX + 10, Y: contentTop + 2, Button: tea.MouseButtonWheelDown})
	if m.offset != previousOffset+3 {
		t.Fatal("diff wheel did not scroll three rows")
	}
	m.Update(tea.MouseMsg{X: g.diffX + 10, Y: contentTop + 2, Button: tea.MouseButtonWheelDown, Shift: true})
	if m.xOffset == 0 {
		t.Fatal("shift-wheel did not scroll horizontally")
	}
	m.Update(tea.MouseMsg{X: g.diffX + 10, Y: contentTop + 2, Button: tea.MouseButtonWheelLeft})
	if m.xOffset != 0 {
		t.Fatal("horizontal wheel did not scroll back")
	}
	m.Update(tea.MouseMsg{X: m.width - 1, Y: contentTop, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: m.width - 1, Y: contentTop + g.bodyHeight - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.Update(tea.MouseMsg{X: m.width - 1, Y: contentTop + g.bodyHeight - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if m.offset != len(m.rows)-m.bodyHeight() || m.dragging != "" {
		t.Fatal("diff scrollbar drag did not reach the bottom or release")
	}
	clickAt(m, g.sideWidth-1, contentTop+g.bodyHeight-1)
	if m.sideOffset != len(m.visible)-m.sidebarCapacity() {
		t.Fatal("sidebar scrollbar did not reach the last files")
	}
}

func TestMouseResizingEmptyResultsAndHelp(t *testing.T) {
	m := sampleModel(false)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	clickText(t, m, "▾ main.go")
	if !m.collapsed["main.go"] {
		t.Fatal("file card hit region broke without the sidebar")
	}
	clickText(t, m, "Viewed")
	if !m.viewed["main.go"] {
		t.Fatal("viewed hit region broke without the sidebar")
	}
	press(m, "/missing")
	press(m, "enter")
	clickAt(m, 10, contentTop+1)
	if m.viewedCount() != 1 {
		t.Fatal("clicking empty results changed progress")
	}
	press(m, "esc")
	press(m, "?")
	m.Update(tea.MouseMsg{X: 10, Y: 8, Button: tea.MouseButtonWheelDown})
	if m.helpOffset == 0 {
		t.Fatal("help wheel did not scroll")
	}
	press(m, "esc")
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	before := m.viewedCount()
	clickAt(m, 5, 4)
	if m.viewedCount() != before {
		t.Fatal("invisible controls responded in an undersized terminal")
	}
}

func TestMouseWheelNavigatesDiffThatFits(t *testing.T) {
	m := sampleModel(false)
	press(m, "z")
	x := m.layout().diffX + 10
	m.Update(tea.MouseMsg{X: x, Y: contentTop + 1, Button: tea.MouseButtonWheelDown})
	if m.selected != 1 || m.offset != 0 {
		t.Fatal("wheel did not select the next file in a fully visible diff")
	}
	press(m, " ")
	if m.collapsed["docs/中文.md"] {
		t.Fatal("wheel selection could not be unfolded")
	}
	press(m, "z")
	m.Update(tea.MouseMsg{X: x, Y: contentTop + 1, Button: tea.MouseButtonWheelUp})
	if m.selected != 0 || m.offset != 0 {
		t.Fatal("wheel did not select the previous file in a fully visible diff")
	}
}

func TestMouseSidebarDragResize(t *testing.T) {
	for _, tree := range []bool{false, true} {
		for _, split := range []bool{false, true} {
			t.Run(fmt.Sprintf("tree=%t/split=%t", tree, split), func(t *testing.T) {
				m := sampleModel(true)
				if tree {
					press(m, "t")
				}
				if split {
					press(m, "s")
				}
				g := m.layout()
				y := contentTop + g.bodyHeight/2
				line := strings.Split(m.View(), "\n")[y]
				if ansi.Cut(ansi.Strip(line), g.sideWidth, g.sideWidth+1) != "⋮" {
					t.Fatal("resize handle is not rendered at its hit region")
				}
				selected, offset, sideOffset, focus := m.selected, m.offset, m.sideOffset, m.fileFocus
				m.Update(tea.MouseMsg{X: g.sideWidth, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				m.Update(tea.MouseMsg{X: 48, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
				if m.layout().sideWidth != 48 || m.layout().diffX != 49 {
					t.Fatal("drag did not resize both panes")
				}
				if m.selected != selected || m.offset != offset || m.sideOffset != sideOffset || m.fileFocus != focus {
					t.Fatal("resizing changed selection, scroll position, or focus")
				}
				for _, x := range []int{-10, 200, 48} {
					m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
					g = m.layout()
					if g.sideWidth < minSidebarWidth || g.diffWidth < minDiffWidth {
						t.Fatalf("drag exceeded pane limits: %+v", g)
					}
					for _, line := range strings.Split(m.View(), "\n") {
						if ansi.StringWidth(line) != m.width {
							t.Fatal("resized rendering does not fit the terminal")
						}
					}
				}
				m.Update(tea.MouseMsg{X: 48, Y: y, Action: tea.MouseActionRelease})
				m.Update(tea.MouseMsg{X: 30, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
				if m.dragging != "" || m.layout().sideWidth != 48 {
					t.Fatal("release did not stop resizing")
				}
				clickText(t, m, "Viewed")
				if !m.viewed["main.go"] {
					t.Fatal("resizing misaligned the diff's viewed button")
				}
			})
		}
	}
}

func TestSidebarWidthSurvivesTerminalResize(t *testing.T) {
	m := sampleModel(false)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m.Update(tea.MouseMsg{X: m.layout().sideWidth, Y: contentTop, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.Update(tea.MouseMsg{X: 80, Y: contentTop, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	for _, size := range []struct{ terminal, sidebar int }{{90, 42}, {60, 0}, {160, 80}} {
		m.Update(tea.WindowSizeMsg{Width: size.terminal, Height: 30})
		if m.layout().sideWidth != size.sidebar || m.dragging != "" {
			t.Fatalf("terminal width %d: sidebar=%d, dragging=%q", size.terminal, m.layout().sideWidth, m.dragging)
		}
	}
	m.install(m.snapshot)
	if m.layout().sideWidth != 80 {
		t.Fatal("refresh lost the chosen sidebar width")
	}
}
