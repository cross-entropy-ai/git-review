package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cross-entropy-ai/git-review/internal/backend"
	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func remoteTarget(parent context.Context, args []string, dir string, numericFallback bool) (*backend.PullRequest, error) {
	if len(args) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	arg := args[0]
	if strings.Contains(arg, "://") || strings.HasPrefix(arg, "git@") {
		target, err := backend.ParseGitHubURL(arg)
		if err != nil {
			return nil, err
		}
		if target.Number == 0 && len(args) == 2 {
			number, ok := backend.PRNumber(args[1])
			if !ok {
				return nil, fmt.Errorf("repository URL must be followed by a PR number")
			}
			target.Number = number
		} else if len(args) != 1 {
			return nil, fmt.Errorf("a PR URL cannot be combined with other refs")
		}
		if target.Number == 0 {
			return nil, fmt.Errorf("specify a full /pull/NUMBER URL or a repository URL followed by a PR number")
		}
		return &target, nil
	}
	explicitPR := strings.HasPrefix(arg, "#")
	number, numeric := backend.PRNumber(arg)
	if explicitPR && (!numeric || len(args) != 1) {
		return nil, fmt.Errorf("expected a single PR number such as '#918'")
	}
	if !explicitPR && (!numeric || !numericFallback || len(args) != 1) {
		return nil, nil
	}
	if !explicitPR {
		exists, err := gitdiff.HasLocalRef(ctx, dir, arg)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, nil
		}
	}
	origin, err := gitdiff.OriginURL(ctx, dir)
	if err != nil {
		return nil, err
	}
	target, err := backend.ParseGitHubURL(origin)
	if err != nil {
		return nil, fmt.Errorf("origin is not a supported GitHub repository URL: %w", err)
	}
	if target.Number != 0 {
		return nil, fmt.Errorf("origin must identify a repository, not a PR")
	}
	target.Number = number
	return &target, nil
}
