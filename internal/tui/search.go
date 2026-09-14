package tui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type searchMatch struct {
	file, hunk, line int
	start, end       int // Display columns in sanitized code, before horizontal scrolling.
}

// Both text inputs consume shortcuts as text, including bracketed paste.
func editQuery(query string, msg tea.KeyMsg) string {
	switch msg.String() {
	case "backspace", "ctrl+h":
		runes := []rune(query)
		if len(runes) > 0 {
			return string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		return ""
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			return query + string(msg.Runes)
		}
	}
	return query
}

func (m *Model) openSearch() {
	m.lineSelecting, m.rangeSelecting = false, false
	m.searching, m.fileFocus, m.dragging = true, false, ""
	m.searchAnchor = row{file: m.selected}
	if m.offset < len(m.rows) && m.rows[m.offset].file == m.selected {
		m.searchAnchor = m.rows[m.offset]
		if m.searchAnchor.kind == 'd' && m.searchAnchor.line < 0 {
			m.searchAnchor.line = m.searchAnchor.rightLine
		}
	}
}

func (m *Model) searchKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "enter":
		m.searching = false
	case "esc":
		m.searching, m.search = false, ""
		m.updateSearch(false)
	default:
		query := editQuery(m.search, msg)
		if query != m.search {
			m.search, m.message = query, ""
			m.updateSearch(true)
		}
	}
}

// Match literal text without case sensitivity, retaining display columns for
// Unicode, tabs, and escaped terminal controls. Count individual occurrences.
func matchColumns(text, query string) [][2]int {
	if query == "" {
		return nil
	}
	source, needle := []rune(text), []rune(query)
	var matches [][2]int
	previous, column := 0, 0
	for i := 0; i+len(needle) <= len(source); i++ {
		found := true
		for j, r := range needle {
			if unicode.ToLower(source[i+j]) != unicode.ToLower(r) {
				found = false
				break
			}
		}
		if found {
			column += ansi.StringWidth(safeText(string(source[previous:i])))
			end := column + ansi.StringWidth(safeText(string(source[i:i+len(needle)])))
			matches = append(matches, [2]int{column, end})
			previous, column = i+len(needle), end
			i += len(needle) - 1
		}
	}
	return matches
}

func (m *Model) updateSearch(jump bool) {
	m.matches, m.matchIndex = nil, -1
	if m.search == "" {
		return
	}
	for fi, file := range m.comparison.Files {
		for hi, hunk := range file.Hunks {
			for li, line := range hunk.Lines {
				if line.Kind != ' ' && line.Kind != '+' && line.Kind != '-' {
					continue
				}
				for _, span := range matchColumns(line.Text, m.search) {
					m.matches = append(m.matches, searchMatch{fi, hi, li, span[0], span[1]})
				}
			}
		}
	}
	if len(m.matches) == 0 {
		return
	}
	m.matchIndex = 0
	if jump {
		a := m.searchAnchor
		for i, match := range m.matches {
			if match.file > a.file || (match.file == a.file && (match.hunk > a.hunk || (match.hunk == a.hunk && match.line >= a.line))) {
				m.matchIndex = i
				break
			}
		}
		m.jumpMatch()
	}
}

func (m *Model) nextMatch(delta int) {
	if len(m.matches) == 0 {
		return
	}
	m.matchIndex = (m.matchIndex + delta + len(m.matches)) % len(m.matches)
	m.jumpMatch()
}

func (m *Model) jumpMatch() {
	if m.matchIndex < 0 || m.matchIndex >= len(m.matches) {
		return
	}
	match := m.matches[m.matchIndex]
	m.selected, m.fileFocus = match.file, false
	path := m.comparison.Files[match.file].Path
	if m.collapsed[path] {
		m.collapsed[path] = false
		m.rebuild()
	}
	for i, r := range m.rows {
		if r.file == match.file && r.hunk == match.hunk &&
			((r.kind == 'l' && r.line == match.line) || (r.kind == 'd' && (r.line == match.line || r.rightLine == match.line))) {
			m.offset = max(0, i-m.bodyHeight()/2)
			m.clampOffset()
			break
		}
	}
	contentWidth := m.layout().diffWidth - 2*fileFrameInset
	available := contentWidth - 14
	if m.splitMode {
		available = (contentWidth-1)/2 - 9
	}
	if match.start < m.xOffset || match.end > m.xOffset+max(1, available) {
		m.xOffset = max(0, match.start-4)
	}
	m.ensureSelectedVisible()
}

func (m *Model) searchStatus() string {
	if m.search == "" {
		return ""
	}
	if len(m.matches) == 0 {
		return "No matches"
	}
	return fmt.Sprintf("%d/%d matches", m.matchIndex+1, len(m.matches))
}

func (m *Model) currentMatch(file, hunk, line int) bool {
	if m.matchIndex < 0 || m.matchIndex >= len(m.matches) {
		return false
	}
	match := m.matches[m.matchIndex]
	return match.file == file && match.hunk == hunk && match.line == line
}

func (m *Model) highlightSearch(code string, file, hunk, line int) string {
	if !m.color || m.search == "" {
		return code
	}
	text := m.comparison.Files[file].Hunks[hunk].Lines[line].Text
	var out strings.Builder
	previous := 0
	for _, span := range matchColumns(text, m.search) {
		out.WriteString(ansi.Cut(code, previous, span[0]))
		fg, bg := m.palette.foreground, m.palette.selectionBackground
		if m.currentMatch(file, hunk, line) && span[0] == m.matches[m.matchIndex].start {
			fg, bg = m.palette.headerBackground, m.palette.accent
		}
		// Strip syntax colors inside a match so it stays legible on its background.
		out.WriteString(m.surfaceWithBackground(fg, bg,
			m.bold(ansi.Strip(ansi.Cut(code, span[0], span[1]))), span[1]-span[0]))
		previous = span[1]
	}
	out.WriteString(ansi.Cut(code, previous, ansi.StringWidth(code)))
	return out.String()
}
