package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/diff"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

func notePaste(m *Model, s string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true})
}
func noteSave(m *Model) { m.Update(tea.KeyMsg{Type: tea.KeyCtrlS}) }

func TestLineCommentsCreateEditDeleteAndExport(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.comparison.Root = t.TempDir()
	press(m, "c")
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	press(m, "c")
	if m.commentModal != "edit" || m.commentDraft.Start != 1 || m.commentDraft.End != 3 {
		t.Fatalf("range: %+v", m.commentDraft)
	}
	notePaste(m, "请保留校验\n\n**理由**：避免空值。")
	noteSave(m)
	if len(m.notes.items) != 1 || m.commentModal != "" || m.lineSelecting {
		t.Fatal("comment was not saved")
	}
	if strings.Count(m.notes.items[0].Code, "\n") != 2 || !strings.Contains(m.View(), "●") {
		t.Fatal("excerpt or gutter marker missing")
	}
	press(m, "C")
	press(m, "enter")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	notePaste(m, "Updated comment")
	noteSave(m)
	if len(m.notes.items) != 1 || m.notes.items[0].Body != "Updated comment" || m.commentModal != "list" {
		t.Fatal("editing duplicated or lost the comment")
	}
	press(m, "x")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	notePaste(m, "notes.md")
	press(m, "enter")
	data, err := os.ReadFile(filepath.Join(m.comparison.Root, "notes.md"))
	if err != nil || !strings.Contains(string(data), "new L1–L3") || !strings.Contains(string(data), "Updated comment") {
		t.Fatalf("export: %s %v", data, err)
	}
	press(m, "d")
	press(m, "esc")
	if len(m.notes.items) != 1 {
		t.Fatal("delete cancellation removed the note")
	}
	press(m, "d")
	press(m, "enter")
	if len(m.notes.items) != 0 || m.commentModal != "list" {
		t.Fatal("confirmed deletion did not remove the note")
	}
}

func TestCommentPersistenceNoStateAndSaveFailure(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	root := t.TempDir()
	m.notes.path = filepath.Join(root, "notes.json")
	press(m, "cc")
	notePaste(m, "saved")
	noteSave(m)
	items, err := review.LoadComments(m.notes.path)
	if err != nil || len(items) != 1 {
		t.Fatalf("save: %v %v", items, err)
	}
	// A failed replacement keeps both the old note and the unsaved draft.
	press(m, "C")
	press(m, "enter")
	notePaste(m, " draft")
	m.notes.path = filepath.Join(root, "notes.json", "not-a-directory")
	noteSave(m)
	if !m.hasAlert() || m.commentModal != "edit" || string(m.noteInput) != "saved draft" || m.notes.items[0].Body != "saved" {
		t.Fatal("failed save lost the draft or modified saved notes")
	}
	press(m, "enter")
	m.notes.path = ""
	noteSave(m)
	if m.hasAlert() || m.notes.items[0].Body != "saved draft" {
		t.Fatal("retry did not save draft in memory")
	}
	old := m.snapshot
	other := *old
	other.Key = "other revision"
	m.install(&other)
	if len(m.notes.items) != 0 {
		t.Fatal("comments leaked into another revision")
	}
	m.install(old)
	if len(m.notes.items) != 1 {
		t.Fatal("memory notes lost when returning to the same comparison")
	}
	if _, err := os.Stat(filepath.Join(root, "notes.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCommentOldNewSidesAndRangeBoundaries(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.comparison.Files = []diff.File{{Path: "new.go", OldPath: "old.go", Hunks: []diff.Hunk{
		{Lines: []diff.Line{{Kind: '-', Old: 1, Text: "removed"}, {Kind: '+', New: 1, Text: "added"}, {Kind: ' ', Old: 2, New: 2, Text: "same"}}},
		{Lines: []diff.Line{{Kind: ' ', Old: 20, New: 20, Text: "far away"}}},
	}}}
	m.rebuild()
	press(m, "c")
	for i := 0; i < 4; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	}
	press(m, "c")
	if m.commentDraft.Side != "old" || m.commentDraft.End != 2 || m.commentDraft.Code != "removed\nsame" {
		t.Fatalf("mixed sides or crossed hunk: %+v", m.commentDraft)
	}
	notePaste(m, "old side")
	noteSave(m)
	press(m, "scc")
	if m.commentDraft.Side != "new" || m.commentDraft.Code != "added" {
		t.Fatalf("split selected wrong side: %+v", m.commentDraft)
	}
	notePaste(m, "new side")
	noteSave(m)
	if len(m.notes.items) != 2 || m.notes.items[0].OldPath != "old.go" {
		t.Fatal("side-specific comments were combined")
	}
	press(m, "C")
	press(m, "g")
	if !m.lineSelecting || m.commentLine.side != "old" || m.lineNumber(m.commentLine) != 1 {
		t.Fatal("jump to old-side comment failed")
	}
}

