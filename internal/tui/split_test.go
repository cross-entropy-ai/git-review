package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func TestSplitPairsChangesAndNewlineNotices(t *testing.T) {
	for _, tc := range []struct {
		name, kinds string
		want        [][2]int
	}{
		{"replacement", " --+++ ", [][2]int{{0, 0}, {1, 3}, {2, 4}, {-1, 5}, {6, 6}}},
		{"deletion", "-- ", [][2]int{{0, -1}, {1, -1}, {2, 2}}},
		{"addition", "++", [][2]int{{-1, 0}, {-1, 1}}},
		{"separate blocks", "-+ -+", [][2]int{{0, 1}, {2, 2}, {3, 4}}},
		{"both newline notices", "-\\+\\", [][2]int{{0, 2}, {1, 3}}},
		{"old newline notice", "-\\+", [][2]int{{0, 2}, {1, -1}}},
		{"new newline notice", "-+\\", [][2]int{{0, 1}, {-1, 2}}},
		{"context newline notice", " \\", [][2]int{{0, 0}, {1, 1}}},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var lines []gitdiff.Line
			for _, kind := range []byte(tc.kinds) {
				lines = append(lines, gitdiff.Line{Kind: kind})
			}
			var got [][2]int
			for _, row := range splitRows(2, 3, lines) {
				if row.file != 2 || row.hunk != 3 {
					t.Fatal("split row lost its file/hunk")
				}
				got = append(got, [2]int{row.line, row.rightLine})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSplitRenderingAndDimensions(t *testing.T) {
	for _, theme := range []Theme{DarkTheme, LightTheme} {
		for _, color := range []bool{false, true} {
			m := sampleModel(color)
			m.palette = paletteFor(theme)
			m.comparison.Files[0].Hunks[0].Lines = []gitdiff.Line{
				{Kind: '-', Text: "var old = 1 // 中文", Old: 10},
				{Kind: '+', Text: "var new = 2 // 中文", New: 20},
				{Kind: '+', Text: "", New: 21},
				{Kind: ' ', Text: "context", Old: 11, New: 22},
			}
			press(m, "s")
			paired := m.rows[2]
			if paired.kind != 'd' {
				t.Fatal("split shortcut did not rebuild rows")
			}
			text := m.renderRowContent(paired, 100)
			plain := ansi.Strip(text)
			if !strings.Contains(plain[:49], "10 - │ var old") || !strings.Contains(plain[50:], "20 + │ var new") {
				t.Fatalf("wrong old/new sides: %q", plain)
			}
			if color {
				for _, bg := range []string{m.palette.deletedBackground, m.palette.addedBackground} {
					if !strings.Contains(text, colorCode(48, bg)) {
						t.Fatal("missing split diff background")
					}
				}
				blankAddition := m.renderRowContent(m.rows[3], 100)
				if !strings.Contains(blankAddition, colorCode(48, m.palette.addedBackground)) || strings.Contains(blankAddition, colorCode(48, m.palette.deletedBackground)) {
					t.Fatal("empty added line or unmatched cell has incorrect background")
				}
				if strings.Contains(m.renderRowContent(m.rows[4], 100), "\x1b[48;") {
					t.Fatal("context row has a diff background")
				}
			}
			m.xOffset = 4
			if strings.Contains(ansi.Strip(m.renderRowContent(paired, 100)), "var ") {
				t.Fatal("horizontal scrolling did not move both code cells")
			}
			for _, width := range []int{45, 60, 90, 100, 120, 180} {
				m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				view := m.View()
				if !color && strings.Contains(view, "\x1b") {
					t.Fatal("no-color split output contains escapes")
				}
				lines := strings.Split(view, "\n")
				if len(lines) != 24 {
					t.Fatalf("split view has %d rows", len(lines))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != width {
						t.Fatalf("width %d: row has width %d", width, ansi.StringWidth(line))
					}
				}
			}
		}
	}
}

func TestSplitTogglePreservesReviewAndPosition(t *testing.T) {
	m := sampleModel(false)
	m.comparison.Files[0].Hunks[0].Lines[0].Kind = '-'
	m.comparison.Files[0].Hunks[0].Lines[1].Kind = '+'
	m.viewed["last.txt"], m.collapsed["last.txt"] = true, true
	m.rebuild()
	inlineRows := len(m.rows)
	m.offset, m.xOffset = 10, 8
	anchor := m.rows[m.offset]
	press(m, "s")
	if !m.splitMode || len(m.rows) != inlineRows-1 || m.rows[m.offset].line != anchor.line || m.xOffset != 8 {
		t.Fatal("split toggle lost reading position or did not pair changes")
	}
	if m.selected != 0 || !m.viewed["last.txt"] || !m.collapsed["last.txt"] {
		t.Fatal("split toggle changed review state")
	}
	clickText(t, m, "s Inline")
	if m.splitMode || m.offset != 10 || len(m.rows) != inlineRows {
		t.Fatal("mouse toggle did not restore inline view/position")
	}
	press(m, "/s")
	if m.splitMode || m.filter != "s" {
		t.Fatal("s was interpreted as a shortcut while filtering")
	}
	press(m, "esc")
	press(m, "s")
	press(m, "C")
	if len(m.rows) != 6 {
		t.Fatal("split collapse failed")
	}
	press(m, "E")
	selected := m.selected
	press(m, "n")
	if m.selected != selected+1 {
		t.Fatal("split file navigation failed")
	}
	m.install(m.comparison)
	if !m.splitMode || m.rows[2].kind != 'd' {
		t.Fatal("refresh lost split mode")
	}
	press(m, "/missing")
	press(m, "enter")
	press(m, "s")
	if m.splitMode || len(m.rows) != 0 {
		t.Fatal("could not toggle with no matching files")
	}
}
