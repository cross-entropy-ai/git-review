package gitdiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const maxOutput = 64 << 20

type boundedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxOutput {
		b.overflow = true
		return 0, fmt.Errorf("Git output exceeds 64 MiB; narrow the comparison")
	}
	return b.Buffer.Write(p)
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	return runWithEnv(ctx, dir, nil, nil, args...)
}

func runWithEnv(ctx context.Context, dir string, env []string, input io.Reader, args ...string) (string, error) {
	prefix := []string{"--no-pager", "--literal-pathspecs", "-c", "color.ui=false", "-c", "core.quotePath=false", "-c", "diff.suppressBlankEmpty=false"}
	cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = input
	var stdout boundedBuffer
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if stdout.overflow {
			return "", errors.New("Git output exceeds 64 MiB; narrow the comparison")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return stdout.String(), nil
}

// Load selects a review scope, then compares commits or snapshots the working tree.
func Load(parent context.Context, opts Options) (*Comparison, error) {
	return load(parent, opts, false)
}

// Summary resolves the same comparison as Load without reading patches.
func Summary(parent context.Context, opts Options) (*Comparison, error) {
	return load(parent, opts, true)
}

func load(parent context.Context, opts Options, summary bool) (*Comparison, error) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	if opts.Context < 0 || opts.Context > 100 {
		return nil, errors.New("context must be between 0 and 100")
	}
	if opts.Mode != ModeAuto && opts.Mode != ModeWorkingTree && opts.Mode != ModeCommitted {
		return nil, fmt.Errorf("unknown review mode %q", opts.Mode)
	}
	if opts.Mode == ModeWorkingTree && (opts.Base != "" || (opts.Head != "" && opts.Head != "HEAD")) {
		return nil, errors.New("working-tree review compares against HEAD; base and head refs are not supported")
	}
	root, err := run(ctx, opts.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	root = strings.TrimSuffix(root, "\n")
	gitDir, err := run(ctx, root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	c := &Comparison{Root: root, GitDir: strings.TrimSuffix(gitDir, "\n"), Base: opts.Base, Head: opts.Head}
	workingTree := opts.Mode == ModeWorkingTree
	if opts.Mode == ModeAuto {
		workingTree, err = hasLocalChanges(ctx, root)
		if err != nil {
			return nil, fmt.Errorf("check local changes: %w", err)
		}
	}
	if workingTree {
		return loadWorkingTree(ctx, c, opts.Context, summary)
	}
	if c.Head == "" {
		c.Head = "HEAD"
	}
	resolve := func(ref string) (string, error) {
		out, err := run(ctx, root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
		return strings.TrimSpace(out), err
	}
	if c.Base == "" {
		for _, candidate := range []string{"main", "origin/main", "master", "origin/master"} {
			if oid, resolveErr := resolve(candidate); resolveErr == nil {
				c.Base, c.BaseOID = candidate, oid
				break
			}
		}
		if c.Base == "" {
			return nil, errors.New("no default base found (main, origin/main, master, origin/master); use --base <ref>")
		}
	} else {
		c.BaseOID, err = resolve(c.Base)
		if err != nil {
			return nil, fmt.Errorf("resolve base %q: %w", c.Base, err)
		}
	}
	c.HeadOID, err = resolve(c.Head)
	if err != nil {
		return nil, fmt.Errorf("resolve head %q (commit your changes first): %w", c.Head, err)
	}
	mergeBase, err := run(ctx, root, "merge-base", c.BaseOID, c.HeadOID)
	if err != nil {
		return nil, fmt.Errorf("find common ancestor of %s and %s: %w", c.Base, c.Head, err)
	}
	c.MergeBase = strings.TrimSpace(mergeBase)
	if c.Head == "HEAD" {
		branch, branchErr := run(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
		if branchErr == nil {
			c.Head = strings.TrimSpace(branch)
		}
	}
	return readDiff(c, opts.Context, summary, func(args ...string) (string, error) {
		return run(ctx, root, args...)
	})
}

func readDiff(c *Comparison, contextLines int, summary bool, git func(...string) (string, error)) (*Comparison, error) {
	// Every command uses resolved OIDs and identical ordering/rename settings.
	// External diff and textconv helpers are intentionally disabled.
	diffArgs := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--ignore-submodules=none", "--submodule=short", "--diff-algorithm=histogram", "--no-relative", "--src-prefix=a/", "--dst-prefix=b/", "--output-indicator-new=+", "--output-indicator-old=-", "--output-indicator-context= "}
	diff := func(format ...string) (string, error) {
		args := append(append([]string{}, diffArgs...), format...)
		args = append(args, c.MergeBase, c.HeadOID, "--")
		return git(args...)
	}
	raw, err := diff("--raw", "-z", "--no-abbrev")
	if err != nil {
		return nil, fmt.Errorf("read changed files: %w", err)
	}
	c.Files, err = parseRaw(raw)
	if err != nil {
		return nil, err
	}
	if len(c.Files) == 0 {
		return c, nil
	}
	stats, err := diff("--numstat", "-z")
	if err != nil {
		return nil, fmt.Errorf("read statistics: %w", err)
	}
	if err := applyNumstat(c.Files, stats); err != nil {
		return nil, err
	}
	if !summary {
		patch, err := diff("--patch", "--unified="+strconv.Itoa(contextLines))
		if err != nil {
			return nil, fmt.Errorf("read patch: %w", err)
		}
		if err := applyPatch(c.Files, patch); err != nil {
			return nil, err
		}
	}
	for _, f := range c.Files {
		c.Added += f.Added
		c.Deleted += f.Deleted
	}
	return c, nil
}
