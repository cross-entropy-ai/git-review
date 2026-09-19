package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

func markedText(text string, spans []textSpan) []string {
	var result []string
	for _, span := range spans {
		result = append(result, ansi.Cut(safeText(text), span.start, span.end))
	}
	return result
}

func TestIntralineCharacters(t *testing.T) {
	for _, tc := range []struct {
		name, old, added string
		wantOld, wantNew []string
	}{
		{"number", "timeout = 10", "timeout = 30", []string{"1"}, []string{"3"}},
		{"multiple edits", `connect("dev", 1234)`, `connect("PROD", 5678)`, []string{"dev", "1234"}, []string{"PROD", "5678"}},
		{"insertion", "call(value)", "call(new_value)", nil, []string{"new_"}},
		{"deletion", "call(old_value)", "call(value)", []string{"old_"}, nil},
		{"unicode", "名称 = \"你好👍🏻\"", "名称 = \"您好👍🏽\"", []string{"你", "👍🏻"}, []string{"您", "👍🏽"}},
		{"combining mark", "label = cafe\u0301", "label = cafe\u0300", []string{"e\u0301"}, []string{"e\u0300"}},
		{"tab and control", "\tvalue = '\x1b'", "\tvalue = 'x'", []string{"\\u001b"}, []string{"x"}},
		{"identical", "no change", "no change", nil, nil},
		{"unrelated", "abcdef", "uvwxyz", nil, nil},
		{"empty", "", "new", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := inlineWorkLimit
			got := compareLineText(tc.old, tc.added, &budget)
			if old, added := markedText(tc.old, got.old), markedText(tc.added, got.new); !reflect.DeepEqual(old, tc.wantOld) || !reflect.DeepEqual(added, tc.wantNew) {
				t.Fatalf("marked old/new = %q / %q, want %q / %q", old, added, tc.wantOld, tc.wantNew)
			}
		})
	}
}

func TestIntralineAlignsReplacementBlocks(t *testing.T) {
	lines := []diff.Line{
		{Kind: '-', Text: "timeout = 10"},
		{Kind: '-', Text: "retries = 3"},
		{Kind: '\\', Text: "No newline at end of file"},
		{Kind: '+', Text: "// new configuration"},
		{Kind: '+', Text: "timeout = 30"},
		{Kind: '+', Text: "retries = 5"},
		{Kind: '\\', Text: "No newline at end of file"},
		{Kind: ' ', Text: "context"},
		{Kind: '-', Text: "timeout = 50"},
		{Kind: ' ', Text: "context"},
		{Kind: '+', Text: "timeout = 60"},
	}
	spans := intralineSpans(lines)
	for i, want := range map[int][]string{0: {"1"}, 1: {"3"}, 4: {"3"}, 5: {"5"}} {
		if got := markedText(lines[i].Text, spans[i]); !reflect.DeepEqual(got, want) {
			t.Errorf("line %d: marked %q, want %q", i, got, want)
		}
	}
	for _, i := range []int{2, 3, 6, 7, 8, 9, 10} {
		if len(spans[i]) > 0 {
			t.Errorf("unpaired/context/notice line %d has inline highlights", i)
		}
	}
}

func TestIntralineWorkIsBounded(t *testing.T) {
	budget := 1000
	if got := compareLineText(strings.Repeat("a", 200), strings.Repeat("b", 200), &budget); got.score != 0 {
		t.Fatal("expensive comparison did not fall back to row colors")
	}
	budget = 1
	if got := compareLineText("same", "same", &budget); got.score != 0 || budget < 0 {
		t.Fatal("comparison exceeded its work budget")
	}
	lines := make([]diff.Line, 200)
	for i := range lines {
		lines[i] = diff.Line{Kind: '-', Text: "value = 10"}
		if i >= 100 {
			lines[i].Kind = '+'
		}
	}
	for _, spans := range intralineSpans(lines) {
		if len(spans) != 0 {
			t.Fatal("oversized block should use row colors")
		}
	}
}

func TestIntralinePreservesSyntaxAndText(t *testing.T) {
	m := sampleModel(true)
	t.Cleanup(m.Close)
	m.comparison.Files[0].Hunks[0].Lines = []diff.Line{
		{Kind: '-', Text: "\treturn \"你好\", 1234"},
		{Kind: '+', Text: "\treturn \"您好\", 5678"},
	}
	file := m.comparison.Files[0]
	syntax := highlightFile(file, true, m.palette.syntaxStyle)[0]
	highlighted := m.highlightedHunk(0, 0)
	for i, code := range highlighted {
		if ansi.Strip(code) != safeText(file.Hunks[0].Lines[i].Text) {
			t.Fatal("intraline highlighting changed source text")
		}
		// Background spans can split syntax tokens, but must retain their color.
		for _, token := range []string{"return", "好"} {
			if !strings.Contains(code, token) || !strings.Contains(syntax[i], token) {
				t.Fatalf("missing syntax token %q", token)
			}
		}
		if !strings.Contains(code, "\x1b[38;2;") || !strings.Contains(code, "\x1b[48;2;") {
			t.Fatal("syntax and diff colors must coexist")
		}
	}
}

func TestIntralineRendering(t *testing.T) {
	for _, theme := range []Theme{DarkTheme, LightTheme} {
		for _, color := range []bool{false, true} {
			m := sampleModel(color)
			t.Cleanup(m.Close)
			m.palette = paletteFor(theme)
			m.comparison.Files[0].Path = "config.txt"
			m.comparison.Files[0].Hunks[0].Lines = []diff.Line{
				{Kind: '-', Text: "prefix old suffix", Old: 1},
				{Kind: '+', Text: "prefix NEW suffix", New: 1},
			}
			for _, split := range []bool{false, true} {
				for _, offset := range []int{0, 8} {
					m.xOffset = offset
					for index, token := range []string{"old", "NEW"} {
						r := row{kind: 'l', file: 0, hunk: 0, line: index}
						var rendered string
						if split {
							rendered = m.renderSplitCell(r, index, index == 1, 40)
						} else {
							rendered = m.renderRowContent(r, 40)
						}
						if ansi.StringWidth(rendered) != 40 {
							t.Fatalf("rendered width = %d", ansi.StringWidth(rendered))
						}
						if !color {
							if strings.Contains(rendered, "\x1b") {
								t.Fatal("no-color output contains ANSI styles")
							}
							continue
						}
						inlineBG, rowBG := m.palette.deletedInlineBackground, m.palette.deletedBackground
						if index == 1 {
							inlineBG, rowBG = m.palette.addedInlineBackground, m.palette.addedBackground
						}
						if offset > 0 {
							token = token[1:]
						}
						if got := backgroundAt(t, rendered, token); got != inlineBG {
							t.Errorf("%s split=%v offset=%d: changed text background = %s, want %s", theme, split, offset, got, inlineBG)
						}
						if got := backgroundAt(t, rendered, " suffix"); got != rowBG {
							t.Errorf("highlight leaked into suffix: %s", got)
						}
					}
				}
			}
			if color {
				m.search = "E"
				m.updateSearch(false)
				m.xOffset = 0
				rendered := m.renderRowContent(row{kind: 'l', line: 1}, 60)
				if got := backgroundAt(t, rendered, "E"); got != m.palette.selectionBackground && got != m.palette.accent {
					t.Errorf("search does not override inline background: %s", got)
				}
				if got := backgroundAt(t, rendered, "W"); got != m.palette.addedInlineBackground {
					t.Errorf("inline background not restored after search: %s", got)
				}
			}
		}
	}
}
