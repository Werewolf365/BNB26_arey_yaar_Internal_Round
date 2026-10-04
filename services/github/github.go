// Package github resolves public GitHub repositories to pinned commits for
// onboarding. It talks to the GitHub REST API over plain HTTPS (no shell,
// no git subprocess, no repository code execution): repository metadata,
// tag lists (with annotated-tag peeling), and root file listings for
// ecosystem detection. Public repositories only; no auth, no OAuth.
//
// Everything is bounded: 10s timeouts, 1MB bodies, redirects confined to
// api.github.com. Errors are typed (INVALID_INPUT / REPO_NOT_FOUND /
// TAG_NOT_FOUND / GITHUB_UNAVAILABLE) and carry no internals.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultBase is the public GitHub REST API.
const DefaultBase = "https://api.github.com"

// maxBody caps any single API response (tags/files listings are small).
const maxBody = 1 << 20

// Client queries the GitHub REST API. BaseURL is overrideable for tests;
// HTTP, when nil, is a default client with Timeout. Token is an optional
// personal access token used ONLY as a rate-limit bearer on public endpoints
// (unauthenticated quota is 60 req/hour per IP and shared networks exhaust
// it; a token raises it to 5000). It is sent as a Bearer header and never
// logged, stored, or placed in URLs. Empty scope is sufficient.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Timeout time.Duration
	Token   string
}

func (c Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimSuffix(c.BaseURL, "/")
	}
	return DefaultBase
}

func (c Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 10 * time.Second
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{
		Timeout: c.timeout(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Confine redirects to the API host: a compromised or hijacked
			// response must not bounce the client to an internal address.
			if !strings.EqualFold(req.URL.Host, firstHost(c.base())) {
				return fmt.Errorf("GITHUB_UNAVAILABLE: unexpected redirect to %s", req.URL.Host)
			}
			if len(via) >= 5 {
				return fmt.Errorf("GITHUB_UNAVAILABLE: too many redirects")
			}
			return nil
		},
	}
}

func firstHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

