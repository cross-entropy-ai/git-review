// Package backend abstracts comparison loading and viewed-progress persistence.
package backend

import (
	"context"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type ViewedState string

const (
	Unviewed  ViewedState = "UNVIEWED"
	Viewed    ViewedState = "VIEWED"
	Dismissed ViewedState = "DISMISSED"
)

type Persistence string

const (
	Memory    Persistence = "memory"
	LocalDisk Persistence = "local"
	Remote    Persistence = "github"
)

// Snapshot owns a consistent comparison and the progress read for that revision.
// Backends and asynchronous commands treat it as immutable.
type Snapshot struct {
	Comparison  *diff.Comparison
	Key         string
	Mode        diff.Mode
	Label       string
	URL         string
	Title       string
	TargetHead  string // Explicit local review ref; empty preserves startup options.
	Viewer      string
	Viewed      map[string]ViewedState
	Persistence Persistence
	Warning     string
	// RemoteID identifies the PR on GitHub; Revision also guards base changes.
	RemoteID      string
	Revision      string
	CommentKey    string // Stable across revisions, isolated by PR and account.
	Comments      []review.Comment
	CommentsError string
}

// CommentBackend is available for PRs; local backends keep their notes on disk.
type CommentBackend interface {
	SaveComment(context.Context, *Snapshot, review.Comment) (review.Comment, error)
	DeleteComment(context.Context, *Snapshot, review.Comment) error
}

// CommentNotSubmittedError confirms that creating a comment did not succeed.
// Other failures may have happened after GitHub accepted the request.
type CommentNotSubmittedError struct{ Err error }

func (e *CommentNotSubmittedError) Error() string { return e.Err.Error() }
func (e *CommentNotSubmittedError) Unwrap() error { return e.Err }

type ThreadBackend interface {
	SetThreadResolved(context.Context, *Snapshot, review.Comment, bool) (review.Comment, error)
}

type Backend interface {
	Load(context.Context, diff.Mode) (*Snapshot, error)
	SetViewed(context.Context, *Snapshot, string, bool) error
	Modes() []diff.Mode
}

// Target describes a local review scope and its optional line totals.
type Target struct {
	Mode           diff.Mode
	Head           string
	Label          string
	Added, Deleted int
	StatsReady     bool
	StatsError     string
}

// TargetBackend lets local reviews select a scope without checking out a branch.
type TargetBackend interface {
	Targets(context.Context) ([]Target, error)
	TargetStats(context.Context, Target) (Target, error)
	LoadTarget(context.Context, Target) (*Snapshot, error)
}
