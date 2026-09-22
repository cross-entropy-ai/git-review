package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/rivo/uniseg"
)

type textSpan struct{ start, end int } // Terminal columns, after safeText.

type lineEdits struct {
	score    int
	old, new []textSpan
}

// Bound both line alignment and word comparisons so generated patches do
// not stall the UI. Comparisons beyond the limit keep their ordinary row backgrounds.
const inlineWorkLimit = 2_000_000

func intralineSpans(lines []diff.Line) [][]textSpan {
	result := make([][]textSpan, len(lines))
	budget := inlineWorkLimit
	for i := 0; i < len(lines); {
		if lines[i].Kind != '-' && lines[i].Kind != '+' {
			i++
			continue
		}
		var old, added []int
		for i < len(lines) {
			switch lines[i].Kind {
			case '-':
				old = append(old, i)
			case '+':
				added = append(added, i)
			case '\\': // A missing newline notice is not source text.
			default:
				goto align
			}
			i++
		}
	align:
		if len(old) == 0 || len(added) == 0 || len(old) > 4096/len(added) {
			continue
		}
		// Align similar lines in source order, allowing inserted/deleted lines
		// within a replacement block without shifting every subsequent pair.
		pairs := make([]lineEdits, len(old)*len(added))
		stride := len(added) + 1
		scores := make([]int, (len(old)+1)*stride)
		for a := len(old) - 1; a >= 0; a-- {
			for b := len(added) - 1; b >= 0; b-- {
				pair := compareLineText(lines[old[a]].Text, lines[added[b]].Text, &budget)
				pairs[a*len(added)+b] = pair
				scores[a*stride+b] = max(scores[(a+1)*stride+b], scores[a*stride+b+1], pair.score+scores[(a+1)*stride+b+1])
			}
		}
		for a, b := 0, 0; a < len(old) && b < len(added); {
			pair := pairs[a*len(added)+b]
			if pair.score > 0 && scores[a*stride+b] == pair.score+scores[(a+1)*stride+b+1] {
				result[old[a]], result[added[b]] = pair.old, pair.new
				a++
				b++
			} else if scores[(a+1)*stride+b] >= scores[a*stride+b+1] {
				a++
			} else {
				b++
			}
		}
	}
	return result
}

// Keep identifiers and numbers whole, including Unicode letters and combining
// marks. Punctuation and whitespace remain separate tokens so an unchanged
// delimiter does not get highlighted along with a neighboring word.
func textWords(text string) []string {
	var parts []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			parts = append(parts, word.String())
			word.Reset()
		}
	}
	g := uniseg.NewGraphemes(safeText(text))
	for g.Next() {
		part := g.Str()
		r, _ := utf8.DecodeRuneInString(part)
		if r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			word.WriteString(part)
		} else {
			flush()
			parts = append(parts, part)
		}
	}
	flush()
	return parts
}

func compareLineText(old, added string, budget *int) lineEdits {
	// Charge linear work as well as LCS cells, including for very long lines.
	if len(old)+len(added) > min(*budget, 16<<10) {
		return lineEdits{}
	}
	*budget -= len(old) + len(added)
	a, b := textWords(old), textWords(added)
	if len(a) == 0 || len(b) == 0 {
		return lineEdits{}
	}
	prefix := 0
	for prefix < min(len(a), len(b)) && a[prefix] == b[prefix] {
		prefix++
	}
	aEnd, bEnd := len(a), len(b)
	for aEnd > prefix && bEnd > prefix && a[aEnd-1] == b[bEnd-1] {
		aEnd--
		bEnd--
	}
	n, m := aEnd-prefix, bEnd-prefix
	if n+1 > *budget/(m+1) {
		return lineEdits{}
	}
	*budget -= (n + 1) * (m + 1)
	stride := m + 1
	lcs := make([]int, (n+1)*stride)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[prefix+i] == b[prefix+j] {
				lcs[i*stride+j] = 1 + lcs[(i+1)*stride+j+1]
			} else {
				lcs[i*stride+j] = max(lcs[(i+1)*stride+j], lcs[i*stride+j+1])
			}
		}
	}
	score := 2000 * (prefix + len(a) - aEnd + lcs[0]) / (len(a) + len(b))
	if score < 500 { // Unrelated replacements only need the row background.
		return lineEdits{}
	}
	oldChanged, newChanged := make([]bool, len(a)), make([]bool, len(b))
	for i, j := 0, 0; i < n || j < m; {
		switch {
		case i < n && j < m && a[prefix+i] == b[prefix+j]:
			i++
			j++
		case i < n && (j == m || lcs[(i+1)*stride+j] >= lcs[i*stride+j+1]):
			oldChanged[prefix+i] = true
			i++
		default:
			newChanged[prefix+j] = true
			j++
		}
	}
	return lineEdits{score: score, old: changedSpans(a, oldChanged), new: changedSpans(b, newChanged)}
}

func changedSpans(parts []string, changed []bool) []textSpan {
	var spans []textSpan
	column := 0
	for i, part := range parts {
		end := column + ansi.StringWidth(part)
		if changed[i] && end > column {
			if len(spans) > 0 && spans[len(spans)-1].end == column {
				spans[len(spans)-1].end = end
			} else {
				spans = append(spans, textSpan{column, end})
			}
		}
		column = end
	}
	return spans
}

func highlightSpans(code, background string, spans []textSpan) string {
	if len(spans) == 0 {
		return code
	}
	var out strings.Builder
	previous := 0
	for _, span := range spans {
		out.WriteString(ansi.Cut(code, previous, span.start))
		out.WriteString(colorCode(48, background))
		out.WriteString(ansi.Cut(code, span.start, span.end))
		out.WriteString("\x1b[49m")
		previous = span.end
	}
	out.WriteString(ansi.Cut(code, previous, ansi.StringWidth(code)))
	return out.String()
}

// Cache syntax and intraline colors together; search and scrolling are applied
// afterwards by both layouts, so changing the viewport does no diff work.
func (m *Model) highlightedHunk(fileIndex, hunkIndex int) []string {
	key := highlightKey{file: fileIndex, hunk: hunkIndex}
	if cached, ok := m.highlights[key]; ok {
		return cached
	}
	file := m.comparison.Files[fileIndex]
	file.Hunks = file.Hunks[hunkIndex : hunkIndex+1]
	code := highlightFile(file, m.color, m.palette.syntaxStyle)[0]
	if m.color {
		for i, spans := range intralineSpans(file.Hunks[0].Lines) {
			background := m.palette.addedInlineBackground
			if file.Hunks[0].Lines[i].Kind == '-' {
				background = m.palette.deletedInlineBackground
			}
			code[i] = highlightSpans(code[i], background, spans)
		}
	}
	m.highlights[key] = code
	return code
}
