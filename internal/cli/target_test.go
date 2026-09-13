package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteTargetLocalFirst(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("-c", "commit.gpgsign=false", "-c", "core.hooksPath="+os.DevNull, "commit", "--allow-empty", "-qm", "initial")
	git("branch", "918")
	git("tag", "919")
	git("remote", "add", "origin", "git@github.com:owner/repo.git")
	for _, test := range []struct {
		args     []string
		fallback bool
		number   int
	}{
		{[]string{"918"}, true, 0}, {[]string{"919"}, true, 0},
		{[]string{"920"}, true, 920}, {[]string{"920"}, false, 0},
		{[]string{"918", "main"}, true, 0}, {[]string{"#918"}, true, 918},
		{[]string{"https://github.com/owner/repo/pull/921"}, true, 921},
		{[]string{"https://github.com/owner/repo", "#922"}, true, 922},
	} {
		target, err := remoteTarget(context.Background(), test.args, dir, test.fallback)
		if err != nil {
			t.Fatalf("%v: %v", test.args, err)
		}
		if test.number == 0 {
			if target != nil {
				t.Fatalf("local ref routed to GitHub: %v", test.args)
			}
			continue
		}
		if target == nil || target.Number != test.number || target.Owner != "owner" || target.Repo != "repo" {
			t.Fatalf("%v: %+v", test.args, target)
		}
	}
	git("remote", "remove", "origin")
	if _, err := remoteTarget(context.Background(), []string{"920"}, dir, true); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("missing origin error: %v", err)
	}
	if target, err := remoteTarget(context.Background(), []string{"918"}, dir, true); err != nil || target != nil {
		t.Fatal("local refs unexpectedly required origin")
	}
	if _, err := remoteTarget(context.Background(), []string{"https://github.com/owner/repo/pull/921"}, t.TempDir(), true); err != nil {
		t.Fatal("PR URL required a local repository")
	}
}

func TestGitHubRequiresInstalledAuthenticatedGH(t *testing.T) {
	url := "https://github.com/owner/repo/pull/918"
	for _, installed := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "not-logged-in"}[installed], func(t *testing.T) {
			bin := t.TempDir()
			if installed {
				if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'not logged in' >&2\nexit 1\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)
			var out, errOut bytes.Buffer
			code := Run([]string{"--stat", url}, &out, &errOut, "test")
			if code != 1 || !strings.Contains(errOut.String(), "gh auth login --hostname github.com") {
				t.Fatalf("code=%d: %s", code, errOut.String())
			}
			if !installed && !strings.Contains(errOut.String(), "https://cli.github.com") {
				t.Fatal("missing gh installation instructions")
			}
		})
	}
}

func TestRemoteArgumentConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"--base", "main", "https://github.com/o/r/pull/918"},
		{"--head", "HEAD", "https://github.com/o/r/pull/918"},
		{"--auto", "https://github.com/o/r/pull/918"},
		{"-c", "https://github.com/o/r/pull/918"},
		{"-w", "https://github.com/o/r/pull/918"},
		{"--context", "10", "https://github.com/o/r/pull/918"},
		{"https://github.com/o/r/pull/918", "main"},
		{"https://github.com/o/r"}, {"#0"}, {"#918", "HEAD"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut, "test"); code != 2 {
			t.Fatalf("%v: code=%d error=%s", args, code, errOut.String())
		}
	}
}
