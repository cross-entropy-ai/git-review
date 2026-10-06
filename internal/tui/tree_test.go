package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func treeModel(paths ...string) *Model {
	m := sampleModel(true)
	file := m.comparison.Files[0]
	m.comparison.Files = nil
	for _, path := range paths {
		copy := file
		copy.Path = path
		m.comparison.Files = append(m.comparison.Files, copy)
	}
	m.rebuild()
	return m
}

func treeIndex(t *testing.T, m *Model, path string) int {
	t.Helper()
	for i, entry := range m.treeRows {
		if entry.path == path {
			return i
		}
	}
	t.Fatalf("tree entry %q missing: %+v", path, m.treeRows)
	return -1
}

func TestTreeOrderAndTogglePreserveReview(t *testing.T) {
	m := treeModel("z.go", "src/z.go", "docs/中文.md", "src/a/main.go", "a.go", "docs")
	m.selected = 3
	m.viewed["z.go"], m.collapsed["z.go"] = true, true
	m.rebuild()
	m.jumpSelected()
	rows, offset := append([]row(nil), m.rows...), m.offset
	press(m, "t")
	if m.rows[m.offset] != rows[offset] {
		t.Fatal("tree toggle changed the source row at the top of the diff")
	}
	var got []string
	for _, entry := range m.treeRows {
		got = append(got, entry.path)
	}
	want := []string{"docs/", "docs/中文.md", "src/", "src/a/", "src/a/main.go", "src/z.go", "a.go", "docs", "z.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tree order: %v", got)
	}
	var diffOrder []string
	for _, r := range m.rows {
		if r.kind == 'f' {
			diffOrder = append(diffOrder, m.comparison.Files[r.file].Path)
		}
	}
	if want := []string{"docs/中文.md", "src/a/main.go", "src/z.go", "a.go", "docs", "z.go"}; !reflect.DeepEqual(diffOrder, want) {
		t.Fatalf("diff order does not follow the tree: %v", diffOrder)
	}
	if m.treeRows[treeIndex(t, m, "src/")].count != 2 {
		t.Fatal("directory count does not include nested files")
	}
	press(m, "t")
	if m.treeMode || m.selected != 3 || m.offset != offset || !reflect.DeepEqual(m.rows, rows) || !m.viewed["z.go"] || !m.collapsed["z.go"] {
		t.Fatal("view toggle changed diff position or review state")
	}
}

func TestTreeDirectoryKeyboardAndReveal(t *testing.T) {
	m := treeModel("main.go", "src/a.go", "src/nested/z.go", "docs/readme.md")
	press(m, "t")
	m.selectTree(treeIndex(t, m, "src/"))
	rows := len(m.rows)
	press(m, " ")
	if !m.treeClosed["src/"] || len(m.rows) != rows || m.collapsed["src/a.go"] {
		t.Fatal("directory folding modified the diff")
	}
	press(m, "v")
	if m.viewedCount() != 0 {
		t.Fatal("directory focus marked an unrelated file viewed")
	}
	press(m, "right")
	press(m, "right")
	if m.treeRows[m.treeCursor].path != "src/nested/" {
		t.Fatal("right did not expand and enter the first child directory")
	}
	press(m, "j")
	if m.selected != 2 {
		t.Fatal("tree navigation did not select the nested file")
	}
	press(m, "left")
	press(m, "left")
	if !m.treeClosed["src/nested/"] || m.treeRows[m.treeCursor].path != "src/nested/" {
		t.Fatal("left did not select and close the parent")
	}
	press(m, "left")
	press(m, "left")
	if !m.treeClosed["src/"] {
		t.Fatal("left did not close the ancestor")
	}
	press(m, "n")
	if m.selected != 1 || m.treeClosed["src/"] || m.treeRows[m.treeCursor].file != 1 {
		t.Fatal("file navigation did not reveal the selected file")
	}
}

func TestTreeDiffOrderAcrossLayoutsAndDirectoryFolding(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, wrap := range []bool{false, true} {
			t.Run(fmt.Sprintf("split=%v/wrap=%v", split, wrap), func(t *testing.T) {
				m := treeModel("z.go", "src/z.go", "docs/中文.md", "src/a/main.go", "a.go")
				t.Cleanup(m.Close)
				m.splitMode, m.wrapLines = split, wrap
				press(m, "t")
				checkOrder := func() {
					t.Helper()
					var headers []int
					for _, r := range m.rows {
						if r.kind == 'f' {
							headers = append(headers, r.file)
						}
					}
					want := []int{2, 3, 1, 4, 0}
					if !reflect.DeepEqual(headers, want) || !reflect.DeepEqual(m.visible, want) {
						t.Fatalf("headers=%v navigation=%v, want %v", headers, m.visible, want)
					}
				}
				checkOrder()
				press(m, "f")
				if !reflect.DeepEqual(m.fileMatches, m.visible) {
					t.Fatal("file picker does not follow display order")
				}
				press(m, "esc")
				rows := append([]row(nil), m.rows...)
				m.toggleDirectory(treeIndex(t, m, "src/"))
				if !reflect.DeepEqual(m.rows, rows) {
					t.Fatal("closing a directory changed the diff")
				}
				checkOrder()
				press(m, "z")
				checkOrder()
				m.Update(loadedMsg{snapshot: m.snapshot})
				checkOrder()
				if m.selected != 2 || m.rows[m.offset].file != 2 {
					t.Fatal("refresh did not select the first displayed file")
				}
			})
		}
	}
}

