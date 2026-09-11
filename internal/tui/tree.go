package tui

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type treeEntry struct {
	path  string
	name  string
	file  int // A negative index denotes a directory.
	depth int
	count int
}

type treeNode struct {
	entry    treeEntry
	children map[string]*treeNode
}

// Build from filtered diff paths, including deleted files and rename destinations.
// Directory expansion is independent of diff folding and viewed progress.
func (m *Model) rebuildTree() {
	m.treeRows = nil
	if !m.treeMode {
		return
	}
	root := &treeNode{children: make(map[string]*treeNode)}
	for _, index := range m.visible {
		file := m.comparison.Files[index]
		parts := strings.Split(file.Path, "/")
		node := root
		for depth, name := range parts[:len(parts)-1] {
			key := "dir:" + name
			child := node.children[key]
			if child == nil {
				child = &treeNode{
					entry:    treeEntry{path: strings.Join(parts[:depth+1], "/") + "/", name: name, file: -1, depth: depth},
					children: make(map[string]*treeNode),
				}
				node.children[key] = child
			}
			child.entry.count++
			node = child
		}
		name := parts[len(parts)-1]
		node.children["file:"+name] = &treeNode{entry: treeEntry{path: file.Path, name: name, file: index, depth: len(parts) - 1}}
	}
	var flatten func(*treeNode)
	flatten = func(node *treeNode) {
		children := make([]*treeNode, 0, len(node.children))
		for _, child := range node.children {
			children = append(children, child)
		}
		sort.Slice(children, func(i, j int) bool {
			a, b := children[i].entry, children[j].entry
			if (a.file < 0) != (b.file < 0) {
				return a.file < 0
			}
			return a.name < b.name
		})
		for _, child := range children {
			m.treeRows = append(m.treeRows, child.entry)
			if !m.treeClosed[child.entry.path] || m.filter != "" {
				flatten(child)
			}
		}
	}
	flatten(root)
	m.treeCursor = min(m.treeCursor, max(0, len(m.treeRows)-1))
	m.clampSidebar()
}

func (m *Model) revealTreeFile() {
	if len(m.visible) == 0 {
		m.treeCursor = 0
		return
	}
	changed := false
	for dir := path.Dir(m.comparison.Files[m.selected].Path); dir != "."; dir = path.Dir(dir) {
		if m.treeClosed[dir+"/"] {
			delete(m.treeClosed, dir+"/")
			changed = true
		}
	}
	if changed {
		m.rebuildTree()
	}
	for i, entry := range m.treeRows {
		if entry.file == m.selected {
			m.treeCursor = i
			break
		}
	}
}

func (m *Model) treeFocus() bool {
	return m.treeMode && m.fileFocus && m.layout().sideWidth > 0
}

func (m *Model) treeDirectoryFocused() bool {
	return m.treeFocus() && len(m.treeRows) > 0 && m.treeRows[m.treeCursor].file < 0
}

func (m *Model) selectTree(index int) {
	if index < 0 || index >= len(m.treeRows) {
		return
	}
	m.treeCursor, m.fileFocus = index, true
	if entry := m.treeRows[index]; entry.file >= 0 {
		m.selected, m.xOffset = entry.file, 0
		m.jumpSelected()
	} else {
		m.ensureSidebarIndexVisible(index)
	}
}

func (m *Model) moveTree(delta int) {
	m.selectTree(min(max(0, m.treeCursor+delta), len(m.treeRows)-1))
}

func (m *Model) toggleDirectory(index int) {
	if index < 0 || index >= len(m.treeRows) || m.treeRows[index].file >= 0 {
		return
	}
	if m.filter != "" {
		m.message = "Directories stay expanded while filtering; Esc clears the filter"
		return
	}
	dir := m.treeRows[index].path
	m.treeClosed[dir] = !m.treeClosed[dir]
	m.rebuildTree()
	for i, entry := range m.treeRows {
		if entry.path == dir {
			m.treeCursor = i
			break
		}
	}
	m.ensureSidebarIndexVisible(m.treeCursor)
}

func (m *Model) treeLeft() {
	if len(m.treeRows) == 0 {
		return
	}
	entry := m.treeRows[m.treeCursor]
	if entry.file < 0 && !m.treeClosed[entry.path] {
		m.toggleDirectory(m.treeCursor)
		return
	}
	for i := m.treeCursor - 1; i >= 0; i-- {
		if m.treeRows[i].depth < entry.depth {
			m.selectTree(i)
			return
		}
	}
}

func (m *Model) treeRight() {
	if len(m.treeRows) == 0 {
		return
	}
	entry := m.treeRows[m.treeCursor]
	if entry.file >= 0 {
		if m.collapsed[entry.path] {
			m.toggleFold()
		}
	} else if m.treeClosed[entry.path] && m.filter == "" {
		m.toggleDirectory(m.treeCursor)
	} else if m.treeCursor+1 < len(m.treeRows) && m.treeRows[m.treeCursor+1].depth > entry.depth {
		m.moveTree(1)
	}
}

// Cap indentation so deeply nested files retain a usable name and checkbox.
func treeIndent(entry treeEntry, width int) int {
	return min(entry.depth*2, max(0, width-26))
}

func (m *Model) treeSidebar(g geometry) []string {
	width := g.sideWidth - 2
	result := make([]string, 0, g.bodyHeight)
	for i := m.sideOffset; i < len(m.treeRows) && len(result) < g.bodyHeight; i++ {
		entry := m.treeRows[i]
		active := (entry.file == m.selected && !m.treeDirectoryFocused()) || (m.fileFocus && i == m.treeCursor)
		cursor, bg := " ", ""
		if active {
			cursor, bg = m.ink(m.palette.accent, "›"), m.palette.selectionBackground
		}
		prefix := cursor + strings.Repeat(" ", treeIndent(entry, width))
		fold := "▾"
		var text string
		if entry.file < 0 {
			if m.treeClosed[entry.path] && m.filter == "" {
				fold = "▸"
			}
			text = prefix + m.ink(m.palette.accent, fold) + " " + m.bold(safeText(entry.name)+"/") + m.ink(m.palette.muted, fmt.Sprintf(" (%d)", entry.count))
		} else {
			file := m.comparison.Files[entry.file]
			if m.collapsed[entry.path] {
				fold = "▸"
			}
			box := "[ ]"
			if m.viewed[entry.path] {
				box = m.ink(m.palette.green, "[✓]")
			}
			name := safeText(entry.name)
			if active {
				name = m.bold(m.ink(m.palette.accent, name))
			}
			stats := m.stats(file.Added, file.Deleted)
			if file.Binary {
				stats = m.ink(m.palette.muted, "binary")
			}
			text = fit(prefix+m.ink(m.palette.accent, fold)+" "+box+" "+name, width-ansi.StringWidth(stats)-1) + " " + stats
		}
		result = append(result, m.surfaceWithBackground(m.palette.foreground, bg, text, width))
	}
	return m.frameSidebar(result, g)
}
