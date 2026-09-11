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
	title := "  " + m.bold(m.ink(accent, "git review")) + m.ink(muted, "  /  "+safeText(filepath.Base(c.Root)))
	if c.Root == "" {
		title = "  " + m.bold(m.ink(accent, "git review"))
	}
	if m.loading {
		title += m.ink(accent, "  refreshing…")
	}
	out := []string{m.controlRow(0, title)}

	stats := fmt.Sprintf("%d files", len(c.Files)) + "  " + m.stats(c.Added, c.Deleted)
	branches := m.ink(accent, safeText(c.Base)) + m.ink(muted, " → ") + m.bold(safeText(c.Head))
	progress := fmt.Sprintf(" %d/%d viewed ", m.viewedCount(), len(c.Files))
	if m.width >= 100 {
		filled := 0
		if len(c.Files) > 0 {
			filled = 10 * m.viewedCount() / len(c.Files)
		}
		progress = m.ink(green, strings.Repeat("━", filled)) + m.ink(border, strings.Repeat("━", 10-filled)) + progress
	}
	left := "  " + stats
	if m.width >= 90 {
		left = "  " + branches + m.ink(border, "  │  ") + stats
	}
	out = append(out, m.surface(foreground, background, fit(left, m.width-ansi.StringWidth(progress))+progress, m.width))
	out = append(out, m.controlRow(2, ""))

	if m.help {
		out = append(out, m.helpView(m.height-5)...)
	} else {
		sideTitle := fmt.Sprintf(" FILES · %d ", len(m.visible))
		diffTitle := " DIFF "
		if len(m.visible) > 0 {
			diffTitle += "· " + safeText(c.Files[m.selected].Path) + " "
		}
		top := m.panelEdge(diffTitle, m.width-g.diffX, !m.fileFocus, true)
		if g.sideWidth > 0 {
			top = m.panelEdge(sideTitle, g.sideWidth, m.fileFocus, true) + m.surface(foreground, background, " ", 1) + top
		}
		out = append(out, top)
		side := m.sidebar(g)
		for y := 0; y < g.bodyHeight; y++ {
			content := m.surface(foreground, background, "", g.diffWidth)
			if m.offset+y < len(m.rows) {
				content = m.renderRow(m.rows[m.offset+y], g.diffWidth)
			}
			if len(m.rows) == 0 {
				content = m.emptyRow(y, g.diffWidth)
			}
			rail := m.scrollRail(y, g.bodyHeight, len(m.rows), m.offset, !m.fileFocus)
			line := m.surface(border, background, "│", 1) + content + rail
			if g.sideWidth > 0 {
				line = side[y] + m.surface(foreground, background, " ", 1) + line
			}
			out = append(out, line)
		}
		bottom := m.panelEdge("", m.width-g.diffX, !m.fileFocus, false)
		if g.sideWidth > 0 {
			bottom = m.panelEdge("", g.sideWidth, m.fileFocus, false) + m.surface(foreground, background, " ", 1) + bottom
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
	out = append(out, m.surface(muted, background, fit(status, m.width-ansi.StringWidth(position))+position, m.width))
	out = append(out, m.controlRow(m.height-1, ""))
	return strings.Join(out, "\n")
}

func (m *Model) stats(added, deleted int) string {
	return m.ink(green, fmt.Sprintf("+%d", added)) + " " + m.ink(red, fmt.Sprintf("−%d", deleted))
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
			out.WriteString(m.surface(foreground, background, prefix, gap))
			prefix = ""
		}
		label, fg, bg := button.label, muted, panel
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
				fg, bg = accent, selection
			}
		}
		if button.key == "?" || button.key == "v" {
			fg = accent
		}
		out.WriteString(m.surface(fg, bg, label, button.width))
		x = button.x + button.width
	}
	if x < m.width {
		out.WriteString(m.surface(foreground, background, prefix, m.width-x))
	}
	return out.String()
}

func (m *Model) panelEdge(title string, width int, active, top bool) string {
	left, right := "╰", "╯"
	if top {
		left, right = "╭", "╮"
	}
	color := border
	if active {
		color = accent
	}
	inside := strings.Repeat("─", max(0, width-2))
	if title != "" {
		title = ansi.Truncate(title, max(0, width-4), "…")
		inside = "─" + m.bold(title) + strings.Repeat("─", max(0, width-3-ansi.StringWidth(title)))
	}
	return m.surface(color, background, left+inside+right, width)
}

func (m *Model) scrollRail(y, height, total, offset int, active bool) string {
	glyph, color := "│", border
	if total > height {
		thumb := max(1, height*height/total)
		start := min(height-thumb, offset*(height-thumb)/max(1, total-height))
		if y >= start && y < start+thumb {
			glyph, color = "┃", muted
			if active {
				color = accent
			}
		}
	}
	return m.surface(color, background, glyph, 1)
}

