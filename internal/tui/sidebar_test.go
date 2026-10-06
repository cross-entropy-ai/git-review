package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSidebarHideShow(t *testing.T) {
	for _, tree := range []bool{false, true} {
		for _, split := range []bool{false, true} {
			t.Run(fmt.Sprintf("tree=%v/split=%v", tree, split), func(t *testing.T) {
				m := sampleModel(true)
				t.Cleanup(m.Close)
				m.sideWidth = 42
				m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
				if tree {
					press(m, "t")
				}
				if split {
					press(m, "s")
				}
				// Leave room to scroll regardless of the active file ordering.
				m.selected = m.visible[1]
				m.jumpSelected()
				press(m, "tab")
				selected, offset := m.selected, m.offset
				clickText(t, m, "b Hide")
				g := m.layout()
				if g.sideWidth != 0 || g.diffX != 0 || g.diffWidth != m.width-2 || m.fileFocus {
					t.Fatalf("sidebar did not give full width/focus to diff: %+v", g)
				}
				press(m, "tab")
				if m.fileFocus || m.selected != selected || m.offset != offset {
					t.Fatal("hidden sidebar took focus or changed reading position")
				}
				// The old sidebar region now belongs to the diff.
				m.Update(tea.MouseMsg{X: 10, Y: contentTop + 2, Button: tea.MouseButtonWheelDown})
				if m.offset <= offset {
					t.Fatal("wheel over hidden sidebar did not scroll diff")
				}
				clickText(t, m, "b Files")
				if m.layout().sideWidth != 42 || m.treeMode != tree || m.splitMode != split {
					t.Fatal("showing sidebar lost width or display mode")
				}
				press(m, "b")
				m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
				m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
				if m.layout().sideWidth != 0 {
					t.Fatal("resize reopened hidden sidebar")
				}
				press(m, "b")
				if m.layout().sideWidth != 42 {
					t.Fatal("keyboard did not restore sidebar")
				}
			})
		}
	}
}

func TestSidebarToggleReflowsWrappedDiff(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := sampleModel(false)
		t.Cleanup(m.Close)
		m.splitMode = split
		m.comparison.Files[0].Hunks[0].Lines[5].Text = strings.Repeat("long text ", 80)
		press(m, "w")
		m.offset = 7
		anchor := m.rows[m.offset]
		rows := len(m.rows)
		press(m, "b")
		if len(m.rows) >= rows || m.rows[m.offset].line != anchor.line {
			t.Fatal("hiding sidebar did not rewrap or preserve source position")
		}
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) != m.width {
				t.Fatal("hidden sidebar rendered an incorrect width")
			}
		}
		press(m, "b")
		if len(m.rows) != rows || m.rows[m.offset].line != anchor.line {
			t.Fatal("showing sidebar did not restore wrapping and position")
		}
	}
}
