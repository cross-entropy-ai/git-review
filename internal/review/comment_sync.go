package review

import (
	"errors"
	"github.com/cross-entropy-ai/git-review/internal/diff"
	"strings"
)

// MatchPendingComment recognizes only comments created after this draft's
// submission checkpoint, by this account, on the same anchor and discussion.
// Multiple matches remain unresolved rather than silently picking a comment.
func MatchPendingComment(draft Comment, comments []Comment, viewer string) (int, error) {
	if draft.PendingBody == "" {
		return -1, nil
	}
	found := -1
	for i, c := range comments {
		sameThread := c.ReplyTo == draft.ReplyTo || (draft.ReplyTo > 0 && c.ReplyTo > 0 && draft.ThreadID != "" && c.ThreadID == draft.ThreadID)
		if c.RemoteID <= draft.PendingAfterID || c.AuthorID != viewer || !sameThread || (c.Body != draft.PendingBody && c.Body != draft.Body) {
			continue
		}
		if draft.ReplyTo == 0 {
			if c.Path != draft.Path {
				continue
			}
			start, end := c.Start, c.End
			if draft.CommitID != "" && c.CommitID != "" {
				if draft.CommitID != c.CommitID && !strings.HasSuffix(draft.Revision, ":"+c.CommitID) {
					continue
				}
				if c.OriginalEnd > 0 {
					start, end = c.OriginalStart, c.OriginalEnd
					if start == 0 {
						start = end
					}
				}
			}
			startSide, draftStartSide := c.StartSide, draft.StartSide
			if startSide == "" {
				startSide = c.Side
			}
			if draftStartSide == "" {
				draftStartSide = draft.Side
			}
			if c.Side != draft.Side || startSide != draftStartSide || start != draft.Start || end != draft.End {
				continue
			}
		}
		if found >= 0 {
			return -1, errors.New("multiple GitHub comments match this pending draft; inspect the discussion before retrying")
		}
		found = i
	}
	return found, nil
}

func ValidateCommentAnchor(comparison *diff.Comparison, c Comment) error {
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
