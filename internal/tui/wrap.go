package tui

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// Each visual row retains its source line and a range of display columns so
// scrolling, search, and mouse selection share the same row coordinates.
type codeSpan struct{ start, end int }

func wrapSpans(text string, width int) []codeSpan {
	width = max(1, width)
	var spans []codeSpan
	start, column := 0, 0
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		size := ansi.StringWidth(graphemes.Str())
		if column > start && column-start+size > width {
			spans = append(spans, codeSpan{start, column})
			start = column
		}
		column += size
	}
	return append(spans, codeSpan{start, column})
}

func (m *Model) wrapRows(rows []row) []row {
	width := m.layout().diffWidth - 2*fileFrameInset
	var wrapped []row
	for _, r := range rows {
		if r.kind != 'l' && r.kind != 'd' {
			wrapped = append(wrapped, r)
			continue
		}
		spans := func(index, cellWidth int, split, newSide bool) []codeSpan {
			if index < 0 {
				return nil
			}
			line := m.comparison.Files[r.file].Hunks[r.hunk].Lines[index]
			// Match the minimum four-column line numbers used by the renderer,
			// including unusually large source line numbers.
			digits := func(n int) int { return max(4, len(fmt.Sprint(n))) }
			gutter := digits(line.Old) + digits(line.New) + 6
			if split {
				number := line.Old
				if newSide {
					number = line.New
				}
				gutter = digits(number) + 5
			}
			return wrapSpans(safeText(line.Text), cellWidth-gutter)
		}
		leftWidth := width
		if r.kind == 'd' {
			leftWidth = (width - 1) / 2
		}
		left := spans(r.line, leftWidth, r.kind == 'd', false)
		var right []codeSpan
		if r.kind == 'd' {
			right = spans(r.rightLine, width-1-leftWidth, true, true)
		}
		for i := 0; i < max(len(left), len(right)); i++ {
			part := r
			part.continued = i > 0
			if i < len(left) {
				part.leftSpan = left[i]
			}
			if i < len(right) {
				part.rightSpan = right[i]
			}
			wrapped = append(wrapped, part)
		}
	}
	return wrapped
}

func (m *Model) visibleCode(code string, span codeSpan, width int) string {
	if m.wrapLines {
		return ansi.Cut(code, span.start, span.end)
	}
	return ansi.Cut(code, m.xOffset, m.xOffset+width)
}

func (m *Model) toggleWrap() {
	m.wrapLines = !m.wrapLines
	m.xOffset, m.dragging = 0, ""
	m.rebuildKeepingPosition()
	if m.lineSelecting {
		m.revealCommentLine()
	}
}

func (m *Model) scrollHorizontal(delta int) {
	if !m.wrapLines {
		m.xOffset = max(0, m.xOffset+delta)
	}
}
