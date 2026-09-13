package backend

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type PullRequest struct {
	Host   string
	Owner  string
	Repo   string
	Number int
}

var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var hostname = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$`)

func PRNumber(value string) (int, bool) {
	value = strings.TrimPrefix(value, "#")
	if value == "" {
		return 0, false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(value)
	return n, err == nil && n > 0 && n <= 2147483647
}

// ParseGitHubURL accepts a PR URL, repository URL, or an origin SSH URL.
// Number is zero for repository URLs; those require a separate PR number.
func ParseGitHubURL(raw string) (PullRequest, error) {
	var target PullRequest
	if strings.HasPrefix(raw, "git@") && !strings.Contains(raw, "://") {
		host, path, ok := strings.Cut(strings.TrimPrefix(raw, "git@"), ":")
		if !ok {
			return target, fmt.Errorf("invalid origin URL")
		}
		raw = "ssh://git@" + host + "/" + path
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") || u.Port() != "" || !hostname.MatchString(u.Hostname()) {
		return target, fmt.Errorf("expected a GitHub PR URL or an HTTPS/SSH repository URL")
	}
	if u.User != nil && (u.Scheme != "ssh" || u.User.Username() != "git") {
		return target, fmt.Errorf("repository URLs must not contain credentials")
	}
	if _, password := u.User.Password(); password {
		return target, fmt.Errorf("repository URLs must not contain credentials")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return target, fmt.Errorf("repository URL must include owner/repository")
	}
	target.Host, target.Owner, target.Repo = strings.ToLower(u.Hostname()), parts[0], strings.TrimSuffix(parts[1], ".git")
	for _, part := range []string{target.Owner, target.Repo} {
		if !repositoryPart.MatchString(part) || part == "." || part == ".." {
			return PullRequest{}, fmt.Errorf("invalid repository owner or name")
		}
	}
	if len(parts) == 2 {
		return target, nil
	}
	if u.Scheme != "https" || len(parts) < 4 || parts[2] != "pull" {
		return PullRequest{}, fmt.Errorf("expected a GitHub URL ending in /pull/NUMBER")
	}
	if len(parts) > 5 || (len(parts) == 5 && parts[4] != "files" && parts[4] != "commits" && parts[4] != "checks") {
		return PullRequest{}, fmt.Errorf("unsupported pull request URL")
	}
	n, ok := PRNumber(parts[3])
	if !ok {
		return PullRequest{}, fmt.Errorf("invalid pull request number")
	}
	target.Number = n
	return target, nil
}

func (p PullRequest) URL() string {
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", p.Host, p.Owner, p.Repo, p.Number)
}

func (p PullRequest) endpoint() string {
	return fmt.Sprintf("repos/%s/%s/pulls/%d", p.Owner, p.Repo, p.Number)
}
