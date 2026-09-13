// Package backend abstracts comparison loading and viewed-progress persistence.
package backend

import (
	"context"

	"github.com/cross-entropy-ai/git-review/internal/diff"
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
	RemoteID string
	Revision string
}

type Backend interface {
	Load(context.Context, diff.Mode) (*Snapshot, error)
	SetViewed(context.Context, *Snapshot, string, bool) error
	Modes() []diff.Mode
}
