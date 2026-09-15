package backend

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type Local struct {
	Options gitdiff.Options
	Persist bool
	mu      sync.Mutex
}

func NewLocal(opts gitdiff.Options, persist bool) *Local {
	return &Local{Options: opts, Persist: persist}
}

func (l *Local) Modes() []diff.Mode { return []diff.Mode{diff.ModeWorkingTree, diff.ModeCommitted} }

func (l *Local) Load(ctx context.Context, mode diff.Mode) (*Snapshot, error) {
	opts := l.Options
	opts.Mode = mode
	if mode == diff.ModeWorkingTree {
		opts.Base, opts.Head = "", "HEAD"
	}
	c, err := gitdiff.Load(ctx, opts)
	if err != nil {
		return nil, err
	}
	return l.Restore(c), nil
}

// Restore attaches saved progress to a locally captured comparison.
func (l *Local) Restore(c *diff.Comparison) *Snapshot {
	s := &Snapshot{Comparison: c, Key: review.Path(c), Mode: diff.ModeCommitted,
		Label: filepath.Base(c.Root), Viewed: make(map[string]ViewedState), Persistence: Memory}
	if c.Root == "" {
		s.Label = ""
	}
	if c.WorkingTree {
		s.Mode = diff.ModeWorkingTree
	}
	if !l.Persist {
		return s
	}
	s.Persistence = LocalDisk
	viewed, err := review.Load(review.Path(c))
	if err != nil {
		s.Warning = "Could not restore progress: " + err.Error()
	}
	for path, v := range viewed {
		if v {
			s.Viewed[path] = Viewed
		} else {
			s.Viewed[path] = Unviewed
		}
	}
	return s
}

func (l *Local) SetViewed(ctx context.Context, s *Snapshot, path string, viewed bool) error {
	if !l.Persist {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	found := false
	for _, file := range s.Comparison.Files {
		found = found || file.Path == path
	}
	if !found {
		return fmt.Errorf("file %q is not part of this comparison", path)
	}
	filename := review.Path(s.Comparison)
	state, err := review.Load(filename)
	if err != nil {
		return err
	}
	state[path] = viewed
	return review.Save(filename, state)
}

func (l *Local) Targets(ctx context.Context) ([]Target, error) {
	refs, err := gitdiff.LocalBranches(ctx, l.Options.Dir)
	if err != nil {
		return nil, err
	}
	targets := []Target{{Mode: diff.ModeWorkingTree, Label: "Working tree"}}
	for _, ref := range refs {
		targets = append(targets, Target{Mode: diff.ModeCommitted, Head: ref, Label: strings.TrimPrefix(ref, "refs/heads/")})
	}
	return targets, nil
}

func (l *Local) targetOptions(target Target) gitdiff.Options {
	opts := l.Options
	opts.Mode = target.Mode
	if target.Mode == diff.ModeWorkingTree {
		opts.Base, opts.Head = "", "HEAD"
	} else if target.Head != "" {
		opts.Head = target.Head
	}
	return opts
}

func (l *Local) TargetStats(ctx context.Context, target Target) (Target, error) {
	c, err := gitdiff.Summary(ctx, l.targetOptions(target))
	if err != nil {
		return target, err
	}
	target.Added, target.Deleted, target.StatsReady = c.Added, c.Deleted, true
	return target, nil
}

func (l *Local) LoadTarget(ctx context.Context, target Target) (*Snapshot, error) {
	c, err := gitdiff.Load(ctx, l.targetOptions(target))
	if err != nil {
		return nil, err
	}
	c.Head = strings.TrimPrefix(c.Head, "refs/heads/")
	s := l.Restore(c)
	if target.Mode == diff.ModeCommitted {
		s.TargetHead = target.Head
	}
	return s, nil
}