func TestTreeDiffNavigationFollowsDisplayOrder(t *testing.T) {
	m := treeModel("z.go", "docs/a.go", "src/nested/b.go", "src/a.go")
	t.Cleanup(m.Close)
	press(m, "t")
	m.selected = 1
	m.jumpSelected()
	press(m, "n")
	if m.selected != 2 {
		t.Fatal("next file did not follow tree order")
	}
	press(m, "v")
	if m.selected != 3 || !m.viewed["src/nested/b.go"] {
		t.Fatal("marking viewed did not advance in tree order")
	}
	press(m, "n")
	if m.selected != 0 {
		t.Fatal("next file did not reach the root file after directories")
	}
	press(m, "p")
	if m.selected != 3 {
		t.Fatal("previous file did not follow tree order")
	}
	// Cross the final file boundary, where backend indices run backwards.
	for i, r := range m.rows {
		if r.kind == 'f' && r.file == 0 {
			m.offset = i - 1
			m.scroll(1)
			if m.selected != 0 {
				t.Fatal("scrolling down did not select the next displayed file")
			}
			m.scroll(-1)
			if m.selected != 3 {
				t.Fatal("scrolling up did not select the previous displayed file")
			}
			return
		}
	}
	t.Fatal("root file header missing")
}

func TestTreeMouseCheckboxAndPicker(t *testing.T) {
	m := treeModel("main.go", "目录/deep/a.go", "目录/deep/b.go")
	m.comparison.Files[1].OldPath = "old/name.go"
	press(m, "t")
	clickText(t, m, "目录/")
	if !m.treeClosed["目录/"] {
		t.Fatal("directory click did not collapse its children")
	}
	clickText(t, m, "目录/")
	entry := treeIndex(t, m, "目录/deep/a.go")
	y := contentTop + entry - m.sideOffset
	line := ansi.Strip(strings.Split(m.View(), "\n")[y])
	x := strings.Index(line, "[ ]")
	clickAt(m, ansi.StringWidth(line[:x])+1, y)
	if !m.viewed["目录/deep/a.go"] || m.selected != 1 || !m.collapsed["目录/deep/a.go"] {
		t.Fatal("indented checkbox hit the wrong file")
	}
	press(m, "fold/name")
	if len(m.fileMatches) != 1 || m.fileMatches[0] != 1 {
		t.Fatal("rename search did not match old path")
	}
	press(m, "enter")
	if m.selected != 1 || m.treeRows[m.treeCursor].file != 1 || len(m.visible) != 3 {
		t.Fatal("picker did not reveal rename destination")
	}
	before := len(m.treeRows)
	press(m, "fmissing")
	for _, key := range []string{"j", "k", "left", "right", " ", "v", "t", "t"} {
		press(m, key)
	}
	if len(m.treeRows) != before || m.viewedCount() != 1 || len(m.fileMatches) != 0 {
		t.Fatal("empty picker input changed review state")
	}
	press(m, "esc")

}

func TestTreeScrollResizeAndThemes(t *testing.T) {
	m := treeModel("root.go")
	for i := 0; i < 40; i++ {
		m.comparison.Files = append(m.comparison.Files, gitdiff.File{Path: fmt.Sprintf("src/deep/%02d.go", i), Status: "A"})
	}
	m.rebuild()
	press(m, "t")
	m.selectTree(0)
	m.Update(tea.MouseMsg{X: 12, Y: contentTop + 2, Button: tea.MouseButtonWheelDown})
	if m.sideOffset != 3 {
		t.Fatalf("tree wheel offset = %d", m.sideOffset)
	}
	index := m.treeRows[m.sideOffset].file
	clickAt(m, 18, contentTop)
	if index < 0 || m.selected != index {
		t.Fatal("scrolled tree click selected the wrong file")
	}
	g := m.layout()
	clickAt(m, g.sideWidth-1, contentTop+g.bodyHeight-1)
	if m.sideOffset != len(m.treeRows)-m.sidebarCapacity() {
		t.Fatal("tree scrollbar did not reach the last row")
	}
	for _, theme := range []Theme{LightTheme, DarkTheme} {
		m.palette = paletteFor(theme)
		for _, size := range [][2]int{{120, 35}, {90, 24}, {60, 18}, {45, 12}} {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			lines := strings.Split(m.View(), "\n")
			if len(lines) != size[1] {
				t.Fatal("tree view changed terminal height")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Fatalf("tree view width at %v: %d", size, ansi.StringWidth(line))
				}
			}
		}
	}
}
