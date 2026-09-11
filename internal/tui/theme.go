package tui

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	background        = "0d1117"
	panel             = "161b22"
	raised            = "21262d"
	border            = "30363d"
	foreground        = "c9d1d9"
	muted             = "8b949e"
	accent            = "79c0ff"
	green             = "7ee787"
	red               = "ffa198"
	addedBackground   = "12261e"
	deletedBackground = "2b161b"
	selection         = "1c2d41"
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

// Restore the enclosing surface after nested styles or syntax token resets.
func (m *Model) surface(fg, bg, text string, width int) string {
	text = fit(text, width)
	if !m.color {
		return text
	}
	fgCode, bgCode := colorCode(38, fg), colorCode(48, bg)
	text = strings.NewReplacer("\x1b[0m", fgCode+bgCode, "\x1b[39m", fgCode, "\x1b[49m", bgCode).Replace(text)
	return fgCode + bgCode + text + "\x1b[0m"
}
