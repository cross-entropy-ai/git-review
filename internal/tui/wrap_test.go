package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func TestWrapRendering(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, color := range []bool{false, true} {
			m := sampleModel(color)
			m.splitMode = split
			text := strings.Repeat("abc中文e\u0301\t", 12) + "TAIL"
			m.comparison.Files[0].Hunks[0].Lines = []gitdiff.Line{
				{Kind: '-', Old: 1, Text: "short"},
				{Kind: '+', New: 1, Text: text},
			}
			press(m, "w")
			for _, width := range []int{45, 60, 90, 100, 120, 180} {
				m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				var reconstructed strings.Builder
				for _, r := range m.rows {
					if r.file != 0 || (r.kind != 'l' && r.kind != 'd') {
						continue
					}
					span := r.leftSpan
					if split {
						span = r.rightSpan
					} else if r.line != 1 {
						continue
					}
					part := ansi.Cut(safeText(text), span.start, span.end)
					reconstructed.WriteString(part)
					if !strings.Contains(ansi.Strip(m.renderRow(r, m.layout().diffWidth)), part) {
						t.Fatalf("split=%v width=%d: lost wrapped text %q", split, width, part)
					}
				}
				if reconstructed.String() != safeText(text) {
					t.Fatalf("split=%v width=%d: wrapping lost source text", split, width)
				}
				lines := strings.Split(m.View(), "\n")
				if len(lines) != 24 {
					t.Fatalf("view has %d rows", len(lines))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != width {
						t.Fatalf("width=%d: rendered row width=%d", width, ansi.StringWidth(line))
					}
				}
			}
		}
	}
}

func TestWrapTogglePreservesPositionAndInputs(t *testing.T) {
	m := sampleModel(false)
	m.comparison.Files[0].Hunks[0].Lines[5].Text = strings.Repeat("long", 100)
	m.rebuild()
	m.offset, m.xOffset = 7, 8
	anchor := m.rows[m.offset]
	press(m, "w")
	if !m.wrapLines || m.xOffset != 0 || m.rows[m.offset].line != anchor.line {
		t.Fatal("wrap toggle lost source position or horizontal offset")
	}
	press(m, "l")
	m.mouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelRight, X: m.width - 5, Y: contentTop})
	if m.xOffset != 0 {
		t.Fatal("wrapped lines scrolled horizontally")
	}
	m.offset += 3
	column := m.rows[m.offset].leftSpan.start
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	r := m.rows[m.offset]
	if r.line != anchor.line || column < r.leftSpan.start || column >= r.leftSpan.end {
		t.Fatal("resize lost reading position within a wrapped line")
	}
	press(m, "w")
	if m.wrapLines || m.rows[m.offset].line != anchor.line || len(m.rows) != 69 {
		t.Fatal("unwrapping did not restore source rows")
	}
	press(m, "/w")
	if m.wrapLines || m.search != "w" {
		t.Fatal("search interpreted w as a shortcut")
	}
	press(m, "esc")
	press(m, "fw")
	if m.wrapLines || m.fileQuery != "w" {
		t.Fatal("file picker interpreted w as a shortcut")
	}
	press(m, "esc")
	press(m, "w")
	m.install(m.snapshot)
	if !m.wrapLines {
		t.Fatal("refresh lost wrap preference")
	}
}

func TestWrappedSearchAndCommentSelection(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := sampleModel(true)
		m.splitMode = split
		m.comparison.Files[0].Hunks[0].Lines[0].Text = strings.Repeat("x", 1500) + "NEEDLE"
		press(m, "w/NEEDLE")
		press(m, "enter")
		if m.xOffset != 0 || !strings.Contains(ansi.Strip(m.View()), "NEEDLE") {
			t.Fatalf("split=%v: wrapped search match is not visible", split)
		}
		press(m, "esc")
		press(m, "c")
		if !m.lineSelecting || m.commentLine.line != 0 {
			t.Fatal("continuation row selected the wrong source line")
		}
		press(m, "j")
		if m.commentLine.line != 1 {
			t.Fatal("comment navigation did not move to the next source line")
		}
		press(m, "w")
		if m.wrapLines || m.rows[m.lineCursor].line != 1 {
			t.Fatal("toggle lost comment selection")
		}
		press(m, "c")
		press(m, "w")
		if m.wrapLines || string(m.noteInput) != "w" {
			t.Fatal("comment editor interpreted w as a shortcut")
		}
	}
}
