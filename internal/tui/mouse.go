package tui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action == tea.MouseActionRelease {
		m.dragging = ""
		return nil
	}
	if m.width < 45 || m.height < 12 {
		return nil
	}
	g := m.layout()
	if msg.Action == tea.MouseActionMotion {
		if m.dragging != "" && msg.Button == tea.MouseButtonLeft {
			m.dragScroll(msg.Y)
		}
		return nil
	}
	if msg.X < 0 || msg.X >= m.width || msg.Y < 0 || msg.Y >= m.height {
		return nil
	}
	if tea.MouseEvent(msg).IsWheel() {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelLeft {
			delta = -3
		}
		if m.help {
			m.helpOffset = min(max(0, m.helpOffset+delta), max(0, len(m.helpLines())-(m.height-5)))
		} else if msg.Y >= contentTop && msg.Y < contentTop+g.bodyHeight {
			if g.sideWidth > 0 && msg.X < g.sideWidth {
				m.sideOffset += delta
				m.clampSidebar()
			} else if msg.X >= g.diffX {
				if msg.Shift || msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
					m.xOffset = max(0, m.xOffset+delta*4)
				} else {
					m.scroll(delta)
				}
			}
		}
		return nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return nil
	}
	for _, button := range m.controls() {
		if !button.contains(msg.X, msg.Y) {
			continue
		}
		if button.key == "/" && !m.help {
			m.filtering, m.message = true, ""
			return nil
		}
		if m.filtering && button.key != "esc" && button.key != "enter" {
			m.filtering = false
		}
		return m.activate(button.key)
	}
	if m.help {
		return nil
	}
	if msg.Y == contentTop-1 {
		m.fileFocus = g.sideWidth > 0 && msg.X < g.sideWidth
		return nil
	}
	if msg.Y < contentTop || msg.Y >= contentTop+g.bodyHeight {
		return nil
	}
	m.filtering, m.message = false, ""
	if g.sideWidth > 0 && msg.X == g.sideWidth-1 {
		m.dragging = "files"
		m.dragScroll(msg.Y)
		return nil
	}
	if msg.X == m.width-1 {
		m.dragging = "diff"
		m.fileFocus = false
		m.dragScroll(msg.Y)
		return nil
	}
	if g.sideWidth > 0 && msg.X > 0 && msg.X < g.sideWidth-1 {
		item := (msg.Y - contentTop) / m.sidebarRowHeight()
		index := m.sideOffset + item
		if item >= m.sidebarCapacity() || index >= m.sidebarCount() {
			return nil
		}
		if m.treeMode {
			entry := m.treeRows[index]
			m.selectTree(index)
			if entry.file < 0 {
				m.toggleDirectory(index)
			} else {
				x := msg.X - treeIndent(entry, g.sideWidth-2)
				switch {
				case x == 2:
					m.toggleFold()
				case x >= 4 && x <= 6:
					m.toggleViewed(false)
				}
			}
			return nil
		}
		m.selected, m.fileFocus, m.xOffset = m.visible[index], true, 0
		m.jumpSelected()
		if (msg.Y-contentTop)%2 == 0 {
			switch {
			case msg.X == 2:
				m.toggleFold()
			case msg.X >= 4 && msg.X <= 6:
				m.toggleViewed(false)
			}
		}
		return nil
	}
	if msg.X > g.diffX && msg.X < m.width-1 {
		index := m.offset + msg.Y - contentTop
		if index >= len(m.rows) {
			return nil
		}
		row := m.rows[index]
		m.selected, m.fileFocus = row.file, false
		m.ensureSelectedVisible()
		if row.kind == 'f' {
			x := msg.X - g.diffX - 1
			buttonEnd := g.diffWidth - fileFrameInset
			if x >= buttonEnd-viewedButtonWidth && x < buttonEnd {
				m.toggleViewed(false)
			} else {
				m.toggleFold()
			}
		}
	}
	return nil
}

func (m *Model) activate(key string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "tab":
		msg.Type = tea.KeyTab
	case "esc":
		msg.Type = tea.KeyEsc
	case "enter":
		msg.Type = tea.KeyEnter
	case " ":
		msg.Type = tea.KeySpace
	}
	_, cmd := m.Update(msg)
	return cmd
}

func (m *Model) dragScroll(y int) {
	position := min(max(0, y-contentTop), m.bodyHeight()-1)
	track := max(1, m.bodyHeight()-1)
	if m.dragging == "files" {
		m.sideOffset = position * max(0, m.sidebarCount()-m.sidebarCapacity()) / track
		m.clampSidebar()
	} else {
		m.offset = position * max(0, len(m.rows)-m.bodyHeight()) / track
		m.selectAtOffset()
	}
}
