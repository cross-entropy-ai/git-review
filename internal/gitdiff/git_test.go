package gitdiff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	// Fixture commits must not leave background maintenance racing metadata checks.
	git(t, dir, "config", "maintenance.auto", "false")
	git(t, dir, "config", "gc.auto", "0")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "core.hooksPath", os.DevNull)
	write(t, dir, "source.go", "package main\n\nfunc old() {}\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	return dir
}

func commit(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "change")
}

func TestLoadUsesMergeBaseAndIgnoresUncommittedChanges(t *testing.T) {
	dir := repo(t)
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "switch", "-qc", "feature")
	write(t, dir, "source.go", "package main\n\nfunc newer() {}\nfunc extra() {}\n")
	commit(t, dir)
	git(t, dir, "switch", "-q", "main")
	write(t, dir, "main-only.txt", "not part of feature\n")
	commit(t, dir)
	git(t, dir, "switch", "-q", "feature")
	write(t, dir, "staged.txt", "excluded\n")
	git(t, dir, "add", "staged.txt")
	write(t, dir, "source.go", "uncommitted\n")
	before := git(t, dir, "status", "--porcelain=v1")
	c, err := Load(context.Background(), Options{Mode: ModeCommitted, Dir: dir, Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	if c.MergeBase != base || c.Head != "feature" || c.Base != "main" {
		t.Fatalf("wrong revisions: %+v", c)
	}
	if len(c.Files) != 1 || c.Files[0].Path != "source.go" || c.Added != 2 || c.Deleted != 1 {
		t.Fatalf("wrong diff: %+v", c)
	}
	if after := git(t, dir, "status", "--porcelain=v1"); after != before {
		t.Fatalf("repository changed: %s => %s", before, after)
	}
	lines := c.Files[0].Hunks[0].Lines
	if lines[len(lines)-1].New != 4 || lines[len(lines)-1].Kind != '+' {
		t.Fatalf("wrong line numbers: %+v", lines)
	}
}

func TestLoadChangeKindsAndUnusualPaths(t *testing.T) {
	dir := repo(t)
	write(t, dir, "old name.txt", strings.Repeat("unchanged rename content\n", 10))
	write(t, dir, "remove.txt", "removed\n")
	write(t, dir, "binary.dat", "\x00before")
	write(t, dir, "mode.sh", "echo hello\n")
	write(t, dir, "type.txt", "a regular file\n")
	commit(t, dir)
	git(t, dir, "switch", "-qc", "feature")
	git(t, dir, "mv", "old name.txt", "renamed\t文件\n.txt")
	git(t, dir, "rm", "-q", "remove.txt")
	write(t, dir, "binary.dat", "\x00after")
	write(t, dir, "space tab\tand\nnewline.txt", "added without final newline")
	write(t, dir, "-leading-dash.txt", "diff --git fake header\n+++ content\n")
	if err := os.Chmod(filepath.Join(dir, "mode.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "type.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("source.go", filepath.Join(dir, "type.txt")); err != nil {
		t.Fatal(err)
	}
	commit(t, dir)
	c, err := Load(context.Background(), Options{Mode: ModeCommitted, Dir: dir, Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]File)
	for _, f := range c.Files {
		files[f.Path] = f
	}
	if len(files) != 7 {
		t.Fatalf("got %d files: %+v", len(files), files)
	}
	if f := files["renamed\t文件\n.txt"]; f.OldPath != "old name.txt" || f.Status != "R100" {
		t.Fatalf("wrong rename: %+v", f)
	}
	if f := files["binary.dat"]; !f.Binary || len(f.Hunks) != 0 {
		t.Fatalf("wrong binary: %+v", f)
	}
	if f := files["mode.sh"]; f.Added != 0 || f.OldMode == f.NewMode {
		t.Fatalf("wrong mode change: %+v", f)
	}
	if f := files["type.txt"]; f.Status != "T" {
		t.Fatalf("wrong type change: %+v", f)
	}
	f := files["space tab\tand\nnewline.txt"]
	if f.Added != 1 || f.Hunks[0].Lines[1].Kind != '\\' {
		t.Fatalf("missing EOF marker: %+v", f)
	}
}

func TestLoadEmptyInvalidAndCancelled(t *testing.T) {
	dir := repo(t)
	c, err := Load(context.Background(), Options{Mode: ModeCommitted, Dir: dir, Context: 0})
	if err != nil || len(c.Files) != 0 {
		t.Fatalf("empty diff: %+v, %v", c, err)
	}
	for _, opts := range []Options{{Dir: dir, Base: "missing"}, {Dir: dir, Head: "missing"}, {Dir: dir, Context: -1}, {Dir: t.TempDir()}} {
		if _, err := Load(context.Background(), opts); err == nil {
			t.Fatalf("expected error for %+v", opts)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, Options{Mode: ModeCommitted, Dir: dir}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestLoadSubdirectoryAndGitConfig(t *testing.T) {
	dir := repo(t)
	git(t, dir, "switch", "-qc", "feature")
	write(t, dir, "sub/new.txt", "first\n\nlast\n")
	commit(t, dir)
	git(t, dir, "config", "diff.noprefix", "true")
	git(t, dir, "config", "diff.suppressBlankEmpty", "true")
	git(t, dir, "config", "diff.external", "/does-not-exist")
	git(t, dir, "config", "diff.relative", "true")
	c, err := Load(context.Background(), Options{Mode: ModeCommitted, Dir: filepath.Join(dir, "sub"), Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 1 || c.Files[0].Path != "sub/new.txt" {
		t.Fatalf("wrong subdirectory diff: %+v", c)
	}
}

func TestParseRejectsMalformedRecords(t *testing.T) {
	if _, err := parseRaw(":bad\x00path\x00"); err == nil {
		t.Fatal("accepted malformed raw record")
	}
	if err := applyNumstat([]File{{Path: "a"}}, "1\t2\tunknown\x00"); err == nil {
		t.Fatal("accepted unknown statistics")
	}
	if err := applyPatch([]File{{Path: "a"}}, ""); err == nil {
		t.Fatal("accepted missing patch")
	}
}

func TestLoadSubmodulePointerAndWorktree(t *testing.T) {
	dir := repo(t)
	first := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "source.go", "package main\nfunc updated() {}\n")
	commit(t, dir)
	second := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+first+",vendor/example")
	git(t, dir, "commit", "-qm", "base submodule")
	git(t, dir, "switch", "-qc", "feature")
	git(t, dir, "update-index", "--cacheinfo", "160000,"+second+",vendor/example")
	git(t, dir, "commit", "-qm", "update submodule")
	worktree := filepath.Join(t.TempDir(), "linked")
	git(t, dir, "worktree", "add", "-q", "--detach", worktree, "HEAD")
	c, err := Load(context.Background(), Options{Mode: ModeCommitted, Dir: worktree, Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 1 || c.Files[0].Path != "vendor/example" || c.Added != 1 || c.Deleted != 1 {
		t.Fatalf("wrong submodule diff: %+v", c)
	}
	if !strings.Contains(c.GitDir, "worktrees") {
		t.Fatalf("wrong linked worktree Git dir: %s", c.GitDir)
	}
	if !strings.Contains(c.Files[0].Hunks[0].Lines[1].Text, second) {
		t.Fatal("submodule target missing from patch")
	}
}

func TestDoesNotExecuteTextconv(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitattributes", "*.go diff=unsafe\n")
	commit(t, dir)
	git(t, dir, "config", "diff.unsafe.textconv", "/does-not-exist")
	git(t, dir, "switch", "-qc", "feature")
	write(t, dir, "source.go", "package main\nfunc updated() {}\n")
	commit(t, dir)
	c, err := Load(context.Background(), Options{Mode: ModeCommitted, Dir: dir, Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 1 {
		t.Fatal("missing file when textconv is configured")
	}
}
