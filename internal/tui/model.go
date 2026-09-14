// Package tui implements the keyboard-driven review interface.
package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

type row struct {
	kind      byte
	file      int
	hunk      int
	line      int
	rightLine int // Split row's new-side index; -1 denotes an empty cell.
	text      string
}

type loadedMsg struct {
	snapshot *backend.Snapshot
	err      error
}

type viewedMsg struct {
	key, path string
	err       error
}

type pendingViewed struct {
	viewed, collapsed, dismissed bool
}

type highlightKey struct {
	file int
	hunk int
}

type Model struct {
	comparison *diff.Comparison
	snapshot   *backend.Snapshot
	source     backend.Backend
	mode       diff.Mode
	ctx        context.Context
	cancel     context.CancelFunc
	color      bool
	palette    palette
	viewed     map[string]bool
	dismissed  map[string]bool
	pending    map[string]pendingViewed
	collapsed  map[string]bool
	highlights map[highlightKey][]string
	visible    []int
	rows       []row
	selected   int
	offset     int
	xOffset    int
	sideOffset int
	treeMode   bool
	splitMode  bool
	treeRows   []treeEntry
	treeCursor int
	treeClosed map[string]bool
	dragging   string
	width      int
	height     int
	fileFocus  bool
	filtering  bool
	filter     string
	help       bool
	helpOffset int
	loading    bool
	message    string
}

func New(s *backend.Snapshot, source backend.Backend, color bool, theme Theme) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Model{source: source, ctx: ctx, cancel: cancel, color: color, palette: paletteFor(theme), width: 100, height: 30, treeClosed: make(map[string]bool), pending: make(map[string]pendingViewed)}
	m.install(s)
	return m
}

func (m *Model) Close() { m.cancel() }

