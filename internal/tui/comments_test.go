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
	t.Setenv("GIT_EDITOR", "true")
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

func TestExportDestinationsAndEditor(t *testing.T) {
	for _, destination := range []string{"temporary", "remote temporary", "directory", "relative directory", "filename"} {
		t.Run(destination, func(t *testing.T) {
			t.Setenv("GIT_EDITOR", "true")
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			m := sampleModel(false)
			t.Cleanup(m.Close)
			m.comparison.Root = t.TempDir()
			m.notes.items = []review.Comment{{Path: "file.go", Body: "Export this note"}}
			m.commentModal = "list"
			m.openExport()
			if len(m.noteInput) != 0 {
				t.Fatal("default export should use the temporary directory")
			}
			wantDir := temp
			switch destination {
			case "remote temporary":
				m.comparison.Root = ""
			case "directory":
				wantDir = t.TempDir()
				m.noteInput = []rune(wantDir)
			case "relative directory":
				wantDir = filepath.Join(m.comparison.Root, "reports")
				if err := os.Mkdir(wantDir, 0700); err != nil {
					t.Fatal(err)
				}
				m.noteInput = []rune("reports")
			case "filename":
				wantDir = m.comparison.Root
				m.noteInput = []rune("notes.md")
			}
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil || m.hasAlert() || m.commentModal != "list" {
				t.Fatalf("export did not launch editor and restore list: %s", m.message)
			}
			path := strings.TrimPrefix(m.message, "Exported Markdown: ")
			if filepath.Dir(path) != wantDir || filepath.Ext(path) != ".md" {
				t.Fatalf("unexpected export path: %s", path)
			}
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "Export this note") {
				t.Fatalf("export contents: %s, %v", data, err)
			}
			m.Update(exportEditorFinishedMsg{path: path})
			if !strings.Contains(m.message, path) || strings.Contains(m.message, "press r") {
				t.Fatalf("missing export completion path: %s", m.message)
			}
			m.Update(exportEditorFinishedMsg{path: path, err: errors.New("editor failed")})
			if !m.hasAlert() || !strings.Contains(m.message, path) {
				t.Fatalf("editor failure lost export location: %s", m.message)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("editor completion removed report: %v", err)
			}
		})
	}
}

func TestExportFailureKeepsDestination(t *testing.T) {
	t.Setenv("GIT_EDITOR", "true")
	m := sampleModel(false)
	t.Cleanup(m.Close)
	m.comparison.Root = t.TempDir()
	m.notes.items = []review.Comment{{Path: "file.go", Body: "Note"}}
	path := filepath.Join(m.comparison.Root, "existing.md")
	if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"existing.md", "missing/notes.md"} {
		m.openExport()
		m.noteInput = []rune(input)
		if cmd := m.exportComments(); cmd != nil || !m.hasAlert() || m.commentModal != "export" || string(m.noteInput) != input {
			t.Fatalf("failed export launched editor or lost destination: %s", m.message)
		}
		press(m, "enter") // Acknowledge the error.
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("existing file changed: %s, %v", data, err)
	}
}

func TestExportSurvivesEditorResolutionFailure(t *testing.T) {
	t.Setenv("GIT_EDITOR", "")
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	m := sampleModel(false)
	t.Cleanup(m.Close)
	m.notes.items = []review.Comment{{Path: "file.go", Body: "Saved report"}}
	m.openExport()
	if cmd := m.exportComments(); cmd != nil || !m.hasAlert() || m.commentModal != "" {
		t.Fatalf("expected an editor error after successful export: %s", m.message)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(files) != 1 {
		t.Fatalf("missing saved report: %v, %v", files, err)
	}
	if !strings.Contains(m.message, files[0]) {
		t.Fatalf("error must include saved report path: %s", m.message)
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
	if m.rangeSelecting {
		t.Fatal("shift-click entered range selection")
	}
	press(m, "c")
	if m.commentDraft.Start != 3 || m.commentDraft.End != 3 {
		t.Fatal("click did not select a single line")
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
