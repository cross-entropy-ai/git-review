package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cross-entropy-ai/git-review/internal/review"
)

const reviewThreadsQuery = `query($owner:String!, $repo:String!, $number:Int!, $cursor:String) {
 repository(owner:$owner, name:$repo) { pullRequest(number:$number) {
  id baseRefOid headRefOid reviewThreads(first:100, after:$cursor) {
   nodes { id isResolved viewerCanResolve viewerCanUnresolve comments(first:1) { nodes { fullDatabaseId } } }
   pageInfo { hasNextPage endCursor }
  }
 } }
}`

type reviewThread struct {
	ID                                   string
	IsResolved                           bool
	ViewerCanResolve, ViewerCanUnresolve bool
	Comments                             struct {
		Nodes []struct{ FullDatabaseID json.Number }
	}
}

// Reading only the first comment per thread avoids truncating long discussions.
// REST reply IDs link every subsequent comment back to its thread's root.
func (g *GitHub) reviewThreads(ctx context.Context, prID, revision string) (map[int64]reviewThread, error) {
	threads := make(map[int64]reviewThread)
	seen := make(map[string]bool)
	var cursor any
	for page := 0; page < 1000; page++ {
		var data struct {
			Repository struct {
				PullRequest *struct {
					ID, BaseRefOID, HeadRefOID string
					ReviewThreads              struct {
						Nodes    []reviewThread
						PageInfo struct {
							HasNextPage bool
							EndCursor   string
						}
					}
				}
			}
		}
		vars := map[string]any{"owner": g.Target.Owner, "repo": g.Target.Repo, "number": g.Target.Number, "cursor": cursor}
		if err := g.graphql(ctx, reviewThreadsQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("read review threads: %w", err)
		}
		pr := data.Repository.PullRequest
		if pr == nil || pr.ID != prID || pr.BaseRefOID+":"+pr.HeadRefOID != revision {
			return nil, errors.New("PR changed while reading review threads; refresh")
		}
		for _, thread := range pr.ReviewThreads.Nodes {
			if thread.ID == "" || seen[thread.ID] || len(thread.Comments.Nodes) != 1 {
				return nil, errors.New("invalid review thread list; refresh")
			}
			root, err := thread.Comments.Nodes[0].FullDatabaseID.Int64()
			if err != nil || root <= 0 {
				return nil, errors.New("invalid review comment ID; refresh")
			}
			if _, exists := threads[root]; exists {
				return nil, errors.New("duplicate review thread root; refresh")
			}
			seen[thread.ID] = true
			threads[root] = thread
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			return threads, nil
		}
		next := pr.ReviewThreads.PageInfo.EndCursor
		if next == "" || next == cursor {
			return nil, errors.New("invalid review thread cursor")
		}
		cursor = next
	}
	return nil, errors.New("review thread pagination limit exceeded")
}

func attachThreads(comments []review.Comment, threads map[int64]reviewThread) {
	// If a root was deleted, the first remaining reply still identifies the
	// original root through in_reply_to_id.
	byRoot := make(map[int64]reviewThread)
	for _, c := range comments {
		if t, ok := threads[c.RemoteID]; ok {
			root := c.RemoteID
			if c.ReplyTo > 0 {
				root = c.ReplyTo
			}
			byRoot[root] = t
		}
	}
	for i, c := range comments {
		root := c.RemoteID
		if c.ReplyTo > 0 {
			root = c.ReplyTo
		}
		if t, ok := byRoot[root]; ok {
			comments[i].ThreadID, comments[i].Resolved = t.ID, t.IsResolved
		}
	}
}

