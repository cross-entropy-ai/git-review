package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type GitHub struct {
	Target  PullRequest
	Persist bool
	run     ghRunner
	mu      sync.Mutex // Serialize writes, including revision and active-account checks.
}

func NewGitHub(target PullRequest, persist bool) *GitHub {
	return &GitHub{Target: target, Persist: persist, run: runGH}
}

func (g *GitHub) Modes() []diff.Mode { return []diff.Mode{diff.ModePullRequest} }

type prMetadata struct {
	NodeID       string `json:"node_id"`
	Title        string
	ChangedFiles int `json:"changed_files"`
	Base         struct {
		SHA string
		Ref string
	}
	Head struct {
		SHA string
		Ref string
	}
}

func (p prMetadata) revision() string { return p.Base.SHA + ":" + p.Head.SHA }

func (g *GitHub) metadata(ctx context.Context) (prMetadata, error) {
	var p prMetadata
	if err := g.api(ctx, g.Target.endpoint(), &p); err != nil {
		return p, fmt.Errorf("read PR #%d in %s/%s (check repository access and PR number): %w", g.Target.Number, g.Target.Owner, g.Target.Repo, err)
	}
	if p.NodeID == "" || p.Base.SHA == "" || p.Head.SHA == "" || p.ChangedFiles < 0 {
		return p, errors.New("incomplete PR metadata from GitHub")
	}
	return p, nil
}

func (g *GitHub) viewer(ctx context.Context) (string, error) {
	var data struct{ Viewer struct{ ID string } }
	err := g.graphql(ctx, `query { viewer { id } }`, nil, &data)
	if err == nil && data.Viewer.ID == "" {
		err = errors.New("GitHub did not identify the logged-in user")
	}
	return data.Viewer.ID, err
}

func (g *GitHub) Load(parent context.Context, mode diff.Mode) (*Snapshot, error) {
	if mode != diff.ModeAuto && mode != diff.ModePullRequest {
		return nil, errors.New("GitHub PR review cannot switch to a local scope")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	if _, err := g.run(ctx, nil, "auth", "status", "--active", "--hostname", g.Target.Host); err != nil {
		return nil, fmt.Errorf("GitHub review needs gh installed and an active login for %s; run gh auth login --hostname %s: %w", g.Target.Host, g.Target.Host, err)
	}
	viewer, err := g.viewer(ctx)
	if err != nil {
		return nil, fmt.Errorf("check GitHub login: %w", err)
	}
	p, err := g.metadata(ctx)
	if err != nil {
		return nil, err
	}
	if p.ChangedFiles > 3000 {
		return nil, fmt.Errorf("PR has %d files; GitHub's files API is limited to 3000, so a complete review cannot be loaded", p.ChangedFiles)
	}
	files, warning, err := g.files(ctx, p.ChangedFiles)
	if err != nil {
		return nil, err
	}
	states := make(map[string]ViewedState)
	if g.Persist {
		states, err = g.viewed(ctx, p)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if _, ok := states[file.Path]; !ok {
				return nil, errors.New("PR files and Viewed states differ; refresh to load a consistent comparison")
			}
		}
	}
	var comments []review.Comment
	commentError := ""
	if g.Persist {
		comments, err = g.comments(ctx)
		if err == nil && len(comments) > 0 {
			var threads map[int64]reviewThread
			threads, err = g.reviewThreads(ctx, p.NodeID, p.revision())
			if err == nil {
				attachThreads(comments, threads)
			}
		}
		if err != nil {
			commentError = err.Error()
		}
	}
	// REST pagination and GraphQL are separate reads. Reject a moving target.
	latest, err := g.metadata(ctx)
	if err != nil {
		return nil, err
	}
	if latest.NodeID != p.NodeID || latest.revision() != p.revision() || latest.ChangedFiles != p.ChangedFiles {
		return nil, errors.New("PR changed while loading; refresh to review the new version")
	}
	currentViewer, err := g.viewer(ctx)
	if err != nil {
		return nil, err
	}
	if currentViewer != viewer {
		return nil, errors.New("active GitHub account changed while loading; refresh")
	}
	c := &diff.Comparison{Base: p.Base.Ref, Head: p.Head.Ref, BaseOID: p.Base.SHA, HeadOID: p.Head.SHA, Files: files}
	for _, file := range files {
		c.Added += file.Added
		c.Deleted += file.Deleted
	}
	s := &Snapshot{Comparison: c, Mode: diff.ModePullRequest, Label: fmt.Sprintf("%s/%s #%d", g.Target.Owner, g.Target.Repo, g.Target.Number),
		Title: p.Title, URL: g.Target.URL(), Viewer: viewer, RemoteID: p.NodeID, Revision: p.revision(), Viewed: states, Persistence: Memory, Warning: warning}
	s.CommentKey = strings.Join([]string{g.Target.Host, p.NodeID, viewer}, ":")
	s.Key = s.CommentKey + ":" + p.revision()
	if g.Persist {
		s.Persistence = Remote
	}
	s.Comments, s.CommentsError = comments, commentError
	return s, nil
}

type apiFile struct {
	Filename         string
	PreviousFilename string `json:"previous_filename"`
	Status           string
	SHA              string
	Additions        int
	Deletions        int
	Patch            *string
}

