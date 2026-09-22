package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) commentLineStyle(file, hunk, line int, side, marker, bg string) (string, string) {
	l := m.comparison.Files[file].Hunks[hunk].Lines[line]
	n := l.New
	if side == "old" {
		n = l.Old
	}
	if n <= 0 || l.Kind == '\\' {
		return marker, bg
	}
	path := m.comparison.Files[file].Path
	for _, c := range m.notes.items {
		if c.Outdated {
			continue
		}
		if c.StartSide != "" && c.StartSide != c.Side {
			if c.Path == path && (side == c.StartSide && n == c.Start || side == c.Side && n == c.End) {
				marker = "●"
			}
			continue
		}
		if c.Path == path && c.Side == side && n >= c.Start && n <= c.End {
			marker = "●"
			break
		}
	}
	if m.lineSelecting {
		ref := m.commentLine
		start, end := m.lineNumber(ref), m.lineNumber(ref)
		if m.rangeSelecting {
			start = min(start, m.lineNumber(m.rangeAnchor))
			end = max(end, m.lineNumber(m.rangeAnchor))
		}
		if ref.file == file && ref.hunk == hunk && ref.side == side && n >= start && n <= end {
			marker, bg = "▸", m.palette.selectionBackground
		}
	}
	return marker, bg
}

func (m *Model) commentStatus() string {
	if !m.lineSelecting {
		return ""
	}
	return "  " + commentLocation(m.selectedComment()) + " · c Comment · Shift+↑/↓ Range · Esc Cancel"
}

type commentListRow struct {
	index int
	part  int // 0: file divider, 1: comment location, 2: body preview
}

// Rendering and mouse selection use the same rows, including file dividers.
// Start every viewport with a divider so scrolling retains the file context.
func (m *Model) commentListRows() []commentListRow {
	indices := m.visibleComments()
	if len(indices) == 0 {
		return nil
	}
	capacity := m.modalLayout().height - 3
	start, used := max(0, m.commentPosition()), 3
	for start > 0 {
		cost := 2
		if m.notes.items[indices[start-1]].Path != m.notes.items[indices[start]].Path {
			cost++
		}
		if used+cost > capacity {
			break
		}
		used += cost
		start--
	}
	var rows []commentListRow
	path := ""
	for pos := start; pos < len(indices); pos++ {
		index := indices[pos]
		c := m.notes.items[index]
		divider := pos == start || c.Path != path
		cost := 2
		if divider {
			cost++
		}
		if len(rows)+cost > capacity {
			break
		}
		if divider {
			rows = append(rows, commentListRow{index: index})
		}
		rows = append(rows, commentListRow{index: index, part: 1}, commentListRow{index: index, part: 2})
		path = c.Path
	}
	return rows
}

func (m *Model) commentFooter() []control {
	if m.commentSaving {
		return []control{{label: " Waiting for GitHub… "}}
	}
	switch m.commentModal {
	case "edit":
		if m.syncComments() {
			return []control{{label: " Ctrl+S GitHub ", key: "ctrl+s"}, {label: " Esc Cancel ", key: "esc"}}
		}
		return []control{{label: " Ctrl+S Save ", key: "ctrl+s"}, {label: " Esc Cancel ", key: "esc"}}
	case "export":
		return []control{{label: " Enter Export & open ", key: "enter"}, {label: " Esc Cancel ", key: "esc"}}
	case "delete":
		return []control{{label: " Enter Delete ", key: "enter"}, {label: " Esc Keep ", key: "esc"}}
	default:
		scope := " Tab All files "
		if m.commentAllFiles {
			scope = " Tab Current file "
		}
		buttons := []control{{label: scope, key: "tab"}}
		if m.commentPosition() < 0 {
			return append(buttons, control{label: " x Export all ", key: "x"}, control{label: " Esc Close ", key: "esc"})
		}
		buttons = append(buttons, control{label: " Enter Edit ", key: "enter"})
		if m.syncComments() {
			buttons = append(buttons, control{label: " a Reply ", key: "a"})
		}
		label := " r Resolve "
		if len(m.notes.items) > 0 && m.notes.items[m.commentIndex].Resolved {
			label = " r Unresolve "
		}
		buttons = append(buttons, control{label: label, key: "r"})
		return append(buttons, control{label: " d Delete ", key: "d"}, control{label: " x Export all ", key: "x"}, control{label: " Esc Close ", key: "esc"})
	}
}

