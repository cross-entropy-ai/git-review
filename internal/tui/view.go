package tui

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func fit(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func (m *Model) View() string {
	if m.width < 45 || m.height < 12 {
		return fit("Resize to at least 45 × 12.  q quit", m.width)
	}
	g := m.layout()
	c := m.comparison
	title := "  " + m.bold(m.ink(m.palette.accent, "git review")) + m.ink(m.palette.muted, "  /  "+safeText(filepath.Base(c.Root)))
	if c.Root == "" {
		title = "  " + m.bold(m.ink(m.palette.accent, "git review"))
	}
	if m.loading {
		title += m.ink(m.palette.accent, "  refreshing…")
	}
	out := []string{m.controlRow(0, title)}

	stats := fmt.Sprintf("%d files", len(c.Files)) + "  " + m.stats(c.Added, c.Deleted)
	branches := m.ink(m.palette.accent, safeText(c.Base)) + m.ink(m.palette.muted, " → ") + m.bold(safeText(c.Head))
	progress := fmt.Sprintf(" %d/%d viewed ", m.viewedCount(), len(c.Files))
	if m.width >= 100 {
		filled := 0
		if len(c.Files) > 0 {
			filled = 10 * m.viewedCount() / len(c.Files)
		}
		progress = m.ink(m.palette.green, strings.Repeat("━", filled)) + m.ink(m.palette.border, strings.Repeat("━", 10-filled)) + progress
	}
	left := "  " + stats
	if m.width >= 90 {
		left = "  " + branches + m.ink(m.palette.border, "  │  ") + stats
	}
	out = append(out, m.surface(m.palette.foreground, fit(left, m.width-rightInset-ansi.StringWidth(progress))+progress, m.width))
	out = append(out, m.controlRow(2, ""))

	if m.help {
		out = append(out, m.helpView(m.height-5)...)
	} else {
		sideTitle := fmt.Sprintf(" FILES · %d ", len(m.visible))
		if m.treeMode {
			sideTitle = fmt.Sprintf(" TREE · %d ", len(m.visible))
		}
		diffTitle := " DIFF "
		if m.splitMode {
			diffTitle = " DIFF · SPLIT (old │ new) "
		}
		if len(m.visible) > 0 {
			diffTitle += "· " + safeText(c.Files[m.selected].Path) + " "
		}
		top := m.panelEdge(diffTitle, m.width-g.diffX, !m.fileFocus, true)
		if g.sideWidth > 0 {
			top = m.panelEdge(sideTitle, g.sideWidth, m.fileFocus, true) + m.surface(m.palette.foreground, " ", 1) + top
		}
		out = append(out, top)
		side := m.sidebar(g)
		for y := 0; y < g.bodyHeight; y++ {
			content := m.surface(m.palette.foreground, "", g.diffWidth)
			if m.offset+y < len(m.rows) {
				content = m.renderRow(m.rows[m.offset+y], g.diffWidth)
			}
			if len(m.rows) == 0 {
				content = m.emptyRow(y, g.diffWidth)
			}
			rail := m.scrollRail(y, g.bodyHeight, len(m.rows), m.offset, !m.fileFocus)
			line := m.surface(m.palette.border, "│", 1) + content + rail
			if g.sideWidth > 0 {
				line = side[y] + m.surface(m.palette.foreground, " ", 1) + line
			}
			out = append(out, line)
		}
		bottom := m.panelEdge("", m.width-g.diffX, !m.fileFocus, false)
		if g.sideWidth > 0 {
			bottom = m.panelEdge("", g.sideWidth, m.fileFocus, false) + m.surface(m.palette.foreground, " ", 1) + bottom
		}
		out = append(out, bottom)
	}

	status := "  Click to navigate · scroll either pane · drag the scrollbar"
	if m.filtering {
		status = "  Type a path to filter · Enter apply · Esc clear"
	}
	if m.help {
		status = "  Mouse wheel or j/k to scroll help"
	}
	if !m.help && !m.filtering && len(m.visible) > 0 {
		status = "  " + safeText(c.Files[m.selected].Path)
		if m.xOffset > 0 {
			status += fmt.Sprintf(" · column +%d", m.xOffset)
		}
		if m.viewedCount() == len(c.Files) {
			status += " · All files viewed ✓"
		}
	}
	if m.message != "" {
		status = "  " + m.message
	}
	position := ""
	if !m.help && len(m.rows) > 0 {
		percent := min(100, (m.offset+g.bodyHeight)*100/len(m.rows))
		position = fmt.Sprintf(" %d%% ", percent)
	}
	out = append(out, m.surface(m.palette.muted, fit(status, m.width-rightInset-ansi.StringWidth(position))+position, m.width))
	out = append(out, m.controlRow(m.height-1, ""))
	return strings.Join(out, "\n")
}

func (m *Model) stats(added, deleted int) string {
	return m.ink(m.palette.green, fmt.Sprintf("+%d", added)) + " " + m.ink(m.palette.red, fmt.Sprintf("−%d", deleted))
}

func (m *Model) controlRow(y int, prefix string) string {
	var controls []control
	for _, button := range m.controls() {
		if button.y == y {
			controls = append(controls, button)
		}
	}
	sort.Slice(controls, func(i, j int) bool { return controls[i].x < controls[j].x })
	var out strings.Builder
	x := 0
	for _, button := range controls {
		gap := button.x - x
		if gap > 0 {
			out.WriteString(m.surface(m.palette.foreground, prefix, gap))
			prefix = ""
		}
		label, fg := button.label, m.palette.muted
		if button.key == "/" && y == 2 {
			label = " / Filter files…"
			if m.filter != "" || m.filtering {
				label = " / " + safeText(m.filter)
				if m.filtering {
					label += "▏"
				}
				label += fmt.Sprintf("  (%d matches)", len(m.visible))
			}
			if m.filtering {
				fg = m.palette.accent
			}
		}
		if button.key == "?" {
			fg = m.palette.accent
		}
		if button.key == "m" {
			out.WriteString(m.surfaceWithBackground(m.palette.accent, m.palette.selectionBackground, m.bold(label), button.width))
		} else {
			out.WriteString(m.surface(fg, label, button.width))
		}
		x = button.x + button.width
	}
	if x < m.width {
		out.WriteString(m.surface(m.palette.foreground, prefix, m.width-x))
	}
	return out.String()
}

func (m *Model) panelEdge(title string, width int, active, top bool) string {
	left, right := "╰", "╯"
	if top {
		left, right = "╭", "╮"
	}
	color := m.palette.border
	if active {
		color = m.palette.accent
	}
	inside := strings.Repeat("─", max(0, width-2))
	if title != "" {
		title = ansi.Truncate(title, max(0, width-4), "…")
		inside = "─" + m.bold(title) + strings.Repeat("─", max(0, width-3-ansi.StringWidth(title)))
	}
	return m.surface(color, left+inside+right, width)
}

func (m *Model) scrollRail(y, height, total, offset int, active bool) string {
	glyph, color := "│", m.palette.border
	if total > height {
		thumb := max(1, height*height/total)
		start := min(height-thumb, offset*(height-thumb)/max(1, total-height))
		if y >= start && y < start+thumb {
			glyph, color = "┃", m.palette.muted
			if active {
				color = m.palette.accent
			}
		}
	}
	return m.surface(color, glyph, 1)
}

func (m *Model) sidebar(g geometry) []string {
	if g.sideWidth == 0 {
		return nil
	}
	if m.treeMode {
		return m.treeSidebar(g)
	}
	width := g.sideWidth - 2
	result := make([]string, 0, g.bodyHeight)
	for item := 0; item < m.sidebarCapacity() && m.sideOffset+item < len(m.visible); item++ {
		index := m.visible[m.sideOffset+item]
		file := m.comparison.Files[index]
		fold, box, cursor := "▾", "[ ]", " "
		if m.collapsed[file.Path] {
			fold = "▸"
		}
		if m.viewed[file.Path] {
			box = m.ink(m.palette.green, "[✓]")
		}
		bg := ""
		if index == m.selected {
			cursor = m.ink(m.palette.accent, "›")
			bg = m.palette.selectionBackground
		}
		baseName := safeText(path.Base(file.Path))
		if index == m.selected {
			baseName = m.ink(m.palette.accent, baseName)
		}
		name := cursor + m.ink(m.palette.accent, fold) + " " + box + " " + baseName
		if index == m.selected {
			name = m.bold(name)
		}
		result = append(result, m.surfaceWithBackground(m.palette.foreground, bg, name, width))
		directory := path.Dir(file.Path)
		if directory == "." {
			directory = "root"
		}
		stats := m.stats(file.Added, file.Deleted)
		if file.Binary {
			stats = m.ink(m.palette.muted, "binary")
		}
		detail := fit("       "+safeText(directory), max(0, width-ansi.StringWidth(stats)-1)) + stats + " "
		result = append(result, m.surfaceWithBackground(m.palette.muted, bg, detail, width))
	}
	return m.frameSidebar(result, g)
}

func (m *Model) frameSidebar(result []string, g geometry) []string {
	width := g.sideWidth - 2
	for len(result) < g.bodyHeight {
		result = append(result, m.surface(m.palette.foreground, "", width))
	}
	for y := range result {
		rail := m.scrollRail(y, g.bodyHeight, m.sidebarCount()*m.sidebarRowHeight(), m.sideOffset*m.sidebarRowHeight(), m.fileFocus)
		result[y] = m.surface(m.palette.border, "│", 1) + result[y] + rail
	}
	return result
}

func (m *Model) renderRow(row row, width int) string {
	// Reserve the same frame columns for every file so selection never shifts
	// code, line numbers, or the Viewed button. The existing spacer closes it.
	contentWidth := width - 2*fileFrameInset
	if row.file != m.selected {
		padding := m.surface(m.palette.foreground, " ", fileFrameInset)
		return padding + m.renderRowContent(row, contentWidth) + padding
	}
	if row.kind == 's' {
		return m.surface(m.palette.accent, "┗"+strings.Repeat("━", contentWidth)+"┛", width)
	}
	left, right := "┃", "┃"
	if row.kind == 'f' {
		left, right = "┏", "┓"
	}
	return m.surface(m.palette.accent, left, fileFrameInset) +
		m.renderRowContent(row, contentWidth) +
		m.surface(m.palette.accent, right, fileFrameInset)
}

func (m *Model) renderRowContent(row row, width int) string {
	file := m.comparison.Files[row.file]
	switch row.kind {
	case 'f':
		fold, box := "▾", " [ ] Viewed "
		if m.collapsed[file.Path] {
			fold = "▸"
		}
		if m.viewed[file.Path] {
			box = m.ink(m.palette.green, " [✓] Viewed ")
		}
		name := safeText(file.Path)
		if file.OldPath != "" {
			name = safeText(file.OldPath) + " → " + name
		}
		stats := m.stats(file.Added, file.Deleted)
		if file.Binary {
			stats = m.ink(m.palette.muted, "binary")
		}
		right := " " + m.ink(m.palette.muted, file.Status) + " " + stats + " " + fit(box, viewedButtonWidth)
		bg := m.palette.headerBackground
		if row.file == m.selected {
			name = m.ink(m.palette.accent, name)
			bg = m.palette.selectionBackground
		}
		left := " " + m.ink(m.palette.accent, fold) + " " + m.bold(name)
		return m.surfaceWithBackground(m.palette.foreground, bg, fit(left, max(1, width-ansi.StringWidth(right)))+right, width)
	case 'h':
		return m.surface(m.palette.muted, " "+safeText(row.text), width)
	case 'm':
		return m.surface(m.palette.muted, "   "+safeText(row.text), width)
	case 'd':
		leftWidth := max(0, (width-1)/2)
		rightWidth := max(0, width-1-leftWidth)
		return m.renderSplitCell(row, row.line, false, leftWidth) +
			m.surface(m.palette.border, "│", 1) +
			m.renderSplitCell(row, row.rightLine, true, rightWidth)
	case 'l':
		line := file.Hunks[row.hunk].Lines[row.line]
		key := highlightKey{file: row.file, hunk: row.hunk}
		if _, ok := m.highlights[key]; !ok {
			file.Hunks = file.Hunks[row.hunk : row.hunk+1]
			m.highlights[key] = highlightFile(file, m.color, m.palette.syntaxStyle)[0]
		}
		code := m.highlights[key][row.line]
		oldNumber, newNumber := "", ""
		if line.Old > 0 {
			oldNumber = fmt.Sprint(line.Old)
		}
		if line.New > 0 {
			newNumber = fmt.Sprint(line.New)
		}
		signColor := m.palette.muted
		bg := ""
		if line.Kind == '+' {
			signColor = m.palette.green
			bg = m.palette.addedBackground
		}
		if line.Kind == '-' {
			signColor = m.palette.red
			bg = m.palette.deletedBackground
		}
		gutter := m.ink(signColor, fmt.Sprintf("%4s %4s %c │ ", oldNumber, newNumber, line.Kind))
		available := max(0, width-ansi.StringWidth(gutter))
		code = ansi.Cut(code, m.xOffset, m.xOffset+available)
		return m.surfaceWithBackground(m.palette.foreground, bg, gutter+fit(code, available), width)
	}
	return m.surface(m.palette.foreground, "", width)
}

func (m *Model) emptyRow(y, width int) string {
	center := max(0, m.bodyHeight()/2-1)
	text := ""
	if y == center {
		text = "No files match"
		if len(m.comparison.Files) == 0 {
			text = "No changes to review"
		}
		text = m.bold(text)
	}
	if y == center+1 {
		text = "Press Esc to clear the filter."
		if len(m.comparison.Files) == 0 {
			text = "The head matches its merge base."
			if m.comparison.WorkingTree {
				text = "Your working tree matches HEAD."
			}
		}
	}
	text = strings.Repeat(" ", max(0, (width-ansi.StringWidth(text))/2)) + text
	return m.surface(m.palette.muted, text, width)
}

func (m *Model) helpLines() []string {
	lines := []string{
		"", "  REVIEW SHORTCUTS", "",
		"  MOUSE",
		"  Click a file     Select it and focus the file list",
		"  Click ▾ / ▸      Fold / unfold a file",
		"  Click [ ]        Toggle viewed without advancing",
		"  Click diff title Fold / unfold; click Viewed to mark",
		"  Wheel            Scroll the pane under the pointer",
		"  Shift + wheel    Scroll code horizontally",
		"  Drag scrollbar   Jump through files or diff",
		"  Toolbar          Click Mode, Filter, Collapse, Expand, Refresh, Help",
		"", "  KEYBOARD",
		"  Tab              Switch focus between files and diff",
		"  t                Toggle flat file list / directory tree",
		"  s                Toggle inline / split diff (old left, new right)",
		"  j / k · ↑ / ↓    Scroll diff; at an edge, select adjacent file",
		"  n / p            Next / previous file",
		"  Space / Enter    Fold / unfold the selected file",
		"  v                Toggle viewed; fold and advance when viewed",
		"  C / E            Collapse / expand all filtered files",
		"  /                Filter paths; Esc clears the filter",
		"  Ctrl+D / Ctrl+U  Page down / up (also PgDn / PgUp)",
		"  g / G            Top / bottom (also Home / End)",
		"  [ / ]            Previous / next hunk",
		"  h / l · ← / →    Horizontal scroll; 0 resets",
		"  m                Toggle review mode: Working tree / Committed",
		"  r                Reload review scope and diff",
		"  ?                Toggle help",
		"  q / Ctrl+C       Quit (q closes help first)", "",
		"  TREE (with sidebar focus)",
		"  j / k · ↑ / ↓    Navigate directories and files",
		"  h / l · ← / →    Close / open directory; left goes to parent",
		"  Space / Enter    Toggle directory or file folding",
		"  Click directory  Expand / collapse its file tree",
		"  n / p            Review files in diff order in either view",
		"",
		"  Viewed progress is saved locally for this exact comparison.",
		"  Working tree and staged changes are excluded.",
	}
	if !m.persist {
		lines[len(lines)-2] = "  Viewed progress is stored in memory only (--no-state)."
	}
	if m.comparison.WorkingTree {
		lines[len(lines)-1] = "  Staged, unstaged, and untracked files are included; r refreshes."
	}
	return lines
}

func (m *Model) helpView(height int) []string {
	lines := m.helpLines()
	result := make([]string, height)
	start := min(m.helpOffset, max(0, len(lines)-height))
	for i := range result {
		line := ""
		if start+i < len(lines) {
			line = lines[start+i]
		}
		result[i] = m.surface(m.palette.foreground, line, m.width)
	}
	return result
}
