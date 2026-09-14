package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

type fakeBackend struct {
	snapshot *backend.Snapshot
	err      error
	saves    int
	path     string
	viewed   bool
}

func (f *fakeBackend) Modes() []diff.Mode { return []diff.Mode{diff.ModePullRequest} }
func (f *fakeBackend) Load(context.Context, diff.Mode) (*backend.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeBackend) SetViewed(_ context.Context, _ *backend.Snapshot, path string, viewed bool) error {
	f.saves++
	f.path = path
	f.viewed = viewed
	return f.err
}

func remoteModel() (*Model, *fakeBackend) {
	c := sampleModel(false).comparison
	s := &backend.Snapshot{Comparison: c, Key: "pr1:revision1:user1", Mode: diff.ModePullRequest, Label: "owner/repo #918", URL: "https://github.com/owner/repo/pull/918", Persistence: backend.Remote,
		Viewed: map[string]backend.ViewedState{"main.go": backend.Dismissed, "docs/中文.md": backend.Viewed}}
	f := &fakeBackend{snapshot: s}
	return New(s, f, false, DarkTheme), f
}

func TestRemoteViewedAsyncSuccessFailureAndRefresh(t *testing.T) {
	m, f := remoteModel()
	defer m.Close()
	if m.viewed["main.go"] || !m.dismissed["main.go"] || !strings.Contains(m.View(), "[!]") {
		t.Fatal("dismissed state is not visible")
	}
	if !strings.Contains(m.View(), "Mode: GitHub PR") || strings.Contains(m.View(), "m Mode:") {
		t.Fatal("PR has a misleading local toggle")
	}
	if cmd := m.activate("m"); cmd != nil {
		t.Fatal("PR review switched to local")
	}
	cmd := m.activate("v")
	if cmd == nil || f.saves != 0 || !m.viewed["main.go"] || !strings.Contains(m.View(), "[~]") {
		t.Fatal("Viewed did not save asynchronously")
	}
	if refresh := m.activate("r"); refresh != nil {
		t.Fatal("refresh raced an in-flight save")
	}
	if quit := m.activate("q"); quit != nil {
		t.Fatal("quit silently abandoned a Viewed update")
	}
	m.Update(cmd())
	if f.saves != 1 || f.path != "main.go" || !f.viewed || len(m.pending) != 0 {
		t.Fatal("Viewed update was not completed")
	}
	// The next refresh must use GitHub's state, even for the same revision.
	m.Update(m.activate("r")())
	if m.viewed["main.go"] || !m.dismissed["main.go"] {
		t.Fatal("refresh overwrote GitHub with stale local progress")
	}
	f.err = errors.New("permission denied")
	cmd = m.activate("v")
	m.Update(cmd())
	if m.viewed["main.go"] || !m.dismissed["main.go"] || !strings.Contains(m.message, "permission denied") || len(m.pending) != 0 {
		t.Fatal("failed write did not restore prior state")
	}
	if !m.hasAlert() || m.alerts[0].title != "Viewed update failed" {
		t.Fatal("failed Viewed update did not show a confirmation alert")
	}
}

func TestRemoteMouseViewedAndNoState(t *testing.T) {
	m, f := remoteModel()
	defer m.Close()
	// A click on the sidebar checkbox returns the asynchronous backend command.
	_, cmd := m.Update(tea.MouseMsg{X: 5, Y: contentTop, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil {
		t.Fatal("mouse click dropped the save command")
	}
	m.Update(cmd())
	if f.saves != 1 {
		t.Fatal("mouse did not save Viewed")
	}
	m.snapshot.Persistence = backend.Memory
	m.selected = 2
	if cmd := m.activate("v"); cmd != nil || f.saves != 1 || !m.viewed["last.txt"] {
		t.Fatal("--no-state contacted remote persistence")
	}
}
