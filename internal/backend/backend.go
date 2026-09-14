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
	Viewer      string
	Viewed      map[string]ViewedState
	Persistence Persistence
	Warning     string
	// RemoteID identifies the PR on GitHub; Revision also guards base changes.
	RemoteID      string
	Revision      string
	Comments      []review.Comment
	CommentsError string
}

// CommentBackend is available for PRs; local backends keep their notes on disk.
type CommentBackend interface {
	SaveComment(context.Context, *Snapshot, review.Comment) (review.Comment, error)
	DeleteComment(context.Context, *Snapshot, review.Comment) error
}

type ThreadBackend interface {
	SetThreadResolved(context.Context, *Snapshot, review.Comment, bool) (review.Comment, error)
}

type Backend interface {
	Load(context.Context, diff.Mode) (*Snapshot, error)
	SetViewed(context.Context, *Snapshot, string, bool) error
	Modes() []diff.Mode
}
