package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/tui"
)

func TestArguments(t *testing.T) {
	for _, test := range []struct {
		args []string
		code int
		text string
	}{
		{[]string{"--help"}, 0, "Usage: git review"},
		{[]string{"--version"}, 0, "git-review test"},
		{[]string{"--unknown"}, 2, "flag provided but not defined"},
		{[]string{"--context", "-1"}, 2, "between 0 and 100"},
		{[]string{"--theme", "sepia"}, 2, "--theme must be auto, light, or dark"},
		{[]string{"--auto", "-w"}, 2, "mutually exclusive"},
		{[]string{"-c", "--auto"}, 2, "mutually exclusive"},
		{[]string{"--working-tree", "--committed"}, 2, "mutually exclusive"},
		{[]string{"-c", "-w"}, 2, "mutually exclusive"},
		{[]string{"-w", "main"}, 2, "cannot be combined"},
		{[]string{"--working-tree", "--base", "main"}, 2, "cannot be combined"},
		{[]string{"-w", "--head", "HEAD"}, 2, "cannot be combined"},
		{[]string{"--base", "main", "master"}, 2, "not both"},
		{[]string{"-w", "main...HEAD"}, 2, "cannot be combined"},
		{[]string{"--base", "main", "main...HEAD"}, 2, "cannot be combined"},
		{[]string{"--head", "HEAD", "main...HEAD"}, 2, "cannot be combined"},
		{[]string{"main...HEAD", "HEAD"}, 2, "cannot be combined"},
		{[]string{"main", "main...HEAD"}, 2, "cannot be combined"},
		{[]string{"...HEAD"}, 2, "both refs"},
		{[]string{"main..."}, 2, "both refs"},
		{[]string{"..."}, 2, "both refs"},
		{[]string{"main...HEAD...other"}, 2, "exactly three dots"},
		{[]string{"main....HEAD"}, 2, "exactly three dots"},
		{[]string{"one", "two", "three"}, 2, "at most two refs"},
		{nil, 1, "interactive terminal"},
	} {
		var out, errOut bytes.Buffer
		code := Run(test.args, &out, &errOut, "test")
		if code != test.code || !strings.Contains(out.String()+errOut.String(), test.text) {
			t.Errorf("%v: code %d, out %q, error %q", test.args, code, out.String(), errOut.String())
		}
	}
}

func TestResolveTheme(t *testing.T) {
	for _, test := range []struct {
		name  string
		color bool
		dark  bool
		want  tui.Theme
		query bool
	}{
		{"auto", true, true, tui.DarkTheme, true},
		{"auto", true, false, tui.LightTheme, true},
		{"dark", true, false, tui.DarkTheme, false},
		{"light", true, true, tui.LightTheme, false},
		{"auto", false, false, tui.DarkTheme, false},
	} {
		queried := false
		got := resolveTheme(test.name, test.color, func() bool {
			queried = true
			return test.dark
		})
		if got != test.want || queried != test.query {
			t.Errorf("%+v: got theme %q, queried %v", test, got, queried)
		}
	}
}

