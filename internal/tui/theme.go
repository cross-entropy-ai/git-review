package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// Theme selects a palette after terminal background detection in the CLI.
type Theme string

const (
	DarkTheme  Theme = "dark"
	LightTheme Theme = "light"
)

type palette struct {
	headerBackground    string
	selectionBackground string
	addedBackground     string
	deletedBackground   string
	border              string
	foreground          string
	muted               string
	accent              string
	green               string
	red                 string
	syntaxStyle         string
}

func paletteFor(theme Theme) palette {
	if theme == LightTheme {
		return palette{
			headerBackground:    "e4e8ed",
			selectionBackground: "d8e9fc",
			addedBackground:     "dafbe1",
			deletedBackground:   "ffebe9",
			border:              "a0a8b2",
			foreground:          "24292f",
			muted:               "57606a",
			accent:              "0959b0",
			green:               "116329",
			red:                 "b42332",
			syntaxStyle:         "github",
		}
	}
	return palette{
		headerBackground:    "2b3038",
		selectionBackground: "233e5a",
		addedBackground:     "173b27",
		deletedBackground:   "4b2026",
		border:              "4b5563",
		foreground:          "e1e7ef",
		muted:               "a0aab8",
		accent:              "8bc4ff",
		green:               "85d996",
		red:                 "ff9b98",
		syntaxStyle:         "github-dark",
	}
}

func colorCode(layer int, hex string) string {
	rgb, _ := strconv.ParseUint(hex, 16, 24)
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", layer, rgb>>16, (rgb>>8)&255, rgb&255)
}

func (m *Model) ink(color, text string) string {
	if !m.color {
		return text
	}
	return colorCode(38, color) + text + "\x1b[39m"
}

func (m *Model) bold(text string) string {
	if !m.color {
		return text
	}
	return "\x1b[1m" + text + "\x1b[22m"
}

// Preserve the terminal background and restore text color after nested styles.
func (m *Model) surface(fg, text string, width int) string {
	return m.surfaceWithBackground(fg, "", text, width)
}

func (m *Model) surfaceWithBackground(fg, bg, text string, width int) string {
	text = fit(text, width)
	if !m.color {
		return text
	}
	fgCode := colorCode(38, fg)
	bgCode := ""
	if bg != "" {
		bgCode = colorCode(48, bg)
	}
	text = strings.NewReplacer("\x1b[0m", fgCode+bgCode, "\x1b[39m", fgCode, "\x1b[49m", bgCode).Replace(text)
	return fgCode + bgCode + text + "\x1b[0m"
}