func (m *Model) install(s *backend.Snapshot) {
	previousViewed := m.viewed
	sameSnapshot := m.snapshot != nil && m.snapshot.Key == s.Key
	m.snapshot, m.comparison, m.mode = s, s.Comparison, s.Mode
	m.viewed, m.dismissed = make(map[string]bool), make(map[string]bool)
	m.collapsed = make(map[string]bool)
	m.highlights = make(map[highlightKey][]string)
	m.selected, m.offset, m.xOffset = 0, 0, 0
	m.sideOffset, m.dragging = 0, ""
	for path, state := range s.Viewed {
		m.viewed[path] = state == backend.Viewed
		m.dismissed[path] = state == backend.Dismissed
	}
	if s.Persistence == backend.Memory && sameSnapshot && previousViewed != nil {
		m.viewed = previousViewed
	}
	for path, viewed := range m.viewed {
		m.collapsed[path] = viewed
	}
	if s.Warning != "" {
		m.message = safeText(s.Warning)
	}
	m.rebuild()
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.clampOffset()
		m.ensureSelectedVisible()
	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.message = "Review failed: " + safeText(msg.err.Error())
		} else {
			m.message = "Refreshed comparison"
			m.install(msg.snapshot)
		}
	case editorFinishedMsg:
		if msg.err != nil {
			m.message = "Editor failed: " + safeText(msg.err.Error())
		} else {
			m.message = "Editor closed; press r to refresh the comparison"
		}
	case viewedMsg:
		if msg.key != m.snapshot.Key {
			return m, nil
		}
		previous, ok := m.pending[msg.path]
		if !ok {
			return m, nil
		}
		delete(m.pending, msg.path)
		if msg.err != nil {
			m.viewed[msg.path], m.collapsed[msg.path], m.dismissed[msg.path] = previous.viewed, previous.collapsed, previous.dismissed
			m.message = "Viewed update failed for " + safeText(msg.path) + ": " + safeText(msg.err.Error())
			m.rebuild()
		} else {
			m.message = "Viewed state saved for " + safeText(msg.path)
		}
	case tea.MouseMsg:
		return m, m.mouse(msg)
	case tea.KeyMsg:
		// Terminals can deliver several ordinary keystrokes in one read.
		// Preserve their order so a leading '/' starts filtering immediately.
		if msg.Type == tea.KeyRunes && !msg.Paste && len(msg.Runes) > 1 {
			var commands []tea.Cmd
			for _, r := range msg.Runes {
				_, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if command != nil {
					commands = append(commands, command)
				}
			}
			return m, tea.Batch(commands...)
		}
		// Bracketed paste is text input, never a stream of review shortcuts.
		if msg.Paste && !m.filtering {
			return m, nil
		}
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.filtering {
			switch key {
			case "enter":
				m.filtering = false
			case "esc":
				m.filter, m.filtering = "", false
			case "backspace", "ctrl+h":
				runes := []rune(m.filter)
				if len(runes) > 0 {
					m.filter = string(runes[:len(runes)-1])
				}
			case "ctrl+u":
				m.filter = ""
			default:
				if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
					m.filter += string(msg.Runes)
				}
			}
			m.rebuild()
			m.jumpSelected()
			return m, nil
		}
		if m.help {
			switch key {
			case "?", "esc", "q":
				m.help = false
			case "j", "down", "pgdown", "ctrl+d":
				m.helpOffset = min(m.helpOffset+1, max(0, len(m.helpLines())-(m.height-5)))
			case "k", "up", "pgup", "ctrl+u":
				m.helpOffset = max(0, m.helpOffset-1)
			}
			return m, nil
		}
		m.message = ""
		switch key {
		case "q":
			if len(m.pending) > 0 {
				m.message = "Viewed updates are still saving; wait, or Ctrl+C to exit"
				return m, nil
			}
			return m, tea.Quit
		case "?":
			m.help = true
			m.helpOffset = 0
		case "tab", "shift+tab":
			m.fileFocus = !m.fileFocus
		case "t":
			m.treeMode = !m.treeMode
			m.sideOffset, m.dragging = 0, ""
			m.rebuildTree()
			m.ensureSelectedVisible()
			if m.layout().sideWidth == 0 {
				m.message = "Sidebar view changed; resize to at least 90 columns to show it"
			}
		case "s":
			m.toggleSplit()
		case "/":
			m.filtering = true
		case "esc":
			m.filter = ""
			m.rebuild()
			m.jumpSelected()
		case "j", "down":
			if m.treeFocus() {
				m.moveTree(1)
			} else if m.fileFocus {
				m.moveFile(1)
			} else {
				m.scroll(1)
			}
		case "k", "up":
			if m.treeFocus() {
				m.moveTree(-1)
			} else if m.fileFocus {
				m.moveFile(-1)
			} else {
				m.scroll(-1)
			}
		case "n":
			m.moveFile(1)
		case "p":
			m.moveFile(-1)
		case "pgdown", "ctrl+d":
			m.scroll(max(1, m.bodyHeight()/2))
		case "pgup", "ctrl+u":
			m.scroll(-max(1, m.bodyHeight()/2))
		case "g", "home":
			m.offset = 0
			m.selectAtOffset()
		case "G", "end":
			m.offset = len(m.rows)
			m.clampOffset()
			if len(m.visible) > 0 {
				m.selected = m.visible[len(m.visible)-1]
				m.ensureSelectedVisible()
			}
		case "h", "left":
			if m.treeFocus() {
				m.treeLeft()
			} else {
				m.xOffset = max(0, m.xOffset-8)
			}
		case "l", "right":
			if m.treeFocus() {
				m.treeRight()
			} else {
				m.xOffset += 8
			}
		case "0":
			m.xOffset = 0
		case " ", "enter":
			if m.treeDirectoryFocused() {
				m.toggleDirectory(m.treeCursor)
			} else {
				m.toggleFold()
			}
		case "C", "E":
			for _, i := range m.visible {
				m.collapsed[m.comparison.Files[i].Path] = key == "C"
			}
			m.rebuild()
			m.jumpSelected()
		case "v":
			if m.treeDirectoryFocused() {
				m.message = "Select a file to mark it viewed"
			} else {
				return m, m.toggleViewed(true)
			}
		case "e":
			return m, m.openEditor()
		case "]":
			m.moveHunk(1)
		case "[":
			m.moveHunk(-1)
		case "m":
			modes := m.source.Modes()
			if len(modes) < 2 {
				return m, nil
			}
			for i, mode := range modes {
				if mode == m.mode {
					return m, m.reload(modes[(i+1)%len(modes)])
				}
			}
		case "r":
			return m, m.reload(m.mode)
		}
	}
	return m, nil
}