func (g *GitHub) files(ctx context.Context, count int) ([]diff.File, string, error) {
	var files []diff.File
	seen := make(map[string]bool)
	missing := 0
	for page := 1; len(files) < count; page++ {
		var records []apiFile
		if err := g.api(ctx, fmt.Sprintf("%s/files?per_page=100&page=%d", g.Target.endpoint(), page), &records); err != nil {
			return nil, "", fmt.Errorf("read PR files: %w", err)
		}
		if len(records) == 0 || len(files)+len(records) > count {
			return nil, "", errors.New("GitHub returned an incomplete or changing PR file list; refresh")
		}
		for _, record := range records {
			if record.Filename == "" || seen[record.Filename] || record.Additions < 0 || record.Deletions < 0 {
				return nil, "", errors.New("invalid or duplicate PR file metadata")
			}
			seen[record.Filename] = true
			status := map[string]string{"added": "A", "removed": "D", "modified": "M", "renamed": "R", "copied": "C", "changed": "T", "unchanged": "="}[record.Status]
			if status == "" {
				return nil, "", fmt.Errorf("unsupported GitHub change status %q", record.Status)
			}
			f := diff.File{Path: record.Filename, OldPath: record.PreviousFilename, Status: status, NewOID: record.SHA, Added: record.Additions, Deleted: record.Deletions}
			if record.Patch == nil || *record.Patch == "" {
				f.Metadata = append(f.Metadata, "GitHub did not provide a text patch (binary, metadata-only, or omitted diff).")
				missing++
			} else {
				hunks, err := diff.ParseHunks(*record.Patch)
				added, deleted := 0, 0
				for _, hunk := range hunks {
					for _, line := range hunk.Lines {
						if line.Kind == '+' {
							added++
						}
						if line.Kind == '-' {
							deleted++
						}
					}
				}
				if err != nil || added != f.Added || deleted != f.Deleted {
					f.Metadata = append(f.Metadata, "GitHub returned an incomplete text patch; open the PR in a browser to review this file.")
					missing++
				} else {
					f.Hunks = hunks
				}
			}
			files = append(files, f)
		}
	}
	warning := ""
	if missing > 0 {
		warning = fmt.Sprintf("%d files have no complete text patch; see each file's notice or open the PR in a browser", missing)
	}
	return files, warning, nil
}

const viewedQuery = `query($owner:String!, $repo:String!, $number:Int!, $cursor:String) {
 repository(owner:$owner, name:$repo) { pullRequest(number:$number) {
  id baseRefOid headRefOid files(first:100, after:$cursor) {
   nodes { path viewerViewedState } pageInfo { hasNextPage endCursor }
  }
 } }
}`

func (g *GitHub) viewed(ctx context.Context, p prMetadata) (map[string]ViewedState, error) {
	states := make(map[string]ViewedState)
	var cursor any
	for page := 0; page < 31; page++ {
		var data struct {
			Repository struct {
				PullRequest *struct {
					ID         string
					BaseRefOID string
					HeadRefOID string
					Files      struct {
						Nodes []struct {
							Path              string
							ViewerViewedState ViewedState
						}
						PageInfo struct {
							HasNextPage bool
							EndCursor   string
						}
					}
				}
			}
		}
		vars := map[string]any{"owner": g.Target.Owner, "repo": g.Target.Repo, "number": g.Target.Number, "cursor": cursor}
		if err := g.graphql(ctx, viewedQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("read Viewed state: %w", err)
		}
		pr := data.Repository.PullRequest
		if pr == nil || pr.ID != p.NodeID || pr.BaseRefOID != p.Base.SHA || pr.HeadRefOID != p.Head.SHA {
			return nil, errors.New("PR changed while reading Viewed state; refresh")
		}
		for _, file := range pr.Files.Nodes {
			if _, exists := states[file.Path]; exists || file.Path == "" {
				return nil, errors.New("invalid Viewed file list")
			}
			switch file.ViewerViewedState {
			case Viewed, Unviewed, Dismissed:
			default:
				return nil, errors.New("unsupported GitHub Viewed state")
			}
			states[file.Path] = file.ViewerViewedState
		}
		if !pr.Files.PageInfo.HasNextPage {
			if len(states) != p.ChangedFiles {
				return nil, errors.New("incomplete Viewed file list; refresh")
			}
			return states, nil
		}
		if pr.Files.PageInfo.EndCursor == "" || pr.Files.PageInfo.EndCursor == cursor {
			return nil, errors.New("invalid Viewed pagination cursor")
		}
		cursor = pr.Files.PageInfo.EndCursor
	}
	return nil, errors.New("Viewed pagination exceeded GitHub's file limit")
}

func (g *GitHub) SetViewed(parent context.Context, s *Snapshot, path string, viewed bool) error {
	if !g.Persist {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	found := false
	for _, file := range s.Comparison.Files {
		found = found || file.Path == path
	}
	if !found {
		return errors.New("file is not part of this PR snapshot")
	}
	viewer, err := g.viewer(ctx)
	if err != nil {
		return err
	}
	if viewer != s.Viewer {
		return errors.New("active GitHub account changed; refresh before changing Viewed state")
	}
	p, err := g.metadata(ctx)
	if err != nil {
		return err
	}
	if p.NodeID != s.RemoteID || p.revision() != s.Revision {
		return errors.New("PR has new changes; refresh before changing Viewed state")
	}
	// GitHub's Viewed mutations have no expected-revision argument. This check
	// detects stale snapshots, but cannot atomically exclude a concurrent push.
	mutation := "unmarkFileAsViewed"
	inputType := "UnmarkFileAsViewedInput!"
	if viewed {
		mutation, inputType = "markFileAsViewed", "MarkFileAsViewedInput!"
	}
	query := fmt.Sprintf("mutation($input:%s) { %s(input:$input) { pullRequest { id } } }", inputType, mutation)
	var result map[string]struct{ PullRequest struct{ ID string } }
	if err := g.graphql(ctx, query, map[string]any{"input": map[string]any{"pullRequestId": s.RemoteID, "path": path}}, &result); err != nil {
		return err
	}
	if result[mutation].PullRequest.ID != s.RemoteID {
		return errors.New("GitHub did not confirm the Viewed update; refresh to check its state")
	}
	return nil
}
