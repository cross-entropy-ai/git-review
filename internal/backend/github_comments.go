package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type apiComment struct {
	ID                int64
	Body, Path, Side  string
	StartSide         string `json:"start_side"`
	Line              *int
	StartLine         *int   `json:"start_line"`
	OriginalLine      int    `json:"original_line"`
	OriginalStartLine int    `json:"original_start_line"`
	ReplyTo           int64  `json:"in_reply_to_id"`
	URL               string `json:"html_url"`
	PRURL             string `json:"pull_request_url"`
	DiffHunk          string `json:"diff_hunk"`
	SubjectType       string `json:"subject_type"`
	User              struct {
		Login  string
		NodeID string `json:"node_id"`
	}
}

func commentSide(side string) string {
	if side == "LEFT" {
		return "old"
	}
	return "new"
}

func (a apiComment) comment() review.Comment {
	c := review.Comment{ID: fmt.Sprintf("github:%d", a.ID), RemoteID: a.ID, ReplyTo: a.ReplyTo, Body: a.Body, Path: a.Path,
		Side: commentSide(a.Side), Author: a.User.Login, AuthorID: a.User.NodeID, URL: a.URL}
	if a.StartSide != "" {
		c.StartSide = commentSide(a.StartSide)
	}
	if a.Line == nil {
		c.Outdated = a.SubjectType != "file"
		c.Start, c.End = a.OriginalStartLine, a.OriginalLine
	} else {
		c.End = *a.Line
		if a.StartLine != nil {
			c.Start = *a.StartLine
		}
	}
	if c.Start == 0 {
		c.Start = c.End
	}
	if c.StartSide != "" && c.StartSide != c.Side {
		c.Code = a.DiffHunk
	} else if hunks, err := diff.ParseCommentHunks(a.DiffHunk); err == nil {
		var lines []string
		for _, h := range hunks {
			for _, l := range h.Lines {
				n := l.New
				if c.Side == "old" {
					n = l.Old
				}
				if n > 0 && n >= c.Start && n <= c.End && l.Kind != '\\' {
					lines = append(lines, l.Text)
				}
			}
		}
		c.Code = strings.Join(lines, "\n")
	}
	return c
}

func (g *GitHub) comments(ctx context.Context) ([]review.Comment, error) {
	var comments []review.Comment
	seen := make(map[int64]bool)
	for page := 1; page <= 1000; page++ {
		var records []apiComment
		if err := g.api(ctx, fmt.Sprintf("%s/comments?per_page=100&page=%d", g.Target.endpoint(), page), &records); err != nil {
			return nil, err
		}
		for _, record := range records {
			if record.ID <= 0 || seen[record.ID] || record.Path == "" {
				return nil, errors.New("invalid or changing GitHub comment list; refresh")
			}
			seen[record.ID] = true
			comments = append(comments, record.comment())
		}
		if len(records) < 100 {
			return comments, nil
		}
	}
	return nil, errors.New("GitHub comment pagination limit exceeded")
}

func (g *GitHub) checkCommentSnapshot(ctx context.Context, s *Snapshot) error {
	if !g.Persist {
		return errors.New("GitHub comment sync is disabled by --no-state")
	}
	viewer, err := g.viewer(ctx)
	if err != nil {
		return err
	}
	if viewer != s.Viewer {
		return errors.New("active GitHub account changed; refresh before changing comments")
	}
	p, err := g.metadata(ctx)
	if err != nil {
		return err
	}
	if p.NodeID != s.RemoteID || p.revision() != s.Revision {
		return errors.New("PR has new changes; refresh before changing comments")
	}
	return nil
}

func (g *GitHub) commentEndpoint(id int64) string {
	return fmt.Sprintf("repos/%s/%s/pulls/comments/%d", g.Target.Owner, g.Target.Repo, id)
}

func (g *GitHub) existingComment(ctx context.Context, id int64) (apiComment, error) {
	var c apiComment
	if id <= 0 {
		return c, errors.New("invalid GitHub comment ID")
	}
	if err := g.api(ctx, g.commentEndpoint(id), &c); err != nil {
		return c, err
	}
	u, err := url.Parse(c.PRURL)
	if err != nil || c.ID != id || !strings.HasSuffix(u.Path, "/"+g.Target.endpoint()) {
		return c, errors.New("comment does not belong to this pull request")
	}
	return c, nil
}

