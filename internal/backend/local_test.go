package backend

import (
	"context"
	"sync"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func TestLocalConcurrentViewedUpdatesPreserveOtherFiles(t *testing.T) {
	c := &diff.Comparison{GitDir: t.TempDir(), MergeBase: "base", HeadOID: "head",
		Files: []diff.File{{Path: "one.go"}, {Path: "two.go"}, {Path: "three.go"}}}
	l := NewLocal(gitdiff.Options{}, true)
	s := l.Restore(c)
	if err := l.SetViewed(context.Background(), s, "one.go", true); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, path := range []string{"two.go", "three.go"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.SetViewed(context.Background(), s, path, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := l.SetViewed(context.Background(), s, "two.go", false); err != nil {
		t.Fatal(err)
	}
	state, err := review.Load(review.Path(c))
	if err != nil || !state["one.go"] || state["two.go"] || !state["three.go"] {
		t.Fatalf("updates lost saved progress: %v, %v", state, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.SetViewed(ctx, s, "one.go", false); err == nil {
		t.Fatal("cancelled update was saved")
	}
	if err := l.SetViewed(context.Background(), s, "unknown.go", true); err == nil {
		t.Fatal("saved a file outside the comparison")
	}
	restored := l.Restore(c)
	if restored.Viewed["one.go"] != Viewed || restored.Viewed["two.go"] != Unviewed || restored.Viewed["three.go"] != Viewed {
		t.Fatalf("saved progress was not restored: %v", restored.Viewed)
	}
}