func (m *Model) sidebar(g geometry) []string {
	if g.sideWidth == 0 {
		return nil
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
			box = m.ink(green, "[✓]")
		}
		bg := panel
		if index == m.selected {
			cursor, bg = m.ink(accent, "›"), selection
		}
		name := cursor + m.ink(accent, fold) + " " + box + " " + safeText(path.Base(file.Path))
		if index == m.selected {
			name = m.bold(name)
		}
		result = append(result, m.surface(foreground, bg, name, width))
		directory := path.Dir(file.Path)
		if directory == "." {
			directory = "root"
		}
		stats := m.stats(file.Added, file.Deleted)
		if file.Binary {
			stats = m.ink(muted, "binary")
		}
		detail := fit("       "+safeText(directory), max(0, width-ansi.StringWidth(stats)-1)) + stats + " "
		result = append(result, m.surface(muted, bg, detail, width))
	}
	for len(result) < g.bodyHeight {
		result = append(result, m.surface(foreground, panel, "", width))
	}
	for y := range result {
		rail := m.scrollRail(y, g.bodyHeight, len(m.visible)*2, m.sideOffset*2, m.fileFocus)
		result[y] = m.surface(border, background, "│", 1) + result[y] + rail
	}
	return result
}

func (m *Model) renderRow(row row, width int) string {
	file := m.comparison.Files[row.file]
	switch row.kind {
	case 'f':
		fold, box := "▾", " [ ] Viewed "
		if m.collapsed[file.Path] {
			fold = "▸"
		}
		if m.viewed[file.Path] {
			box = m.ink(green, " [✓] Viewed ")
		}
		name := safeText(file.Path)
		if file.OldPath != "" {
			name = safeText(file.OldPath) + " → " + name
		}
		stats := m.stats(file.Added, file.Deleted)
		if file.Binary {
			stats = m.ink(muted, "binary")
		}
		right := " " + m.ink(muted, file.Status) + " " + stats + " " + fit(box, viewedButtonWidth)
		left := " " + m.ink(accent, fold) + " " + m.bold(name)
		return m.surface(foreground, raised, fit(left, max(1, width-ansi.StringWidth(right)))+right, width)
	case 'h':
		return m.surface(muted, panel, " "+safeText(row.text), width)
	case 'm':
		return m.surface(muted, background, "   "+safeText(row.text), width)
	case 'l':
		line := file.Hunks[row.hunk].Lines[row.line]
		key := highlightKey{file: row.file, hunk: row.hunk}
		if _, ok := m.highlights[key]; !ok {
			file.Hunks = file.Hunks[row.hunk : row.hunk+1]
			m.highlights[key] = highlightFile(file, m.color)[0]
		}
		code := m.highlights[key][row.line]
		oldNumber, newNumber := "", ""
		if line.Old > 0 {
			oldNumber = fmt.Sprint(line.Old)
		}
		if line.New > 0 {
			newNumber = fmt.Sprint(line.New)
		}
		bg, signColor := background, muted
		if line.Kind == '+' {
			bg, signColor = addedBackground, green
		}
		if line.Kind == '-' {
			bg, signColor = deletedBackground, red
		}
		gutter := m.ink(muted, fmt.Sprintf("%4s %4s ", oldNumber, newNumber)) + m.ink(signColor, string(line.Kind)) + m.ink(border, " │ ")
		available := max(0, width-ansi.StringWidth(gutter))
		code = ansi.Cut(code, m.xOffset, m.xOffset+available)
		return m.surface(foreground, bg, gutter+fit(code, available), width)
	}
	return m.surface(foreground, background, "", width)
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
			text = "Your branch matches its merge base."
		}
	}
	text = strings.Repeat(" ", max(0, (width-ansi.StringWidth(text))/2)) + text
	return m.surface(muted, background, text, width)
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
		"  Toolbar          Click Filter, Collapse, Expand, Refresh, Help",
		"", "  KEYBOARD",
		"  Tab              Switch focus between files and diff",
		"  j / k · ↑ / ↓    Scroll diff, or select a file in Files",
		"  n / p            Next / previous file",
		"  Space / Enter    Fold / unfold the selected file",
		"  v                Toggle viewed; fold and advance when viewed",
		"  C / E            Collapse / expand all filtered files",
		"  /                Filter paths; Esc clears the filter",
		"  Ctrl+D / Ctrl+U  Page down / up (also PgDn / PgUp)",
		"  g / G            Top / bottom (also Home / End)",
		"  [ / ]            Previous / next hunk",
		"  h / l · ← / →    Horizontal scroll; 0 resets",
		"  r                Reload branches and diff",
		"  ?                Toggle help",
		"  q / Ctrl+C       Quit (q closes help first)", "",
		"  Viewed progress is saved locally for this exact comparison.",
		"  Working tree and staged changes are excluded.",
	}
	if !m.persist {
		lines[len(lines)-2] = "  Viewed progress is stored in memory only (--no-state)."
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
		result[i] = m.surface(foreground, background, line, m.width)
	}
	return result
}
