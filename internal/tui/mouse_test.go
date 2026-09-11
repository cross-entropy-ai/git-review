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
	clickText(t, m, "Filter files")
	if !m.filtering {
		t.Fatal("filter field did not accept focus")
	}
	press(m, "docs")
	clickText(t, m, "Enter Apply")
	if m.filtering || len(m.visible) != 1 {
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
	clickText(t, m, "C Collapse")
	if len(m.rows) != 6 {
		t.Fatal("collapse toolbar action failed")
	}
	clickText(t, m, "E Expand")
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
	press(m, "C")
	x := m.layout().diffX + 10
	m.Update(tea.MouseMsg{X: x, Y: contentTop + 1, Button: tea.MouseButtonWheelDown})
	if m.selected != 1 || m.offset != 0 {
		t.Fatal("wheel did not select the next file in a fully visible diff")
	}
	press(m, " ")
	if m.collapsed["docs/中文.md"] {
		t.Fatal("wheel selection could not be unfolded")
	}
	press(m, "C")
	m.Update(tea.MouseMsg{X: x, Y: contentTop + 1, Button: tea.MouseButtonWheelUp})
	if m.selected != 0 || m.offset != 0 {
		t.Fatal("wheel did not select the previous file in a fully visible diff")
	}
}
