package tui

import (
	"sort"
	"strings"

	"github.com/cross-entropy-ai/git-review/internal/diff"
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

// fileOrder defines the shared order for the sidebar, diff, and navigation.
// List mode preserves backend order. Tree mode visits directories before files,
// sorting siblings by name. The tree includes closed directories' descendants;
// sidebar folding is applied separately and never changes the file order.
func fileOrder(files []diff.File, tree bool) (order []int, entries []treeEntry) {
	if !tree {
		for i := range files {
			order = append(order, i)
		}
		return order, nil
	}
	root := &treeNode{children: make(map[string]*treeNode)}
	for index, file := range files {
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
			entries = append(entries, child.entry)
			if child.entry.file >= 0 {
				order = append(order, child.entry.file)
			} else {
				flatten(child)
			}
		}
	}
	flatten(root)
	return order, entries
}
