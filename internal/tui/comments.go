package tui

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/review"
)

type commentState struct {
	items []review.Comment
	path  string
	err   error
}

type lineRef struct {
	file, hunk, line int
	side             string
}

// Local drafts are saved before a GitHub write. A failed operation leaves the
// editor and its draft intact, and cannot be mistaken for a successful sync.
func (m *Model) installComments() {
	m.lineSelecting, m.rangeSelecting = false, false
	if m.commentStates == nil {
		m.commentStates = make(map[string]*commentState)
	}
	key := m.snapshot.Key
	if m.syncComments() && m.snapshot.CommentKey != "" {
		key = m.snapshot.CommentKey
	}
	if state, ok := m.commentStates[key]; ok {
		m.notes = state
		m.mergeGitHubComments()
		return
	}
	m.notes = &commentState{}
	m.commentStates[key] = m.notes
	if m.snapshot.Persistence == backend.Memory {
		return
	}
	m.notes.path, m.notes.err = review.CommentsPath(m.comparison, m.snapshot.Key)
	legacy := m.notes.path
	if m.notes.err == nil && key != m.snapshot.Key {
		m.notes.path, m.notes.err = review.RemoteCommentsPath(m.comparison, key)
	}
	if m.notes.err == nil {
		m.notes.items, m.notes.err = review.LoadComments(m.notes.path)
		if _, err := os.Stat(m.notes.path); os.IsNotExist(err) && legacy != m.notes.path {
			m.notes.items, m.notes.err = review.LoadComments(legacy)
			for i := range m.notes.items {
				if m.notes.items[i].Revision == "" {
					m.notes.items[i].Revision = m.snapshot.Revision
				}
			}
			if m.notes.err == nil && len(m.notes.items) > 0 {
				m.notes.err = review.SaveComments(m.notes.path, m.notes.items)
			}
		}
	}
	if m.notes.err != nil {
		m.showAlert("Cannot restore comments", m.notes.err.Error())
	}
	m.mergeGitHubComments()
}

func (m *Model) persistComments(items []review.Comment) bool {
	if m.notes.err != nil {
		m.showAlert("Cannot save comments", "Saved comments could not be loaded. Resolve the storage error and restart before saving.\n"+m.notes.err.Error())
		return false
	}
	if m.notes.path != "" {
		if err := review.SaveComments(m.notes.path, items); err != nil {
			m.showAlert("Cannot save comments", err.Error())
			return false
		}
	}
	m.notes.items = items
	return true
}

func (m *Model) lineAt(index int, side string) (lineRef, bool) {
	if index < 0 || index >= len(m.rows) {
		return lineRef{}, false
	}
	r := m.rows[index]
	if r.kind != 'l' && r.kind != 'd' {
		return lineRef{}, false
	}
	li := r.line
	if r.kind == 'd' {
		if side == "" {
			side = "new"
			if r.rightLine < 0 {
				side = "old"
			}
		}
		if side == "new" {
			li = r.rightLine
		}
	}
	if li < 0 {
		return lineRef{}, false
	}
	l := m.comparison.Files[r.file].Hunks[r.hunk].Lines[li]
	if l.Kind != '+' && l.Kind != '-' && l.Kind != ' ' {
		return lineRef{}, false
	}
	if side == "" {
		side = "new"
		if l.Kind == '-' {
			side = "old"
		}
	}
	if (side == "old" && l.Old < 1) || (side == "new" && l.New < 1) {
		return lineRef{}, false
	}
	return lineRef{r.file, r.hunk, li, side}, true
}

func (m *Model) lineNumber(ref lineRef) int {
	l := m.comparison.Files[ref.file].Hunks[ref.hunk].Lines[ref.line]
	if ref.side == "old" {
		return l.Old
	}
	return l.New
}

