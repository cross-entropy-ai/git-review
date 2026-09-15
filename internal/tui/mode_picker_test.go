package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

type pickerBackend struct {
	backend.Backend
	targets  []backend.Target
	chosen   backend.Target
	snapshot *backend.Snapshot
	err      error
}

func (b *pickerBackend) Targets(context.Context) ([]backend.Target, error) { return b.targets, b.err }
func (b *pickerBackend) TargetStats(_ context.Context, target backend.Target) (backend.Target, error) {
	target.Added, target.Deleted, target.StatsReady = 12, 3, true
	return target, nil
}
func (b *pickerBackend) LoadTarget(_ context.Context, target backend.Target) (*backend.Snapshot, error) {
	b.chosen = target
	return b.snapshot, b.err
}
func modePickerModel(t *testing.T) (*Model, *pickerBackend) {
	t.Helper()
	m := sampleModel(false)
	t.Cleanup(m.Close)
	b := &pickerBackend{Backend: m.source, snapshot: m.snapshot, targets: []backend.Target{
		{Mode: diff.ModeWorkingTree, Label: "Working tree"},
		{Mode: diff.ModeCommitted, Head: "refs/heads/feature/search", Label: "feature/search"},
		{Mode: diff.ModeCommitted, Head: "refs/heads/main", Label: "main"},
	}}
	m.source = b
	return m, b
}
func finishPickerCommands(m *Model, cmd tea.Cmd) {
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
}

func TestModePickerSearchStatsAndMouse(t *testing.T) {
	m, b := modePickerModel(t)
	previous := m.comparison
	finishPickerCommands(m, m.activate("m"))
	if !m.modePicking || len(m.fileMatches) != 3 || m.modeTargets[m.fileMatches[0]].Label != "Working tree" || m.comparison != previous {
		t.Fatal("picker did not list scopes without changing the review")
	}
	for _, width := range []int{45, 70, 120} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		view := m.View()
		if !strings.Contains(ansi.Strip(view), "+12 −3") {
			t.Fatal("missing line totals")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) != width {
				t.Fatalf("overflow at width %d: %q", width, line)
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("FTR SRCH"), Paste: true})
	if len(m.fileMatches) != 1 || m.fileMatches[0] != 1 {
		t.Fatal("branch fuzzy search failed")
	}
	g := m.modalLayout()
	_, cmd := m.Update(tea.MouseMsg{X: g.x + 5, Y: g.y + 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil || m.picking || !m.loading {
		t.Fatal("mouse selection dropped load command")
	}
	m.Update(cmd())
	if b.chosen.Head != "refs/heads/feature/search" {
		t.Fatal("wrong branch selected")
	}
}

func TestModePickerCancellationAndStaleResults(t *testing.T) {
	m, _ := modePickerModel(t)
	cmd := m.activate("m")
	old := cmd()
	oldContext := m.pickerContext
	m.activate("esc")
	if oldContext.Err() == nil {
		t.Fatal("cancel did not stop background work")
	}
	fresh := m.activate("m")
	if _, cmd := m.Update(old); cmd != nil || !m.targetsLoading {
		t.Fatal("stale branch list was accepted")
	}
	_, stats := m.Update(fresh())
	result := stats()
	m.activate("esc")
	m.activate("f")
	if _, cmd := m.Update(result); cmd != nil || m.modePicking {
		t.Fatal("late stats changed file picker")
	}
}

func TestModePickerScrollingEmptyAndErrors(t *testing.T) {
	m, b := modePickerModel(t)
	for i := 0; i < 40; i++ {
		b.targets = append(b.targets, backend.Target{Mode: diff.ModeCommitted, Label: fmt.Sprintf("extra/%02d", i)})
	}
	finishPickerCommands(m, m.activate("m"))
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("extra"), Paste: true})
	for i := 0; i < 30; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m.Update(tea.WindowSizeMsg{Width: 45, Height: 12})
	if !strings.Contains(m.View(), "extra/30") {
		t.Fatal("resize hid selected branch")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("no-match"), Paste: true})
	if m.activate("enter") != nil || !m.picking || !strings.Contains(m.View(), "No review modes match") {
		t.Fatal("empty search changed review")
	}
	m.activate("esc")
	b.err = errors.New("repository unavailable")
	finishPickerCommands(m, m.activate("m"))
	if m.picking || !m.hasAlert() || !strings.Contains(m.message, "repository unavailable") {
		t.Fatal("listing error was hidden")
	}
}

func TestPRModeClickExplainsDisabledSwitch(t *testing.T) {
	m, _ := remoteModel()
	defer m.Close()
	for _, c := range m.controls() {
		if c.y == 0 && strings.Contains(c.label, "Mode:") {
			_, cmd := m.Update(tea.MouseMsg{X: c.x + 2, Y: c.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if cmd != nil || m.picking || !m.hasAlert() || m.mode != diff.ModePullRequest {
				t.Fatal("PR mode click did not explain fixed scope")
			}
			return
		}
	}
	t.Fatal("missing mode control")
}
