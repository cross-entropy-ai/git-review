package tui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type commentSavedMsg struct {
	key             string
	original, saved review.Comment
	deleted         bool
	threadUpdate    bool
	err             error
}

func (m *Model) syncComments() bool {
	_, ok := m.source.(backend.CommentBackend)
	return ok && m.snapshot.Persistence == backend.Remote
}

func (m *Model) mergeGitHubComments() {
	if !m.syncComments() {
		return
	}
	if m.snapshot.CommentsError != "" {
		m.showAlert("Cannot load GitHub comments", m.snapshot.CommentsError)
		return
	}
	drafts := make(map[int64]string)
	var items []review.Comment
	reconciled := false
	for _, c := range m.notes.items {
		if c.RemoteID == 0 {
			index, err := review.MatchPendingComment(c, m.snapshot.Comments, m.snapshot.Viewer)
			if err != nil {
				m.showAlert("Cannot reconcile comment", err.Error())
			}
			if index >= 0 {
				reconciled = true
				remote := m.snapshot.Comments[index]
				if c.Body != remote.Body {
					drafts[remote.RemoteID] = c.Body
				}
				continue
			}
			if c.ReplyTo == 0 && c.Revision != "" && c.Revision != m.snapshot.Revision {
				c.Outdated = false
				c.Outdated = review.ValidateCommentAnchor(m.comparison, c) != nil
				if !c.Outdated {
					c.Revision = m.snapshot.Revision
					// Keep a pending submission's original commit for reconciliation.
					if c.PendingBody == "" {
						c.CommitID = m.comparison.HeadOID
					}
				}
			}
			items = append(items, c)
		} else if c.DraftBody != "" {
			if c.DraftBody != c.Body {
				drafts[c.RemoteID] = c.DraftBody
			}
		}
	}
	for _, c := range m.snapshot.Comments {
		if body, ok := drafts[c.RemoteID]; ok && body == c.Body {
			reconciled = true
		}
		if drafts[c.RemoteID] != c.Body {
			c.DraftBody = drafts[c.RemoteID]
		}
		delete(drafts, c.RemoteID)
		items = append(items, c)
	}
	// Do not lose a draft when its server comment was deleted by someone else.
	for _, c := range m.notes.items {
		if body, ok := drafts[c.RemoteID]; ok {
			c.ID = fmt.Sprintf("draft:%d", c.RemoteID)
			c.RemoteID, c.Author, c.AuthorID, c.URL = 0, "", "", ""
			c.Body, c.DraftBody = body, ""
			items = append(items, c)
		}
	}
	m.notes.items = items
	if reconciled && m.notes.err == nil {
		m.persistComments(items)
	}
}

func (m *Model) sendComment(c review.Comment, deleted bool) tea.Cmd {
	source := m.source.(backend.CommentBackend)
	snapshot := m.snapshot
	m.commentSaving = true
	m.message = "Saving comment to GitHub…"
	if deleted {
		m.message = "Deleting GitHub comment…"
	}
	ctx := m.ctx
	return func() tea.Msg {
		msg := commentSavedMsg{key: snapshot.Key, original: c, deleted: deleted}
		if deleted {
			msg.err = source.DeleteComment(ctx, snapshot, c)
		} else {
			msg.saved, msg.err = source.SaveComment(ctx, snapshot, c)
		}
		return msg
	}
}

