package tui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Interpret the background SGR state at a visible token, rather than merely
// checking whether a desired color appears somewhere in the rendered output.
func backgroundAt(t *testing.T, rendered, token string) string {
	t.Helper()
	index := strings.Index(rendered, token)
	if index < 0 {
		t.Fatalf("missing rendered token %q", token)
	}
	background := ""
	for _, match := range regexp.MustCompile(`\x1b\[([0-9;]*)m`).FindAllStringSubmatch(rendered[:index], -1) {
		codes := strings.Split(match[1], ";")
		for i := 0; i < len(codes); i++ {
			switch codes[i] {
			case "", "0", "49":
				background = ""
			case "38", "48":
				if i+4 < len(codes) && codes[i+1] == "2" {
					if codes[i] == "48" {
						r, _ := strconv.Atoi(codes[i+2])
						g, _ := strconv.Atoi(codes[i+3])
						b, _ := strconv.Atoi(codes[i+4])
						background = fmt.Sprintf("%02x%02x%02x", r, g, b)
					}
					i += 4
				}
			}
		}
	}
	return background
}

func TestNestedHighlightRestoresRowBackground(t *testing.T) {
	for _, theme := range []Theme{LightTheme, DarkTheme} {
		m := sampleModel(true)
		t.Cleanup(m.Close)
		m.palette = paletteFor(theme)
		for _, background := range []string{"", m.palette.addedBackground, m.palette.deletedBackground} {
			highlight := m.surfaceWithBackground(m.palette.headerBackground, m.palette.accent, "hit", 3)
			rendered := m.surfaceWithBackground(m.palette.foreground, background, highlight+" suffix", 20)
			if got := backgroundAt(t, rendered, "hit"); got != m.palette.accent {
				t.Fatalf("%s: lost nested highlight background: %q", theme, got)
			}
			if got := backgroundAt(t, rendered, " suffix"); got != background {
				t.Fatalf("%s: highlight leaked into row: got %q, want %q", theme, got, background)
			}
		}
	}
}

func contrast(a, b string) float64 {
	luminance := func(hex string) float64 {
		rgb, _ := strconv.ParseUint(hex, 16, 24)
		linear := func(value uint64) float64 {
			c := float64(value&255) / 255
			if c <= 0.04045 {
				return c / 12.92
			}
			return math.Pow((c+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(rgb>>16) + 0.7152*linear(rgb>>8) + 0.0722*linear(rgb)
	}
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

func TestThemeTextContrast(t *testing.T) {
	for _, theme := range []Theme{LightTheme, DarkTheme} {
		p := paletteFor(theme)
		pairs := map[string][2]string{
			"modal body":      {p.foreground, p.modal.background},
			"modal hints":     {p.modal.muted, p.modal.header},
			"modal muted":     {p.modal.muted, p.modal.background},
			"modal title":     {p.accent, p.modal.header},
			"modal selected":  {p.accent, p.modal.selection},
			"selected rename": {p.foreground, p.modal.selection},
			"picker added":    {p.green, p.modal.selection},
			"picker deleted":  {p.red, p.modal.selection},
			"alert title":     {p.red, p.modal.header},
			"alert button":    {p.modal.background, p.red},
			"search active":   {p.headerBackground, p.accent},
			"search other":    {p.foreground, p.selectionBackground},
			"added code":      {p.foreground, p.addedBackground},
			"deleted code":    {p.foreground, p.deletedBackground},
			"added inline":    {p.foreground, p.addedInlineBackground},
			"deleted inline":  {p.foreground, p.deletedInlineBackground},
			"added numbers":   {p.green, p.addedBackground},
			"deleted numbers": {p.red, p.deletedBackground},
		}
		for name, pair := range pairs {
			if ratio := contrast(pair[0], pair[1]); ratio < 4.5 {
				t.Errorf("%s %s text contrast is %.2f:1", theme, name, ratio)
			}
		}
		if ratio := contrast(p.modal.border, p.modal.background); ratio < 3 {
			t.Errorf("%s modal border contrast is %.2f:1", theme, ratio)
		}
	}
}
