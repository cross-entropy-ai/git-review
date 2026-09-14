package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/diff"
)

type fakeGitHub struct {
	head, viewerID string
	count          int
	metadataReads  int
	changeOnRead   int
	authError      bool
	mutationError  bool
	mutations      []map[string]any
	files          map[int][]map[string]any
	states         []map[string]any
	viewedReads    int
}

func fakeServer() *fakeGitHub {
	return &fakeGitHub{head: "head-sha", viewerID: "user-id", count: 2,
		files: map[int][]map[string]any{1: {
			{"filename": "new 文件.txt", "previous_filename": "old 文件.txt", "status": "renamed", "sha": "blob1", "additions": 1, "deletions": 1, "patch": "@@ -1 +1 @@\n-old\n+new"},
			{"filename": "nested/new.go", "status": "added", "sha": "blob2", "additions": 1, "deletions": 0, "patch": "@@ -0,0 +1 @@\n+package main\n\\ No newline at end of file"},
		}},
		states: []map[string]any{{"path": "new 文件.txt", "viewerViewedState": "VIEWED"}, {"path": "nested/new.go", "viewerViewedState": "DISMISSED"}},
	}
}

func encoded(v any) ([]byte, error) { return json.Marshal(v) }

func (f *fakeGitHub) run(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, errors.New("missing gh arguments")
	}
	if args[0] == "auth" {
		if f.authError {
			return nil, errors.New("not logged in")
		}
		return []byte("authenticated"), nil
	}
	if args[0] != "api" {
		return nil, errors.New("backend attempted a non-API gh command")
	}
	if !strings.Contains(strings.Join(args, " "), "--hostname github.com") {
		return nil, errors.New("missing explicit GitHub host")
	}
	if input != nil {
		var request struct {
			Query     string
			Variables map[string]any
		}
		if err := json.Unmarshal(input, &request); err != nil {
			return nil, err
		}
		if strings.HasPrefix(request.Query, "mutation") {
			f.mutations = append(f.mutations, request.Variables)
			if f.mutationError {
				return encoded(map[string]any{"errors": []map[string]any{{"message": "permission denied"}}})
			}
			field := "markFileAsViewed"
			if strings.Contains(request.Query, "unmarkFileAsViewed") {
				field = "unmarkFileAsViewed"
			}
			return encoded(map[string]any{"data": map[string]any{field: map[string]any{"pullRequest": map[string]any{"id": "pr-id"}}}})
		}
		if strings.Contains(request.Query, "viewer { id }") {
			return encoded(map[string]any{"data": map[string]any{"viewer": map[string]any{"id": f.viewerID}}})
		}
		f.viewedReads++
		start, end := 0, len(f.states)
		if request.Variables["cursor"] == "next" {
			start = 100
		} else if end > 100 {
			end = 100
		}
		next := end < len(f.states)
		return encoded(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
			"id": "pr-id", "baseRefOid": "base-sha", "headRefOid": f.head,
			"files": map[string]any{"nodes": f.states[start:end], "pageInfo": map[string]any{"hasNextPage": next, "endCursor": "next"}},
		}}}})
	}
	endpoint := args[len(args)-1]
	if strings.Contains(endpoint, "/comments?") {
		return encoded([]any{})
	}
	if strings.Contains(endpoint, "/files?") {
		page := 0
		fmt.Sscanf(endpoint, "repos/owner/repo/pulls/918/files?per_page=100&page=%d", &page)
		return encoded(f.files[page])
	}
	if endpoint != "repos/owner/repo/pulls/918" {
		return nil, fmt.Errorf("unexpected endpoint %s", endpoint)
	}
	f.metadataReads++
	if f.changeOnRead == f.metadataReads {
		f.head = "new-head-sha"
	}
	return encoded(map[string]any{"node_id": "pr-id", "title": "Review title", "changed_files": f.count,
		"base": map[string]any{"sha": "base-sha", "ref": "main"}, "head": map[string]any{"sha": f.head, "ref": "feature"}})
}

func fakeBackend(f *fakeGitHub, persist bool) *GitHub {
	g := NewGitHub(PullRequest{Host: "github.com", Owner: "owner", Repo: "repo", Number: 918}, persist)
	g.run = f.run
	return g
}

