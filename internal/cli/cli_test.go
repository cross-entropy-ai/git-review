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
		{[]string{"--base", "main", "master"}, 2, "not both"},
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
}
