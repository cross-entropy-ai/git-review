package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) paint(code, text string) string {
	if !m.color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

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
	c := m.comparison
	var out []string
	title := m.paint("1;38;5;81", " git review ") + m.paint("38;5;245", "│ ") + safeText(c.Base) + m.paint("38;5;245", " → ") + safeText(c.Head)
	if m.loading {
		title += m.paint("38;5;221", "  refreshing…")
	}
	out = append(out, m.paint("48;5;235", fit(title, m.width)))
	summary := fmt.Sprintf(" %d files  ", len(c.Files)) + m.paint("38;5;114", fmt.Sprintf("+%d", c.Added)) + " " + m.paint("38;5;210", fmt.Sprintf("-%d", c.Deleted))
	summary += fmt.Sprintf("   %d/%d viewed", m.viewedCount(), len(c.Files))
	summary += m.paint("38;5;243", "   merge-base "+shortOID(c.MergeBase)+" · committed changes")
	out = append(out, fit(summary, m.width))
	if m.filtering || m.filter != "" {
		prompt := " / " + safeText(m.filter)
		if m.filtering {
			prompt += "▏"
		}
		prompt += fmt.Sprintf("  (%d matches)", len(m.visible))
		out = append(out, m.paint("38;5;221", fit(prompt, m.width)))
	} else {
		out = append(out, m.paint("38;5;240", strings.Repeat("─", m.width)))
	}
	if m.help {
		out = append(out, m.helpView(m.height-5)...)
	} else {
		sideWidth := 0
		if m.width >= 90 {
			sideWidth = min(40, m.width/3)
		}
		diffWidth := m.width
		if sideWidth > 0 {
			diffWidth -= sideWidth + 1
		}
		side := m.sidebar(sideWidth, m.bodyHeight()+1)
		focus := " DIFF"
		if !m.fileFocus {
			focus = " ● DIFF"
		}
		if len(m.visible) > 0 {
			focus += "  " + safeText(c.Files[m.selected].Path)
		}
		diff := []string{m.paint("1;38;5;81", fit(focus, diffWidth))}
		for i := 0; i < m.bodyHeight(); i++ {
			line := ""
			if m.offset+i < len(m.rows) {
				line = m.renderRow(m.rows[m.offset+i], diffWidth)
			}
			if i == 1 && len(m.rows) == 0 {
				if len(c.Files) == 0 {
					line = "  No changes to review. Your branch matches its merge base."
				} else {
					line = "  No files match. Press Esc to clear the filter."
				}
			}
			diff = append(diff, fit(line, diffWidth))
		}
		for i, line := range diff {
			if sideWidth > 0 {
				line = side[i] + m.paint("38;5;239", "│") + line
			}
			out = append(out, line)
		}
	}
	status := m.message
	if status == "" && len(m.visible) > 0 && !m.help && !m.filtering {
		status = " " + safeText(c.Files[m.selected].Path)
		if m.xOffset > 0 {
			status += fmt.Sprintf(" · column +%d", m.xOffset)
		}
		if m.viewedCount() == len(c.Files) {
			status += " · All files viewed ✓"
		}
	}
	out = append(out, m.paint("38;5;244", fit(status, m.width)))
	status = " ? help  q quit  Tab focus  n/p file  Space fold  v viewed  / filter"
	if m.filtering {
		status = " Enter apply · Esc clear · Ctrl+U erase"
	}
	if m.help {
		status = " j/k scroll help · ? / Esc close · q close help · Ctrl+C quit"
	}
	out = append(out, m.paint("38;5;250;48;5;235", fit(status, m.width)))
	return strings.Join(out, "\n")
}

func shortOID(oid string) string {
	if len(oid) > 8 {
		return oid[:8]
	}
	return oid
}

