package backend

import "testing"

func TestGitHubTargets(t *testing.T) {
	for _, test := range []struct {
		value, host string
		number      int
	}{
		{"https://github.com/owner/repo/pull/918", "github.com", 918},
		{"https://github.com/owner/repo/pull/918/files?diff=split#diff-a", "github.com", 918},
		{"https://github.example.com/owner/repo.git", "github.example.com", 0},
		{"git@github.com:owner/repo.git", "github.com", 0},
		{"ssh://git@github.example.com/owner/repo.git", "github.example.com", 0},
	} {
		got, err := ParseGitHubURL(test.value)
		if err != nil || got.Host != test.host || got.Owner != "owner" || got.Repo != "repo" || got.Number != test.number {
			t.Fatalf("%s: %+v %v", test.value, got, err)
		}
	}
	for _, value := range []string{"https://github.com/owner/repo/issues/918", "https://github.com/owner/repo/pull/0", "https://github.com/owner/repo/pull/x", "https://user:secret@github.com/owner/repo", "ssh://git:secret@github.com/owner/repo", "http://github.com/owner/repo/pull/918", "https://github.com/owner/%2e%2e", "https://github.com/owner/repo/pull/918/unexpected", "file:///owner/repo"} {
		if _, err := ParseGitHubURL(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
