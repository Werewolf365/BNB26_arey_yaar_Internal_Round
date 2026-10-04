package github_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gh "github.com/quorum/quorum/services/github"
)

func TestParseRepoURL(t *testing.T) {
	good := map[string][2]string{
		"https://github.com/Kashaan-Ekhlas/quorum-demo-c": {"Kashaan-Ekhlas", "quorum-demo-c"},
		"https://github.com/tukaani-project/xz.git":       {"tukaani-project", "xz"},
		"  https://github.com/a/b  ":                      {"a", "b"},
		"https://github.com/my_org/my.repo-name_2":        {"my_org", "my.repo-name_2"},
	}
	for in, want := range good {
		o, r, err := gh.ParseRepoURL(in)
		if err != nil || o != want[0] || r != want[1] {
			t.Fatalf("parse %q: %v %q %q", in, err, o, r)
		}
	}
	bad := []string{
		"", "not a url", "http://github.com/a/b", "git@github.com:a/b.git",
		"https://gitlab.com/a/b", "https://github.com.evil.com/a/b",
		"https://user:pass@github.com/a/b", "https://github.com/onlyone",
		"https://github.com/a/b/c", "https://github.com/a/", "https://github.com//",
		"https://github.com/a/b?x=1#y", // query-only suffix is not a repo path
		"https://169.254.169.254/a/b", "http://localhost:8545/a/b",
		"https://github.com/a/b;rm -rf /", "https://github.com/a/$(id)",
	}
	for _, in := range bad {
		if _, _, err := gh.ParseRepoURL(in); err == nil {
			t.Fatalf("parse %q must fail", in)
		} else if !strings.HasPrefix(err.Error(), "INVALID_INPUT") {
			t.Fatalf("parse %q must be INVALID_INPUT, got %v", in, err)
		}
	}
}

// fixture emulates the GitHub REST surface the client uses.
func fixture(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	return httptest.NewServer(mux)
}

func TestTagNames(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"v0.1.0","commit":{"sha":"` + strings.Repeat("a", 40) + `"}},{"name":"v0.2.0","commit":{"sha":"` + strings.Repeat("b", 40) + `"}}]`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	names, err := c.TagNames(t.Context(), "o", "r")
	if err != nil || len(names) != 2 || names[0] != "v0.1.0" || names[1] != "v0.2.0" {
		t.Fatalf("tag names: %+v %v", names, err)
	}
}

func TestPeel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"v0.1.0","commit":{"sha":"` + strings.Repeat("a", 40) + `"}},{"name":"v0.2.0","commit":{"sha":"` + strings.Repeat("b", 40) + `"}}]`))
	})
	mux.HandleFunc("/repos/o/r/git/refs/tags/v0.1.0", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":{"type":"commit","sha":"` + strings.Repeat("a", 40) + `"}}`))
	})
	mux.HandleFunc("/repos/o/r/git/refs/tags/v0.2.0", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":{"type":"tag","sha":"` + strings.Repeat("b", 40) + `"}}`))
	})
	mux.HandleFunc("/repos/o/r/git/tags/"+strings.Repeat("b", 40), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":{"sha":"` + strings.Repeat("c", 40) + `"}}`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	tag, sha, err := c.Resolve(t.Context(), "o", "r", "v0.1.0")
	if err != nil || tag != "v0.1.0" || sha != strings.Repeat("a", 40) {
		t.Fatalf("lightweight tag must pass through: %v %v %v", tag, sha, err)
	}
	tag, sha, err = c.Resolve(t.Context(), "o", "r", "v0.2.0")
	if err != nil || tag != "v0.2.0" || sha != strings.Repeat("c", 40) {
		t.Fatalf("annotated tag must peel to commit: %v %v %v", tag, sha, err)
	}
}

