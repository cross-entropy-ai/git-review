package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func TestFuzzyPickerRankingAndScrolling(t *testing.T) {
	m := treeModel("src/model.go", "src/many_other_details_long.go", "docs/中文.md", "renamed.go")
	t.Cleanup(m.Close)
	m.comparison.Files[3].OldPath = "old/original.go"
	press(m, "fmdl")
	if len(m.fileMatches) != 2 || m.fileMatches[0] != 0 {
		t.Fatalf("fuzzy matching did not rank the compact match first: %v", m.fileMatches)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	press(m, "old orig")
	if len(m.fileMatches) != 1 || m.fileMatches[0] != 3 {
		t.Fatal("multi-term search did not match a rename source")
	}
	press(m, "esc")
	if m.selected != 0 {
		t.Fatal("cancel changed file selection")
	}
	for i := 0; i < 40; i++ {
		m.comparison.Files = append(m.comparison.Files, gitdiff.File{Path: fmt.Sprintf("extra/%02d.txt", i)})
	}
	m.rebuild()
	press(m, "fextra")
	for i := 0; i < 30; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.fileOffset == 0 || !strings.Contains(m.View(), "› extra/30.txt") {
		t.Fatal("picker did not scroll its current selection into view")
	}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 12})
	if !strings.Contains(m.View(), "› extra/30.txt") {
		t.Fatal("resize hid picker selection")
	}
	press(m, "enter")
	if m.comparison.Files[m.selected].Path != "extra/30.txt" || len(m.visible) != 44 {
		t.Fatal("picker opened the wrong file or filtered the comparison")
	}
}

func TestModalsCaptureInputAndMouse(t *testing.T) {
	for _, key := range []string{"?", "f"} {
		m := sampleModel(false)
		t.Cleanup(m.Close)
		press(m, "j")
		selected, offset := m.selected, m.offset
		m.dragging = "diff"
		press(m, key)
		if m.dragging != "" {
			t.Fatal("opening modal left a scrollbar drag active")
		}
		g := m.modalLayout()
		clickAt(m, 2, contentTop)
		m.Update(tea.MouseMsg{X: 1, Y: contentTop, Button: tea.MouseButtonWheelDown})
		m.Update(tea.MouseMsg{X: 1, Y: contentTop + 5, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
		for _, input := range []string{"v", "r", "e", "s"} {
			if cmd := m.activate(input); cmd != nil {
				t.Fatal("modal allowed a background command")
			}
		}
		if m.selected != selected || m.offset != offset || m.viewedCount() != 0 || m.splitMode {
			t.Fatal("modal input mutated the underlying review")
		}
		if key == "?" {
			m.Update(tea.MouseMsg{X: g.x + 2, Y: g.y + 2, Button: tea.MouseButtonWheelDown})
			if m.helpOffset == 0 {
				t.Fatal("help mouse wheel did not scroll")
			}
			clickText(t, m, "Esc Close help")
		} else {
			press(m, "esc")
		}
		if m.help || m.picking || m.selected != selected || m.offset != offset {
			t.Fatal("closing modal failed to preserve review position")
		}
	}
	m := sampleModel(false)
	t.Cleanup(m.Close)
	press(m, "f中文")
	clickText(t, m, "› docs/中文.md")
	if m.picking || m.selected != 1 {
		t.Fatal("clicking picker result did not select it")
	}
}

func TestModalDimensionsAndTerminalSafety(t *testing.T) {
	for _, color := range []bool{false, true} {
		for _, theme := range []Theme{LightTheme, DarkTheme} {
			for _, size := range [][2]int{{140, 40}, {90, 24}, {60, 18}, {45, 12}} {
				for _, key := range []string{"?", "f"} {
					m := sampleModel(color)
					t.Cleanup(m.Close)
					m.palette = paletteFor(theme)
					m.comparison.Files[1].Path = "中文\x1b[2J\nfile.go"
					m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
					press(m, key)
					if key == "f" {
						m.fileQuery = strings.Repeat("长", 100) + "\x1b[2J"
					}
					view := m.View()
					lines := strings.Split(view, "\n")
					if len(lines) != size[1] {
						t.Fatalf("modal %s %v: wrong height %d", key, size, len(lines))
					}
					for _, line := range lines {
						if ansi.StringWidth(line) != size[0] {
							t.Fatalf("modal %s %v: wrong row width %d", key, size, ansi.StringWidth(line))
						}
					}
					if strings.Contains(view, "\x1b[2J") || (!color && strings.Contains(view, "\x1b")) {
						t.Fatal("unsafe terminal control or color in plain mode")
					}
					if !strings.Contains(lines[0], "git review") {
						t.Fatal("modal replaced the underlying review screen")
					}
				}
			}
		}
	}
}

func TestModalFooterHintsDoNotDismiss(t *testing.T) {
	for _, key := range []string{"?", "f"} {
		m := sampleModel(false)
		t.Cleanup(m.Close)
		press(m, key)
		g := m.modalLayout()
		clickAt(m, g.x+g.width-5, g.y+g.height-2)
		if !m.help && !m.picking {
			t.Fatal("clicking non-interactive footer text closed the modal")
		}
		if key == "?" {
			clickText(t, m, "Esc Close help")
		} else {
			clickText(t, m, "Esc Cancel")
		}
		if m.help || m.picking {
			t.Fatal("clicking the close control did not dismiss the modal")
		}
	}
}
