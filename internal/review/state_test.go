package review

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/imwithye/git-review/internal/gitdiff"
)

func TestStateRoundTripAndSnapshotIsolation(t *testing.T) {
	c := &gitdiff.Comparison{GitDir: t.TempDir(), MergeBase: "base", HeadOID: "head"}
	path := Path(c)
	viewed, err := Load(path)
	if err != nil || len(viewed) != 0 {
		t.Fatalf("missing state: %v %v", viewed, err)
	}
	want := map[string]bool{"weird\n文件.go": true, "other.go": false}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	c.HeadOID = "new-head"
	if Path(c) == path {
		t.Fatal("changed head reused stale review progress")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid state error")
	}
	if err := Save(path, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}
