package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/rivo/uniseg"
)

type textPoint struct{ row, column int }
type textSelection struct {
	anchor, end textPoint
	newSide     bool
	moved       bool
}

type clipboardMsg struct{ err error }

// SetClipboard supplies the application's clipboard transport. Keeping it out
// of rendering ensures repainting never repeats a clipboard write.
func (m *Model) SetClipboard(copy func(string) error) { m.copyText = copy }

func (m *Model) clearTextSelection() {
	m.textSelection = nil
	if m.dragging == "text" {
		m.dragging = ""
	}
}

// Share gutter widths between wrapping and mouse hit testing, including source
// line numbers wider than the usual four columns.
func codeGutterWidth(line diff.Line, split, newSide bool) int {
	digits := func(n int) int { return max(4, len(fmt.Sprint(n))) }
	if split {
		n := line.Old
		if newSide {
			n = line.New
		}
		return digits(n) + 5
	}
	return digits(line.Old) + digits(line.New) + 6
}

type codeCell struct {
	line, x, width int
	start, end     int // Display columns in the unwrapped source line.
}

func (m *Model) selectionCell(index int, newSide bool) (codeCell, bool) {
	if index < 0 || index >= len(m.rows) {
		return codeCell{}, false
	}
	r := m.rows[index]
	if r.kind != 'l' && r.kind != 'd' {
		return codeCell{}, false
	}
	g := m.layout()
	cell := codeCell{line: r.line, x: g.diffX + 1 + fileFrameInset, width: g.diffWidth - 2*fileFrameInset}
	span := r.leftSpan
	if r.kind == 'd' {
		leftWidth := (cell.width - 1) / 2
		if newSide {
			cell.line, span = r.rightLine, r.rightSpan
			cell.x += leftWidth + 1
			cell.width -= leftWidth + 1
		} else {
			cell.width = leftWidth
		}
	}
	if cell.line < 0 || (m.wrapLines && r.continued && span == (codeSpan{})) {
		return codeCell{}, false
	}
	line := m.comparison.Files[r.file].Hunks[r.hunk].Lines[cell.line]
	if line.Kind != ' ' && line.Kind != '+' && line.Kind != '-' {
		return codeCell{}, false
	}
	gutter := codeGutterWidth(line, r.kind == 'd', newSide)
	cell.x += gutter
	cell.width = max(0, cell.width-gutter)
	cell.start = m.xOffset
	cell.end = ansi.StringWidth(safeText(line.Text))
	if m.wrapLines {
		cell.start, cell.end = span.start, span.end
	}
	return cell, cell.width > 0
}

func (m *Model) startTextSelection(x, y int) {
	index := m.offset + y - contentTop
	g := m.layout()
	newSide := m.splitMode && x >= g.diffX+1+fileFrameInset+(g.diffWidth-2*fileFrameInset-1)/2+1
	cell, ok := m.selectionCell(index, newSide)
	if !ok || x < cell.x || x >= cell.x+cell.width {
		return
	}
	point := textPoint{index, min(cell.end, cell.start+x-cell.x)}
	m.textSelection = &textSelection{anchor: point, end: point, newSide: newSide}
	m.dragging = "text"
}

func (m *Model) extendTextSelection(x, y int, scroll bool) {
	s := m.textSelection
	if s == nil {
		return
	}
	if scroll {
		if y < contentTop {
			m.offset--
		} else if y >= contentTop+m.bodyHeight() {
			m.offset++
		}
		m.clampOffset()
	}
	index := min(len(m.rows)-1, m.offset+min(max(y-contentTop, 0), m.bodyHeight()-1))
	requested := index
	step := -1
	if index < s.anchor.row {
		step = 1
	}
	for index >= 0 && index < len(m.rows) {
		if cell, ok := m.selectionCell(index, s.newSide); ok {
			column := min(cell.end, cell.start+min(max(x-cell.x, 0), cell.width-1))
			if index < requested {
				column = cell.end
			} else if index > requested {
				column = cell.start
			}
			point := textPoint{index, column}
			s.moved = s.moved || point != s.anchor
			s.end = point
			if s.moved {
				m.message = "Selecting code · release to copy · Esc cancel"
			}
			return
		}
		if index == s.anchor.row {
			return
		}
		index += step
	}
}

