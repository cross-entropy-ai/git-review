package tui

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

// Pair contiguous changes in source order. Newline notices stay attached to
// their source line, and unmatched additions/deletions get an empty other side.
func splitRows(file, hunk int, lines []gitdiff.Line) []row {
	type entry struct{ line, note int }
	var rows []row
	appendRow := func(left, right int) {
		rows = append(rows, row{kind: 'd', file: file, hunk: hunk, line: left, rightLine: right})
	}
	for i := 0; i < len(lines); {
		if lines[i].Kind != '-' && lines[i].Kind != '+' {
			appendRow(i, i)
			i++
			continue
		}
		var old, new []entry
		for i < len(lines) && (lines[i].Kind == '-' || lines[i].Kind == '+') {
			kind := lines[i].Kind
			e := entry{line: i, note: -1}
			i++
			if i < len(lines) && lines[i].Kind == '\\' {
				e.note = i
				i++
			}
			if kind == '-' {
				old = append(old, e)
			} else {
				new = append(new, e)
			}
		}
		for j := 0; j < max(len(old), len(new)); j++ {
			left, right := entry{-1, -1}, entry{-1, -1}
			if j < len(old) {
				left = old[j]
			}
			if j < len(new) {
				right = new[j]
			}
			appendRow(left.line, right.line)
			if left.note >= 0 || right.note >= 0 {
				appendRow(left.note, right.note)
			}
		}
	}
	return rows
}

func (m *Model) toggleSplit() {
	var anchor row
	hasAnchor := m.offset < len(m.rows)
	if hasAnchor {
		anchor = m.rows[m.offset]
		if anchor.kind == 'd' && anchor.line < 0 {
			anchor.line = anchor.rightLine
		}
	}
	m.splitMode = !m.splitMode
	m.dragging = ""
	m.rebuild()
	if !hasAnchor {
		return
	}
	for i, candidate := range m.rows {
		if candidate.file != anchor.file || candidate.hunk != anchor.hunk {
			continue
		}
		match := candidate.kind == anchor.kind && candidate.text == anchor.text
		if anchor.kind == 'l' || anchor.kind == 'd' {
			match = (candidate.kind == 'l' || candidate.kind == 'd') &&
				(candidate.line == anchor.line || (candidate.kind == 'd' && candidate.rightLine == anchor.line))
		}
		if match {
			m.offset = i
			break
		}
	}
	m.clampOffset()
}

func (m *Model) renderSplitCell(row row, index int, newSide bool, width int) string {
	if index < 0 {
		return m.surface(m.palette.foreground, "", width)
	}
	file := m.comparison.Files[row.file]
	line := file.Hunks[row.hunk].Lines[index]
	key := highlightKey{file: row.file, hunk: row.hunk}
	if _, ok := m.highlights[key]; !ok {
		file.Hunks = file.Hunks[row.hunk : row.hunk+1]
		m.highlights[key] = highlightFile(file, m.color, m.palette.syntaxStyle)[0]
	}
	number := line.Old
	if newSide {
		number = line.New
	}
	lineNumber := ""
	if number > 0 {
		lineNumber = fmt.Sprint(number)
	}
	fg, bg := m.palette.muted, ""
	switch line.Kind {
	case '-':
		fg, bg = m.palette.red, m.palette.deletedBackground
	case '+':
		fg, bg = m.palette.green, m.palette.addedBackground
	}
	gutter := m.ink(fg, fmt.Sprintf("%4s %c │ ", lineNumber, line.Kind))
	available := max(0, width-ansi.StringWidth(gutter))
	code := ansi.Cut(m.highlights[key][index], m.xOffset, m.xOffset+available)
	return m.surfaceWithBackground(m.palette.foreground, bg, gutter+fit(code, available), width)
}
