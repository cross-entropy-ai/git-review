package tui

import (
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openPicker() {
	m.closePicker()
	m.modePicking = false
	m.picking, m.dragging, m.fileQuery = true, "", ""
	m.updateFileMatches()
	for i, file := range m.fileMatches {
		if file == m.selected {
			m.fileCursor = i
			break
		}
	}
	m.clampPicker()
}

// Fzf-style subsequence matching: each space-separated term must match.
// Prefer contiguous matches, path boundaries, and short paths; ties preserve
// comparison order. This picker does not require an external fzf executable.
func fuzzyScore(path, query string) (int, bool) {
	path = strings.ToLower(path)
	score := len([]rune(path))
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if index := strings.Index(path, term); index >= 0 {
			score += index - 100
			continue
		}
		runes := []rune(path)
		position, previous := 0, -1
		for _, want := range term {
			found := false
			for position < len(runes) {
				i := position
				position++
				if runes[i] != want {
					continue
				}
				score += (i - previous - 1) * 3
				if i == 0 || !unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1]) {
					score -= 5
				}
				previous, found = i, true
				break
			}
			if !found {
				return 0, false
			}
		}
	}
	return score, true
}

func (m *Model) updateFileMatches() {
	if m.modePicking {
		m.updateTargetMatches()
		return
	}
	type candidate struct{ file, score int }
	var candidates []candidate
	for i, file := range m.comparison.Files {
		score, ok := fuzzyScore(file.Path, m.fileQuery)
		if file.OldPath != "" {
			if oldScore, oldOK := fuzzyScore(file.OldPath, m.fileQuery); oldOK && (!ok || oldScore < score) {
				score, ok = oldScore, true
			}
		}
		if ok {
			candidates = append(candidates, candidate{i, score})
		}
	}
	if strings.TrimSpace(m.fileQuery) != "" {
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })
	}
	m.fileMatches = nil
	for _, candidate := range candidates {
		m.fileMatches = append(m.fileMatches, candidate.file)
	}
	m.fileCursor, m.fileOffset = 0, 0
}

func (m *Model) clampPicker() {
	m.fileCursor = min(max(0, m.fileCursor), max(0, len(m.fileMatches)-1))
	capacity := m.pickerCapacity()
	m.fileOffset = min(max(0, m.fileOffset), max(0, len(m.fileMatches)-capacity))
	if m.fileCursor < m.fileOffset {
		m.fileOffset = m.fileCursor
	} else if m.fileCursor >= m.fileOffset+capacity {
		m.fileOffset = m.fileCursor - capacity + 1
	}
}

func (m *Model) chooseFile() tea.Cmd {
	if m.modePicking {
		return m.chooseTarget()
	}
	if len(m.fileMatches) == 0 {
		return nil
	}
	m.selected = m.fileMatches[m.fileCursor]
	m.picking, m.fileFocus, m.xOffset = false, false, 0
	m.collapsed[m.comparison.Files[m.selected].Path] = false
	m.rebuild()
	m.jumpSelected()
	return nil
}

func (m *Model) pickerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.closePicker()
	case "enter":
		return m.chooseFile()
	case "down", "ctrl+n", "tab":
		m.fileCursor++
	case "up", "ctrl+p", "shift+tab":
		m.fileCursor--
	case "pgdown", "ctrl+d":
		m.fileCursor += m.pickerCapacity()
	case "pgup":
		m.fileCursor -= m.pickerCapacity()
	default:
		query := editQuery(m.fileQuery, msg)
		if query != m.fileQuery {
			m.fileQuery = query
			m.updateFileMatches()
		}
	}
	m.clampPicker()
	return nil
}
