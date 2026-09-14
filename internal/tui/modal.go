package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type modalGeometry struct{ x, y, width, height int }

const (
	helpCloseLabel  = " Esc Close help "
	fileOpenLabel   = " Enter Open "
	fileCancelLabel = " Esc Cancel "
)

func (m *Model) modalEdge(title string, width int, top bool) string {
	return m.tintedModalEdge(title, width, top, m.palette.accent, m.palette.modal.border)
}

func (m *Model) tintedModalEdge(title string, width int, top bool, accent, border string) string {
	left, right, bg := "╰", "╯", m.palette.modal.background
	inside := strings.Repeat("─", max(0, width-2))
	if top {
		left, right, bg = "╭", "╮", m.palette.modal.header
		label := "─ " + m.bold(m.ink(accent, title)) + " "
		inside = fit(label+strings.Repeat("─", max(0, width-2-ansi.StringWidth(label))), width-2)
	}
	return m.surfaceWithBackground(border, bg, left+inside+right, width)
}

func (m *Model) styleHelpLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed != "" && trimmed == strings.ToUpper(trimmed) {
		return m.bold(m.ink(m.palette.accent, line))
	}
	// Keep wrapped prose neutral and distinguish shortcut labels from descriptions.
	label := strings.TrimLeft(line, " ")
	if gap := strings.Index(label, "  "); gap > 0 {
		end := len(line) - len(label) + gap
		return m.bold(m.ink(m.palette.accent, line[:end])) + line[end:]
	}
	return m.ink(m.palette.modal.muted, line)
}

func overlayCells(screen []string, x, y int, text string, width int) {
	if y < 0 || y >= len(screen) || x < 0 || x >= width {
		return
	}
	end := min(width, x+ansi.StringWidth(text))
	screen[y] = fit(ansi.Cut(screen[y], 0, x), x) + ansi.Cut(text, 0, end-x) + fit(ansi.Cut(screen[y], end, width), width-end)
}

func (m *Model) modalLayout() modalGeometry {
	width, height := min(96, max(1, m.width-6)), min(26, max(1, m.height-4))
	return modalGeometry{(m.width - width) / 2, (m.height - height) / 2, width, height}
}

func (m *Model) pickerCapacity() int { return max(1, m.modalLayout().height-5) }
func (m *Model) helpCapacity() int   { return max(1, m.modalLayout().height-3) }

func (m *Model) helpContent() []string {
	var lines []string
	for _, line := range m.helpLines() {
		lines = append(lines, strings.Split(ansi.Hardwrap(line, max(1, m.modalLayout().width-4), true), "\n")...)
	}
	return lines
}

