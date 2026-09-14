package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type commentServer struct {
	base                  *fakeGitHub
	comments              []map[string]any
	requests              []map[string]any
	methods               []string
	writeError, readError error
	prURL                 string
	resolved              map[string]bool
	denyResolve           bool
	threadMutations       []string
}

func commentRecord(id int64, body string) map[string]any {
	return map[string]any{"id": id, "body": body, "path": "nested/new.go", "side": "RIGHT", "line": 1,
		"diff_hunk": "@@ -0,0 +1 @@\n+package main", "html_url": fmt.Sprintf("https://github.com/owner/repo/pull/918#discussion_r%d", id),
		"pull_request_url": "https://api.github.com/repos/owner/repo/pulls/918", "user": map[string]any{"login": "me", "node_id": "user-id"}}
}

func (f *commentServer) run(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	if input != nil {
		var request struct {
			Query     string
			Variables map[string]any
		}
		if err := json.Unmarshal(input, &request); err != nil {
			return nil, err
		}
		if strings.Contains(request.Query, "reviewThreads(") {
			return f.threadResponse(request.Variables)
		}
		if strings.Contains(request.Query, "resolveReviewThread(") {
			mutation := "resolveReviewThread"
			if strings.Contains(request.Query, "unresolveReviewThread(") {
				mutation = "unresolveReviewThread"
			}
			f.threadMutations = append(f.threadMutations, mutation)
			if f.writeError != nil {
				return encoded(map[string]any{"errors": []map[string]any{{"message": f.writeError.Error()}}})
			}
			id := request.Variables["input"].(map[string]any)["threadId"].(string)
			if f.resolved == nil {
				f.resolved = make(map[string]bool)
			}
			f.resolved[id] = mutation == "resolveReviewThread"
			return encoded(map[string]any{"data": map[string]any{mutation: map[string]any{"thread": map[string]any{"id": id, "isResolved": f.resolved[id]}}}})
		}
	}
	endpoint, method := "", "GET"
	for i, arg := range args {
		if arg == "--method" {
			method = args[i+1]
		}
		if strings.HasPrefix(arg, "repos/") {
			endpoint = arg
		}
	}
	if !strings.Contains(endpoint, "/comments") {
		return f.base.run(ctx, input, args...)
	}
	if method == "GET" {
		if f.readError != nil {
			return nil, f.readError
		}
		if strings.Contains(endpoint, "?") {
			page := 0
			fmt.Sscanf(endpoint, "repos/owner/repo/pulls/918/comments?per_page=100&page=%d", &page)
			if page < 1 {
				return nil, errors.New("invalid comment pagination")
			}
			start := min(len(f.comments), (page-1)*100)
			return encoded(f.comments[start:min(len(f.comments), start+100)])
		}
		var id int64
		fmt.Sscanf(endpoint, "repos/owner/repo/pulls/comments/%d", &id)
		for _, c := range f.comments {
			if c["id"] == id {
				if f.prURL != "" {
					c["pull_request_url"] = f.prURL
				}
				return encoded(c)
			}
		}
		return nil, errors.New("HTTP 404")
	}
	var body map[string]any
	if input != nil {
		if err := json.Unmarshal(input, &body); err != nil {
			return nil, err
		}
	}
	f.requests = append(f.requests, body)
	f.methods = append(f.methods, method+" "+endpoint)
	if f.writeError != nil {
		return nil, f.writeError
	}
	if method == "DELETE" {
		return nil, nil
	}
	id := int64(1001)
	if method == "PATCH" {
		fmt.Sscanf(endpoint, "repos/owner/repo/pulls/comments/%d", &id)
	}
	result := commentRecord(id, body["body"].(string))
	for _, name := range []string{"path", "side", "line", "start_line", "start_side"} {
		if value, ok := body[name]; ok {
			result[name] = value
		}
	}
	if strings.HasSuffix(endpoint, "/replies") {
		result["in_reply_to_id"] = int64(41)
	}
	return encoded(result)
}

func (f *commentServer) threadResponse(vars map[string]any) ([]byte, error) {
	var nodes []map[string]any
	seen := make(map[int64]bool)
	for _, c := range f.comments {
		id := c["id"].(int64)
		root := id
		if parent, ok := c["in_reply_to_id"].(int64); ok && parent > 0 {
			root = parent
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		threadID := fmt.Sprintf("thread-%d", id)
		nodes = append(nodes, map[string]any{"id": threadID, "isResolved": f.resolved[threadID], "viewerCanResolve": !f.denyResolve, "viewerCanUnresolve": !f.denyResolve,
			"comments": map[string]any{"nodes": []map[string]any{{"fullDatabaseId": fmt.Sprint(id)}}}})
	}
	start := 0
	if cursor, ok := vars["cursor"].(string); ok {
		fmt.Sscanf(cursor, "thread-%d", &start)
	}
	end := min(len(nodes), start+100)
	return encoded(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
		"id": "pr-id", "baseRefOid": "base-sha", "headRefOid": f.base.head,
		"reviewThreads": map[string]any{"nodes": nodes[start:end], "pageInfo": map[string]any{"hasNextPage": end < len(nodes), "endCursor": fmt.Sprintf("thread-%d", end)}},
	}}}})
}

func commentBackend(t *testing.T) (*GitHub, *commentServer, *Snapshot) {
	t.Helper()
	f := &commentServer{base: fakeServer(), comments: []map[string]any{commentRecord(41, "Existing review")}}
	g := fakeBackend(f.base, true)
	g.run = f.run
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil || s.CommentsError != "" {
		t.Fatalf("load: %v %s", err, s.CommentsError)
	}
	return g, f, s
}

