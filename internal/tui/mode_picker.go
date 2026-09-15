package tui

import (
	"context"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

type targetsMsg struct {
	generation int
	targets    []backend.Target
	err        error
}

type targetStatsMsg struct {
	generation, index int
	target            backend.Target
	err               error
}

func (m *Model) closePicker() {
	if m.pickerCancel != nil {
		m.pickerCancel()
		m.pickerCancel = nil
	}
	m.picking, m.modePicking = false, false
}

func (m *Model) openModePicker() tea.Cmd {
	source, ok := m.source.(backend.TargetBackend)
	if m.mode == diff.ModePullRequest || !ok {
		m.showAlert("Mode switching unavailable", "PR review has a fixed scope. Open a local review to switch between Working tree and local branches.")
		return nil
	}
	if m.loading {
		return nil
	}
	m.closePicker()
	m.picking, m.modePicking, m.targetsLoading = true, true, true
	m.dragging, m.fileQuery = "", ""
	m.modeTargets = []backend.Target{{Mode: diff.ModeWorkingTree, Label: "Working tree"}}
	m.updateTargetMatches()
	m.pickerGeneration++
	generation := m.pickerGeneration
	ctx, cancel := context.WithCancel(m.ctx)
	m.pickerContext, m.pickerCancel = ctx, cancel
	return func() tea.Msg {
		targets, err := source.Targets(ctx)
		return targetsMsg{generation, targets, err}
	}
}

func (m *Model) finishTargets(msg targetsMsg) tea.Cmd {
	if !m.modePicking || !m.picking || msg.generation != m.pickerGeneration {
		return nil
	}
	m.targetsLoading = false
	if msg.err != nil {
		m.closePicker()
		m.showAlert("Load review modes failed", msg.err.Error())
		return nil
	}
	m.modeTargets = msg.targets
	m.updateTargetMatches()
	return m.targetStatsCommand(0)
}

func (m *Model) targetStatsCommand(index int) tea.Cmd {
	if index >= len(m.modeTargets) {
		return nil
	}
	source := m.source.(backend.TargetBackend)
	ctx, generation, target := m.pickerContext, m.pickerGeneration, m.modeTargets[index]
	return func() tea.Msg {
		result, err := source.TargetStats(ctx, target)
		return targetStatsMsg{generation, index, result, err}
	}
}

func (m *Model) finishTargetStats(msg targetStatsMsg) tea.Cmd {
	if !m.picking || !m.modePicking || msg.generation != m.pickerGeneration {
		return nil
	}
	if msg.err != nil {
		m.modeTargets[msg.index].StatsError = msg.err.Error()
	} else {
		m.modeTargets[msg.index] = msg.target
	}
	return m.targetStatsCommand(msg.index + 1)
}

func (m *Model) updateTargetMatches() {
	type candidate struct{ index, score int }
	var candidates []candidate
	for i, target := range m.modeTargets {
		if score, ok := fuzzyScore(target.Label, m.fileQuery); ok {
			candidates = append(candidates, candidate{i, score})
		}
	}
	if strings.TrimSpace(m.fileQuery) != "" {
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })
	}
	m.fileMatches = nil
	for _, c := range candidates {
		m.fileMatches = append(m.fileMatches, c.index)
	}
	m.fileCursor, m.fileOffset = 0, 0
}

func (m *Model) chooseTarget() tea.Cmd {
	if len(m.fileMatches) == 0 {
		return nil
	}
	target := m.modeTargets[m.fileMatches[m.fileCursor]]
	source := m.source.(backend.TargetBackend)
	cmd := m.loadComparison(target.Label, func() (*backend.Snapshot, error) { return source.LoadTarget(m.ctx, target) })
	if cmd != nil {
		m.closePicker()
	}
	return cmd
}

func (m *Model) pickerTotal() int {
	if m.modePicking {
		return len(m.modeTargets)
	}
	return len(m.comparison.Files)
}

func (m *Model) pickerEmptyLabel() string {
	if m.modePicking {
		if m.targetsLoading {
			return " Loading local branches…"
		}
		return " No review modes match"
	}
	return " No files match"
}

func (m *Model) pickerRow(index int, selected bool) (string, string) {
	if m.modePicking {
		target := m.modeTargets[index]
		name := safeText(target.Label)
		if target.Mode == m.mode && (m.mode == diff.ModeWorkingTree || target.Head == m.snapshot.TargetHead || target.Label == m.comparison.Head) {
			name += m.ink(m.palette.modal.muted, " (current)")
		}
		stats := m.ink(m.palette.modal.muted, "…")
		if target.StatsReady {
			stats = m.stats(target.Added, target.Deleted)
		}
		if target.StatsError != "" {
			stats = m.ink(m.palette.modal.muted, "unavailable")
		}
		return name, " " + stats + " "
	}
	file := m.comparison.Files[index]
	name := safeText(file.Path)
	if file.OldPath != "" {
		fg := m.palette.modal.muted
		if selected {
			fg = m.palette.foreground
		}
		name += m.ink(fg, " ← "+safeText(file.OldPath))
	}
	return name, " " + m.stats(file.Added, file.Deleted) + " "
}
