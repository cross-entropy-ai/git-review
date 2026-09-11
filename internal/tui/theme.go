package tui

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	headerBackground    = "161b22"
	selectionBackground = "1c2d41"
	border              = "30363d"
	foreground          = "c9d1d9"
	muted               = "8b949e"
	accent              = "79c0ff"
	green               = "7ee787"
	red                 = "ffa198"
)

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
