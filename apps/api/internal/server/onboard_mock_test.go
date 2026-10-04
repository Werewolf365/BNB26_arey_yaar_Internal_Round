package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/github"
	"github.com/quorum/quorum/services/storage"
)

const (
	obShaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	obShaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// mockGitHub emulates the REST surface the onboarding flow uses.
func mockGitHub(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"v0.1.0","commit":{"sha":"` + obShaA + `"}}]`))
	})
	mux.HandleFunc("/repos/o/r/git/refs/tags/v0.1.0", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":{"type":"commit","sha":"` + obShaA + `"}}`))
	})
	mux.HandleFunc("/repos/o/r/commits/"+obShaA, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sha":"` + obShaA + `"}`))
	})
	mux.HandleFunc("/repos/o/r/contents/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"Makefile","type":"file"},{"name":"main.c","type":"file"}]`))
	})
	mux.HandleFunc("/repos/o/empty/contents/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"README.md","type":"file"}]`))
	})
	mux.HandleFunc("/repos/o/empty/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"v9","commit":{"sha":"` + obShaB + `"}}]`))
	})
	mux.HandleFunc("/repos/o/empty/git/refs/tags/v9", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":{"type":"commit","sha":"` + obShaB + `"}}`))
	})
	mux.HandleFunc("/repos/o/empty/commits/"+obShaB, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sha":"` + obShaB + `"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func onboardServer(t *testing.T, githubBase string) *server.Server {
	t.Helper()
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(store.NewMemoryStore(), fs)
	srv.Github = &github.Client{BaseURL: githubBase}
	return srv
}

func TestOnboardingValidation(t *testing.T) {
	srv := onboardServer(t, mockGitHub(t))
	// Bad URLs never reach the network.
	for _, u := range []string{"", "http://github.com/o/r", "https://gitlab.com/o/r", "https://github.com/only", "git@github.com:o/r.git"} {
		if code, env := do(t, srv, "GET", "/api/v1/onboarding/discover?repo="+u, "", nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
			t.Fatalf("discover %q: %d %+v", u, code, env.Error)
		}
	}
	// Missing repo is rejected even though ParseRepoURL("") fails the same way.
	if code, _ := do(t, srv, "GET", "/api/v1/onboarding/discover", "", nil); code != 400 {
		t.Fatalf("missing repo: %d", code)
	}
	// Unknown repo surfaces REPO_NOT_FOUND, not internals.
	if code, env := do(t, srv, "GET", "/api/v1/onboarding/discover?repo=https://github.com/o/missing", "", nil); code != 404 || env.Error.Code != "REPO_NOT_FOUND" {
		t.Fatalf("missing repo: %d %+v", code, env.Error)
	}
	// Resolve validates ref + ecosystem.
	if code, _ := do(t, srv, "POST", "/api/v1/onboarding/resolve", `{"repo":"https://github.com/o/r"}`, nil); code != 400 {
		t.Fatalf("missing ref: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/onboarding/resolve", `{"repo":"https://github.com/o/r","ref":"v0.1.0","ecosystem":"docker"}`, nil); code != 400 {
		t.Fatalf("unsupported ecosystem: %d", code)
	}
	if code, env := do(t, srv, "POST", "/api/v1/onboarding/resolve", `{"repo":"https://github.com/o/r","ref":"v404"}`, nil); code != 404 || env.Error.Code != "TAG_NOT_FOUND" {
		t.Fatalf("missing tag: %d %+v", code, env.Error)
	}
}

func TestOnboardingResolveAndCreate(t *testing.T) {
	srv := onboardServer(t, mockGitHub(t))
	code, env := do(t, srv, "GET", "/api/v1/onboarding/discover?repo=https://github.com/o/r.git", "", nil)
	if code != 200 || !strings.Contains(string(env.Data), "v0.1.0") {
		t.Fatalf("discover: %d %s", code, env.Data)
	}
	code, env = do(t, srv, "POST", "/api/v1/onboarding/resolve", `{"repo":"https://github.com/o/r","ref":"v0.1.0"}`, nil)
	if code != 200 {
		t.Fatalf("resolve: %d", code)
	}
	for _, want := range []string{obShaA, `"ecosystem":"c"`, `"buildKind":"git-archive"`, `"minBuilders":2`} {
		if !strings.Contains(string(env.Data), want) {
			t.Fatalf("resolve must carry %s: %s", want, env.Data)
		}
	}
	// Undetectable ecosystem resolves with empty ecosystem; creation refuses.
	code, env = do(t, srv, "POST", "/api/v1/onboarding/resolve", `{"repo":"https://github.com/o/empty","ref":"v9"}`, nil)
	if code != 200 || !strings.Contains(string(env.Data), `"ecosystem":""`) {
		t.Fatalf("unknown ecosystem must resolve empty: %d %s", code, env.Data)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/projects", `{"repo":"https://github.com/o/empty","ref":"v9"}`, nil); code != 400 {
		t.Fatalf("create without ecosystem must be 400, got %d", code)
	}
	// Create, duplicate returns existing (200, no second row), get + list.
	code, env = do(t, srv, "POST", "/api/v1/projects", `{"repo":"https://github.com/o/r","ref":"v0.1.0","displayName":"Demo R"}`, nil)
	if code != 201 {
		t.Fatalf("create: %d %s", code, env.Data)
	}
	id := mustData[map[string]any](t, env)["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/projects", `{"repo":"https://github.com/o/r","ref":"v0.1.0"}`, nil)
	if code != 200 || mustData[map[string]any](t, env)["id"].(string) != id {
		t.Fatalf("duplicate must return existing with 200: %d %s", code, env.Data)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/projects/"+id, "", nil); code != 200 {
		t.Fatalf("get: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/projects/prj_nope", "", nil); code != 404 {
		t.Fatalf("missing project: %d", code)
	}
	if code, env := do(t, srv, "GET", "/api/v1/projects", "", nil); code != 200 || strings.Count(string(env.Data), id) != 1 {
		t.Fatalf("list must contain exactly one row: %d %s", code, env.Data)
	}
}
