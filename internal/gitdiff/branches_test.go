package gitdiff

import (
	"context"
	"reflect"
	"testing"
)

func TestLocalBranchesAndSummary(t *testing.T) {
	dir := repo(t)
	git(t, dir, "switch", "-qc", "feature/中文")
	write(t, dir, "source.go", "package main\n\nfunc newer() {}\nfunc extra() {}\n")
	write(t, dir, "binary", "\x00\x01\x02")
	git(t, dir, "mv", "source.go", "renamed.go")
	commit(t, dir)
	git(t, dir, "update-ref", "refs/remotes/origin/remote-only", "HEAD")
	git(t, dir, "tag", "tag-only")
	git(t, dir, "switch", "-q", "main")
	write(t, dir, "untracked.txt", "local\n")
	before := git(t, dir, "status", "--porcelain=v1")
	branches, err := LocalBranches(context.Background(), dir)
	if err != nil || !reflect.DeepEqual(branches, []string{"refs/heads/feature/中文", "refs/heads/main"}) {
		t.Fatalf("local branches: %v, %v", branches, err)
	}
	for _, opts := range []Options{
		{Dir: dir, Mode: ModeCommitted, Head: branches[0], Context: 3},
		{Dir: dir, Mode: ModeWorkingTree, Context: 3},
	} {
		summary, err := Summary(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		full, err := Load(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if summary.Added != full.Added || summary.Deleted != full.Deleted || summary.MergeBase != full.MergeBase || len(summary.Files) != len(full.Files) {
			t.Fatalf("summary differs from review: %+v / %+v", summary, full)
		}
		for _, file := range summary.Files {
			if len(file.Hunks) != 0 {
				t.Fatal("summary loaded patches")
			}
		}
	}
	if git(t, dir, "symbolic-ref", "--short", "HEAD") != "main" || git(t, dir, "status", "--porcelain=v1") != before {
		t.Fatal("listing changed checkout or working tree")
	}
}
