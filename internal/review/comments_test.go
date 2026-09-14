package review

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/diff"
)

func TestCommentStorageAndSnapshotIsolation(t *testing.T) {
	c := &diff.Comparison{GitDir: t.TempDir(), MergeBase: "base", HeadOID: "head"}
	path, err := CommentsPath(c, "local")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := LoadComments(path); err != nil || len(got) != 0 {
		t.Fatalf("missing notes: %v %v", got, err)
	}
	want := []Comment{{ID: "one", Path: "目录/file.go", Side: "old", Start: 3, End: 4, Code: "old line\nsecond line", Body: "为什么删除？\n请说明"}}
	if err := SaveComments(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadComments(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	if err := Save(Path(c), map[string]bool{"目录/file.go": true}); err != nil {
		t.Fatal(err)
	}
	got, err = LoadComments(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("viewed progress overwrote comments")
	}
	c.HeadOID = "next"
	next, _ := CommentsPath(c, "local")
	if next == path {
		t.Fatal("comments reused for a different revision")
	}
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadComments(path); err == nil {
		t.Fatal("unknown format accepted")
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"comments":[{"id":"bad"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadComments(path); err == nil {
		t.Fatal("invalid line anchor accepted")
	}
}

func TestMarkdownExportPreservesNotesAndExistingFiles(t *testing.T) {
	c := &diff.Comparison{Base: "main", Head: "feature", MergeBase: "base-sha", HeadOID: "head-sha"}
	comments := []Comment{
		{ID: "new", Path: "目录/a`file.go", OldPath: "old.go", Side: "new", Start: 7, End: 9, Code: "```\nsource\n```", Body: "**建议**\n\n- 保留校验\n- 返回错误"},
		{ID: "old", Path: "目录/a`file.go", OldPath: "old.go", Side: "old", Start: 2, End: 2, Code: "removed", Body: "Why remove this?"},
	}
	md := Markdown("repo #12", "https://github.com/example/repo/pull/12", c, comments)
	for _, fragment := range []string{"base-sha", "head-sha", "repo #12", "Renamed from", "new L7–L9", "old L2", "````text\n```\nsource\n```\n````", comments[0].Body} {
		if !strings.Contains(md, fragment) {
			t.Fatalf("missing %q:\n%s", fragment, md)
		}
	}
	if strings.Index(md, "old L2") > strings.Index(md, "new L7") {
		t.Fatal("old and new sides are not grouped consistently")
	}
	if comments[0].ID != "new" {
		t.Fatal("export reordered the source slice")
	}
	path := filepath.Join(t.TempDir(), "review.md")
	if err := Export(path, md); err != nil {
		t.Fatal(err)
	}
	if err := Export(path, "overwrite"); !os.IsExist(err) {
		t.Fatalf("existing export not protected: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != md {
		t.Fatal("report was truncated")
	}
}