func (m *Model) beginComment() {
	if m.loading {
		m.message = "Wait for the comparison to finish loading"
		return
	}
	if m.lineSelecting {
		m.editSelectedLines()
		return
	}
	if len(m.visible) == 0 || m.treeDirectoryFocused() {
		m.message = "Select a file with text changes to comment"
		return
	}
	path := m.comparison.Files[m.selected].Path
	if m.collapsed[path] {
		m.collapsed[path] = false
		m.rebuild()
		m.jumpSelected()
	}
	// Start at the active search match when one exists, otherwise the first
	// visible source line in the selected file.
	if m.search != "" && m.matchIndex >= 0 && m.matchIndex < len(m.matches) && m.matches[m.matchIndex].file == m.selected {
		match := m.matches[m.matchIndex]
		for i, r := range m.rows {
			if r.file == match.file && r.hunk == match.hunk && (r.kind == 'l' || r.kind == 'd') && (r.line == match.line || r.kind == 'd' && r.rightLine == match.line) {
				side := ""
				if r.kind == 'd' {
					side = "old"
					if r.rightLine == match.line {
						side = "new"
					}
				}
				if ref, ok := m.lineAt(i, side); ok {
					m.selectCommentLine(i, ref)
					return
				}
			}
		}
	}
	for _, start := range []int{m.offset, 0} {
		for i := start; i < len(m.rows); i++ {
			if ref, ok := m.lineAt(i, ""); ok && ref.file == m.selected {
				m.selectCommentLine(i, ref)
				return
			}
		}
	}
	m.showAlert("Cannot comment here", "This file has no available text lines. Binary files and omitted patches cannot have line comments.")
}

func (m *Model) selectCommentLine(index int, ref lineRef) {
	m.lineSelecting, m.fileFocus, m.searching = true, false, false
	m.lineCursor, m.commentLine, m.selected = index, ref, ref.file
	m.revealCommentLine()
}

func (m *Model) revealCommentLine() {
	if m.lineCursor < m.offset {
		m.offset = m.lineCursor
	}
	if m.lineCursor >= m.offset+m.bodyHeight() {
		m.offset = m.lineCursor - m.bodyHeight() + 1
	}
	m.clampOffset()
	m.ensureSelectedVisible()
}

func (m *Model) moveCommentLine(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	for count := 0; count < max(delta, -delta); count++ {
		for i := m.lineCursor + step; i >= 0 && i < len(m.rows); i += step {
			side := ""
			if m.splitMode || m.rangeSelecting {
				side = m.commentLine.side
			}
			ref, ok := m.lineAt(i, side)
			// An unmatched split cell must not make an added/deleted block
			// unreachable from the keyboard. Ranges keep their original side.
			if !ok && m.splitMode && !m.rangeSelecting {
				ref, ok = m.lineAt(i, "")
			}
			if !ok {
				continue
			}
			if m.rangeSelecting && (ref.file != m.rangeAnchor.file || ref.hunk != m.rangeAnchor.hunk) {
				break
			}
			m.selectCommentLine(i, ref)
			break
		}
	}
}

func (m *Model) selectionKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "esc", "q":
		m.lineSelecting, m.rangeSelecting = false, false
	case "j", "down":
		m.rangeSelecting = false
		m.moveCommentLine(1)
	case "k", "up":
		m.rangeSelecting = false
		m.moveCommentLine(-1)
	case "shift+down", "shift+up":
		if !m.rangeSelecting {
			m.rangeAnchor = m.commentLine
		}
		m.rangeSelecting = true
		delta := 1
		if msg.String() == "shift+up" {
			delta = -1
		}
		m.moveCommentLine(delta)
	case "pgdown", "ctrl+d":
		m.rangeSelecting = false
		m.moveCommentLine(max(1, m.bodyHeight()/2))
	case "pgup", "ctrl+u":
		m.rangeSelecting = false
		m.moveCommentLine(-max(1, m.bodyHeight()/2))
	case "tab", "shift+tab":
		if m.rangeSelecting {
			return
		}
		side := "new"
		if m.commentLine.side == "new" {
			side = "old"
		}
		if ref, ok := m.lineAt(m.lineCursor, side); ok {
			m.commentLine = ref
		}
	case "c", "enter":
		m.editSelectedLines()
	case "C":
		m.openComments()
	case "x":
		m.openExport()
	case "h", "left":
		m.xOffset = max(0, m.xOffset-8)
	case "l", "right":
		m.xOffset += 8
	}
}

