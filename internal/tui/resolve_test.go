package tui

import (
	"strings"
	"testing"

	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func TestLocalResolvePersistsExportsAndUnresolves(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.comparison.GitDir = t.TempDir()
	m.snapshot.Persistence = backend.LocalDisk
	var err error
	m.notes.path, err = review.CommentsPath(m.comparison, m.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	press(m, "cc")
	notePaste(m, "Keep this comment\n中文")
	noteSave(m)
	press(m, "C")
	clickText(t, m, "r Resolve")
	if len(m.notes.items) != 1 || !m.notes.items[0].Resolved || !strings.Contains(m.View(), "✓ Resolved") {
		t.Fatal("local resolve removed the comment or hid its state")
	}
	restored := New(m.snapshot, m.source, false, DarkTheme)
	defer restored.Close()
	if len(restored.notes.items) != 1 || !restored.notes.items[0].Resolved {
		t.Fatal("resolved state was not restored")
	}
	md := review.Markdown("local", "", m.comparison, restored.notes.items)
	if !strings.Contains(md, "(resolved)") || !strings.Contains(md, "Keep this comment\n中文") {
		t.Fatal("export lost state or content")
	}
	press(restored, "C")
	clickText(t, restored, "r Unresolve")
	saved, err := review.LoadComments(m.notes.path)
	if err != nil || len(saved) != 1 || saved[0].Resolved || saved[0].Body != m.notes.items[0].Body {
		t.Fatalf("unresolve: %+v %v", saved, err)
	}
	// A failed disk write must not report a successful resolution.
	restored.notes.path += "/not-a-directory"
	press(restored, "r")
	if !restored.hasAlert() || restored.notes.items[0].Resolved {
		t.Fatal("failed local save changed resolved state")
	}
}
