// Package tui implements the keyboard-driven review interface.
package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
	"github.com/cross-entropy-ai/git-review/internal/review"
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
	comparison *gitdiff.Comparison
	err        error
	options    *gitdiff.Options
}

type highlightKey struct {
	file int
	hunk int
}

type Model struct {
	comparison *gitdiff.Comparison
	options    gitdiff.Options
	color      bool
	palette    palette
	persist    bool
	viewed     map[string]bool
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

func New(c *gitdiff.Comparison, opts gitdiff.Options, color, persist bool, theme Theme) *Model {
	// Auto is a startup choice; refreshes and toggles use the resolved scope.
	if opts.Mode == gitdiff.ModeAuto {
		opts.Mode = gitdiff.ModeCommitted
		if c.WorkingTree {
			opts.Mode = gitdiff.ModeWorkingTree
		}
	}
	m := &Model{comparison: c, options: opts, color: color, persist: persist, palette: paletteFor(theme), width: 100, height: 30, treeClosed: make(map[string]bool)}
	m.install(c)
	return m
}

func (m *Model) install(c *gitdiff.Comparison) {
	previousViewed := m.viewed
	sameSnapshot := m.comparison != nil && m.comparison.HeadOID == c.HeadOID && m.comparison.MergeBase == c.MergeBase
	m.comparison = c
	m.viewed = make(map[string]bool)
	m.collapsed = make(map[string]bool)
	m.highlights = make(map[highlightKey][]string)
	m.selected, m.offset, m.xOffset = 0, 0, 0
	m.sideOffset, m.dragging = 0, ""
	if m.persist {
		viewed, err := review.Load(review.Path(c))
		if err != nil {
			m.message = "Could not restore progress: " + safeText(err.Error())
		} else {
			m.viewed = viewed
		}
	} else if sameSnapshot && previousViewed != nil {
		m.viewed = previousViewed
	}
	for path, viewed := range m.viewed {
		m.collapsed[path] = viewed
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
			if msg.options != nil {
				m.options = *msg.options
			}
			m.install(msg.comparison)
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
				m.toggleViewed(true)
			}
		case "]":
			m.moveHunk(1)
		case "[":
			m.moveHunk(-1)
		case "m":
			opts := m.options
			if opts.Mode == gitdiff.ModeWorkingTree {
				opts.Mode = gitdiff.ModeCommitted
			} else {
				opts.Mode = gitdiff.ModeWorkingTree
			}
			return m, m.reload(opts)
		case "r":
			return m, m.reload(m.options)
		}
	}
	return m, nil
}

// Keep the displayed scope and its options together until the new load succeeds.
// The saved refs survive a visit to working-tree mode and are reused on return.
func (m *Model) reload(opts gitdiff.Options) tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	m.message = "Loading " + modeName(opts.Mode) + "…"
	return func() tea.Msg {
		loadOpts := opts
		if loadOpts.Mode == gitdiff.ModeWorkingTree {
			loadOpts.Base, loadOpts.Head = "", "HEAD"
		}
		c, err := gitdiff.Load(context.Background(), loadOpts)
		return loadedMsg{comparison: c, err: err, options: &opts}
	}
}

func modeName(mode gitdiff.Mode) string {
	if mode == gitdiff.ModeWorkingTree {
		return "Working tree"
	}
	return "Committed"
}

func (m *Model) modeLabel() string {
	return " m Mode: " + modeName(m.options.Mode) + " "
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

func (m *Model) toggleViewed(advance bool) {
	if len(m.visible) == 0 {
		return
	}
	path := m.comparison.Files[m.selected].Path
	viewed := !m.viewed[path]
	m.viewed[path], m.collapsed[path] = viewed, viewed
	if m.persist {
		if err := review.Save(review.Path(m.comparison), m.viewed); err != nil {
			m.message = "Progress kept in memory; save failed: " + safeText(err.Error())
		}
	}
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