func (m *Model) sidebar(width, height int) []string {
	if width == 0 {
		return nil
	}
	title := " FILES"
	if m.fileFocus {
		title = " ● FILES"
	}
	result := []string{m.paint("1;38;5;81", fit(title, width))}
	selectedIndex := 0
	for i, file := range m.visible {
		if file == m.selected {
			selectedIndex = i
		}
	}
	capacity := max(1, (height-1)/2)
	start := max(0, min(selectedIndex-capacity/2, len(m.visible)-capacity))
	for i := start; i < len(m.visible) && len(result)+1 < height; i++ {
		index := m.visible[i]
		file := m.comparison.Files[index]
		fold, viewed, cursor := "▾", "○", " "
		if m.collapsed[file.Path] {
			fold = "▸"
		}
		if m.viewed[file.Path] {
			viewed = "✓"
		}
		if index == m.selected {
			cursor = "›"
		}
		line := fmt.Sprintf("%s%s %s %s", cursor, fold, viewed, safeText(file.Path))
		detail := "     " + file.Status + "  " + fileStats(file)
		if index == m.selected {
			line = m.paint("1;38;5;117;48;5;237", fit(line, width))
			detail = m.paint("38;5;252;48;5;237", fit(detail, width))
		} else {
			line = fit(line, width)
			detail = m.paint("38;5;244", fit(detail, width))
		}
		result = append(result, line, detail)
	}
	for len(result) < height {
		result = append(result, strings.Repeat(" ", width))
	}
	return result
}

func (m *Model) renderRow(row row, width int) string {
	file := m.comparison.Files[row.file]
	switch row.kind {
	case 'f':
		fold, viewed := "▾", ""
		if m.collapsed[file.Path] {
			fold = "▸"
		}
		if m.viewed[file.Path] {
			viewed = "  ✓ viewed"
		}
		path := safeText(file.Path)
		if file.OldPath != "" {
			path = safeText(file.OldPath) + " → " + path
		}
		stats := "  " + file.Status + "  " + fileStats(file) + viewed + " "
		line := fit(" "+fold+" "+path, max(1, width-ansi.StringWidth(stats))) + stats
		return m.paint("1;38;5;252;48;5;237", fit(line, width))
	case 'h':
		return m.paint("38;5;110;48;5;236", fit(" "+safeText(row.text), width))
	case 'm':
		return m.paint("38;5;244", fit("   "+safeText(row.text), width))
	case 'l':
		line := file.Hunks[row.hunk].Lines[row.line]
		key := highlightKey{file: row.file, hunk: row.hunk}
		if _, ok := m.highlights[key]; !ok {
			// Only lex visible hunks, even when a file has thousands of hunks.
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
		gutter := fmt.Sprintf("%5s %5s %c ", oldNumber, newNumber, line.Kind)
		available := max(0, width-ansi.StringWidth(gutter))
		code = ansi.Cut(code, m.xOffset, m.xOffset+available)
		text := gutter + fit(code, available)
		switch line.Kind {
		case '+':
			return m.paint("38;5;151;48;5;22", text)
		case '-':
			return m.paint("38;5;217;48;5;52", text)
		case '\\':
			return m.paint("38;5;244", text)
		default:
			return m.paint("38;5;250", text)
		}
	}
	return strings.Repeat(" ", width)
}

func (m *Model) helpLines() []string {
	lines := []string{
		"", "  REVIEW SHORTCUTS", "",
		"  Tab             Switch focus between files and diff",
		"  j / k · ↑ / ↓   Scroll diff, or select a file in Files",
		"  n / p           Next / previous file",
		"  Space / Enter   Fold / unfold the selected file",
		"  v               Toggle viewed; fold and advance when viewed",
		"  C / E           Collapse / expand all filtered files",
		"  /               Filter paths (case insensitive)",
		"  Esc             Clear filter",
		"  Ctrl+D / Ctrl+U Page down / up (also PgDn / PgUp)",
		"  g / G           Top / bottom (also Home / End)",
		"  [ / ]           Previous / next hunk",
		"  h / l · ← / →   Scroll code horizontally; 0 resets",
		"  r               Reload branches and diff",
		"  ?               Toggle this help",
		"  q / Ctrl+C      Quit (q closes help first)", "",
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
		result[i] = fit(line, m.width)
	}
	return result
}
