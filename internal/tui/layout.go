package tui

import "github.com/charmbracelet/x/ansi"

const (
	contentTop        = 4
	viewedButtonWidth = 12
	fileFrameInset    = 1
)

// Geometry is shared by rendering and mouse hit testing, including narrow views.
type geometry struct {
	sideWidth  int
	diffX      int
	diffWidth  int
	bodyHeight int
}

func (m *Model) layout() geometry {
	g := geometry{bodyHeight: m.bodyHeight()}
	if m.width >= 90 {
		g.sideWidth = min(38, m.width/3)
		g.diffX = g.sideWidth + 1
	}
	g.diffWidth = m.width - g.diffX - 2
	return g
}

type control struct {
	x, y, width int
	label, key  string
}

func (c control) contains(x, y int) bool {
	return y == c.y && x >= c.x && x < c.x+c.width
}

func (m *Model) controls() []control {
	var controls []control
	right := func(y int, buttons ...control) {
		x := m.width - 1
		for i := len(buttons) - 1; i >= 0; i-- {
			button := buttons[i]
			button.width = ansi.StringWidth(button.label)
			x -= button.width
			button.x, button.y = x, y
			controls = append(controls, button)
			x--
		}
	}
	if m.width >= 70 {
		right(0, control{label: " r Refresh ", key: "r"}, control{label: " ? Help ", key: "?"})
	} else {
		right(0, control{label: " ? Help ", key: "?"})
	}
	searchWidth := m.width - 2
	if m.width >= 90 {
		right(2, control{label: " C Collapse ", key: "C"}, control{label: " E Expand ", key: "E"})
		searchWidth = m.width - 26
	}
	controls = append(controls, control{x: 1, y: 2, width: searchWidth, key: "/"})
	buttons := []control{{label: " ? Help ", key: "?"}, {label: " q Quit ", key: "q"}, {label: " Tab Focus ", key: "tab"}, {label: " Space Fold ", key: " "}, {label: " v Viewed ", key: "v"}, {label: " / Filter ", key: "/"}}
	treeLabel := " t Tree "
	if m.treeMode {
		treeLabel = " t List "
	}
	buttons = append(buttons, control{label: treeLabel, key: "t"})
	if m.help {
		buttons = []control{{label: " Esc Close help ", key: "esc"}}
	} else if m.filtering {
		buttons = []control{{label: " Enter Apply ", key: "enter"}, {label: " Esc Clear ", key: "esc"}}
	}
	x := 1
	for _, button := range buttons {
		button.width = ansi.StringWidth(button.label)
		if x+button.width > m.width-1 {
			break
		}
		button.x, button.y = x, m.height-1
		controls = append(controls, button)
		x += button.width + 1
	}
	return controls
}

func (m *Model) sidebarRowHeight() int {
	if m.treeMode {
		return 1
	}
	return 2
}

func (m *Model) sidebarCount() int {
	if m.treeMode {
		return len(m.treeRows)
	}
	return len(m.visible)
}

func (m *Model) sidebarCapacity() int { return max(1, m.bodyHeight()/m.sidebarRowHeight()) }

func (m *Model) clampSidebar() {
	m.sideOffset = min(max(0, m.sideOffset), max(0, m.sidebarCount()-m.sidebarCapacity()))
}

func (m *Model) ensureSelectedVisible() {
	if m.treeMode {
		m.revealTreeFile()
		m.ensureSidebarIndexVisible(m.treeCursor)
		return
	}
	for i, file := range m.visible {
		if file != m.selected {
			continue
		}
		m.ensureSidebarIndexVisible(i)
		break
	}
	m.clampSidebar()
}

func (m *Model) ensureSidebarIndexVisible(index int) {
	if index < m.sideOffset {
		m.sideOffset = index
	}
	if index >= m.sideOffset+m.sidebarCapacity() {
		m.sideOffset = index - m.sidebarCapacity() + 1
	}
	m.clampSidebar()
}