// Compose the same floating frame over the existing screen for every modal.
// ANSI-aware slicing preserves cell widths and syntax styles behind the frame.
func (m *Model) overlayModal(screen []string) []string {
	if m.hasAlert() {
		return m.overlayAlert(screen)
	}
	g := m.modalLayout()
	width := g.width - 2
	title := "Help"
	var content []string
	footer := m.bold(m.ink(m.palette.accent, helpCloseLabel)) + "· ↑/↓ scroll "
	if m.help {
		lines := m.helpContent()
		start := min(m.helpOffset, max(0, len(lines)-m.helpCapacity()))
		footer += fmt.Sprintf("· %d–%d/%d", start+1, min(start+m.helpCapacity(), len(lines)), len(lines))
		for i := 0; i < m.helpCapacity(); i++ {
			line := ""
			if start+i < len(lines) {
				line = lines[start+i]
			}
			content = append(content, m.styleHelpLine(line))
		}
	} else {
		title = "Find file · fuzzy search"
		query := safeText(m.fileQuery) + "▏"
		count := fmt.Sprintf(" %d/%d ", len(m.fileMatches), len(m.comparison.Files))
		queryWidth := max(1, width-ansi.StringWidth(count)-3)
		query = m.ink(m.palette.accent, " > ") + ansi.Cut(query, max(0, ansi.StringWidth(query)-queryWidth), ansi.StringWidth(query))
		badge := m.surfaceWithBackground(m.palette.accent, m.palette.modal.selection, count, ansi.StringWidth(count))
		input := m.surfaceWithBackground(m.palette.foreground, m.palette.modal.header, fit(query, width-ansi.StringWidth(count))+badge, width)
		content = append(content, input, m.ink(m.palette.modal.border, strings.Repeat("─", width)))
		for i := 0; i < m.pickerCapacity(); i++ {
			index := m.fileOffset + i
			line := ""
			if index < len(m.fileMatches) {
				file := m.comparison.Files[m.fileMatches[index]]
				name := safeText(file.Path)
				if file.OldPath != "" {
					fg := m.palette.modal.muted
					if index == m.fileCursor {
						fg = m.palette.foreground
					}
					name += m.ink(fg, " ← "+safeText(file.OldPath))
				}
				stats := " " + m.stats(file.Added, file.Deleted) + " "
				nameWidth := max(0, width-ansi.StringWidth(stats))
				line = fit("   "+name, nameWidth) + stats
				if index == m.fileCursor {
					line = m.surfaceWithBackground(m.palette.accent, m.palette.modal.selection, fit(m.bold(" › "+name), nameWidth)+stats, width)
				}
			} else if i == 0 {
				line = m.ink(m.palette.modal.muted, " No files match")
			}
			content = append(content, line)
		}
		footer = m.bold(m.ink(m.palette.accent, fileOpenLabel)) + "·" + m.bold(m.ink(m.palette.accent, fileCancelLabel)) + "· ↑/↓ select "
	}
	frame := []string{m.modalEdge(title, g.width, true)}
	content = append(content, m.surfaceWithBackground(m.palette.modal.muted, m.palette.modal.header, footer, width))
	for _, line := range content {
		body := m.ink(m.palette.modal.border, "│") + fit(line, width) + m.ink(m.palette.modal.border, "│")
		frame = append(frame, m.surfaceWithBackground(m.palette.foreground, m.palette.modal.background, body, g.width))
	}
	frame = append(frame, m.modalEdge("", g.width, false))
	return m.overlayFrame(screen, g, frame)
}

func (m *Model) overlayFrame(screen []string, g modalGeometry, frame []string) []string {
	if m.color {
		for i, line := range screen {
			screen[i] = m.surface(m.palette.modal.backdrop, ansi.Strip(line), m.width)
		}
		shadow := m.surfaceWithBackground(m.palette.modal.shadow, m.palette.modal.shadow, "", g.width)
		for y := g.y + 1; y <= g.y+g.height; y++ {
			overlayCells(screen, g.x+2, y, shadow, m.width)
		}
	}
	for i, line := range frame {
		overlayCells(screen, g.x, g.y+i, line, m.width)
	}
	return screen
}

func (m *Model) modalMouse(msg tea.MouseMsg) {
	if msg.Action == tea.MouseActionRelease {
		m.dragging = ""
		return
	}
	g := m.modalLayout()
	if msg.X < g.x || msg.X >= g.x+g.width || msg.Y < g.y || msg.Y >= g.y+g.height {
		return
	}
	if tea.MouseEvent(msg).IsWheel() {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelLeft {
			delta = -3
		}
		if m.help {
			m.helpOffset = min(max(0, m.helpOffset+delta), max(0, len(m.helpContent())-m.helpCapacity()))
		} else {
			m.fileCursor += delta
			m.clampPicker()
		}
		return
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return
	}
	if msg.Y == g.y+g.height-2 {
		x := msg.X - g.x - 1
		if m.help {
			if x >= 0 && x < len(helpCloseLabel) {
				m.help = false
			}
		} else if x >= 0 && x < len(fileOpenLabel) {
			m.chooseFile()
		} else if x >= len(fileOpenLabel)+1 && x < len(fileOpenLabel)+1+len(fileCancelLabel) {
			m.picking = false
		}
		return
	}
	if m.picking && msg.Y >= g.y+3 && msg.Y < g.y+g.height-2 {
		index := m.fileOffset + msg.Y - g.y - 3
		if index < len(m.fileMatches) {
			m.fileCursor = index
			m.chooseFile()
		}
	}
}