func TestGitHubLoadAndViewedWrites(t *testing.T) {
	f := fakeServer()
	g := fakeBackend(f, true)
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != diff.ModePullRequest || s.Persistence != Remote || s.Viewed["nested/new.go"] != Dismissed || s.Viewed["new 文件.txt"] != Viewed {
		t.Fatalf("wrong snapshot: %+v", s)
	}
	if s.Comparison.GitDir != "" || s.Comparison.Root != "" || s.Comparison.Added != 2 || s.Comparison.Deleted != 1 || s.Comparison.Files[0].OldPath != "old 文件.txt" {
		t.Fatalf("wrong diff: %+v", s.Comparison)
	}
	if len(s.Comparison.Files[1].Hunks) != 1 || s.Comparison.Files[1].Hunks[0].Lines[1].Kind != '\\' {
		t.Fatal("lost patch or EOF marker")
	}
	for _, viewed := range []bool{true, false} {
		if err := g.SetViewed(context.Background(), s, "nested/new.go", viewed); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.mutations) != 2 {
		t.Fatal("Viewed changes did not reach GitHub")
	}
	input := f.mutations[0]["input"].(map[string]any)
	if input["path"] != "nested/new.go" || input["pullRequestId"] != "pr-id" {
		t.Fatalf("wrong mutation variables: %+v", input)
	}
	f.mutationError = true
	if err := g.SetViewed(context.Background(), s, "nested/new.go", true); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("GraphQL errors were ignored: %v", err)
	}
}

func TestGitHubRejectsMovingPRAndAccount(t *testing.T) {
	for _, scenario := range []string{"load", "head", "account", "unknown-file", "auth"} {
		t.Run(scenario, func(t *testing.T) {
			f := fakeServer()
			g := fakeBackend(f, true)
			if scenario == "load" {
				f.changeOnRead = 2
			}
			if scenario == "auth" {
				f.authError = true
			}
			s, err := g.Load(context.Background(), diff.ModePullRequest)
			if scenario == "load" || scenario == "auth" {
				if err == nil {
					t.Fatal("expected load error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			path := "nested/new.go"
			switch scenario {
			case "head":
				f.head = "new-head"
			case "account":
				f.viewerID = "another-user"
			case "unknown-file":
				path = "not-in-pr"
			}
			if err := g.SetViewed(context.Background(), s, path, true); err == nil {
				t.Fatal("expected stale snapshot error")
			}
			if len(f.mutations) != 0 {
				t.Fatal("unsafe mutation was sent")
			}
		})
	}
}

func TestGitHubPaginationAndIncompleteResponses(t *testing.T) {
	f := fakeServer()
	f.count = 101
	f.files = make(map[int][]map[string]any)
	f.states = nil
	for i := 0; i < 101; i++ {
		path := fmt.Sprintf("file-%d", i)
		f.files[1+i/100] = append(f.files[1+i/100], map[string]any{"filename": path, "status": "added", "additions": 1, "deletions": 0, "patch": "@@ -0,0 +1 @@\n+new"})
		f.states = append(f.states, map[string]any{"path": path, "viewerViewedState": "UNVIEWED"})
	}
	g := fakeBackend(f, true)
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil || len(s.Comparison.Files) != 101 || len(s.Viewed) != 101 || f.viewedReads != 2 {
		t.Fatalf("pagination failed: %v", err)
	}
	delete(f.files, 2)
	if _, err := g.Load(context.Background(), diff.ModePullRequest); err == nil {
		t.Fatal("silently accepted missing files")
	}
	f.count = 3001
	if _, err := g.Load(context.Background(), diff.ModePullRequest); err == nil {
		t.Fatal("accepted PR beyond API file limit")
	}
}

func TestGitHubNoStateAndMissingPatch(t *testing.T) {
	f := fakeServer()
	delete(f.files[1][0], "patch")
	f.files[1][1]["patch"] = "@@ -0,0 +1,2 @@\n+truncated"
	g := fakeBackend(f, false)
	s, err := g.Load(context.Background(), diff.ModePullRequest)
	if err != nil {
		t.Fatal(err)
	}
	if f.viewedReads != 0 || len(s.Viewed) != 0 || s.Persistence != Memory {
		t.Fatal("--no-state contacted Viewed storage")
	}
	if s.Warning == "" || len(s.Comparison.Files[0].Metadata) == 0 || len(s.Comparison.Files[1].Hunks) != 0 {
		t.Fatal("missing or truncated patches were presented as complete")
	}
	if err := g.SetViewed(context.Background(), s, "nested/new.go", true); err != nil || len(f.mutations) != 0 {
		t.Fatal("--no-state sent a mutation")
	}
}
