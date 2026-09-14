package tui

import "sort"

// Sort only display indices; persistence and mutation targets keep their identity.
func (m *Model) orderComments(indices []int) []int {
	type threadKey struct {
		path  string
		root  int64
		local int
	}
	var groups [][]int
	positions := make(map[threadKey]int)
	for _, index := range indices {
		c := m.notes.items[index]
		key := threadKey{path: c.Path, root: c.RemoteID}
		if c.ReplyTo > 0 {
			key.root = c.ReplyTo
		}
		if key.root == 0 {
			key.local = index + 1
		}
		pos, ok := positions[key]
		if !ok {
			pos = len(groups)
			positions[key] = pos
			groups = append(groups, nil)
		}
		groups[pos] = append(groups[pos], index)
	}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			a, b := m.notes.items[group[i]], m.notes.items[group[j]]
			if (a.ReplyTo == 0) != (b.ReplyTo == 0) {
				return a.ReplyTo == 0
			}
			if (a.RemoteID == 0) != (b.RemoteID == 0) {
				return b.RemoteID == 0 // Unsynced replies follow published replies.
			}
			return a.RemoteID < b.RemoteID
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := m.notes.items[groups[i][0]], m.notes.items[groups[j][0]]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.Side != b.Side {
			return a.Side == "old"
		}
		if a.End != b.End {
			return a.End < b.End
		}
		return a.RemoteID < b.RemoteID
	})
	ordered := make([]int, 0, len(indices))
	for _, group := range groups {
		ordered = append(ordered, group...)
	}
	return ordered
}