// A deleted REST root cannot receive replies. Resolve the surviving thread
// through this PR's fresh comments and thread list, never a cached node alone.
func (g *GitHub) replyToSurvivingThread(ctx context.Context, s *Snapshot, c review.Comment) (review.Comment, error) {
	comments, err := g.comments(ctx)
	if err != nil {
		return c, err
	}
	threads, err := g.reviewThreads(ctx, s.RemoteID, s.Revision)
	if err != nil {
		return c, err
	}
	attachThreads(comments, threads)
	var anchor review.Comment
	for _, member := range comments {
		// GitHub can promote a surviving reply and clear its old reply_to ID.
		// In that case validate the cached thread against this PR's fresh
		// thread membership and path before using the thread reply mutation.
		promoted := c.ThreadID != "" && member.ThreadID == c.ThreadID && member.Path == c.Path
		if (member.ReplyTo == c.ReplyTo || promoted) && member.ThreadID != "" {
			anchor = member
			break
		}
	}
	if anchor.ThreadID == "" {
		return c, errors.New("the discussion no longer exists in this PR; the reply draft has been kept")
	}
	query := `mutation($input:AddPullRequestReviewThreadReplyInput!) {
 addPullRequestReviewThreadReply(input:$input) {
  comment { fullDatabaseId body url author { login ... on User { id } ... on Bot { id } } }
 }
}`
	var result struct {
		AddPullRequestReviewThreadReply struct {
			Comment *struct {
				FullDatabaseID json.Number
				Body, URL      string
				Author         struct{ Login, ID string }
			}
		}
	}
	if err := g.graphql(ctx, query, map[string]any{"input": map[string]any{"pullRequestReviewThreadId": anchor.ThreadID, "body": c.Body}}, &result); err != nil {
		return c, commentWriteError(err)
	}
	created := result.AddPullRequestReviewThreadReply.Comment
	if created == nil {
		return c, errors.New("GitHub did not confirm the reply; refresh before retrying")
	}
	id, err := created.FullDatabaseID.Int64()
	if err != nil || id <= 0 || created.Body != c.Body {
		return c, errors.New("GitHub did not confirm the reply; refresh before retrying")
	}
	root := anchor.ReplyTo
	if root == 0 {
		root = anchor.RemoteID
	}
	anchor.ID, anchor.RemoteID, anchor.ReplyTo = fmt.Sprintf("github:%d", id), id, root
	anchor.Body, anchor.URL = created.Body, created.URL
	anchor.Author, anchor.AuthorID = created.Author.Login, created.Author.ID
	anchor.DraftBody, anchor.PendingBody, anchor.PendingAfterID = "", "", 0
	return anchor, nil
}

func (g *GitHub) SetThreadResolved(parent context.Context, s *Snapshot, c review.Comment, resolved bool) (review.Comment, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	if err := g.checkCommentSnapshot(ctx, s); err != nil {
		return c, err
	}
	current, err := g.existingComment(ctx, c.RemoteID)
	if err != nil {
		return c, err
	}
	threads, err := g.reviewThreads(ctx, s.RemoteID, s.Revision)
	if err != nil {
		return c, err
	}
	root := current.ID
	if current.ReplyTo > 0 {
		root = current.ReplyTo
	}
	thread, ok := threads[root]
	if !ok {
		thread, ok = threads[current.ID]
	}
	// Fall back to a known thread ID only after checking membership against
	// its first remaining comment, never trust an arbitrary cached node ID.
	if !ok && c.ThreadID != "" {
		for first, candidate := range threads {
			if candidate.ID != c.ThreadID {
				continue
			}
			firstComment, err := g.existingComment(ctx, first)
			if err != nil {
				return c, err
			}
			if firstComment.ReplyTo == root {
				thread, ok = candidate, true
			}
			break
		}
	}
	if !ok {
		return c, errors.New("could not locate this review thread; refresh comments")
	}
	c.ThreadID = thread.ID
	if thread.IsResolved == resolved {
		c.Resolved = resolved
		return c, nil
	}
	if resolved && !thread.ViewerCanResolve || !resolved && !thread.ViewerCanUnresolve {
		return c, errors.New("permission denied: the active GitHub account cannot change this thread's resolved state")
	}
	mutation, inputType := "resolveReviewThread", "ResolveReviewThreadInput!"
	if !resolved {
		mutation, inputType = "unresolveReviewThread", "UnresolveReviewThreadInput!"
	}
	query := fmt.Sprintf("mutation($input:%s) { %s(input:$input) { thread { id isResolved } } }", inputType, mutation)
	var result map[string]struct {
		Thread *struct {
			ID         string
			IsResolved *bool
		}
	}
	if err := g.graphql(ctx, query, map[string]any{"input": map[string]any{"threadId": thread.ID}}, &result); err != nil {
		return c, err
	}
	updated := result[mutation].Thread
	if updated == nil || updated.ID != thread.ID || updated.IsResolved == nil || *updated.IsResolved != resolved {
		return c, errors.New("GitHub did not confirm the thread state; refresh before retrying")
	}
	c.Resolved = resolved
	return c, nil
}
