package gitdiff

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func gitDirectoryContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestWorkingTreeIncludesAllPendingChangesWithoutGitWrites(t *testing.T) {
	dir := repo(t)
	write(t, dir, "remove.txt", "remove this\n")
	write(t, dir, ".gitignore", "cache/\nignored.txt\n")
	commit(t, dir)
	git(t, dir, "switch", "-qc", "feature")
	write(t, dir, "committed.txt", "already in HEAD\n")
	commit(t, dir)
	write(t, dir, "source.go", "package main\nfunc staged() {}\n")
	write(t, dir, "staged.txt", "staged addition\n")
	write(t, dir, "cache/included.txt", "explicitly tracked\n")
	git(t, dir, "add", "source.go", "staged.txt")
	git(t, dir, "add", "-f", "cache/included.txt")
	write(t, dir, "source.go", "package main\nfunc final() {}\n")
	write(t, dir, "new 文件\t\n.txt", "untracked without newline")
	write(t, dir, "-new.txt", "another new file\n")
	write(t, dir, "empty.txt", "")
	write(t, dir, "binary.dat", "\x00binary")
	write(t, dir, "ignored.txt", "excluded\n")
	write(t, dir, "cache/excluded.txt", "excluded\n")
	if err := os.Remove(filepath.Join(dir, "remove.txt")); err != nil {
		t.Fatal(err)
	}
	before := gitDirectoryContents(t, filepath.Join(dir, ".git"))
	c, err := Load(context.Background(), Options{Dir: dir, WorkingTree: true, Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, gitDirectoryContents(t, filepath.Join(dir, ".git"))) {
		t.Fatal("review modified the index, refs, objects, or other Git files")
	}
	if !c.WorkingTree || c.Base != "HEAD" || c.Head != "working tree" || c.MergeBase != git(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("wrong comparison endpoints: %+v", c)
	}
	files := make(map[string]File)
	for _, file := range c.Files {
		files[file.Path] = file
	}
	if len(files) != 8 {
		t.Fatalf("unexpected changed files: %+v", files)
	}
	for _, name := range []string{"staged.txt", "cache/included.txt", "new 文件\t\n.txt", "-new.txt", "empty.txt", "binary.dat"} {
		if files[name].Status != "A" {
			t.Errorf("missing added file %q: %+v", name, files[name])
		}
	}
	if !files["binary.dat"].Binary || files["empty.txt"].Added != 0 || files["remove.txt"].Status != "D" {
		t.Fatal("binary, empty addition, or deletion rendered incorrectly")
	}
	var text strings.Builder
	for _, hunk := range files["source.go"].Hunks {
		for _, line := range hunk.Lines {
			text.WriteString(line.Text)
		}
	}
	if !strings.Contains(text.String(), "final()") || strings.Contains(text.String(), "staged()") {
		t.Fatal("partially staged file did not use its final working-tree content")
	}
	if c.Added != 5 || c.Deleted != 3 {
		t.Fatalf("wrong totals: +%d -%d", c.Added, c.Deleted)
	}
}

func TestWorkingTreeSnapshotIdentityAndRenames(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitignore", "ignored.txt\n")
	commit(t, dir)
	if err := os.Rename(filepath.Join(dir, "source.go"), filepath.Join(dir, "renamed.go")); err != nil {
		t.Fatal(err)
	}
	load := func(contextLines int) *Comparison {
		t.Helper()
		c, err := Load(context.Background(), Options{Dir: dir, WorkingTree: true, Context: contextLines})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	first := load(3)
	if len(first.Files) != 1 || first.Files[0].OldPath != "source.go" || first.Files[0].Path != "renamed.go" || first.Files[0].Status != "R100" {
		t.Fatalf("untracked rename was not recognized: %+v", first.Files)
	}
	write(t, dir, "ignored.txt", "not reviewed\n")
	if load(0).HeadOID != first.HeadOID {
		t.Fatal("ignored files or context size changed snapshot identity")
	}
	git(t, dir, "add", "-A")
	if load(3).HeadOID != first.HeadOID {
		t.Fatal("staging unchanged content changed snapshot identity")
	}
	write(t, dir, "renamed.go", "package main\nfunc changedAgain() {}\n")
	if load(3).HeadOID == first.HeadOID {
		t.Fatal("edited content reused a stale snapshot")
	}
}

func TestWorkingTreeDoesNotExecuteFiltersOrHooks(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitattributes", "*.go filter=unsafe diff=unsafe\n")
	commit(t, dir)
	git(t, dir, "config", "filter.unsafe.clean", "/does-not-exist")
	git(t, dir, "config", "filter.unsafe.process", "/does-not-exist")
	git(t, dir, "config", "filter.unsafe.required", "true")
	git(t, dir, "config", "diff.unsafe.textconv", "/does-not-exist")
	git(t, dir, "config", "core.hooksPath", filepath.Join(dir, ".git", "hooks"))
	hook := filepath.Join(dir, ".git", "hooks", "post-index-change")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf invoked > hook-ran\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "source.go", "package main\nfunc changed() {}\n")
	c, err := Load(context.Background(), Options{Dir: dir, WorkingTree: true, Context: 3})
	if err != nil || len(c.Files) != 1 {
		t.Fatalf("filter-free snapshot failed: %+v %v", c, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hook-ran")); !os.IsNotExist(err) {
		t.Fatal("review invoked the index-change hook")
	}
}

func TestWorkingTreeSparseAndSplitIndexes(t *testing.T) {
	for _, sparse := range []bool{false, true} {
		dir := repo(t)
		write(t, dir, "keep/a.txt", "before\n")
		write(t, dir, "outside/b.txt", "must remain unchanged\n")
		commit(t, dir)
		if sparse {
			git(t, dir, "sparse-checkout", "init", "--cone", "--sparse-index")
			git(t, dir, "sparse-checkout", "set", "keep")
		} else {
			git(t, dir, "update-index", "--split-index")
			git(t, dir, "config", "core.splitIndex", "true")
		}
		write(t, dir, "keep/a.txt", "after\n")
		before := gitDirectoryContents(t, filepath.Join(dir, ".git"))
		c, err := Load(context.Background(), Options{Dir: dir, WorkingTree: true, Context: 3})
		if err != nil {
			t.Fatalf("sparse=%v: %v", sparse, err)
		}
		if len(c.Files) != 1 || c.Files[0].Path != "keep/a.txt" {
			t.Fatalf("sparse=%v: wrong changes %+v", sparse, c.Files)
		}
		after := gitDirectoryContents(t, filepath.Join(dir, ".git"))
		for path, content := range after {
			if original, ok := before[path]; !ok || content != original {
				t.Errorf("sparse=%v: Git metadata changed: %s (existed=%v)", sparse, path, ok)
			}
		}
		for path := range before {
			if _, ok := after[path]; !ok {
				t.Errorf("sparse=%v: Git metadata removed: %s", sparse, path)
			}
		}
	}
}

func TestWorkingTreeLinkedWorktreeAndInvalidRefs(t *testing.T) {
	dir := repo(t)
	linked := filepath.Join(t.TempDir(), "linked checkout")
	git(t, dir, "worktree", "add", "-q", "--detach", linked, "HEAD")
	write(t, linked, "new.txt", "included\n")
	c, err := Load(context.Background(), Options{Dir: linked, WorkingTree: true, Context: 3})
	if err != nil || len(c.Files) != 1 || !strings.Contains(c.GitDir, "worktrees") {
		t.Fatalf("linked worktree failed: %+v %v", c, err)
	}
	for _, opts := range []Options{{Dir: dir, WorkingTree: true, Base: "main"}, {Dir: dir, WorkingTree: true, Head: "main"}} {
		if _, err := Load(context.Background(), opts); err == nil {
			t.Fatal("working-tree mode accepted branch comparison refs")
		}
	}
}
