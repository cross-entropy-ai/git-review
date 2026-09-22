package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cross-entropy-ai/git-review/internal/diff"
)

type editorFinishedMsg struct{ err error }

func (m *Model) openEditor() tea.Cmd {
	if len(m.visible) == 0 || m.treeDirectoryFocused() {
		m.message = "Select a file to open in the editor"
		return nil
	}
	if m.mode == diff.ModePullRequest && m.comparison.Root == "" {
		m.showAlert("Cannot open editor", "GitHub PR reviews have no local working tree.\nOpen a local checkout of this repository to edit files with e.")
		return nil
	}
	command, err := editorCommand(m.ctx, m.comparison.Root, m.comparison.Files[m.selected].Path)
	if err != nil {
		m.showAlert("Cannot open editor", err.Error())
		return nil
	}
	// Give terminal editors control of stdin and the screen, then restore the TUI.
	return tea.ExecProcess(command, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

func editorCommand(ctx context.Context, root, path string) (*exec.Cmd, error) {
	if root == "" {
		return nil, fmt.Errorf("this comparison has no local working tree")
	}
	if !filepath.IsLocal(path) {
		return nil, fmt.Errorf("invalid local file path %q", path)
	}
	filename, err := filepath.Abs(filepath.Join(root, path))
	if err != nil {
		return nil, err
	}
	return fileEditorCommand(ctx, root, filename)
}

// fileEditorCommand also opens exported reports outside a local working tree.
// Keep root as the working directory so repository editor settings still apply.
func fileEditorCommand(ctx context.Context, root, path string) (*exec.Cmd, error) {
	filename, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	// Let Git resolve GIT_EDITOR, core.editor, VISUAL, EDITOR, and its fallback.
	resolve := exec.CommandContext(ctx, "git", "var", "GIT_EDITOR")
	resolve.Dir = root
	output, err := resolve.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve default editor: %w", err)
	}
	editor := strings.TrimSuffix(string(output), "\n")
	if strings.TrimSpace(editor) == "" {
		return nil, fmt.Errorf("default editor is empty")
	}
	// Editor settings may include quoted arguments. Pass the file separately so
	// spaces and shell metacharacters in repository paths stay literal.
	command := exec.CommandContext(ctx, "sh", "-c", editor+` "$@"`, "git-review-editor", filename)
	command.Dir = root
	return command, nil
}
