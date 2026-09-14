package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type fakeCommentBackend struct {
	fakeBackend
	err             error
	saved           review.Comment
	writes, deletes int
	resolves        int
}

func (f *fakeCommentBackend) SaveComment(_ context.Context, _ *backend.Snapshot, c review.Comment) (review.Comment, error) {
	f.writes++
	f.saved = c
	if f.err != nil {
		return c, f.err
	}
	if c.RemoteID == 0 {
		c.ID, c.RemoteID = "github:101", 101
	}
	c.Author, c.AuthorID, c.URL, c.DraftBody = "me", "me-id", "https://github.com/test/repo/pull/1#discussion_r101", ""
	return c, nil
}

func (f *fakeCommentBackend) DeleteComment(_ context.Context, _ *backend.Snapshot, c review.Comment) error {
	f.deletes++
	f.saved = c
	return f.err
}

func (f *fakeCommentBackend) SetThreadResolved(_ context.Context, _ *backend.Snapshot, c review.Comment, resolved bool) (review.Comment, error) {
	f.resolves++
	if f.err != nil {
		return c, f.err
	}
	c.ThreadID, c.Resolved = "thread-41", resolved
	return c, nil
}

func TestGitHubResolveFromReplyKeepsCommentsAndDrafts(t *testing.T) {
	m, f := githubCommentModel(t)
	root := m.notes.items[0]
	root.ThreadID, root.DraftBody = "thread-41", "Unsynced edit"
	reply := root
	reply.ID, reply.RemoteID, reply.ReplyTo, reply.Body, reply.ThreadID = "github:42", 42, 41, "Reply body", ""
	unrelated := root
	unrelated.ID, unrelated.RemoteID, unrelated.ThreadID = "github:90", 90, "other-thread"
	m.notes.items = []review.Comment{root, reply, unrelated}
	press(m, "Cj")
	f.err = errors.New("HTTP 403: permission denied")
	cmd := m.activate("r")
	if cmd == nil || !m.commentSaving || m.notes.items[1].Resolved {
		t.Fatal("resolve should wait for GitHub before changing state")
	}
	if duplicate := m.activate("r"); duplicate != nil {
		t.Fatal("duplicate resolve operation")
	}
	m.Update(cmd())
	if !m.hasAlert() || !strings.Contains(m.View(), "403") || len(m.notes.items) != 3 || m.notes.items[0].Resolved {
		t.Fatal("permission failure was hidden or changed the discussion")
	}
	press(m, "enter")
	f.err = nil
	cmd = m.activate("r")
	m.Update(cmd())
	if !m.notes.items[0].Resolved || !m.notes.items[1].Resolved || m.notes.items[2].Resolved || m.commentIndex != 1 {
		t.Fatal("resolve did not affect exactly the selected discussion")
	}
	if m.notes.items[0].Body != root.Body || m.notes.items[0].DraftBody != root.DraftBody || m.notes.items[1].Body != reply.Body || !strings.Contains(m.View(), "r Unresolve") {
		t.Fatal("resolve lost content or toggle label")
	}
	cmd = m.activate("r")
	m.Update(cmd())
	if m.notes.items[0].Resolved || m.notes.items[1].Resolved || f.resolves != 3 || f.writes != 0 || f.deletes != 0 {
		t.Fatal("unresolve edited/deleted comments or failed to reopen the thread")
	}
}

func githubCommentModel(t *testing.T) (*Model, *fakeCommentBackend) {
	t.Helper()
	base := sampleModel(false)
	t.Cleanup(base.Close)
	c := *base.comparison
	c.GitDir, c.HeadOID = t.TempDir(), "head"
	s := &backend.Snapshot{Comparison: &c, Key: "test-pr", Mode: diff.ModePullRequest, Persistence: backend.Remote, Viewer: "me-id",
		Comments: []review.Comment{{ID: "github:41", RemoteID: 41, Path: "main.go", Side: "new", Start: 1, End: 1, Code: "package main // content", Body: "Existing comment", Author: "other", AuthorID: "other-id"}}}
	f := &fakeCommentBackend{fakeBackend: fakeBackend{snapshot: s}}
	m := New(s, f, false, DarkTheme)
	t.Cleanup(m.Close)
	return m, f
}