func (m *Model) selectedComment() review.Comment {
	ref := m.commentLine
	start, end := m.lineNumber(ref), m.lineNumber(ref)
	if m.rangeSelecting {
		start = min(start, m.lineNumber(m.rangeAnchor))
		end = max(end, m.lineNumber(m.rangeAnchor))
	}
	f := m.comparison.Files[ref.file]
	var code []string
	for _, l := range f.Hunks[ref.hunk].Lines {
		n := l.New
		if ref.side == "old" {
			n = l.Old
		}
		if n >= start && n <= end && l.Kind != '\\' {
			code = append(code, l.Text)
		}
	}
	return review.Comment{Path: f.Path, OldPath: f.OldPath, Side: ref.side, Start: start, End: end, Code: strings.Join(code, "\n")}
}

func commentLocation(c review.Comment) string {
	label := fmt.Sprintf("%s · %s L%d", safeText(c.Path), c.Side, c.Start)
	if c.StartSide != "" && c.StartSide != c.Side {
		label = fmt.Sprintf("%s · %s L%d → %s L%d", safeText(c.Path), c.StartSide, c.Start, c.Side, c.End)
	} else if c.End != c.Start {
		label += fmt.Sprintf("–L%d", c.End)
	}
	if c.Start == 0 {
		label = safeText(c.Path) + " · file comment"
	}
	if c.Outdated {
		label += " · outdated"
	}
	return label
}

func (m *Model) editSelectedLines() {
	m.commentDraft = m.selectedComment()
	// Revisit the same range to edit its existing note.
	for _, c := range m.notes.items {
		if c.RemoteID == 0 && c.ReplyTo == 0 && c.Path == m.commentDraft.Path && c.Side == m.commentDraft.Side && c.Start == m.commentDraft.Start && c.End == m.commentDraft.End {
			if c.Outdated && c.PendingBody == "" {
				c.Code, c.OldPath, c.Outdated = m.commentDraft.Code, m.commentDraft.OldPath, false
				c.Revision, c.CommitID = m.snapshot.Revision, m.comparison.HeadOID
			}
			m.commentDraft = c
			break
		}
	}
	m.commentReturn = ""
	m.startCommentEditor()
}

func (m *Model) startCommentEditor() {
	m.commentModal, m.dragging = "edit", ""
	m.noteInput = []rune(m.commentDraft.Body)
	if m.commentDraft.DraftBody != "" {
		m.noteInput = []rune(m.commentDraft.DraftBody)
	}
	m.noteCursor = len(m.noteInput)
}

func (m *Model) saveComment() tea.Cmd {
	body := strings.TrimSpace(string(m.noteInput))
	if body == "" {
		m.showAlert("Comment is empty", "Write a comment before saving. Use Esc to cancel.")
		return nil
	}
	c := m.commentDraft
	c.Body = body
	outgoing := c
	if m.syncComments() && c.RemoteID == 0 {
		if c.Revision == "" {
			c.Revision = m.snapshot.Revision
			outgoing.Revision = c.Revision
			c.CommitID = m.comparison.HeadOID
			outgoing.CommitID = c.CommitID
		}
		if c.PendingBody == "" {
			c.PendingBody = body
			for _, existing := range m.notes.items {
				c.PendingAfterID = max(c.PendingAfterID, existing.RemoteID)
			}
		}
	}
	items := append([]review.Comment(nil), m.notes.items...)
	found := false
	for i := range items {
		if items[i].ID == c.ID && c.ID != "" {
			items[i], found = c, true
			if m.syncComments() && c.RemoteID > 0 {
				items[i] = m.notes.items[i]
				items[i].DraftBody = c.Body
			}
			break
		}
	}
	if !found {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			m.showAlert("Cannot save comments", err.Error())
			return nil
		}
		c.ID = fmt.Sprintf("%x", id)
		items = append(items, c)
	}
	if m.persistComments(items) {
		if m.syncComments() {
			m.commentDraft = c
			outgoing.ID = c.ID
			return m.sendComment(outgoing, false)
		}
		m.commentModal, m.lineSelecting, m.rangeSelecting = m.commentReturn, false, false
		m.message = "Comment saved · C Comments · x Export Markdown"
	}
	return nil
}

func (m *Model) openComments() {
	if m.loading {
		m.message = "Wait for the comparison to finish loading"
		return
	}
	if m.commentModal == "" {
		m.commentAllFiles = false
	}
	m.commentModal, m.dragging = "list", ""
	m.clampCommentSelection()
}