func (m *Model) overlayComments(screen []string) []string {
	g := m.modalLayout()
	width, capacity := g.width-2, g.height-3
	title := "Comments"
	var lines []string
	switch m.commentModal {
	case "edit":
		title = "Add comment"
		if m.commentDraft.ID != "" {
			title = "Edit comment"
		}
		if m.syncComments() {
			title = "Add comment on GitHub"
			if m.commentDraft.RemoteID > 0 {
				title = "Edit GitHub comment · @" + safeText(m.commentDraft.Author)
			} else if m.commentDraft.ReplyTo > 0 {
				title = fmt.Sprintf("Reply on GitHub · thread #%d", m.commentDraft.ReplyTo)
			}
		}
		lines = append(lines, " "+m.bold(m.ink(m.palette.accent, commentLocation(m.commentDraft))))
		code := strings.Split(m.commentDraft.Code, "\n")[0]
		lines = append(lines, m.ink(m.palette.modal.muted, " "+safeText(code)), m.ink(m.palette.modal.border, strings.Repeat("─", width)))
		// Sanitize per line so intentional newlines survive rendering.
		clean := func(s string) string {
			parts := strings.Split(s, "\n")
			for i := range parts {
				parts[i] = safeText(parts[i])
			}
			return strings.Join(parts, "\n")
		}
		text := clean(string(m.noteInput[:m.noteCursor]))
		before := ansi.Hardwrap(text+"▏", max(1, width-2), true)
		cursorRow := strings.Count(before, "\n")
		wrapped := strings.Split(ansi.Hardwrap(text+"▏"+clean(string(m.noteInput[m.noteCursor:])), max(1, width-2), true), "\n")
		rows := max(1, capacity-4)
		start := max(0, cursorRow-rows+1)
		for i := 0; i < rows; i++ {
			line := ""
			if start+i < len(wrapped) {
				line = " " + wrapped[start+i]
			}
			lines = append(lines, line)
		}
		lines = append(lines, m.ink(m.palette.modal.muted, fmt.Sprintf(" Enter newline · %d characters", len(m.noteInput))))
	case "export":
		title = "Export Markdown"
		lines = append(lines, fmt.Sprintf(" %d comments · opens in your default editor", len(m.notes.items)), " Directory or filename (blank = temporary directory):")
		text := safeText(string(m.noteInput[:m.noteCursor])) + "▏" + safeText(string(m.noteInput[m.noteCursor:]))
		cursor := ansi.StringWidth(safeText(string(m.noteInput[:m.noteCursor])))
		start := max(0, cursor-width+5)
		lines = append(lines, " > "+ansi.Cut(text, start, start+width-4), "")
		base := "current directory"
		if m.comparison.Root != "" {
			base = m.comparison.Root
		}
		lines = append(lines, m.ink(m.palette.modal.muted, " Relative to: "+safeText(base)), m.ink(m.palette.modal.muted, " Directories get a unique .md file; existing files are kept."))
	case "delete":
		title = "Delete comment?"
		c := m.notes.items[m.commentIndex]
		if m.syncComments() && c.RemoteID > 0 {
			title = "Delete comment from GitHub?"
		}
		lines = append(lines, " "+commentLocation(c), "")
		for _, paragraph := range strings.Split(c.Body, "\n") {
			for _, line := range strings.Split(ansi.Hardwrap(safeText(paragraph), max(1, width-2), true), "\n") {
				lines = append(lines, " "+line)
			}
		}
	default:
		indices := m.visibleComments()
		scope := "Current file"
		if m.commentAllFiles {
			scope = "All files"
		}
		title = fmt.Sprintf("Comments · %s · %d · g Jump to line", scope, len(indices))
		for _, row := range m.commentListRows() {
			i := row.index
			c := m.notes.items[i]
			if row.part == 0 {
				label := " ─ " + safeText(c.Path) + " "
				label = ansi.Truncate(label, width-1, "…")
				label += strings.Repeat("─", max(0, width-ansi.StringWidth(label)))
				lines = append(lines, m.surfaceWithBackground(m.palette.accent, m.palette.modal.header, m.bold(label), width))
				continue
			}
			location := strings.TrimPrefix(commentLocation(c), safeText(c.Path)+" · ")
			if c.RemoteID > 0 {
				prefix := "@" + safeText(c.Author)
				if c.ReplyTo > 0 {
					prefix = "↳ " + prefix
				}
				location = prefix + " · " + location
			} else if m.syncComments() {
				location = "Local draft · " + location
				if c.PendingBody != "" {
					location = "Sync uncertain · " + location
				}
			}
			if c.DraftBody != "" {
				location = "Unsynced edit · " + location
			}
			if c.Resolved {
				location = "✓ Resolved · " + location
			}
			label := "   " + location
			body := "   " + safeText(strings.ReplaceAll(c.Body, "\n", " "))
			if i == m.commentIndex {
				label = m.surfaceWithBackground(m.palette.accent, m.palette.modal.selection, m.bold(" › "+location), width)
				body = m.surfaceWithBackground(m.palette.foreground, m.palette.modal.selection, body, width)
			}
			if row.part == 1 {
				lines = append(lines, label)
			} else {
				lines = append(lines, body)
			}
		}
		if len(indices) == 0 {
			if !m.commentAllFiles {
				lines = append(lines, " No comments in this file. Press Tab to view all files.")
			} else {
				lines = append(lines, " No comments yet. Press Esc, then c to add one.")
			}
		}
	}
	frame := []string{m.modalEdge(title, g.width, true)}
	appendLine := func(line, bg string) {
		frame = append(frame, m.surfaceWithBackground(m.palette.foreground, bg,
			m.ink(m.palette.modal.border, "│")+fit(line, width)+m.ink(m.palette.modal.border, "│"), g.width))
	}
	for i := 0; i < capacity; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		appendLine(line, m.palette.modal.background)
	}
	footer := ""
	for _, button := range m.commentFooter() {
		footer += m.bold(m.ink(m.palette.accent, button.label))
	}
	appendLine(footer, m.palette.modal.header)
	frame = append(frame, m.modalEdge("", g.width, false))
	return m.overlayFrame(screen, g, frame)
}

func (m *Model) commentMouse(msg tea.MouseMsg) tea.Cmd {
	if m.commentSaving {
		return nil
	}
	g := m.modalLayout()
	if msg.X <= g.x || msg.X >= g.x+g.width-1 || msg.Y <= g.y || msg.Y >= g.y+g.height-1 {
		return nil
	}
	if tea.MouseEvent(msg).IsWheel() {
		if m.commentModal == "list" {
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			m.moveCommentSelection(delta)
		}
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	if msg.Y == g.y+g.height-2 {
		x := g.x + 1
		for _, button := range m.commentFooter() {
			if msg.X >= x && msg.X < x+len(button.label) {
				msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(button.key)}
				return m.commentKey(msg)
			}
			x += len(button.label)
		}
	} else if m.commentModal == "list" {
		index := msg.Y - g.y - 1
		if rows := m.commentListRows(); index < len(rows) && rows[index].part != 0 {
			m.commentIndex = rows[index].index
		}
	}
	return nil
}
