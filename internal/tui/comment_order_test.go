package tui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func TestCommentOrderGroupsFilesLinesAndReplies(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.commentAllFiles = true
	m.notes.items = []review.Comment{
		{ID: "z", Path: "z.go", Start: 1},
		{ID: "reply2", RemoteID: 500, ReplyTo: 100, Path: "a.go", Start: 10, Side: "new"},
		{ID: "later", RemoteID: 110, Path: "a.go", Start: 20, Side: "new"},
		{ID: "root", RemoteID: 100, Path: "a.go", Start: 10, Side: "new"},
		{ID: "old", Path: "a.go", Start: 10, Side: "old"},
		{ID: "reply1", RemoteID: 400, ReplyTo: 100, Path: "a.go", Start: 10, Side: "new"},
		{ID: "same-line-thread", RemoteID: 101, Path: "a.go", Start: 10, Side: "new"},
		{ID: "draft-reply", ReplyTo: 100, Path: "a.go", Start: 10, Side: "new"},
		{ID: "first", Path: "a.go", Start: 2, Side: "new"},
		{ID: "orphan2", RemoteID: 700, ReplyTo: 600, Path: "a.go", Start: 30, Side: "new"},
		{ID: "orphan1", RemoteID: 650, ReplyTo: 600, Path: "a.go", Start: 30, Side: "new"},
	}
	ids := func() []string {
		var result []string
		for _, index := range m.visibleComments() {
			result = append(result, m.notes.items[index].ID)
		}
		return result
	}
	want := []string{"first", "old", "root", "reply1", "reply2", "draft-reply", "same-line-thread", "later", "orphan1", "orphan2", "z"}
	if got := ids(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order: %v; want %v", got, want)
	}
	// Saving a GitHub edit moves the record to the end of the backing list.
	root := m.notes.items[3]
	root.Body = "edited"
	m.notes.items = append(append(m.notes.items[:3:3], m.notes.items[4:]...), root)
	if got := ids(); !reflect.DeepEqual(got, want) {
		t.Fatalf("editing moved the discussion: %v", got)
	}
	m.commentModal = "list"
	m.commentIndex = m.visibleComments()[2]
	press(m, "j")
	press(m, "enter")
	if m.commentDraft.ID != "reply1" {
		t.Fatal("sorted navigation edited the wrong comment")
	}
}

func TestCommentDividersScrollAndIgnoreClicks(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.commentAllFiles, m.commentModal = true, "list"
	m.notes.items = []review.Comment{
		{ID: "1", Path: "a.go"}, {ID: "2", Path: "a.go"},
		{ID: "3", Path: "b.go"}, {ID: "4", Path: "c.go"},
		{ID: "5", Path: "c.go"}, {ID: "6", Path: "d.go"},
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for selected := range m.notes.items {
		m.commentIndex = selected
		rows := m.commentListRows()
		if len(rows) == 0 || rows[0].part != 0 || len(rows) > m.modalLayout().height-3 {
			t.Fatalf("missing file context or overflowing rows: %+v", rows)
		}
		location, body, path := false, false, ""
		for _, row := range rows {
			c := m.notes.items[row.index]
			if row.part == 0 {
				path = c.Path
			} else if path != c.Path {
				t.Fatal("comment displayed under another file's divider")
			}
			if row.index == selected {
				location = location || row.part == 1
				body = body || row.part == 2
			}
		}
		if !location || !body {
			t.Fatalf("selected comment clipped: %+v", rows)
		}
		g := m.modalLayout()
		m.commentMouse(tea.MouseMsg{X: g.x + 2, Y: g.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.commentIndex != selected {
			t.Fatal("clicking a file divider changed selection")
		}
	}
}
