package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cross-entropy-ai/git-review/internal/diff"
)

// Comment anchors a local note to one side of an exact comparison. Keeping the
// excerpt allows exports to remain self-contained, including deleted files.
type Comment struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	OldPath        string `json:"old_path,omitempty"`
	Side           string `json:"side"` // "old" or "new"
	Start          int    `json:"start"`
	End            int    `json:"end"`
	Code           string `json:"code"`
	Body           string `json:"body"`
	RemoteID       int64  `json:"github_id,omitempty"`
	ReplyTo        int64  `json:"reply_to,omitempty"`
	Author         string `json:"author,omitempty"`
	AuthorID       string `json:"author_id,omitempty"`
	URL            string `json:"url,omitempty"`
	Outdated       bool   `json:"outdated,omitempty"`
	StartSide      string `json:"start_side,omitempty"`
	DraftBody      string `json:"draft_body,omitempty"`
	ThreadID       string `json:"thread_id,omitempty"`
	Resolved       bool   `json:"resolved,omitempty"`
	Revision       string `json:"revision,omitempty"`
	CommitID       string `json:"commit_id,omitempty"`
	OriginalStart  int    `json:"original_start,omitempty"`
	OriginalEnd    int    `json:"original_end,omitempty"`
	PendingBody    string `json:"pending_body,omitempty"`
	PendingAfterID int64  `json:"pending_after_id,omitempty"`
}

// Remote drafts outlive a diff revision, but never cross PR or account boundaries.
func RemoteCommentsPath(c *diff.Comparison, key string) (string, error) {
	legacy, err := CommentsPath(c, key)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(filepath.Dir(legacy), "pr-"+hex.EncodeToString(sum[:])+".json"), nil
}

func CommentsPath(c *diff.Comparison, key string) (string, error) {
	if c.GitDir != "" {
		return strings.TrimSuffix(Path(c), ".json") + ".comments.json", nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, "git-review", "comments", hex.EncodeToString(sum[:])+".json"), nil
}

func LoadComments(path string) ([]Comment, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var saved struct {
		Version int       `json:"version"`
		Items   []Comment `json:"comments"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("read comments: %w", err)
	}
	if saved.Version != 1 {
		return nil, fmt.Errorf("unsupported comments version %d", saved.Version)
	}
	seen := make(map[string]bool)
	for _, c := range saved.Items {
		validLine := (c.Side == "old" || c.Side == "new") && c.Start > 0 && (c.End >= c.Start || c.StartSide != "" && c.StartSide != c.Side && c.End > 0)
		if c.ID == "" || seen[c.ID] || c.Path == "" || (!validLine && c.RemoteID == 0 && c.ReplyTo == 0) || strings.TrimSpace(c.Body) == "" {
			return nil, errors.New("invalid saved comment")
		}
		seen[c.ID] = true
	}
	return saved.Items, nil
}

func SaveComments(path string, comments []Comment) error {
	data, err := json.MarshalIndent(struct {
		Version int       `json:"version"`
		Items   []Comment `json:"comments"`
	}{1, comments}, "", "  ")
	if err != nil {
		return err
	}
	return saveAtomic(path, data)
}

// Markdown emits deterministic, grouped notes. Paths and revision metadata are
// quoted, and code fences grow to accommodate backticks inside source code.
func Markdown(label, url string, c *diff.Comparison, comments []Comment) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Review comments\n\nReview: %s\n\n", markdownCode(label))
	if url != "" {
		fmt.Fprintf(&out, "Source: %s\n\n", markdownCode(url))
	}
	fmt.Fprintf(&out, "Comparison: %s → %s\n\n", markdownCode(c.Base), markdownCode(c.Head))
	base := c.MergeBase
	if base == "" {
		base = c.BaseOID
	}
	fmt.Fprintf(&out, "Base revision: %s  \nHead revision: %s\n\n", markdownCode(base), markdownCode(c.HeadOID))
	items := append([]Comment(nil), comments...)
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Side != b.Side {
			return a.Side == "old"
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End < b.End
	})
	fmt.Fprintf(&out, "%d comment(s)\n", len(items))
	path := ""
	for _, item := range items {
		if item.Path != path {
			path = item.Path
			fmt.Fprintf(&out, "\n## %s\n", markdownCode(path))
			if item.OldPath != "" && item.OldPath != path {
				fmt.Fprintf(&out, "\nRenamed from %s\n", markdownCode(item.OldPath))
			}
		}
		location := fmt.Sprintf("%s L%d", item.Side, item.Start)
		if item.StartSide != "" && item.StartSide != item.Side {
			location = fmt.Sprintf("%s L%d → %s L%d", item.StartSide, item.Start, item.Side, item.End)
		}
		if item.End != item.Start {
			if item.StartSide == "" || item.StartSide == item.Side {
				location += fmt.Sprintf("–L%d", item.End)
			}
		}
		if item.Start == 0 {
			location = "File comment"
		}
		if item.Outdated {
			location += " (outdated)"
		}
		if item.Resolved {
			location += " (resolved)"
		}
		if item.PendingBody != "" {
			location += " (sync uncertain)"
		}
		fmt.Fprintf(&out, "\n### %s\n\n", location)
		if item.RemoteID > 0 {
			fmt.Fprintf(&out, "GitHub: %s · @%s\n\n", markdownCode(item.URL), markdownCode(item.Author))
		}
		if item.ReplyTo > 0 {
			fmt.Fprintf(&out, "Reply to comment #%d\n\n", item.ReplyTo)
		}
		fence := strings.Repeat("`", max(3, longestBackticks(item.Code)+1))
		fmt.Fprintf(&out, "%stext\n%s\n%s\n\n%s\n", fence, strings.TrimSuffix(item.Code, "\n"), fence, item.Body)
		if item.DraftBody != "" {
			fmt.Fprintf(&out, "\n**Local draft (not synced)**\n\n%s\n", item.DraftBody)
		}
	}
	return out.String()
}

func longestBackticks(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

func markdownCode(s string) string {
	// Quote metadata as one line even when a Git filename contains controls.
	s = strconv.QuoteToGraphic(s)
	fence := strings.Repeat("`", longestBackticks(s)+1)
	return fence + " " + s + " " + fence
}

// Export creates a new file exclusively. An existing report is never truncated.
func Export(path, markdown string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	return writeExport(f, markdown)
}

// ExportDirectory creates a uniquely named report. An empty directory uses the
// OS temporary directory; the report remains available after the editor closes.
func ExportDirectory(dir, markdown string) (string, error) {
	f, err := os.CreateTemp(dir, "review-comments-*.md")
	if err != nil {
		return "", err
	}
	return f.Name(), writeExport(f, markdown)
}

func writeExport(f *os.File, markdown string) error {
	if _, err := f.WriteString(markdown); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}
