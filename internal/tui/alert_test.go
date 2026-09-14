package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPREditorErrorNeedsAcknowledgement(t *testing.T) {
	m, _ := remoteModel()
	t.Cleanup(m.Close)
	selected, offset := m.selected, m.offset
	if cmd := m.activate("e"); cmd != nil || !m.hasAlert() {
		t.Fatal("unsupported PR editor action did not show an alert")
	}
	view := ansi.Strip(m.View())
	for _, text := range []string{"Cannot open editor", "no local working tree", "local checkout", "Enter OK"} {
		if !strings.Contains(view, text) {
			t.Fatalf("PR alert is missing %q", text)
		}
	}
	for _, key := range []string{"q", "e", "v", "r", "n", "p", "f", "/", "?"} {
		if cmd := m.activate(key); cmd != nil || !m.hasAlert() {
			t.Fatalf("%s bypassed alert confirmation", key)
		}
	}
	clickAt(m, 2, contentTop)
	if !m.hasAlert() {
		t.Fatal("background click dismissed the error")
	}
	clickText(t, m, "Enter OK")
	if m.hasAlert() || m.selected != selected || m.offset != offset {
		t.Fatal("confirmation failed to return to the original review")
	}
	press(m, "e")
	press(m, "esc")
	if m.hasAlert() {
		t.Fatal("Esc did not acknowledge the error")
	}
}

func TestAlertsPreserveInputModesAndQueueErrors(t *testing.T) {
	for _, input := range []string{"fdocs", "/package", "?"} {
		m := sampleModel(false)
		t.Cleanup(m.Close)
		press(m, input)
		selected, offset, query, search := m.selected, m.offset, m.fileQuery, m.search
		picking, searching, help := m.picking, m.searching, m.help
		m.Update(loadedMsg{err: errors.New("first failure")})
		m.Update(editorFinishedMsg{err: errors.New("second failure")})
		// Success status messages must not replace an unacknowledged failure.
		m.Update(editorFinishedMsg{})
		if len(m.alerts) != 2 || !strings.Contains(m.View(), "first failure") {
			t.Fatal("an asynchronous result replaced an unacknowledged alert")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("vqf/"), Paste: true})
		press(m, "v")
		if m.selected != selected || m.offset != offset || m.fileQuery != query || m.search != search {
			t.Fatal("alert input mutated the underlying input mode")
		}
		press(m, "enter")
		if len(m.alerts) != 1 || !strings.Contains(m.View(), "second failure") {
			t.Fatal("confirming an alert discarded the next error")
		}
		press(m, "enter")
		if m.hasAlert() || m.picking != picking || m.searching != searching || m.help != help {
			t.Fatal("confirming alerts lost the underlying modal/input state")
		}
	}
}

func TestAlertLongDetailsResizeAndThemes(t *testing.T) {
	for _, theme := range []Theme{LightTheme, DarkTheme} {
		for _, color := range []bool{false, true} {
			m := sampleModel(color)
			t.Cleanup(m.Close)
			m.palette = paletteFor(theme)
			m.showAlert("Error\x1b[2J", strings.Repeat("中文 details with a long/path/to/a/file\n", 40)+"last detail\x1b[2J")
			for _, size := range [][2]int{{120, 34}, {60, 18}, {45, 12}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for i := 0; i < 50; i++ {
					m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				}
				view := m.View()
				if !strings.Contains(view, "last detail") || !strings.Contains(view, "Enter OK") {
					t.Fatal("long error details or confirmation became inaccessible")
				}
				lines := strings.Split(view, "\n")
				if len(lines) != size[1] {
					t.Fatalf("alert height %d, want %d", len(lines), size[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != size[0] {
						t.Fatal("alert overflowed the terminal width")
					}
				}
				if strings.Contains(view, "\x1b[2J") || (!color && strings.Contains(view, "\x1b")) {
					t.Fatal("alert emitted unsafe controls or styling in no-color mode")
				}
				before := m.alertOffset
				g := m.alertLayout()
				m.Update(tea.MouseMsg{X: g.x + 2, Y: g.y + 2, Button: tea.MouseButtonWheelUp})
				if m.alertOffset >= before {
					t.Fatal("mouse wheel did not scroll alert details")
				}
			}
		}
	}
}

func TestInitialWarningAndEditorFailureAlerts(t *testing.T) {
	m := sampleModel(false)
	t.Cleanup(m.Close)
	snapshot := *m.snapshot
	snapshot.Warning = "Could not restore progress"
	m.install(&snapshot)
	if !m.hasAlert() || m.alerts[0].title != "Review warning" {
		t.Fatal("backend warning was not shown in an alert")
	}
	press(m, "enter")
	m.comparison.Root = t.TempDir()
	if cmd := m.activate("e"); cmd != nil || !m.hasAlert() || m.alerts[0].title != "Cannot open editor" {
		t.Fatal("missing local file did not show an editor alert")
	}
}