func TestStatsWithRealRepository(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", os.DevNull)
	git("commit", "--allow-empty", "-qm", "base")
	git("switch", "-qc", "feature")
	if err := os.WriteFile(filepath.Join(dir, "a\nfile.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "change")
	var out, errOut bytes.Buffer
	if code := Run([]string{"--stat", "-C", dir}, &out, &errOut, "test"); code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "1 files changed, 2 insertions(+), 0 deletions(-)") || !strings.Contains(out.String(), `"a\nfile.txt"`) {
		t.Fatalf("unexpected statistics: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "git-review")); !os.IsNotExist(err) {
		t.Fatal("--stat wrote review state")
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"", "--auto", "-w", "--working-tree"} {
		out.Reset()
		errOut.Reset()
		args := []string{"--stat", "-C", dir}
		if option != "" {
			args = append(args, option)
		}
		if code := Run(args, &out, &errOut, "test"); code != 0 {
			t.Fatalf("%s: code %d: %s", option, code, errOut.String())
		}
		if !strings.Contains(out.String(), `"HEAD" -> "working tree"`) || !strings.Contains(out.String(), "1 files changed, 1 insertions(+), 0 deletions(-)") || !strings.Contains(out.String(), "untracked.txt") {
			t.Fatalf("%s: unexpected working-tree statistics: %s", option, out.String())
		}
	}
	git("tag", "base-tag", "main")
	for _, test := range []struct {
		args  []string
		local bool
	}{
		{[]string{"-c"}, false},
		{[]string{"--committed"}, false},
		{[]string{"--base", "base-tag"}, false},
		{[]string{"--head", "HEAD"}, false},
		{[]string{"base-tag", "HEAD"}, false},
		{[]string{"base-tag...HEAD"}, false},
		{[]string{"-c", "base-tag...HEAD"}, false},
		{[]string{"-c", "base-tag", "HEAD"}, false},
		{[]string{"--auto", "--base", "base-tag"}, true},
		{[]string{"--auto", "base-tag", "HEAD"}, true},
		{[]string{"--auto", "base-tag...HEAD"}, true},
	} {
		out.Reset()
		errOut.Reset()
		args := append([]string{"--stat", "-C", dir}, test.args...)
		if code := Run(args, &out, &errOut, "test"); code != 0 {
			t.Fatalf("%v: code %d: %s", args, code, errOut.String())
		}
		if strings.Contains(out.String(), "untracked.txt") != test.local || strings.Contains(out.String(), `"a\nfile.txt"`) == test.local {
			t.Fatalf("%v: wrong review scope: %s", args, out.String())
		}
	}
	if err := os.Remove(filepath.Join(dir, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"--auto", "-w", "--working-tree"} {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"--stat", "-C", dir, option}, &out, &errOut, "test"); code != 0 {
			t.Fatalf("%s: %s", option, errOut.String())
		}
		if option == "--auto" {
			if !strings.Contains(out.String(), `"main"..."feature"`) {
				t.Fatalf("auto did not fall back to committed changes: %s", out.String())
			}
		} else if !strings.Contains(out.String(), "No uncommitted changes to review.") {
			t.Fatalf("%s did not stay in working-tree mode: %s", option, out.String())
		}
	}

	if _, err := os.Stat(filepath.Join(dir, ".git", "git-review")); !os.IsNotExist(err) {
		t.Fatal("working-tree --stat wrote review state")
	}
	// Use divergent tags and a head other than HEAD to catch endpoint parsing
	// mistakes and accidental two-dot (direct endpoint) diff semantics.
	git("tag", "v1.2.0")
	git("switch", "-q", "main")
	writeAndCommit := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("not in the review\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", name)
		git("commit", "-qm", name)
	}
	writeAndCommit("base-only.txt")
	git("tag", "v1.1.0")
	git("switch", "-q", "feature")
	writeAndCommit("head-only.txt")
	var expected string
	for _, refs := range [][]string{{"v1.1.0", "v1.2.0"}, {"v1.1.0...v1.2.0"}} {
		out.Reset()
		errOut.Reset()
		if code := Run(append([]string{"--stat", "-C", dir}, refs...), &out, &errOut, "test"); code != 0 {
			t.Fatalf("%v: %s", refs, errOut.String())
		}
		if expected == "" {
			expected = out.String()
		}
		if out.String() != expected || !strings.Contains(out.String(), `"v1.1.0"..."v1.2.0"`) ||
			!strings.Contains(out.String(), "1 files changed, 2 insertions(+), 0 deletions(-)") ||
			strings.Contains(out.String(), "base-only.txt") || strings.Contains(out.String(), "head-only.txt") {
			t.Fatalf("%v: wrong tag comparison: %s", refs, out.String())
		}
	}

}