func TestResolve(t *testing.T) {
	sha := strings.Repeat("d", 40)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"v1","commit":{"sha":"` + sha + `"}}]`))
	})
	mux.HandleFunc("/repos/o/r/git/refs/tags/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":{"type":"commit","sha":"` + sha + `"}}`))
	})
	mux.HandleFunc("/repos/o/r/commits/"+sha, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sha":"` + sha + `"}`))
	})
	mux.HandleFunc("/repos/o/r/commits/"+strings.Repeat("e", 40), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	tag, got, err := c.Resolve(t.Context(), "o", "r", "v1")
	if err != nil || tag != "v1" || got != sha {
		t.Fatalf("resolve tag: %v %v %v", tag, got, err)
	}
	tag, got, err = c.Resolve(t.Context(), "o", "r", sha)
	if err != nil || tag != "" || got != sha {
		t.Fatalf("resolve sha: %v %v %v", tag, got, err)
	}
	if _, _, err = c.Resolve(t.Context(), "o", "r", "nope"); err == nil || !strings.HasPrefix(err.Error(), "TAG_NOT_FOUND") {
		t.Fatalf("missing tag must be TAG_NOT_FOUND: %v", err)
	}
	if _, _, err = c.Resolve(t.Context(), "o", "r", strings.Repeat("e", 40)); err == nil || !strings.HasPrefix(err.Error(), "TAG_NOT_FOUND") {
		t.Fatalf("missing sha must be TAG_NOT_FOUND: %v", err)
	}
	if _, _, err = c.Resolve(t.Context(), "o", "r", ""); err == nil {
		t.Fatal("empty ref must fail")
	}
	if _, _, err = c.Resolve(t.Context(), "o", "r", "../escape"); err == nil {
		t.Fatal("path traversal tag must fail")
	}
}

func TestRepoNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	if _, err := c.TagNames(t.Context(), "o", "missing"); err == nil || !strings.HasPrefix(err.Error(), "REPO_NOT_FOUND") {
		t.Fatalf("missing repo must be REPO_NOT_FOUND: %v", err)
	}
}

func TestDetectEcosystem(t *testing.T) {
	if gh.DetectEcosystem([]string{"go.mod"}) != "go" {
		t.Fatal("go.mod")
	}
	if gh.DetectEcosystem([]string{"package.json"}) != "npm" {
		t.Fatal("package.json")
	}
	if gh.DetectEcosystem([]string{"Cargo.toml"}) != "cargo" {
		t.Fatal("Cargo.toml")
	}
	if gh.DetectEcosystem([]string{"pyproject.toml"}) != "pypi" {
		t.Fatal("pyproject")
	}
	if gh.DetectEcosystem([]string{"setup.py"}) != "pypi" {
		t.Fatal("setup.py")
	}
	if gh.DetectEcosystem([]string{"Makefile", "main.c"}) != "c" {
		t.Fatal("Makefile")
	}
	if gh.DetectEcosystem([]string{"README.md"}) != "" {
		t.Fatal("unknown must be empty, never guessed")
	}
	if gh.DetectEcosystem(nil) != "" {
		t.Fatal("nil must be empty")
	}
}

func TestRootFiles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/contents/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != strings.Repeat("a", 40) {
			t.Errorf("ref must be forwarded")
		}
		w.Write([]byte(`[{"name":"go.mod","type":"file"},{"name":"src","type":"dir"}]`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	files, err := c.RootFiles(t.Context(), "o", "r", strings.Repeat("a", 40))
	if err != nil || len(files) != 1 || files[0] != "go.mod" {
		t.Fatalf("root files: %+v %v", files, err)
	}
}

func TestRedirectBlocked(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer evil.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/x", http.StatusFound)
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	if _, err := c.TagNames(t.Context(), "o", "r"); err == nil || !strings.Contains(err.Error(), "GITHUB_UNAVAILABLE") {
		t.Fatalf("off-host redirect must be refused: %v", err)
	}
}

func TestMalformedUpstream(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json{`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL}
	if _, err := c.TagNames(t.Context(), "o", "r"); err == nil {
		t.Fatal("malformed JSON must fail")
	}
}

func TestTokenSentAsBearerNeverLogged(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token-123" {
			t.Errorf("bearer token missing")
		}
		if r.URL.Query().Get("x") != "" {
			t.Errorf("token must not appear in URL")
		}
		w.Write([]byte(`[]`))
	})
	srv := fixture(t, mux)
	defer srv.Close()
	c := gh.Client{BaseURL: srv.URL, Token: "test-token-123"}
	names, err := c.TagNames(t.Context(), "o", "r")
	if err != nil || len(names) != 0 {
		t.Fatalf("tag names: %+v %v", names, err)
	}
	// And without a token no Authorization header is sent at all.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			t.Errorf("no token configured, must not send auth: %q", h)
		}
		w.Write([]byte(`[]`))
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	c2 := gh.Client{BaseURL: srv2.URL}
	if _, err := c2.TagNames(t.Context(), "o", "r"); err != nil {
		t.Fatalf("untokened: %v", err)
	}
}
