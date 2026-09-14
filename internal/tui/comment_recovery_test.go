package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func TestPRDraftSurvivesRevisionAndRestart(t *testing.T) {
	m, f := githubCommentModel(t)
	press(m, "cc")
	notePaste(m, "Keep my draft across PR updates")
	f.err = &backend.CommentNotSubmittedError{Err: errors.New("PR has new changes; refresh")}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(cmd())
	press(m, "enter")
	press(m, "esc")
	press(m, "esc")
	next := *m.snapshot
	c := *next.Comparison
	c.HeadOID = "updated-head"
	next.Comparison, next.Key, next.Revision = &c, "test-pr:base:updated-head", "base:updated-head"
	path := m.notes.path
	m.install(&next)
	assertDraft := func(m *Model) {
		t.Helper()
		for _, c := range m.notes.items {
			if c.RemoteID == 0 && c.Body == "Keep my draft across PR updates" {
				if c.Outdated || c.PendingBody != "" || c.Revision != next.Revision {
					t.Fatalf("unchanged anchor not retryable: %+v", c)
				}
				return
			}
		}
		t.Fatal("saved draft disappeared")
	}
	assertDraft(m)
	if m.notes.path != path {
		t.Fatal("cache path changed with PR revision")
	}
	restarted := New(&next, f, false, DarkTheme)
	defer restarted.Close()
	assertDraft(restarted)
	// Changing the underlying line keeps the draft but invalidates its anchor.
	c.Files = append(c.Files[:0:0], c.Files...)
	c.Files[0].Hunks = append(c.Files[0].Hunks[:0:0], c.Files[0].Hunks...)
	c.Files[0].Hunks[0].Lines = append(c.Files[0].Hunks[0].Lines[:0:0], c.Files[0].Hunks[0].Lines...)
	c.Files[0].Hunks[0].Lines[0].Text = "different code"
	next.Key, next.Revision = "test-pr:base:changed", "base:changed"
	restarted.install(&next)
	for _, c := range restarted.notes.items {
		if c.RemoteID == 0 && !c.Outdated {
			t.Fatal("changed anchor remained active")
		}
	}
	restarted.commentModal = ""
	restarted.selected = 0
	press(restarted, "cc")
	if restarted.commentDraft.Outdated || restarted.commentDraft.Code != "different code" || restarted.commentDraft.Body != "Keep my draft across PR updates" {
		t.Fatal("explicitly reselecting changed lines did not reanchor the draft")
	}
	// A different account never receives this account's drafts.
	next.CommentKey, next.Viewer, next.Key = "test-pr:other-account", "other", "other:base:changed"
	other := New(&next, f, false, DarkTheme)
	defer other.Close()
	for _, c := range other.notes.items {
		if c.RemoteID == 0 {
			t.Fatal("draft leaked across accounts")
		}
	}
}

func TestCommentAfterSearchRespectsPickedFile(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	press(m, "/content")
	press(m, "enter")
	press(m, "flast.txt")
	press(m, "enter")
	press(m, "c")
	if m.selectedComment().Path != "last.txt" {
		t.Fatal("comment jumped to the old search result")
	}
}

func TestSuccessfulSaveCacheFailureReconcilesAfterRestart(t *testing.T) {
	m, f := githubCommentModel(t)
	press(m, "cc")
	notePaste(m, "Already sent")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	path := m.notes.path
	m.notes.path += "/not-a-directory"
	m.Update(cmd())
	saved := m.notes.items[len(m.notes.items)-1]
	s := *m.snapshot
	s.Comments = append(s.Comments, saved)
	restarted := New(&s, f, false, DarkTheme)
	defer restarted.Close()
	for _, c := range restarted.notes.items {
		if c.Body == "Already sent" && (c.RemoteID == 0 || c.PendingBody != "") {
			t.Fatal("sent comment became a resendable draft")
		}
	}
	cached, err := review.LoadComments(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cached {
		if c.PendingBody != "" {
			t.Fatal("reconciliation did not clear the cached pending submission")
		}
	}
	if len(restarted.notes.items) != 2 {
		t.Fatal("reconciliation duplicated a comment")
	}
}

func TestLegacyPRDraftMigratesToStableCache(t *testing.T) {
	m, f := githubCommentModel(t)
	legacy, err := review.CommentsPath(m.comparison, m.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	c := review.Comment{ID: "legacy", Path: "main.go", Side: "new", Start: 1, End: 1, Code: "package main // content", Body: "Legacy draft"}
	if err := review.SaveComments(legacy, []review.Comment{c}); err != nil {
		t.Fatal(err)
	}
	restarted := New(m.snapshot, f, false, DarkTheme)
	defer restarted.Close()
	if restarted.notes.path == legacy || !strings.Contains(restarted.notes.path, "pr-") {
		t.Fatal("legacy cache was not migrated")
	}
	items, err := review.LoadComments(restarted.notes.path)
	if err != nil || len(items) != 1 || items[0].Revision != m.snapshot.Revision {
		t.Fatalf("migration: %+v %v", items, err)
	}
}

func TestSuccessfulEditClearsPersistedDraftOnRefresh(t *testing.T) {
	m, f := githubCommentModel(t)
	m.notes.items[0].DraftBody = "Already updated"
	if !m.persistComments(m.notes.items) {
		t.Fatal("could not store edit draft")
	}
	s := *m.snapshot
	s.Comments = append([]review.Comment(nil), s.Comments...)
	s.Comments[0].Body = "Already updated"
	restarted := New(&s, f, false, DarkTheme)
	defer restarted.Close()
	cached, err := review.LoadComments(restarted.notes.path)
	if err != nil || len(cached) != 1 || cached[0].DraftBody != "" || cached[0].Body != "Already updated" {
		t.Fatalf("successful edit stayed in disk cache as a draft: %+v %v", cached, err)
	}
}