var (
	ownerRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,39}$`)
	repoRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
	shaRe   = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	tagRe   = regexp.MustCompile(`^[A-Za-z0-9_.\-/]{1,128}$`)
)

// ParseRepoURL accepts only public GitHub HTTPS repository URLs and returns
// owner + repo. Anything else (schemes, hosts, credentials, paths) is
// INVALID_INPUT — never fetched.
func ParseRepoURL(raw string) (owner, repo string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil || !strings.EqualFold(u.Scheme, "https") {
		return "", "", fmt.Errorf("INVALID_INPUT: repository URL must be https (got %q)", raw)
	}
	if !strings.EqualFold(u.Hostname(), "github.com") {
		return "", "", fmt.Errorf("INVALID_INPUT: only github.com repositories are supported (got host %q)", u.Hostname())
	}
	if u.User != nil {
		return "", "", fmt.Errorf("INVALID_INPUT: repository URL must not embed credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("INVALID_INPUT: repository URL must be a bare owner/repo path")
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("INVALID_INPUT: repository URL must look like https://github.com/owner/repo")
	}
	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	if !ownerRe.MatchString(owner) || !repoRe.MatchString(repo) {
		return "", "", fmt.Errorf("INVALID_INPUT: invalid owner or repository name")
	}
	return owner, repo, nil
}

// IsSHA reports whether ref is a full immutable commit SHA (40 or 64 hex).
func IsSHA(ref string) bool { return shaRe.MatchString(strings.TrimSpace(ref)) }

func (c Client) get(ctx context.Context, path string) (status int, body []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("INVALID_INPUT: bad github request: %v", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "quorum-onboarding")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("GITHUB_UNAVAILABLE: github request failed: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("GITHUB_UNAVAILABLE: github read failed: %v", err)
	}
	if len(raw) > maxBody {
		return resp.StatusCode, nil, fmt.Errorf("GITHUB_UNAVAILABLE: github response too large")
	}
	return resp.StatusCode, raw, nil
}

func decode(status int, body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("GITHUB_UNAVAILABLE: github returned malformed JSON")
	}
	return nil
}

// statusError reports an unexpected upstream status with a short body snippet
// (GitHub error JSON carries no secrets; it tells rate-limit from denial).
func statusError(what string, status int, body []byte) error {
	snip := strings.TrimSpace(string(body))
	if len(snip) > 200 {
		snip = snip[:200]
	}
	return fmt.Errorf("GITHUB_UNAVAILABLE: %s status %d: %s", what, status, snip)
}

// TagNames lists release tag names newest-first (GitHub returns
// most-recently-created first; one page of 100). Peeling happens per-tag in
// Resolve — never for the whole list.
func (c Client) TagNames(ctx context.Context, owner, repo string) ([]string, error) {
	code, body, err := c.get(ctx, "/repos/"+owner+"/"+repo+"/tags?per_page=100")
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("REPO_NOT_FOUND: github repository %s/%s not found or not public", owner, repo)
	}
	if code != http.StatusOK {
		return nil, statusError("github tags", code, body)
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := decode(code, body, &raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		out = append(out, t.Name)
	}
	return out, nil
}

// peelTag resolves annotated tags to their commit SHA. The tags listing
// points at the tag *object* for annotated tags, which is not buildable —
// peeling is required, never skipped.
func (c Client) peelTag(ctx context.Context, owner, repo, tag string) (string, error) {
	var ref struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	code, body, err := c.get(ctx, "/repos/"+owner+"/"+repo+"/git/refs/tags/"+url.PathEscape(tag))
	if err != nil {
		return "", err
	}
	if code == http.StatusNotFound {
		return "", fmt.Errorf("TAG_NOT_FOUND: tag %q not found in %s/%s", tag, owner, repo)
	}
	if code != http.StatusOK {
		return "", statusError("github ref", code, body)
	}
	if err := decode(code, body, &ref); err != nil {
		return "", err
	}
	if ref.Object.Type != "tag" {
		if !IsSHA(ref.Object.SHA) {
			return "", fmt.Errorf("GITHUB_UNAVAILABLE: github ref has malformed SHA")
		}
		return strings.ToLower(ref.Object.SHA), nil
	}
	var tobj struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	code, body, err = c.get(ctx, "/repos/"+owner+"/"+repo+"/git/tags/"+ref.Object.SHA)
	if err != nil {
		return "", err
	}
	if err := decode(code, body, &tobj); err != nil {
		return "", err
	}
	if code != http.StatusOK || !IsSHA(tobj.Object.SHA) {
		return "", fmt.Errorf("GITHUB_UNAVAILABLE: cannot peel annotated tag %q", tag)
	}
	return strings.ToLower(tobj.Object.SHA), nil
}

// Resolve maps a tag name or full commit SHA to an immutable commit SHA.
// Tags are matched against the live listing and peeled; raw SHAs are
// verified to exist so typos fail here instead of in a worker.
func (c Client) Resolve(ctx context.Context, owner, repo, ref string) (tag, sha string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("INVALID_INPUT: a tag or commit SHA is required")
	}
	if IsSHA(ref) {
		code, commitBody, gerr := c.get(ctx, "/repos/"+owner+"/"+repo+"/commits/"+strings.ToLower(ref))
		if gerr != nil {
			return "", "", gerr
		}
		if code == http.StatusNotFound {
			return "", "", fmt.Errorf("TAG_NOT_FOUND: commit %q not found in %s/%s", ref, owner, repo)
		}
		if code != http.StatusOK {
			return "", "", statusError("github commit", code, commitBody)
		}
		return "", strings.ToLower(ref), nil
	}
	if !tagRe.MatchString(ref) {
		return "", "", fmt.Errorf("INVALID_INPUT: invalid tag %q", ref)
	}
	// Resolve directly by ref: the tags listing is only one page and misses
	// old releases, while the ref endpoint answers for any tag. A missing
	// tag surfaces as TAG_NOT_FOUND from peelTag.
	sha, err2 := c.peelTag(ctx, owner, repo, ref)
	if err2 != nil {
		return "", "", err2
	}
	return ref, sha, nil
}

// RootFiles lists top-level paths at a commit (names only, capped).
func (c Client) RootFiles(ctx context.Context, owner, repo, sha string) ([]string, error) {
	var raw []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	code, body, err := c.get(ctx, "/repos/"+owner+"/"+repo+"/contents/?ref="+sha)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("REPO_NOT_FOUND: cannot list %s/%s at %s", owner, repo, shortSHA(sha))
	}
	if code != http.StatusOK {
		return nil, statusError("github contents", code, body)
	}
	if err := decode(code, body, &raw); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(raw))
	for _, f := range raw {
		if f.Type == "file" {
			names = append(names, f.Name)
		}
	}
	return names, nil
}

// DetectEcosystem maps root files to a Quorum ecosystem. Returns "" when
// nothing matches — callers must ask the user, never guess a build.
func DetectEcosystem(files []string) string {
	set := map[string]bool{}
	for _, f := range files {
		set[strings.ToLower(f)] = true
	}
	switch {
	case set["go.mod"]:
		return "go"
	case set["package.json"]:
		return "npm"
	case set["cargo.toml"]:
		return "cargo"
	case set["pyproject.toml"], set["setup.py"]:
		return "pypi"
	case set["makefile"]:
		return "c"
	default:
		return ""
	}
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
