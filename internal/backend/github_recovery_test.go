package backend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func TestReplyAfterRootDeletedUsesVerifiedThread(t *testing.T) {
	g, f, _ := commentBackend(t)
	reply := commentRecord(42, "Remaining reply")
	reply["in_reply_to_id"] = int64(41)
	f.comments = []map[string]any{reply}
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Comments[0]
	c.RemoteID, c.ID, c.Body, c.ThreadID = 0, "draft", "Continue the discussion", "forged-thread"
	saved, err := g.SaveComment(context.Background(), s, c)
	if err != nil || saved.RemoteID != 1002 || saved.ReplyTo != 41 || saved.ThreadID != "thread-42" || saved.Body != c.Body {
		t.Fatalf("reply to surviving thread: %+v %v", saved, err)
	}
	if len(f.threadReplies) != 1 || f.threadReplies[0]["pullRequestReviewThreadId"] != "thread-42" || len(f.requests) != 0 {
		t.Fatal("used an unverified thread or REST reply")
	}
	f.writeError = errors.New("HTTP 403")
	if _, err := g.SaveComment(context.Background(), s, c); err == nil {
		t.Fatal("permission error hidden")
	}
	f.writeError, f.comments = nil, nil
	if _, err := g.SaveComment(context.Background(), s, c); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("missing thread: %v", err)
	}
}

func TestReplyDraftSurvivesGitHubRootPromotion(t *testing.T) {
	g, f, _ := commentBackend(t)
	// GitHub promotes the first surviving reply by clearing in_reply_to_id.
	f.comments = []map[string]any{commentRecord(42, "Promoted reply")}
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Comments[0]
	c.ID, c.RemoteID, c.ReplyTo, c.Body = "draft", 0, 41, "Reply from the previously opened editor"
	saved, err := g.SaveComment(context.Background(), s, c)
	if err != nil || saved.ReplyTo != 42 || saved.ThreadID != c.ThreadID || len(f.threadReplies) != 1 {
		t.Fatalf("promoted root: %+v %v", saved, err)
	}
	c.ThreadID = "foreign-thread"
	if _, err := g.SaveComment(context.Background(), s, c); err == nil || len(f.threadReplies) != 1 {
		t.Fatal("unverified cached thread received a reply")
	}
}

func TestPendingSubmissionNeverPostsAgain(t *testing.T) {
	g, f, s := commentBackend(t)
	c := review.Comment{ID: "draft", Path: "nested/new.go", Side: "new", Start: 1, End: 1, Code: "package main", Body: "Already sent", PendingBody: "Already sent", PendingAfterID: 41}
	f.comments = append(f.comments, commentRecord(1001, c.Body))
	saved, err := g.SaveComment(context.Background(), s, c)
	if err != nil || saved.RemoteID != 1001 || len(f.requests) != 0 {
		t.Fatalf("duplicate post: %+v %v", saved, err)
	}
	c.Body = "Edited draft"
	saved, err = g.SaveComment(context.Background(), s, c)
	if err != nil || saved.Body != c.Body || len(f.methods) != 1 || !strings.HasPrefix(f.methods[0], "PATCH ") {
		t.Fatalf("reconciled edit: %+v %v", saved, err)
	}
	f.comments = f.comments[:1]
	if _, err := g.SaveComment(context.Background(), s, c); err == nil {
		t.Fatal("uncertain submission was blindly retried")
	}
	if len(f.methods) != 1 {
		t.Fatal("uncertain submission reached a write")
	}
	f.comments = append(f.comments, commentRecord(1001, c.PendingBody), commentRecord(1002, c.PendingBody))
	if _, err := g.SaveComment(context.Background(), s, c); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous reconciliation: %v", err)
	}
}

func TestCreateDistinguishesRejectedAndUncertainRequests(t *testing.T) {
	g, f, s := commentBackend(t)
	c := review.Comment{Path: "nested/new.go", Side: "new", Start: 1, End: 1, Code: "package main", Body: "Test"}
	for _, test := range []struct {
		message  string
		rejected bool
	}{{"HTTP 403: denied", true}, {"connection reset", false}} {
		f.writeError = errors.New(test.message)
		_, err := g.SaveComment(context.Background(), s, c)
		var rejected *CommentNotSubmittedError
		if err == nil || errors.As(err, &rejected) != test.rejected {
			t.Fatalf("%s: %v", test.message, err)
		}
	}
	f.base.head = "new-head"
	_, err := g.SaveComment(context.Background(), s, c)
	var rejected *CommentNotSubmittedError
	if !errors.As(err, &rejected) {
		t.Fatalf("preflight failure marked uncertain: %v", err)
	}
}