func (g *GitHub) writeComment(ctx context.Context, method, endpoint string, body any, output any) error {
	args := []string{"api", "--hostname", g.Target.Host, "--method", method, endpoint}
	var input []byte
	if body != nil {
		var err error
		input, err = json.Marshal(body)
		if err != nil {
			return err
		}
		args = append(args, "--input", "-")
	}
	data, err := g.run(ctx, input, args...)
	if err != nil {
		return commentWriteError(err)
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return fmt.Errorf("GitHub response could not be read; refresh comments before retrying: %w", err)
		}
	}
	return nil
}

func commentWriteError(err error) error {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "403") || strings.Contains(text, "401") || strings.Contains(text, "permission") || strings.Contains(text, "not accessible") {
		return fmt.Errorf("GitHub denied the comment operation. Check the active account, repository access and token's Pull requests write permission.\n%w", err)
	}
	return fmt.Errorf("GitHub comment request failed; refresh comments before retrying if the result is uncertain: %w", err)
}

func (g *GitHub) SaveComment(parent context.Context, s *Snapshot, c review.Comment) (review.Comment, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	if err := g.checkCommentSnapshot(ctx, s); err != nil {
		return c, err
	}
	if strings.TrimSpace(c.Body) == "" {
		return c, errors.New("comment body is empty")
	}
	method, endpoint := "POST", g.Target.endpoint()+"/comments"
	body := map[string]any{"body": c.Body}
	switch {
	case c.RemoteID > 0:
		current, err := g.existingComment(ctx, c.RemoteID)
		if err != nil {
			return c, err
		}
		if current.User.NodeID != s.Viewer {
			return c, errors.New("permission denied: only the author can edit this GitHub comment; use Reply to join the discussion")
		}
		method, endpoint = "PATCH", g.commentEndpoint(c.RemoteID)
	case c.ReplyTo > 0:
		parent, err := g.existingComment(ctx, c.ReplyTo)
		if err != nil {
			return c, err
		}
		if parent.ReplyTo > 0 {
			return c, errors.New("reply target must be the thread's first comment")
		}
		endpoint = fmt.Sprintf("%s/comments/%d/replies", g.Target.endpoint(), c.ReplyTo)
	default:
		if err := validateCommentAnchor(s.Comparison, c); err != nil {
			return c, err
		}
		side := "RIGHT"
		if c.Side == "old" {
			side = "LEFT"
		}
		body["commit_id"], body["path"], body["line"], body["side"] = s.Comparison.HeadOID, c.Path, c.End, side
		if c.Start != c.End {
			body["start_line"], body["start_side"] = c.Start, side
		}
	}
	var result apiComment
	if err := g.writeComment(ctx, method, endpoint, body, &result); err != nil {
		return c, err
	}
	if result.ID <= 0 || result.Path == "" || result.Body != c.Body || c.RemoteID > 0 && result.ID != c.RemoteID {
		return c, errors.New("GitHub did not confirm the comment; refresh before retrying")
	}
	updated := result.comment()
	updated.OldPath = c.OldPath
	if c.RemoteID > 0 || c.ReplyTo > 0 {
		updated.ThreadID, updated.Resolved = c.ThreadID, c.Resolved
	}
	if updated.Code == "" {
		updated.Code = c.Code
	}
	return updated, nil
}

func validateCommentAnchor(comparison *diff.Comparison, c review.Comment) error {
	if c.Outdated || c.Start < 1 || c.End < c.Start || c.Side != "old" && c.Side != "new" {
		return errors.New("invalid comment line range")
	}
	for _, f := range comparison.Files {
		if f.Path != c.Path {
			continue
		}
		for _, h := range f.Hunks {
			var code []string
			start, end := false, false
			for _, l := range h.Lines {
				n := l.New
				if c.Side == "old" {
					n = l.Old
				}
				if l.Kind == '\\' || n == 0 {
					continue
				}
				start, end = start || n == c.Start, end || n == c.End
				if n >= c.Start && n <= c.End {
					code = append(code, l.Text)
				}
			}
			if start && end && strings.Join(code, "\n") == c.Code {
				return nil
			}
		}
	}
	return errors.New("comment lines no longer match this diff; select the lines again")
}

func (g *GitHub) DeleteComment(parent context.Context, s *Snapshot, c review.Comment) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	if err := g.checkCommentSnapshot(ctx, s); err != nil {
		return err
	}
	if _, err := g.existingComment(ctx, c.RemoteID); err != nil {
		return err
	}
	return g.writeComment(ctx, "DELETE", g.commentEndpoint(c.RemoteID), nil, nil)
}
