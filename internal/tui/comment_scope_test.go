package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func TestCommentFileScopeSelectionAndDeletion(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	path := m.comparison.Files[m.selected].Path
	m.notes.items = []review.Comment{
		{ID: "other", Path: "other.go", Body: "Other file comment"},
		{ID: "first", Path: path, Body: "First visible comment"},
		{ID: "hidden", Path: "other.go", Body: "Another hidden comment"},
		{ID: "second", Path: path, Body: "Second visible comment"},
	}
	press(m, "C")
	if m.commentIndex != 1 || !strings.Contains(m.View(), "Current file · 2") || strings.Contains(m.View(), "Other file comment") {
		t.Fatal("opening comments did not select the current file")
	}
	g := m.modalLayout()
	m.commentMouse(tea.MouseMsg{X: g.x + 2, Y: g.y + 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.commentIndex != 3 {
		t.Fatal("mouse selected a hidden comment")
	}
	press(m, "enter")
	if m.commentDraft.ID != "second" {
		t.Fatal("edit targeted a hidden comment")
	}
	press(m, "esc")
	press(m, "d")
	press(m, "enter")
	if len(m.notes.items) != 3 || m.notes.items[0].ID != "other" || m.notes.items[2].ID != "hidden" || m.commentIndex != 1 {
		t.Fatal("deletion affected another file or left invalid selection")
	}
	press(m, "tab")
	if !strings.Contains(m.View(), "All files · 3") || !strings.Contains(m.View(), "Other file comment") {
		t.Fatal("all-files toggle did not reveal other comments")
	}
	press(m, "k")
	press(m, "tab")
	if m.commentIndex != 1 {
		t.Fatal("returning to file scope retained a hidden selection")
	}
	press(m, "d")
	press(m, "enter")
	press(m, "r")
	press(m, "enter")
	press(m, "d")
	press(m, "g")
	if m.commentModal != "list" || len(m.notes.items) != 2 || m.notes.items[0].Resolved || !strings.Contains(m.View(), "No comments in this file") {
		t.Fatal("empty file scope allowed actions on hidden comments")
	}
}

func TestGitHubCommentFileScopeGuardsWrites(t *testing.T) {
	m, f := githubCommentModel(t)
	c := m.notes.items[0]
	c.ID, c.RemoteID, c.Path = "github:90", 90, "other.go"
	m.notes.items = append([]review.Comment{c}, m.notes.items...)
	press(m, "C")
	cmd := m.activate("r")
	if cmd == nil {
		t.Fatal("current comment was not selected")
	}
	m.Update(cmd())
	if m.notes.items[0].Resolved || !m.notes.items[1].Resolved || f.resolves != 1 {
		t.Fatal("resolved a hidden file's discussion")
	}
	m.notes.items = m.notes.items[:1]
	m.openComments()
	press(m, "ar")
	if f.resolves != 1 || m.commentModal != "list" {
		t.Fatal("empty scope allowed a reply or resolve")
	}
}