// Keep commentIndex in the backing list so edits never target another file
// when the visible list is filtered.
func (m *Model) visibleComments() []int {
	path := ""
	if m.selected >= 0 && m.selected < len(m.comparison.Files) {
		path = m.comparison.Files[m.selected].Path
	}
	var indices []int
	for i, c := range m.notes.items {
		if m.commentAllFiles || c.Path == path {
			indices = append(indices, i)
		}
	}
	return m.orderComments(indices)
}

func (m *Model) commentPosition() int {
	for pos, index := range m.visibleComments() {
		if index == m.commentIndex {
			return pos
		}
	}
	return -1
}

func (m *Model) clampCommentSelection() {
	if indices := m.visibleComments(); len(indices) > 0 && m.commentPosition() < 0 {
		m.commentIndex = indices[0]
	}
}

func (m *Model) moveCommentSelection(delta int) {
	indices := m.visibleComments()
	if len(indices) > 0 {
		m.commentIndex = indices[min(max(0, m.commentPosition()+delta), len(indices)-1)]
	}
}

func (m *Model) openExport() {
	if m.loading {
		m.message = "Wait for the comparison to finish loading"
		return
	}
	if len(m.notes.items) == 0 {
		m.showAlert("No comments to export", "Add a line comment with c first.")
		return
	}
	m.commentReturn = ""
	if m.commentModal == "list" {
		m.commentReturn = "list"
	}
	m.commentModal, m.dragging = "export", ""
	m.noteInput = []rune("review-comments-" + time.Now().Format("20060102-150405") + ".md")
	m.noteCursor = len(m.noteInput)
}

func (m *Model) exportComments() {
	path := strings.TrimSpace(string(m.noteInput))
	if path == "" {
		m.showAlert("Cannot export comments", "Enter a Markdown file path.")
		return
	}
	if !filepath.IsAbs(path) && m.comparison.Root != "" {
		path = filepath.Join(m.comparison.Root, path)
	}
	path, err := filepath.Abs(path)
	if err == nil {
		err = review.Export(path, review.Markdown(m.snapshot.Label, m.snapshot.URL, m.comparison, m.notes.items))
	}
	if err != nil {
		m.showAlert("Cannot export comments", err.Error()+"\nChoose a new filename or an existing parent directory.")
		return
	}
	m.commentModal = m.commentReturn
	m.message = "Exported Markdown: " + safeText(path)
}

func (m *Model) commentKey(msg tea.KeyMsg) tea.Cmd {
	if m.commentSaving {
		return nil
	}
	key := msg.String()
	if msg.Paste {
		if m.commentModal == "edit" || m.commentModal == "export" {
			m.editNoteInput(msg, m.commentModal == "edit")
		}
		return nil
	}
	switch m.commentModal {
	case "edit", "export":
		if key == "esc" {
			m.commentModal = m.commentReturn
			return nil
		}
		if m.commentModal == "edit" && key == "ctrl+s" {
			return m.saveComment()
		}
		if m.commentModal == "export" && key == "enter" {
			m.exportComments()
			return nil
		}
		m.editNoteInput(msg, m.commentModal == "edit")
	case "delete":
		switch key {
		case "esc":
			m.commentModal = "list"
		case "enter":
			if m.commentPosition() < 0 {
				m.openComments()
				return nil
			}
			if m.syncComments() && m.notes.items[m.commentIndex].RemoteID > 0 {
				return m.sendComment(m.notes.items[m.commentIndex], true)
			}
			items := append([]review.Comment(nil), m.notes.items...)
			items = append(items[:m.commentIndex], items[m.commentIndex+1:]...)
			if m.persistComments(items) {
				m.openComments()
				m.message = "Comment deleted"
			}
		}
	case "list":
		switch key {
		case "esc", "q", "C":
			m.commentModal = ""
		case "j", "down":
			m.moveCommentSelection(1)
		case "k", "up":
			m.moveCommentSelection(-1)
		case "tab", "shift+tab":
			m.commentAllFiles = !m.commentAllFiles
			m.clampCommentSelection()
		case "x":
			m.openExport()
		case "a":
			m.replyComment()
		case "r":
			return m.toggleCommentResolved()
		case "enter", "e":
			if m.commentPosition() >= 0 {
				m.commentDraft = m.notes.items[m.commentIndex]
				m.commentReturn = "list"
				m.startCommentEditor()
			}
		case "d":
			if m.commentPosition() >= 0 {
				m.commentModal = "delete"
			}
		case "g":
			if m.commentPosition() >= 0 {
				m.jumpComment(m.notes.items[m.commentIndex])
			}
		}
	}
	return nil
}