// Mode and progress are replaced only after the backend finishes successfully.
func (m *Model) reload(mode diff.Mode) tea.Cmd {
	if m.loading {
		return nil
	}
	if len(m.pending) > 0 {
		m.message = "Wait for Viewed updates to finish before refreshing"
		return nil
	}
	m.loading = true
	m.message = "Loading " + modeName(mode) + "…"
	return func() tea.Msg {
		s, err := m.source.Load(m.ctx, mode)
		return loadedMsg{snapshot: s, err: err}
	}
}

func modeName(mode diff.Mode) string {
	switch mode {
	case diff.ModeWorkingTree:
		return "Working tree"
	case diff.ModePullRequest:
		return "GitHub PR"
	default:
		return "Committed"
	}
}

func (m *Model) modeLabel() string {
	key := " m"
	if len(m.source.Modes()) < 2 {
		key = ""
	}
	return key + " Mode: " + modeName(m.mode) + " "
}

func (m *Model) viewedBox(path string) string {
	if _, ok := m.pending[path]; ok {
		return m.ink(m.palette.accent, "[~]")
	}
	if m.viewed[path] {
		return m.ink(m.palette.green, "[✓]")
	}
	if m.dismissed[path] {
		return m.ink(m.palette.red, "[!]")
	}
	return "[ ]"
}

func (m *Model) bodyHeight() int { return max(1, m.height-7) }

func (m *Model) rebuild() {
	m.visible = nil
	m.rows = nil
	query := strings.ToLower(m.filter)
	found := false
	for i, file := range m.comparison.Files {
		if !strings.Contains(strings.ToLower(file.Path), query) && !strings.Contains(strings.ToLower(file.OldPath), query) {
			continue
		}
		m.visible = append(m.visible, i)
		found = found || i == m.selected
		m.rows = append(m.rows, row{kind: 'f', file: i})
		if !m.collapsed[file.Path] {
			for _, metadata := range file.Metadata {
				m.rows = append(m.rows, row{kind: 'm', file: i, text: metadata})
			}
			if len(file.Hunks) == 0 && len(file.Metadata) == 0 {
				m.rows = append(m.rows, row{kind: 'm', file: i, text: "No textual changes (mode, rename, or binary change)"})
			}
			for hi, hunk := range file.Hunks {
				m.rows = append(m.rows, row{kind: 'h', file: i, hunk: hi, text: hunk.Header})
				if m.splitMode {
					m.rows = append(m.rows, splitRows(i, hi, hunk.Lines)...)
					continue
				}
				for li := range hunk.Lines {
					m.rows = append(m.rows, row{kind: 'l', file: i, hunk: hi, line: li})
				}
			}
		}
		m.rows = append(m.rows, row{kind: 's', file: i})
	}
	if !found && len(m.visible) > 0 {
		m.selected = m.visible[0]
	}
	m.rebuildTree()
	m.clampOffset()
	m.ensureSelectedVisible()
}

func (m *Model) clampOffset() { m.offset = min(max(0, m.offset), max(0, len(m.rows)-m.bodyHeight())) }

func (m *Model) selectAtOffset() {
	if m.offset < len(m.rows) {
		m.selected = m.rows[m.offset].file
		m.ensureSelectedVisible()
	}
}

