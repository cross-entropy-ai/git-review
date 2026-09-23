package tui

import "github.com/charmbracelet/x/ansi"

const (
	contentTop        = 4
	viewedButtonWidth = 12
	fileFrameInset    = 1
	rightInset        = 1
	minSidebarWidth   = 24
	minDiffWidth      = 45
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
		if m.sideWidth > 0 {
			g.sideWidth = m.clampSidebarWidth(m.sideWidth)
		}
		g.diffX = g.sideWidth + 1
	}
	g.diffWidth = m.width - g.diffX - 2
	return g
}

func (m *Model) clampSidebarWidth(width int) int {
	// Leave one column for the divider and two for the diff frame.
	return max(minSidebarWidth, min(width, m.width-minDiffWidth-3))
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
		x := m.width - rightInset
		for i := len(buttons) - 1; i >= 0; i-- {
			button := buttons[i]
			button.width = ansi.StringWidth(button.label)
			x -= button.width
			button.x, button.y = x, y
			controls = append(controls, button)
			x--
		}
	}
	left := func(y, limit int, buttons ...control) {
		x := 1
		for _, button := range buttons {
			button.width = ansi.StringWidth(button.label)
			if x+button.width > limit {
				break
			}
			button.x, button.y = x, y
			controls = append(controls, button)
			x += button.width + 1
		}
	}
	if m.lineSelecting || m.commentModal != "" {
		label := " Add comment · Select lines "
		buttons := []control{{label: " c Write ", key: "c"}, {label: " ↑/↓ Move "}, {label: " Shift+↑/↓ Range "}}
		buttons = append(buttons, control{label: " Esc Cancel ", key: "esc"})
		if m.commentModal != "" {
			label = " Comments "
			buttons = m.commentFooter()
			switch m.commentModal {
			case "edit":
				label = " Comment · Edit text "
				if !m.commentSaving {
					buttons = append(buttons, control{label: " Enter Newline ", key: "enter"})
				}
			case "export":
				label = " Export Markdown "
			case "delete":
				label = " Delete comment "
			}
		}
		right(0, control{label: label})
		left(2, m.width-rightInset, buttons...)
		if m.commentModal == "" {
			left(m.height-1, m.width-rightInset, buttons...)
		}
		return controls
	}
	mode := control{label: m.modeLabel(), key: "m"}
	if m.width >= 90 {
		right(0, mode, control{label: " r Refresh ", key: "r"}, control{label: " ? Help ", key: "?"})
	} else if m.width >= 70 {
		right(0, mode, control{label: " ? Help ", key: "?"})
	} else {
		right(0, mode)
	}
	searchWidth := m.width - 2
	commentButtons := []control{{label: " c Add comment ", key: "c"}, {label: " C Comments ", key: "C"}, {label: " x Export ", key: "x"}}
	if m.width >= 90 {
		label := " z Collapse all "
		if m.allCollapsed() {
			label = " z Expand all "
		}
		commentButtons = append(commentButtons, control{label: label, key: "z"})
	} else if m.width < 70 {
		commentButtons = commentButtons[:2]
	}
	right(2, commentButtons...)
	for _, c := range controls {
		if c.y == 2 {
			searchWidth = min(searchWidth, c.x-2)
		}
	}
	controls = append(controls, control{x: 1, y: 2, width: searchWidth, key: "/"})
	modeLabel := " s Split "
	if m.splitMode {
		modeLabel = " s Inline "
	}
	buttons := []control{
		{label: " Tab Focus ", key: "tab"},
		{label: " Space Fold ", key: " "},
		{label: " t Tree/List ", key: "t"},
		{label: modeLabel, key: "s"},
		{label: " e Edit ", key: "e"},
		{label: " v Viewed ", key: "v"},
		{label: " f Files ", key: "f"},
		{label: " / Search ", key: "/"},
	}
	footerLimit := m.width - rightInset
	if m.help || m.picking || m.hasAlert() || m.commentModal != "" {
		buttons = nil
	} else if m.searching {
		buttons = []control{{label: " Enter Done ", key: "enter"}, {label: " Esc Clear ", key: "esc"}}
	} else {
		help, quit := control{label: " ? Help ", key: "?"}, control{label: " q Quit ", key: "q"}
		right(m.height-1, help, quit)
		footerLimit -= ansi.StringWidth(help.label) + ansi.StringWidth(quit.label) + 2
	}
	left(m.height-1, footerLimit, buttons...)
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