func TestGitHubCommentSaveFailurePreservesDraftAndRetry(t *testing.T) {
	m, f := githubCommentModel(t)
	press(m, "cc")
	if m.commentDraft.RemoteID != 0 {
		t.Fatal("adding a comment edited someone else's existing thread")
	}
	notePaste(m, "New review\n中文")
	f.err = errors.New("HTTP 403: permission denied")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil || !m.commentSaving {
		t.Fatal("save did not start a GitHub operation")
	}
	if _, duplicate := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); duplicate != nil {
		t.Fatal("duplicate write started while saving")
	}
	press(m, "esc")
	if m.commentModal != "edit" {
		t.Fatal("pending editor was dismissed")
	}
	m.Update(cmd())
	if !m.hasAlert() || !strings.Contains(m.View(), "403") || m.commentSaving || m.commentModal != "edit" || string(m.noteInput) != "New review\n中文" {
		t.Fatal("permission error lost the draft or was hidden")
	}
	items, err := review.LoadComments(m.notes.path)
	if err != nil || len(items) != 2 || items[1].RemoteID != 0 {
		t.Fatalf("draft not saved before network failure: %+v %v", items, err)
	}
	press(m, "enter")
	f.err = nil
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(cmd())
	if m.commentModal != "list" || len(m.notes.items) != 2 || m.notes.items[1].RemoteID != 101 || f.writes != 2 {
		t.Fatal("retry duplicated the draft instead of replacing it")
	}
	if !strings.Contains(m.View(), "@me") {
		t.Fatal("GitHub author is not visible")
	}
}

func TestGitHubCommentEditReplyDeleteAndCacheFailure(t *testing.T) {
	m, f := githubCommentModel(t)
	press(m, "C")
	press(m, "enter")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	notePaste(m, "Unsynced edit")
	f.err = errors.New("permission denied")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(cmd())
	if m.notes.items[0].Body != "Existing comment" || m.notes.items[0].DraftBody != "Unsynced edit" {
		t.Fatal("failed edit overwrote the server body")
	}
	press(m, "enter")
	press(m, "esc")
	// Refresh replaces server data but retains a persisted edit draft.
	m.install(m.snapshot)
	if m.notes.items[0].DraftBody != "Unsynced edit" {
		t.Fatal("refresh discarded unsynced edit")
	}
	press(m, "enter")
	if string(m.noteInput) != "Unsynced edit" {
		t.Fatal("editor did not restore unsynced body")
	}
	press(m, "esc")
	press(m, "a")
	if m.commentDraft.ReplyTo != 41 || m.commentDraft.RemoteID != 0 || len(m.noteInput) != 0 {
		t.Fatal("reply did not create an empty draft on the original thread")
	}
	notePaste(m, "Reply")
	f.err = nil
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	// Local caching may fail after a successful remote mutation.
	m.notes.path += "/not-a-directory"
	m.Update(cmd())
	if m.commentModal != "list" || m.notes.items[len(m.notes.items)-1].RemoteID != 101 || !m.hasAlert() {
		t.Fatal("remote success was lost after local cache failure")
	}
	for m.hasAlert() {
		press(m, "enter")
	}
	m.notes.path = ""
	press(m, "d")
	f.err = errors.New("HTTP 403")
	cmd = m.activate("enter")
	if cmd == nil {
		t.Fatal("delete did not reach GitHub")
	}
	m.Update(cmd())
	if !m.hasAlert() || len(m.notes.items) != 2 || m.commentModal != "delete" {
		t.Fatal("failed delete removed the server comment")
	}
	press(m, "enter")
	f.err = nil
	cmd = m.activate("enter")
	m.Update(cmd())
	if len(m.notes.items) != 1 || f.deletes != 2 || m.commentModal != "list" {
		t.Fatal("delete success did not update the list")
	}
}

func TestGitHubCommentLoadErrorOutdatedAndNoState(t *testing.T) {
	m, f := githubCommentModel(t)
	m.snapshot.CommentsError = "HTTP 403: cannot read comments"
	m.install(m.snapshot)
	if !m.hasAlert() || !strings.Contains(m.View(), "403") || len(m.notes.items) != 1 {
		t.Fatal("read failure hid the error or discarded cached comments")
	}
	press(m, "enter")
	m.notes.items[0].Outdated = true
	if strings.Contains(m.renderRowContent(m.rows[2], 80), "●") {
		t.Fatal("outdated comment marked an unrelated current line")
	}
	m.jumpComment(m.notes.items[0])
	if !m.hasAlert() {
		t.Fatal("outdated comment silently jumped into current code")
	}
	press(m, "enter")
	m.snapshot.Persistence = backend.Memory
	m.notes.path = ""
	press(m, "cc")
	notePaste(m, "Memory only")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || f.writes != 0 || m.commentSaving {
		t.Fatal("--no-state synchronized a comment")
	}
}

func TestGitHubCommentShortcutAndMouseSave(t *testing.T) {
	m, f := githubCommentModel(t)
	press(m, "C")
	if m.commentModal != "list" || m.collapsed["main.go"] {
		t.Fatal("C collapsed files instead of opening comments")
	}
	press(m, "esc")
	press(m, "z")
	if !m.collapsed["main.go"] {
		t.Fatal("z did not collapse files")
	}
	press(m, "zcc")
	notePaste(m, "Mouse save")
	g := m.modalLayout()
	_, cmd := m.Update(tea.MouseMsg{X: g.x + 3, Y: g.y + g.height - 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd == nil {
		t.Fatal("clicking GitHub save did not return its command")
	}
	m.Update(cmd())
	if f.writes != 1 || m.commentSaving {
		t.Fatal("mouse save did not finish")
	}
}
