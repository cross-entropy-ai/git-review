package gitdiff

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAutoSelectsLocalChangesAndRefreshesScope(t *testing.T) {
	for _, kind := range []string{"unstaged", "staged", "untracked", "staged-reversed", "ignored"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			write(t, dir, ".gitignore", "ignored.txt\n")
			commit(t, dir)
			git(t, dir, "switch", "-qc", "feature")
			write(t, dir, "committed.txt", "committed\n")
			commit(t, dir)
			opts := Options{Dir: dir, Context: 3}
			check := func(local bool, count int) {
				t.Helper()
				before := gitDirectoryContents(t, filepath.Join(dir, ".git"))
				c, err := Load(context.Background(), opts)
				if err != nil {
					t.Fatal(err)
				}
				if c.WorkingTree != local || len(c.Files) != count {
					t.Fatalf("want local=%v, files=%d; got %+v", local, count, c)
				}
				if !local && c.Files[0].Path != "committed.txt" {
					t.Fatalf("wrong committed comparison: %+v", c.Files)
				}
				if !reflect.DeepEqual(before, gitDirectoryContents(t, filepath.Join(dir, ".git"))) {
					t.Fatal("auto review modified Git metadata")
				}
			}
			check(false, 1)
			switch kind {
			case "unstaged", "staged", "staged-reversed":
				original := "package main\n\nfunc old() {}\n"
				write(t, dir, "source.go", "local edit\n")
				if kind != "unstaged" {
					git(t, dir, "add", "source.go")
				}
				if kind == "staged-reversed" {
					write(t, dir, "source.go", original)
				}
			case "untracked":
				write(t, dir, "local.txt", "untracked\n")
			case "ignored":
				write(t, dir, "ignored.txt", "ignored\n")
			}
			count := 1
			if kind == "staged-reversed" {
				count = 0
			}
			check(kind != "ignored", count)
			git(t, dir, "reset", "--hard", "-q", "HEAD")
			if kind == "untracked" {
				if err := os.Remove(filepath.Join(dir, "local.txt")); err != nil {
					t.Fatal(err)
				}
			}
			check(false, 1)
		})
	}
}

func TestAutoLocalChangesNeedNoDefaultBase(t *testing.T) {
	dir := repo(t)
	git(t, dir, "branch", "-m", "topic")
	write(t, dir, "new.txt", "local\n")
	c, err := Load(context.Background(), Options{Dir: dir})
	if err != nil || !c.WorkingTree || len(c.Files) != 1 {
		t.Fatalf("auto required a base for local changes: %+v, %v", c, err)
	}
}
