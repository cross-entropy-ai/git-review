package backend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/diff"
)

func TestGitHubResolveUnresolveAndPermissions(t *testing.T) {
	g, f, s := commentBackend(t)
	c := s.Comments[0]
	if c.ThreadID != "thread-41" || c.Resolved {
		t.Fatal("thread state was not loaded")
	}
	c.ThreadID = "forged-cached-node"
	for _, desired := range []bool{true, false} {
		saved, err := g.SetThreadResolved(context.Background(), s, c, desired)
		if err != nil || saved.Resolved != desired || saved.ThreadID != "thread-41" || saved.Body != c.Body {
			t.Fatalf("resolve %v: %+v %v", desired, saved, err)
		}
	}
	if len(f.threadMutations) != 2 || f.threadMutations[0] != "resolveReviewThread" || f.threadMutations[1] != "unresolveReviewThread" || len(f.requests) != 0 {
		t.Fatal("resolve edited or deleted a comment")
	}
	f.denyResolve = true
	if _, err := g.SetThreadResolved(context.Background(), s, c, true); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("permission failure missing: %v", err)
	}
	if len(f.threadMutations) != 2 {
		t.Fatal("mutation was sent despite denied permission")
	}
	f.denyResolve = false
	f.writeError = errors.New("HTTP 403: permission denied")
	if _, err := g.SetThreadResolved(context.Background(), s, c, true); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("server error lost: %v", err)
	}
	f.writeError = nil
	f.base.viewerID = "other-account"
	if _, err := g.SetThreadResolved(context.Background(), s, c, true); err == nil {
		t.Fatal("account change allowed")
	}
	f.base.viewerID = "user-id"
	f.base.head = "new-head"
	if _, err := g.SetThreadResolved(context.Background(), s, c, true); err == nil {
		t.Fatal("stale revision allowed")
	}
	f.base.head = "head-sha"
	g.Persist = false
	if _, err := g.SetThreadResolved(context.Background(), s, c, true); err == nil {
		t.Fatal("--no-state permitted resolve")
	}
	if len(f.threadMutations) != 3 {
		t.Fatal("guard failure reached mutation")
	}
}

func TestGitHubResolvedThreadsIncludeRepliesAndDeletedRoots(t *testing.T) {
	g, f, _ := commentBackend(t)
	for _, deletedRoot := range []bool{false, true} {
		root := commentRecord(41, "Root")
		reply := commentRecord(42, "Reply")
		reply["in_reply_to_id"] = int64(41)
		another := commentRecord(43, "Another reply")
		another["in_reply_to_id"] = int64(41)
		f.comments = []map[string]any{root, reply, another}
		thread := "thread-41"
		if deletedRoot {
			f.comments = f.comments[1:]
			thread = "thread-42"
		}
		f.resolved = map[string]bool{thread: true}
		s, err := g.Load(context.Background(), diff.ModePullRequest)
		if err != nil || s.CommentsError != "" {
			t.Fatalf("load: %v %s", err, s.CommentsError)
		}
		for _, c := range s.Comments {
			if c.ThreadID != thread || !c.Resolved {
				t.Fatalf("reply lost thread state: %+v", c)
			}
		}
		saved, err := g.SetThreadResolved(context.Background(), s, s.Comments[len(s.Comments)-1], false)
		if err != nil || saved.Resolved || saved.ThreadID != thread {
			t.Fatalf("reopen from reply: %+v %v", saved, err)
		}
	}
}

func TestGitHubThreadIDsAre64Bit(t *testing.T) {
	g, f, _ := commentBackend(t)
	const id int64 = 9007199254740993
	f.comments = []map[string]any{commentRecord(id, "Large ID")}
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil || s.CommentsError != "" || len(s.Comments) != 1 || s.Comments[0].ThreadID != "thread-9007199254740993" {
		t.Fatalf("64-bit thread mapping failed: %v %s", err, s.CommentsError)
	}
}