func (s *textSelection) bounds() (textPoint, textPoint) {
	a, b := s.anchor, s.end
	if a.row > b.row || (a.row == b.row && a.column > b.column) {
		a, b = b, a
	}
	return a, b
}

// Source byte boundaries preserve tabs and whole grapheme clusters while mouse
// coordinates use the sanitized display width (wide characters, tabs, controls).
func sourceBoundary(text string, column int, after bool) int {
	g := uniseg.NewGraphemes(text)
	width := 0
	for g.Next() {
		start, end := g.Positions()
		next := width + ansi.StringWidth(safeText(g.Str()))
		if column < next {
			if after {
				return end
			}
			return start
		}
		width = next
	}
	return len(text)
}

func (m *Model) selectionRange(index int, cell codeCell) (int, int, bool) {
	s := m.textSelection
	if s == nil || !s.moved {
		return 0, 0, false
	}
	a, b := s.bounds()
	if index < a.row || index > b.row {
		return 0, 0, false
	}
	r := m.rows[index]
	text := m.comparison.Files[r.file].Hunks[r.hunk].Lines[cell.line].Text
	start, end := 0, len(text)
	if m.wrapLines {
		start = sourceBoundary(text, cell.start, false)
		if cell.end > cell.start {
			end = sourceBoundary(text, cell.end-1, true)
		} else {
			end = start
		}
	}
	if index == a.row {
		start = max(start, sourceBoundary(text, a.column, false))
	}
	if index == b.row {
		end = min(end, sourceBoundary(text, b.column, true))
	}
	return start, max(start, end), true
}

func (m *Model) selectedText() string {
	if m.textSelection == nil || !m.textSelection.moved {
		return ""
	}
	a, b := m.textSelection.bounds()
	var out strings.Builder
	previous := lineRef{file: -1}
	previousEnd := 0
	for i := a.row; i <= b.row; i++ {
		cell, ok := m.selectionCell(i, m.textSelection.newSide)
		if !ok {
			continue
		}
		start, end, ok := m.selectionRange(i, cell)
		if !ok {
			continue
		}
		r := m.rows[i]
		ref := lineRef{file: r.file, hunk: r.hunk, line: cell.line}
		if ref == previous {
			start = max(start, previousEnd)
		} else if previous.file >= 0 {
			out.WriteByte('\n')
		}
		text := m.comparison.Files[r.file].Hunks[r.hunk].Lines[cell.line].Text
		out.WriteString(text[start:max(start, end)])
		previous, previousEnd = ref, end
	}
	return out.String()
}

func (m *Model) finishTextSelection(msg tea.MouseMsg) tea.Cmd {
	m.extendTextSelection(msg.X, msg.Y, false)
	m.dragging = ""
	text := m.selectedText()
	if text == "" {
		m.clearTextSelection()
		m.message = ""
		return nil
	}
	if m.copyText == nil {
		m.message = "Clipboard is unavailable"
		return nil
	}
	copy := m.copyText
	return func() tea.Msg { return clipboardMsg{err: copy(text)} }
}

func (m *Model) highlightTextSelection(content string, index int) string {
	if !m.color || m.textSelection == nil {
		return content
	}
	cell, ok := m.selectionCell(index, m.textSelection.newSide)
	if !ok {
		return content
	}
	start, end, ok := m.selectionRange(index, cell)
	if !ok || start == end {
		return content
	}
	r := m.rows[index]
	text := m.comparison.Files[r.file].Hunks[r.hunk].Lines[cell.line].Text
	left := max(0, ansi.StringWidth(safeText(text[:start]))-cell.start)
	right := min(cell.width, ansi.StringWidth(safeText(text[:end]))-cell.start)
	if right <= left {
		return content
	}
	x := cell.x - m.layout().diffX - 1
	left, right = left+x, right+x
	selected := m.surfaceWithBackground(m.palette.headerBackground, m.palette.accent, ansi.Strip(ansi.Cut(content, left, right)), right-left)
	return ansi.Cut(content, 0, left) + selected + ansi.Cut(content, right, ansi.StringWidth(content))
}
