package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

// safeText keeps repository content from emitting terminal control sequences.
func safeText(s string) string {
	var out strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			out.WriteString("    ")
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			fmt.Fprintf(&out, "\\u%04x", r)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// Highlight each side independently so deleted text cannot corrupt the added
// side's lexer state. Context is limited to the lines included in each hunk.
func highlightFile(file gitdiff.File, color bool, styleName string) [][]string {
	result := make([][]string, len(file.Hunks))
	lexer := lexers.Match(file.Path)
	if lexer == nil && file.OldPath != "" {
		lexer = lexers.Match(file.OldPath)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	for hi, hunk := range file.Hunks {
		result[hi] = make([]string, len(hunk.Lines))
		var oldSource, newSource strings.Builder
		for li, line := range hunk.Lines {
			clean := safeText(line.Text)
			result[hi][li] = clean
			if line.Kind == ' ' || line.Kind == '-' {
				oldSource.WriteString(clean + "\n")
			}
			if line.Kind == ' ' || line.Kind == '+' {
				newSource.WriteString(clean + "\n")
			}
		}
		// Large generated hunks remain fully browsable without costly lexing.
		if !color || oldSource.Len()+newSource.Len() > 256<<10 {
			continue
		}
		oldLines := highlightSource(lexer, oldSource.String(), styleName)
		newLines := highlightSource(lexer, newSource.String(), styleName)
		oldIndex, newIndex := 0, 0
		for li, line := range hunk.Lines {
			switch line.Kind {
			case '-':
				if oldIndex < len(oldLines) {
					result[hi][li] = oldLines[oldIndex]
				}
				oldIndex++
			case '+', ' ':
				if newIndex < len(newLines) {
					result[hi][li] = newLines[newIndex]
				}
				newIndex++
				if line.Kind == ' ' {
					oldIndex++
				}
			}
		}
	}
	return result
}

func highlightSource(lexer chroma.Lexer, source, styleName string) []string {
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return strings.Split(source, "\n")
	}
	style := styles.Get(styleName)
	var out strings.Builder
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		// Error styles may rely on a token background; rows use diff backgrounds.
		if token.Type == chroma.Error {
			entry = style.Get(chroma.Keyword)
		}
		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				out.WriteByte('\n')
			}
			if part == "" {
				continue
			}
			if entry.Colour.IsSet() {
				fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm%s\x1b[39m", entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue(), part)
			} else {
				out.WriteString(part)
			}
		}
	}
	return strings.Split(out.String(), "\n")
}
