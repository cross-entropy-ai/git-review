package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type alert struct{ title, detail string }

const alertOKLabel = " Enter OK "

func (m *Model) hasAlert() bool { return len(m.alerts) > 0 }

func (m *Model) showAlert(title, detail string) {
	// Preserve intentional line breaks, but never allow backend text to inject
	// terminal controls. Keep alerts separate from transient status messages.
	lines := strings.Split(detail, "\n")
	for i, line := range lines {
		lines[i] = safeText(line)
	}
	notice := alert{safeText(title), strings.Join(lines, "\n")}
	m.alerts = append(m.alerts, notice)
	m.message = notice.title + ": " + strings.ReplaceAll(notice.detail, "\n", " ")
	m.dragging = ""
}

func (m *Model) acknowledgeAlert() {
	if m.hasAlert() {
		notice := m.alerts[0]
		if m.message == notice.title+": "+strings.ReplaceAll(notice.detail, "\n", " ") {
			m.message = ""
		}
		m.alerts[0] = alert{}
		m.alerts = m.alerts[1:]
	}
	m.alertOffset = 0
}

func (m *Model) alertWidth() int { return min(76, max(1, m.width-6)) }

func (m *Model) alertLines() []string {
	if !m.hasAlert() {
		return nil
	}
	return strings.Split(ansi.Wrap(m.alerts[0].detail, max(1, m.alertWidth()-6), ""), "\n")
}

func (m *Model) alertLayout() modalGeometry {
	width := m.alertWidth()
	height := min(20, max(1, m.height-4), max(8, len(m.alertLines())+5))
	return modalGeometry{(m.width - width) / 2, (m.height - height) / 2, width, height}
}

func (m *Model) alertCapacity() int { return max(1, m.alertLayout().height-5) }

func (m *Model) clampAlert() {
	m.alertOffset = min(max(0, m.alertOffset), max(0, len(m.alertLines())-m.alertCapacity()))
}

func (m *Model) alertKey(msg tea.KeyMsg) {
	if msg.Paste {
		return
	}
	switch msg.String() {
	case "enter", "esc":
		m.acknowledgeAlert()
	case "j", "down":
		m.alertOffset++
	case "k", "up":
		m.alertOffset--
	case "pgdown", "ctrl+d":
		m.alertOffset += m.alertCapacity()
	case "pgup", "ctrl+u":
		m.alertOffset -= m.alertCapacity()
	}
	m.clampAlert()
}

func (m *Model) overlayAlert(screen []string) []string {
	g := m.alertLayout()
	width := g.width - 2
	lines := m.alertLines()
	start := min(m.alertOffset, max(0, len(lines)-m.alertCapacity()))
	frame := []string{m.tintedModalEdge("! "+m.alerts[0].title, g.width, true, m.palette.red, m.palette.red)}
	appendLine := func(text, bg string) {
		body := m.ink(m.palette.red, "│") + fit(text, width) + m.ink(m.palette.red, "│")
		frame = append(frame, m.surfaceWithBackground(m.palette.foreground, bg, body, g.width))
	}
	appendLine("", m.palette.modal.background)
	for i := 0; i < m.alertCapacity(); i++ {
		line := ""
		if start+i < len(lines) {
			line = "  " + lines[start+i]
		}
		appendLine(line, m.palette.modal.background)
	}
	hint := ""
	if len(lines) > m.alertCapacity() {
		hint = fmt.Sprintf("  %d–%d/%d · ↑/↓ scroll", start+1, min(start+m.alertCapacity(), len(lines)), len(lines))
	} else if len(m.alerts) > 1 {
		hint = fmt.Sprintf("  %d alerts waiting for confirmation", len(m.alerts))
	}
	appendLine(m.ink(m.palette.modal.muted, hint), m.palette.modal.background)
	button := m.surfaceWithBackground(m.palette.modal.background, m.palette.red, m.bold(alertOKLabel), len(alertOKLabel))
	footer := strings.Repeat(" ", max(0, (width-len(alertOKLabel))/2)) + button + m.ink(m.palette.modal.muted, "  Esc close")
	appendLine(footer, m.palette.modal.header)
	frame = append(frame, m.tintedModalEdge("", g.width, false, m.palette.red, m.palette.red))
	return m.overlayFrame(screen, g, frame)
}

func (m *Model) alertMouse(msg tea.MouseMsg) {
	g := m.alertLayout()
	if msg.X < g.x || msg.X >= g.x+g.width || msg.Y < g.y || msg.Y >= g.y+g.height {
		return
	}
	if tea.MouseEvent(msg).IsWheel() {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelLeft {
			delta = -3
		}
		m.alertOffset += delta
		m.clampAlert()
		return
	}
	buttonX := g.x + 1 + max(0, (g.width-2-len(alertOKLabel))/2)
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft &&
		msg.Y == g.y+g.height-2 && msg.X >= buttonX && msg.X < buttonX+len(alertOKLabel) {
		m.acknowledgeAlert()
	}
}