func (m *Model) scroll(delta int) {
	if delta == 0 || len(m.rows) == 0 {
		return
	}
	previousOffset := m.offset
	m.offset += delta
	m.clampOffset()
	if m.offset != previousOffset {
		// A file selected by clicking can be below the viewport's first row.
		// Scrolling down must not move that selection back to an earlier file.
		candidate := m.rows[m.offset].file
		if (delta > 0 && candidate > m.selected) || (delta < 0 && candidate < m.selected) {
			m.selected = candidate
			m.ensureSelectedVisible()
		}
		return
	}
	// The viewport may already be at an edge, or the whole diff may fit.
	// Continue navigating files even when there is no content left to scroll.
	direction := 1
	if delta < 0 {
		direction = -1
	}
	for i, file := range m.visible {
		if file == m.selected {
			next := m.visible[min(max(i+direction, 0), len(m.visible)-1)]
			if next == m.selected {
				return
			}
			m.selected, m.xOffset = next, 0
			m.ensureSelectedVisible()
			break
		}
	}
	for i, row := range m.rows {
		if row.kind == 'f' && row.file == m.selected {
			// Keep an already visible header in place instead of jumping it
			// to the top, which could scroll opposite to the requested direction.
			if i < m.offset {
				m.offset = i
			} else if i >= m.offset+m.bodyHeight() {
				m.offset = i - m.bodyHeight() + 1
			}
			m.clampOffset()
			break
		}
	}
}

func (m *Model) jumpSelected() {
	m.ensureSelectedVisible()
	for i, row := range m.rows {
		if row.kind == 'f' && row.file == m.selected {
			m.offset = i
			m.clampOffset()
			return
		}
	}
}

func (m *Model) moveFile(delta int) {
	for i, file := range m.visible {
		if file == m.selected {
			m.selected = m.visible[min(max(i+delta, 0), len(m.visible)-1)]
			m.xOffset = 0
			m.jumpSelected()
			return
		}
	}
}

func (m *Model) moveHunk(direction int) {
	for i := m.offset + direction; i >= 0 && i < len(m.rows); i += direction {
		if m.rows[i].kind == 'h' {
			m.offset, m.selected = i, m.rows[i].file
			m.clampOffset()
			m.ensureSelectedVisible()
			return
		}
	}
}

func (m *Model) toggleFold() {
	if len(m.visible) == 0 {
		return
	}
	path := m.comparison.Files[m.selected].Path
	m.collapsed[path] = !m.collapsed[path]
	m.rebuild()
	m.jumpSelected()
}

func (m *Model) toggleViewed(advance bool) tea.Cmd {
	if len(m.visible) == 0 {
		return nil
	}
	if m.loading {
		m.message = "Wait for the comparison to finish loading"
		return nil
	}
	path := m.comparison.Files[m.selected].Path
	if _, ok := m.pending[path]; ok {
		m.message = "Viewed update is still saving for " + safeText(path)
		return nil
	}
	previous := pendingViewed{m.viewed[path], m.collapsed[path], m.dismissed[path]}
	viewed := !m.viewed[path]
	m.viewed[path], m.collapsed[path], m.dismissed[path] = viewed, viewed, false
	m.rebuild()
	if viewed && advance {
		start := 0
		for i, file := range m.visible {
			if file == m.selected {
				start = i
				break
			}
		}
		for step := 1; step < len(m.visible); step++ {
			candidate := m.visible[(start+step)%len(m.visible)]
			if !m.viewed[m.comparison.Files[candidate].Path] {
				m.selected = candidate
				break
			}
		}
	}
	m.jumpSelected()
	if m.snapshot.Persistence == backend.Memory {
		return nil
	}
	m.pending[path] = previous
	m.message = "Saving Viewed state for " + safeText(path) + "…"
	s := m.snapshot
	return func() tea.Msg {
		err := m.source.SetViewed(m.ctx, s, path, viewed)
		return viewedMsg{key: s.Key, path: path, err: err}
	}
}

func (m *Model) viewedCount() int {
	count := 0
	for _, f := range m.comparison.Files {
		if m.viewed[f.Path] {
			count++
		}
	}
	return count
}
