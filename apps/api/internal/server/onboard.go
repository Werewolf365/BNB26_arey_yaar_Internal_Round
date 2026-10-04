package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/internal/runner"
	gh "github.com/quorum/quorum/services/github"
)

// SupportedEcosystems are the metadata values onboarding accepts. The build
// itself is always the git-archive source reproduction; ecosystem labels the
// release, it never selects a different build process.
var SupportedEcosystems = map[string]bool{"go": true, "npm": true, "cargo": true, "pypi": true, "c": true}

func (s *Server) githubClient() gh.Client {
	if s.Github != nil {
		return *s.Github
	}
	return gh.Client{}
}

// GET /api/v1/onboarding/discover?repo=URL — validate + list tags.
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	owner, repo, err := gh.ParseRepoURL(r.URL.Query().Get("repo"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	names, err := s.githubClient().TagNames(r.Context(), owner, repo)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, map[string]any{"owner": owner, "repo": repo, "tags": names})
}

// resolveInput is shared by resolve + project creation.
type resolveInput struct {
	Repo        string `json:"repo"`
	Ref         string `json:"ref"`
	Ecosystem   string `json:"ecosystem"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Package     string `json:"package"`
	Version     string `json:"version"`
}

type resolvedProject struct {
	Owner       string         `json:"owner"`
	Repo        string         `json:"repo"`
	Tag         string         `json:"tag,omitempty"`
	Commit      string         `json:"commit"`
	Ecosystem   string         `json:"ecosystem"`
	BuildKind   string         `json:"buildKind"`
	DisplayName string         `json:"displayName"`
	Policy      map[string]any `json:"defaultPolicy"`
}

func (s *Server) resolveProject(r *http.Request, in resolveInput) (*resolvedProject, error) {
	owner, repo, err := gh.ParseRepoURL(in.Repo)
	if err != nil {
		return nil, err
	}
	gc := s.githubClient()
	tag, sha, err := gc.Resolve(r.Context(), owner, repo, in.Ref)
	if err != nil {
		return nil, err
	}
	files, err := gc.RootFiles(r.Context(), owner, repo, sha)
	if err != nil {
		return nil, err
	}
	eco := strings.ToLower(strings.TrimSpace(in.Ecosystem))
	if eco == "" {
		eco = gh.DetectEcosystem(files)
	} else if !SupportedEcosystems[eco] {
		return nil, fmt.Errorf("INVALID_INPUT: ecosystem must be one of go, npm, cargo, pypi, c")
	}
	pol := runner.DefaultPolicy()
	return &resolvedProject{
		Owner: owner, Repo: repo, Tag: tag, Commit: sha,
		Ecosystem: eco, BuildKind: "git-archive",
		DisplayName: strings.TrimSpace(in.DisplayName),
		Policy: map[string]any{
			"policyId": pol.PolicyID, "minBuilders": pol.MinBuilders,
			"requiredAgreement":         pol.RequiredAgreement,
			"requiredIndependentGroups": pol.RequiredIndependentGroups,
			"requireSourceMatch":        pol.RequireSourceMatch,
			"requireAllSignaturesValid": pol.RequireAllSignaturesValid,
			"conflictTolerance":         pol.ConflictTolerance,
		},
	}, nil
}

// POST /api/v1/onboarding/resolve — preview without persisting.
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var in resolveInput
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.resolveProject(r, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if res.DisplayName == "" {
		res.DisplayName = res.Repo
	}
	writeJSON(w, r, 200, res)
}

// POST /api/v1/projects — resolve + persist (dedupe on repo+commit).
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var in resolveInput
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := s.resolveProject(r, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if res.Ecosystem == "" {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: ecosystem could not be detected; specify one of go, npm, cargo, pypi, c"))
		return
	}
	name := res.DisplayName
	if name == "" {
		name = res.Repo
	}
	pkg := strings.TrimSpace(in.Package)
	if pkg == "" {
		pkg = res.Repo
	}
	ver := strings.TrimSpace(in.Version)
	if ver == "" {
		ver = res.Tag
	}
	pr, dup, err := s.store.CreateProject(r.Context(), store.Project{
		Name: name, Description: strings.TrimSpace(in.Description),
		Repo: "https://github.com/" + res.Owner + "/" + res.Repo,
		Tag:  res.Tag, Commit: res.Commit, Package: pkg,
		Ecosystem: res.Ecosystem, Version: ver, BuildKind: res.BuildKind,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if dup {
		writeJSON(w, r, 200, pr)
		return
	}
	writeJSON(w, r, 201, pr)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	limit := queryLimit(r, 50)
	prs, err := s.store.ListProjects(r.Context(), limit)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, prs)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	pr, err := s.store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, pr)
}
