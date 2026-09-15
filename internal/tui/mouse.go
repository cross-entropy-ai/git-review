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
	if m.hasAlert() {
		m.alertMouse(msg)
		return nil
	}
	if m.commentModal != "" {
		return m.commentMouse(msg)
	}
	if m.help || m.picking {
		return m.modalMouse(msg)
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
		if msg.Y >= contentTop && msg.Y < contentTop+g.bodyHeight {
			if g.sideWidth > 0 && msg.X < g.sideWidth {
				m.sideOffset += delta
				m.clampSidebar()
			} else if msg.X >= g.diffX {
				if msg.Shift || msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
					m.xOffset = max(0, m.xOffset+delta*4)
				} else {
					if m.lineSelecting {
						m.rangeSelecting = false
						m.moveCommentLine(delta)
					} else {
						m.scroll(delta)
					}
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
		if button.key == "" {
			return nil
		}
		if button.key == "/" {
			m.openSearch()
			m.message = ""
			return nil
		}
		if m.searching && button.key != "esc" && button.key != "enter" {
			m.searching = false
		}
		if m.lineSelecting && msg.Y != m.height-1 && button.key != "c" && button.key != "C" && button.key != "x" {
			m.lineSelecting, m.rangeSelecting = false, false
		}
		return m.activate(button.key)
	}
	if msg.Y == contentTop-1 {
		m.lineSelecting, m.rangeSelecting = false, false
		m.fileFocus = g.sideWidth > 0 && msg.X < g.sideWidth
		return nil
	}
	if msg.Y < contentTop || msg.Y >= contentTop+g.bodyHeight {
		return nil
	}
	m.searching, m.message = false, ""
	if g.sideWidth > 0 && msg.X == g.sideWidth-1 {
		m.dragging = "files"
		m.dragScroll(msg.Y)
		return nil
	}
	if msg.X == m.width-1 {
		m.lineSelecting, m.rangeSelecting = false, false
		m.dragging = "diff"
		m.fileFocus = false
		m.dragScroll(msg.Y)
		return nil
	}
	if g.sideWidth > 0 && msg.X > 0 && msg.X < g.sideWidth-1 {
		m.lineSelecting, m.rangeSelecting = false, false
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
					return m.toggleViewed(false)
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
				return m.toggleViewed(false)
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
		if row.kind == 'l' || row.kind == 'd' {
			if m.loading {
				return nil
			}
			if !m.lineSelecting {
				m.selected, m.fileFocus = row.file, false
				m.ensureSelectedVisible()
				return nil
			}
			side := ""
			if row.kind == 'd' {
				side = "old"
				if msg.X >= g.diffX+1+fileFrameInset+(g.diffWidth-2*fileFrameInset-1)/2+1 {
					side = "new"
				}
			}
			if ref, ok := m.lineAt(index, side); ok {
				m.rangeSelecting = false
				m.selectCommentLine(index, ref)
			}
			return nil
		}
		m.lineSelecting, m.rangeSelecting = false, false
		m.selected, m.fileFocus = row.file, false
		m.ensureSelectedVisible()
		if row.kind == 'f' {
			x := msg.X - g.diffX - 1
			buttonEnd := g.diffWidth - fileFrameInset
			if x >= buttonEnd-viewedButtonWidth && x < buttonEnd {
				return m.toggleViewed(false)
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
