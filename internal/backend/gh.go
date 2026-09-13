package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxAPIOutput = 64 << 20

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxAPIOutput {
		return 0, fmt.Errorf("GitHub response exceeds 64 MiB")
	}
	return b.Buffer.Write(p)
}

type ghRunner func(context.Context, []byte, ...string) ([]byte, error)

func runGH(parent context.Context, input []byte, args ...string) ([]byte, error) {
	binary, err := exec.LookPath("gh")
	if err != nil {
		return nil, fmt.Errorf("GitHub review requires GitHub CLI (gh); install it from https://cli.github.com, then run gh auth login")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "NO_COLOR=1")
	// An invalid -C is never allowed to silently select an unrelated repository;
	// API requests contain explicit host/owner/repo and need no working directory.
	cmd.Stdin = bytes.NewReader(input)
	var out, errOut limitedOutput
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message := strings.TrimSpace(errOut.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("gh: %s", message)
	}
	return out.Bytes(), nil
}

func (g *GitHub) api(ctx context.Context, endpoint string, output any) error {
	data, err := g.run(ctx, nil, "api", "--hostname", g.Target.Host, "--method", "GET", endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func (g *GitHub) graphql(ctx context.Context, query string, variables map[string]any, output any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	data, err := g.run(ctx, body, "api", "--hostname", g.Target.Host, "graphql", "--input", "-")
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("GitHub: %s", envelope.Errors[0].Message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return io.ErrUnexpectedEOF
	}
	return json.Unmarshal(envelope.Data, output)
}