func TestGitHubCommentsLoadCreateEditReplyDelete(t *testing.T) {
	g, f, s := commentBackend(t)
	if len(s.Comments) != 1 || s.Comments[0].Author != "me" || s.Comments[0].Code != "package main" {
		t.Fatalf("comments: %+v", s.Comments)
	}
	c := review.Comment{ID: "draft", Path: "nested/new.go", Side: "new", Start: 1, End: 1, Code: "package main", Body: "中文 **comment**\n`literal $(text)`"}
	saved, err := g.SaveComment(context.Background(), s, c)
	if err != nil || saved.RemoteID != 1001 || saved.Body != c.Body {
		t.Fatalf("create: %+v %v", saved, err)
	}
	req := f.requests[0]
	if req["commit_id"] != "head-sha" || req["line"] != float64(1) || req["side"] != "RIGHT" || req["path"] != c.Path || req["body"] != c.Body {
		t.Fatalf("wrong anchor: %+v", req)
	}
	if _, ok := req["start_line"]; ok {
		t.Fatal("single-line comment included a range")
	}
	c = s.Comments[0]
	c.Body = "Edited"
	if _, err := g.SaveComment(context.Background(), s, c); err != nil {
		t.Fatal(err)
	}
	if f.methods[1] != "PATCH repos/owner/repo/pulls/comments/41" || len(f.requests[1]) != 1 {
		t.Fatal("edit used incorrect endpoint or fields")
	}
	c.RemoteID, c.ID, c.ReplyTo, c.Body = 0, "reply", 41, "Reply"
	if _, err := g.SaveComment(context.Background(), s, c); err != nil {
		t.Fatal(err)
	}
	if f.methods[2] != "POST repos/owner/repo/pulls/918/comments/41/replies" || len(f.requests[2]) != 1 {
		t.Fatal("reply did not target the thread")
	}
	if err := g.DeleteComment(context.Background(), s, s.Comments[0]); err != nil {
		t.Fatal(err)
	}
	if f.methods[3] != "DELETE repos/owner/repo/pulls/comments/41" {
		t.Fatal("wrong delete endpoint")
	}
}

func TestGitHubCommentRangesPermissionsAndSnapshotGuards(t *testing.T) {
	for _, scenario := range []string{"range", "permission", "delete-permission", "stale", "account", "foreign-pr", "other-author", "invalid-anchor", "no-state"} {
		t.Run(scenario, func(t *testing.T) {
			g, f, s := commentBackend(t)
			c := review.Comment{ID: "draft", Path: "nested/new.go", Side: "new", Start: 1, End: 1, Code: "package main", Body: "Review"}
			switch scenario {
			case "range":
				s.Comparison.Files[1].Hunks[0].Lines = []diff.Line{{Kind: '-', Old: 5, Text: "old one"}, {Kind: '-', Old: 6, Text: "old two"}}
				c.Side, c.Start, c.End, c.Code = "old", 5, 6, "old one\nold two"
			case "permission", "delete-permission":
				f.writeError = errors.New("Resource not accessible by personal access token (HTTP 403)")
			case "stale":
				f.base.head = "moved"
			case "account":
				f.base.viewerID = "another-account"
			case "foreign-pr":
				c = s.Comments[0]
				f.prURL = "https://api.github.com/repos/owner/repo/pulls/919"
			case "other-author":
				c = s.Comments[0]
				f.comments[0]["user"] = map[string]any{"login": "other", "node_id": "other-id"}
			case "invalid-anchor":
				c.End = 100
			case "no-state":
				g.Persist = false
			}
			var err error
			if scenario == "delete-permission" {
				err = g.DeleteComment(context.Background(), s, s.Comments[0])
			} else {
				_, err = g.SaveComment(context.Background(), s, c)
			}
			if scenario == "range" {
				if err != nil {
					t.Fatal(err)
				}
				r := f.requests[0]
				if r["start_side"] != "LEFT" || r["side"] != "LEFT" || r["start_line"] != float64(5) || r["line"] != float64(6) {
					t.Fatalf("wrong range: %v", r)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if scenario == "permission" || scenario == "delete-permission" {
				if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "permission") {
					t.Fatalf("permission detail lost: %v", err)
				}
			} else if len(f.requests) != 0 {
				t.Fatal("unsafe write reached GitHub")
			}
		})
	}
}

func TestGitHubCommentsPaginationOutdatedAndLoadFailure(t *testing.T) {
	g, f, _ := commentBackend(t)
	f.comments = nil
	for i := int64(1); i <= 101; i++ {
		f.comments = append(f.comments, commentRecord(i, "Comment"))
	}
	f.comments[0]["line"] = nil
	f.comments[0]["original_line"] = 1
	f.comments[1]["subject_type"], f.comments[1]["line"] = "file", nil
	f.comments[2]["line"], f.comments[2]["start_line"], f.comments[2]["start_side"] = 2, 5, "LEFT"
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil || s.CommentsError != "" || len(s.Comments) != 101 {
		t.Fatalf("pagination: %v %s", err, s.CommentsError)
	}
	if !s.Comments[0].Outdated || s.Comments[1].Outdated || s.Comments[1].Start != 0 || s.Comments[2].StartSide != "old" {
		t.Fatal("lost outdated/file/cross-side comment metadata")
	}
	f.readError = errors.New("HTTP 403: permission denied")
	s, err = g.Load(context.Background(), diff.ModePullRequest)
	if err != nil || !strings.Contains(s.CommentsError, "403") || len(s.Comments) != 0 {
		t.Fatal("comment failure should reach the TUI without discarding the diff")
	}
	g.Persist = false
	s, err = g.Load(context.Background(), diff.ModePullRequest)
	if err != nil || s.CommentsError != "" || len(s.Comments) != 0 {
		t.Fatal("--no-state accessed remote comments")
	}
}