func (m *Model) finishComment(msg commentSavedMsg) {
	if msg.key != m.snapshot.Key || !m.commentSaving {
		return
	}
	m.commentSaving = false
	if msg.err != nil {
		var rejected *backend.CommentNotSubmittedError
		if msg.original.RemoteID == 0 && msg.original.PendingBody == "" && errors.As(msg.err, &rejected) {
			items := append([]review.Comment(nil), m.notes.items...)
			for i := range items {
				if items[i].ID == msg.original.ID {
					items[i].PendingBody, items[i].PendingAfterID = "", 0
				}
			}
			m.commentDraft.PendingBody, m.commentDraft.PendingAfterID = "", 0
			m.persistComments(items)
		}
		title := "GitHub comment save failed"
		if msg.deleted {
			title = "GitHub comment deletion failed"
		}
		if msg.threadUpdate {
			title = "GitHub resolve update failed"
		}
		m.showAlert(title, msg.err.Error())
		return
	}
	var items []review.Comment
	for _, c := range m.notes.items {
		if msg.threadUpdate {
			root := msg.original.RemoteID
			if msg.original.ReplyTo > 0 {
				root = msg.original.ReplyTo
			}
			if c.RemoteID == root || c.RemoteID == msg.original.RemoteID || c.ReplyTo == root || msg.saved.ThreadID != "" && c.ThreadID == msg.saved.ThreadID {
				c.ThreadID, c.Resolved = msg.saved.ThreadID, msg.saved.Resolved
			}
			items = append(items, c)
			continue
		}
		if c.ID == msg.original.ID || msg.original.RemoteID > 0 && c.RemoteID == msg.original.RemoteID {
			continue
		}
		items = append(items, c)
	}
	if !msg.deleted && !msg.threadUpdate {
		items = append(items, msg.saved)
	}
	// The server has already succeeded. Keep its result in memory even when
	// caching fails, so another Save cannot accidentally publish a duplicate.
	m.notes.items = items
	if !m.persistComments(items) {
		m.showAlert("GitHub comment synced", "GitHub completed the operation, but the local cache could not be saved. Refresh to reload the server result.")
	}
	m.commentModal, m.lineSelecting, m.rangeSelecting = "list", false, false
	if !msg.threadUpdate {
		m.commentIndex = max(0, len(items)-1)
	}
	m.clampCommentSelection()
	m.message = "Comment saved on GitHub"
	if msg.deleted {
		m.message = "Comment deleted on GitHub"
	}
	if msg.threadUpdate {
		m.message = "Discussion resolved on GitHub"
		if !msg.saved.Resolved {
			m.message = "Discussion reopened on GitHub"
		}
	}
}

func (m *Model) toggleCommentResolved() tea.Cmd {
	if m.commentPosition() < 0 {
		return nil
	}
	c := m.notes.items[m.commentIndex]
	resolved := !c.Resolved
	if !m.syncComments() {
		items := append([]review.Comment(nil), m.notes.items...)
		items[m.commentIndex].Resolved = resolved
		if m.persistComments(items) {
			m.message = "Comment resolved"
			if !resolved {
				m.message = "Comment reopened"
			}
		}
		return nil
	}
	if c.RemoteID == 0 {
		m.showAlert("Cannot resolve a draft", "Save this comment to GitHub before resolving the discussion.")
		return nil
	}
	source, ok := m.source.(backend.ThreadBackend)
	if !ok {
		m.showAlert("Cannot resolve discussion", "This backend does not support review thread resolution.")
		return nil
	}
	m.commentSaving = true
	m.message = "Resolving GitHub discussion…"
	if !resolved {
		m.message = "Reopening GitHub discussion…"
	}
	ctx, snapshot := m.ctx, m.snapshot
	return func() tea.Msg {
		saved, err := source.SetThreadResolved(ctx, snapshot, c, resolved)
		return commentSavedMsg{key: snapshot.Key, original: c, saved: saved, threadUpdate: true, err: err}
	}
}

func (m *Model) replyComment() {
	if !m.syncComments() || m.commentPosition() < 0 {
		return
	}
	c := m.notes.items[m.commentIndex]
	if c.RemoteID == 0 {
		m.showAlert("Cannot reply yet", "Save this local draft to GitHub before starting a reply.")
		return
	}
	root := c.RemoteID
	if c.ReplyTo > 0 {
		root = c.ReplyTo
	}
	c.ID, c.RemoteID, c.ReplyTo = "", 0, root
	c.Body, c.DraftBody, c.Author, c.AuthorID, c.URL = "", "", "", "", ""
	m.commentDraft, m.commentReturn = c, "list"
	m.startCommentEditor()
}