func TestCommentModalsInputIsolationAndThemes(t *testing.T) {
	for _, theme := range []Theme{LightTheme, DarkTheme} {
		for _, color := range []bool{false, true} {
			m := sampleModel(color)
			t.Cleanup(m.Close)
			m.palette = paletteFor(theme)
			press(m, "cc")
			notePaste(m, strings.Repeat("中文长评论\n", 40)+"qevr/\x1b[2J")
			if m.viewedCount() != 0 || m.loading || m.searching || m.picking {
				t.Fatal("pasted comment ran shortcuts")
			}
			for _, size := range [][2]int{{120, 34}, {60, 18}, {45, 12}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for _, modal := range []string{"edit", "export", "list"} {
					m.commentModal = modal
					view := m.View()
					if len(strings.Split(view, "\n")) != size[1] {
						t.Fatalf("%s height overflow", modal)
					}
					for _, line := range strings.Split(view, "\n") {
						if ansi.StringWidth(line) != size[0] {
							t.Fatalf("%s width overflow", modal)
						}
					}
					if strings.Contains(view, "\x1b[2J") || (!color && strings.Contains(view, "\x1b")) {
						t.Fatal("unsafe terminal controls")
					}
				}
			}
			m.commentModal = "edit"
			m.Update(editorFinishedMsg{err: errors.New("async failure")})
			press(m, "enter")
			if m.commentModal != "edit" || len(m.noteInput) == 0 {
				t.Fatal("alert confirmation lost the draft")
			}
		}
	}
}

func TestPRCommentsAreLocalAndRefreshCannotReplaceDraft(t *testing.T) {
	m, source := remoteModel()
	defer m.Close()
	m.notes.path = filepath.Join(t.TempDir(), "pr-notes.json")
	press(m, "cc")
	notePaste(m, "PR comment")
	noteSave(m)
	if source.saves != 0 {
		t.Fatal("comment was sent to GitHub")
	}
	if items, err := review.LoadComments(m.notes.path); err != nil || len(items) != 1 {
		t.Fatalf("PR notes not local: %v %v", items, err)
	}
	m.loading = true
	press(m, "c")
	press(m, "C")
	press(m, "x")
	if m.lineSelecting || m.commentModal != "" {
		t.Fatal("draft can be opened against a refreshing comparison")
	}
	// A new model restores local comments independently of Viewed state.
	s := *m.snapshot
	s.Comparison = &diff.Comparison{GitDir: t.TempDir(), HeadOID: "pr-head", Files: m.comparison.Files}
	s.Persistence = backend.LocalDisk
	path, _ := review.CommentsPath(s.Comparison, s.Key)
	if err := review.SaveComments(path, m.notes.items); err != nil {
		t.Fatal(err)
	}
	restored := New(&s, source, false, DarkTheme)
	defer restored.Close()
	if len(restored.notes.items) != 1 {
		t.Fatal("saved comments not restored on startup")
	}
}

func TestCommentMouseSelectionAndConfirmation(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	g := m.layout()
	clickAt(m, g.diffX+20, contentTop+2)
	if m.lineSelecting || m.commentModal != "" {
		t.Fatal("ordinary click entered comment mode")
	}
	press(m, "c")
	clickAt(m, g.diffX+20, contentTop+2)
	if !m.lineSelecting || m.lineNumber(m.commentLine) != 1 {
		t.Fatal("click did not select a source line")
	}
	m.Update(tea.MouseMsg{X: g.diffX + 20, Y: contentTop + 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Shift: true})
	press(m, "c")
	if m.commentDraft.Start != 1 || m.commentDraft.End != 3 {
		t.Fatal("shift-click did not select a range")
	}
	notePaste(m, "mouse note")
	clickText(t, m, "Ctrl+S Save")
	if len(m.notes.items) != 1 {
		t.Fatal("mouse save failed")
	}
	clickText(t, m, "C Comments")
	clickText(t, m, "d Delete")
	clickText(t, m, "Enter Delete")
	if len(m.notes.items) != 0 {
		t.Fatal("mouse delete confirmation failed")
	}
}

func TestCommentSplitNavigationReachesUnmatchedCells(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.comparison.Files = []diff.File{
		{Path: "removed.go", Hunks: []diff.Hunk{{Lines: []diff.Line{{Kind: '-', Old: 1, Text: "old"}}}}},
		{Path: "added.go", Hunks: []diff.Hunk{{Lines: []diff.Line{{Kind: '+', New: 1, Text: "new"}}}}},
	}
	m.rebuild()
	press(m, "scj")
	if m.commentLine.file != 1 || m.commentLine.side != "new" {
		t.Fatal("new-only split cell is unreachable")
	}
	press(m, "k")
	if m.commentLine.file != 0 || m.commentLine.side != "old" {
		t.Fatal("old-only split cell is unreachable")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	if m.commentLine.file != 0 || m.commentLine.side != "old" {
		t.Fatal("range crossed into another file or side")
	}
}

func TestShiftArrowsStartExtendShrinkAndLeaveRange(t *testing.T) {
	m := sampleModel(false)
	defer m.Close()
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	if !m.lineSelecting || !m.rangeSelecting || m.selectedComment().Start != 1 || m.selectedComment().End != 2 {
		t.Fatal("Shift+Down did not start range selection")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
	if m.selectedComment().End != 3 {
		t.Fatal("Shift+Down did not extend range")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.selectedComment().End != 2 {
		t.Fatal("Shift+Up did not shrink range")
	}
	press(m, "down")
	if m.rangeSelecting || m.selectedComment().Start != 3 || m.selectedComment().End != 3 {
		t.Fatal("plain arrow did not return to single-line selection")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.selectedComment().Start != 2 || m.selectedComment().End != 3 {
		t.Fatal("a new range did not anchor at the current cursor")
	}
	press(m, "c")
	if m.commentDraft.Start != 2 || m.commentDraft.End != 3 {
		t.Fatal("comment did not capture the selected range")
	}
}
