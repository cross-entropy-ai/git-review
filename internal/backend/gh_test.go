package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGHGraphQLTransportPreservesLiteralPaths(t *testing.T) {
	bin := t.TempDir()
	// Echo the request inside a GraphQL envelope so the real subprocess and
	// stdin transport are exercised without a network request or credentials.
	script := `#!/bin/sh
[ "$1" = api ] && [ "$2" = --hostname ] && [ "$3" = github.example.com ] &&
[ "$4" = graphql ] && [ "$5" = --input ] && [ "$6" = - ] || exit 9
printf '{"data":'
/bin/cat
printf '}'
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	g := NewGitHub(PullRequest{Host: "github.example.com"}, true)
	path := "目录/quote\"\\line\n$(literal)`name`.go"
	vars := map[string]any{"input": map[string]any{"path": path, "pullRequestId": "pr-id"}}
	var got struct {
		Query     string
		Variables map[string]any
	}
	query := "mutation($input:MarkFileAsViewedInput!){markFileAsViewed(input:$input){pullRequest{id}}}"
	if err := g.graphql(context.Background(), query, vars, &got); err != nil {
		t.Fatal(err)
	}
	if got.Query != query || !reflect.DeepEqual(got.Variables, vars) {
		data, _ := json.Marshal(got)
		t.Fatalf("request changed in transit: %s", data)
	}
}
