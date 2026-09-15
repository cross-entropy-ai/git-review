package gitdiff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HasLocalRef probes commit-ish names without fetching or inspecting diffs.
// Only Git's quiet "not found" result permits fallback to a PR number.
func HasLocalRef(ctx context.Context, dir, ref string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0 {
		return false, nil
	}
	message := strings.TrimSpace(string(out))
	if message == "" {
		message = err.Error()
	}
	return false, fmt.Errorf("resolve local ref %q: %s (use a full GitHub PR URL outside a repository)", ref, message)
}

func OriginURL(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("cannot identify the GitHub repository from origin; use a full PR URL: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// LocalBranches returns full ref names in stable alphabetical order.
func LocalBranches(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, "for-each-ref", "--sort=refname", "--format=%(refname)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n"), nil
}
