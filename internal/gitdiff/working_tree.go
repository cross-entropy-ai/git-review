package gitdiff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Check both the index and worktree: staged edits can be reversed on disk,
// leaving an empty HEAD-to-worktree diff while there are still local changes.
func hasLocalChanges(ctx context.Context, root string) (bool, error) {
	overrides, err := snapshotOverrides(ctx, root, os.DevNull)
	if err != nil {
		return false, err
	}
	status, err := run(ctx, root, append(overrides, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--ignore-submodules=none")...)
	return status != "", err
}

func loadWorkingTree(ctx context.Context, c *Comparison, contextLines int, summary bool) (*Comparison, error) {
	head, err := run(ctx, c.Root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, fmt.Errorf("working-tree review needs an existing HEAD commit: %w", err)
	}
	c.WorkingTree = true
	c.Base, c.Head = "HEAD", "working tree"
	c.BaseOID = strings.TrimSpace(head)
	c.MergeBase = c.BaseOID

	// Keep both the index and newly written objects outside the repository.
	// Comparing the resulting tree makes raw records, statistics, patches, and
	// review progress refer to the same captured content even if files change.
	tempDir, err := os.MkdirTemp("", "git-review-worktree-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	objects := filepath.Join(tempDir, "objects")
	if err := os.Mkdir(objects, 0o700); err != nil {
		return nil, err
	}
	originalObjects, err := run(ctx, c.Root, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return nil, err
	}
	alternates := strconv.Quote(strings.TrimSuffix(originalObjects, "\n"))
	if inherited := os.Getenv("GIT_ALTERNATE_OBJECT_DIRECTORIES"); inherited != "" {
		alternates += string(os.PathListSeparator) + inherited
	}
	env := []string{
		"GIT_INDEX_FILE=" + filepath.Join(tempDir, "index"),
		"GIT_OBJECT_DIRECTORY=" + objects,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=" + alternates,
	}
	overrides, err := snapshotOverrides(ctx, c.Root, tempDir)
	if err != nil {
		return nil, err
	}
	snapshot := func(input string, args ...string) (string, error) {
		return runWithEnv(ctx, c.Root, env, strings.NewReader(input), append(append([]string{}, overrides...), args...)...)
	}

	// Reconstruct the index from entries rather than copying its file, which
	// may refer to shared indexes or sparse extensions in the real Git directory.
	entries, err := run(ctx, c.Root, "-c", "core.fsmonitor=false", "ls-files", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	if _, err := snapshot(entries, "update-index", "-z", "--index-info"); err != nil {
		return nil, fmt.Errorf("prepare working-tree snapshot: %w", err)
	}
	flags, err := run(ctx, c.Root, "-c", "core.fsmonitor=false", "ls-files", "-t", "-z")
	if err != nil {
		return nil, err
	}
	var skipped strings.Builder
	for _, entry := range strings.Split(flags, "\x00") {
		if strings.HasPrefix(entry, "S ") {
			skipped.WriteString(entry[2:])
			skipped.WriteByte(0)
		}
	}
	if skipped.Len() > 0 {
		if _, err := snapshot(skipped.String(), "update-index", "--skip-worktree", "-z", "--stdin"); err != nil {
			return nil, fmt.Errorf("preserve sparse checkout entries: %w", err)
		}
	}
	// Git applies the repository's ignore rules, retains already tracked ignored
	// files, and captures the final on-disk version of partially staged files.
	if _, err := snapshot("", "add", "--all", "--", "."); err != nil {
		return nil, fmt.Errorf("capture working-tree changes: %w", err)
	}
	tree, err := snapshot("", "write-tree")
	if err != nil {
		return nil, fmt.Errorf("write working-tree snapshot: %w", err)
	}
	c.HeadOID = strings.TrimSpace(tree)
	return readDiff(c, contextLines, summary, func(args ...string) (string, error) {
		return snapshot("", args...)
	})
}

func snapshotOverrides(ctx context.Context, root, tempDir string) ([]string, error) {
	args := []string{
		"-c", "core.hooksPath=" + tempDir,
		"-c", "core.fsmonitor=false",
		"-c", "core.splitIndex=false",
		"-c", "core.untrackedCache=false",
		"-c", "index.sparse=false",
		"-c", "gc.auto=0",
		"-c", "maintenance.auto=false",
	}
	keys, err := run(ctx, root, "config", "--null", "--name-only", "--list")
	if err != nil {
		return nil, err
	}
	// A review must not execute clean/process filters while snapshotting files.
	// Text normalization still follows Git attributes and core.autocrlf settings.
	seen := make(map[string]bool)
	for _, key := range strings.Split(keys, "\x00") {
		if !strings.HasPrefix(key, "filter.") {
			continue
		}
		last := strings.LastIndexByte(key, '.')
		if last <= len("filter.") {
			continue
		}
		name := key[:last]
		if !seen[name] {
			args = append(args, "-c", name+".clean=", "-c", name+".process=", "-c", name+".required=false")
			seen[name] = true
		}
	}
	return args, nil
}