func (m *Model) jumpComment(c review.Comment) {
	if c.Outdated {
		m.showAlert("Outdated comment", "This comment belongs to an older diff. You can read, reply or export it from the comments list.")
		return
	}
	for fi, f := range m.comparison.Files {
		if f.Path != c.Path {
			continue
		}
		m.collapsed[f.Path] = false
		m.rebuild()
		if c.Start == 0 {
			m.commentModal, m.lineSelecting, m.rangeSelecting = "", false, false
			m.selected = fi
			m.jumpSelected()
			return
		}
		side := c.Side
		if c.StartSide != "" {
			side = c.StartSide
		}
		for i := range m.rows {
			if ref, ok := m.lineAt(i, side); ok && ref.file == fi && m.lineNumber(ref) == c.Start {
				m.commentModal, m.rangeSelecting = "", false
				m.selectCommentLine(i, ref)
				return
			}
		}
	}
	m.showAlert("Line is not in this diff", "The saved comment is still available to edit and export. Its line is outside the currently loaded diff context.")
}

// A rune cursor supports Chinese input, paste, multiline notes and ordinary
// editing without interpreting note text as review shortcuts.
func (m *Model) editNoteInput(msg tea.KeyMsg, multiline bool) {
	key := msg.String()
	if !msg.Paste {
		switch key {
		case "left":
			m.noteCursor = max(0, m.noteCursor-1)
			return
		case "right":
			m.noteCursor = min(len(m.noteInput), m.noteCursor+1)
			return
		case "home", "ctrl+a":
			for m.noteCursor > 0 && m.noteInput[m.noteCursor-1] != '\n' {
				m.noteCursor--
			}
			return
		case "end", "ctrl+e":
			for m.noteCursor < len(m.noteInput) && m.noteInput[m.noteCursor] != '\n' {
				m.noteCursor++
			}
			return
		case "up", "down":
			start := m.noteCursor
			for start > 0 && m.noteInput[start-1] != '\n' {
				start--
			}
			column := m.noteCursor - start
			if key == "up" {
				if start == 0 {
					return
				}
				end := start - 1
				start = end
				for start > 0 && m.noteInput[start-1] != '\n' {
					start--
				}
				m.noteCursor = min(start+column, end)
			} else {
				end := m.noteCursor
				for end < len(m.noteInput) && m.noteInput[end] != '\n' {
					end++
				}
				if end == len(m.noteInput) {
					return
				}
				start = end + 1
				end = start
				for end < len(m.noteInput) && m.noteInput[end] != '\n' {
					end++
				}
				m.noteCursor = min(start+column, end)
			}
			return
		case "ctrl+u":
			m.noteInput, m.noteCursor = nil, 0
			return
		case "backspace", "ctrl+h":
			if m.noteCursor > 0 {
				m.noteInput = append(m.noteInput[:m.noteCursor-1], m.noteInput[m.noteCursor:]...)
				m.noteCursor--
			}
			return
		case "delete":
			if m.noteCursor < len(m.noteInput) {
				m.noteInput = append(m.noteInput[:m.noteCursor], m.noteInput[m.noteCursor+1:]...)
			}
			return
		}
	}
	text := ""
	if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
		text = string(msg.Runes)
	}
	if !msg.Paste && multiline && key == "enter" {
		text = "\n"
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var inserted []rune
	for _, r := range text {
		if r == '\n' && multiline {
			inserted = append(inserted, r)
		} else if r == '\t' {
			inserted = append(inserted, []rune("    ")...)
		} else if !unicode.IsControl(r) && !unicode.In(r, unicode.Cf) {
			inserted = append(inserted, r)
		}
	}
	if len(m.noteInput)+len(inserted) > 65536 {
		m.showAlert("Comment input is too long", "Keep input within 65,536 characters.")
		return
	}
	updated := append([]rune(nil), m.noteInput[:m.noteCursor]...)
	updated = append(updated, inserted...)
	m.noteInput = append(updated, m.noteInput[m.noteCursor:]...)
	m.noteCursor += len(inserted)
}
