package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/storage"
)

func newSecureServer(t *testing.T, cfg server.Config) *server.Server {
	t.Helper()
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return server.NewWithConfig(store.NewMemoryStore(), fs, cfg)
}

const testPolicy = `{"policyId":"p1","version":"v1","minBuilders":1,"requiredAgreement":1,"requiredIndependentGroups":1,"conflictTolerance":0}`

func TestAdminAuth(t *testing.T) {
	srv := newSecureServer(t, server.Config{APIKeys: []string{"secret-1"}})

	// No key -> 401, and the error must not echo any credential.
	if code, env := do(t, srv, "POST", "/api/v1/policies", testPolicy, nil); code != 401 || env.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("missing key must be 401 UNAUTHENTICATED: %d %+v", code, env.Error)
	}
	// Wrong key -> 403.
	wrong := map[string]string{"X-Api-Key": "wrong"}
	if code, env := do(t, srv, "POST", "/api/v1/policies", testPolicy, wrong); code != 403 || env.Error.Code != "FORBIDDEN" {
		t.Fatalf("wrong key must be 403 FORBIDDEN: %d %+v", code, env.Error)
	}
	// Right key via X-Api-Key -> 201.
	good := map[string]string{"X-Api-Key": "secret-1"}
	if code, _ := do(t, srv, "POST", "/api/v1/policies", testPolicy, good); code != 201 {
		t.Fatalf("valid key must create: %d", code)
	}
	// Right key via Bearer -> 201 (different id to avoid dup path).
	bearer := map[string]string{"Authorization": "Bearer secret-1"}
	p2 := strings.Replace(testPolicy, "p1", "p2", 1)
	if code, _ := do(t, srv, "POST", "/api/v1/policies", p2, bearer); code != 201 {
		t.Fatalf("bearer key must create: %d", code)
	}
	// Reads stay open without a key.
	if code, _ := do(t, srv, "GET", "/api/v1/policies", "", nil); code != 200 {
		t.Fatalf("reads must stay open: %d", code)
	}
	// Builders PATCH is an admin write too.
	if code, _ := do(t, srv, "POST", "/api/v1/builders", `{"id":"b1","independenceGroup":"g1"}`, good); code != 201 {
		t.Fatalf("register builder: %d", code)
	}
	if code, env := do(t, srv, "PATCH", "/api/v1/builders/b1", `{"enabled":false}`, nil); code != 401 || env.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("patch without key must be 401: %d %+v", code, env.Error)
	}
	if code, _ := do(t, srv, "PATCH", "/api/v1/builders/b1", `{"enabled":false}`, good); code != 200 {
		t.Fatalf("patch with key: %d", code)
	}
}

func TestOpenByDefault(t *testing.T) {
	// Zero Config = open local-dev mode (existing behavior preserved).
	srv := newSecureServer(t, server.Config{})
	if code, _ := do(t, srv, "POST", "/api/v1/policies", testPolicy, nil); code != 201 {
		t.Fatalf("open mode must allow admin writes: %d", code)
	}
}

func TestRateLimit(t *testing.T) {
	srv := newSecureServer(t, server.Config{RateLimitRPS: 1, RateBurst: 1})
	if code, _ := do(t, srv, "GET", "/api/v1/health", "", nil); code != 200 {
		t.Fatalf("first request: %d", code)
	}
	code, env := do(t, srv, "GET", "/api/v1/health", "", nil)
	if code != 429 || env.Error.Code != "RATE_LIMITED" {
		t.Fatalf("immediate second request must be 429 RATE_LIMITED: %d %+v", code, env.Error)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := newSecureServer(t, server.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	for k, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Fatalf("%s: want %q, got %q", k, want, got)
		}
	}
}

func TestCORS(t *testing.T) {
	srv := newSecureServer(t, server.Config{CORSOrigins: []string{"https://app.example"}})
	get := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	if got := get("https://app.example").Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allowlisted origin must be echoed, got %q", got)
	}
	if got := get("https://evil.example").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("foreign origin must get no ACAO, got %q", got)
	}
	if got := get("").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("no origin must get no ACAO, got %q", got)
	}
	// Preflight.
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight must be 204, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Fatalf("preflight must allow POST: %q", rec.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("QUORUM_API_KEYS", " a ,, b ")
	t.Setenv("QUORUM_RATE_LIMIT_RPS", "50")
	t.Setenv("QUORUM_RATE_BURST", "7")
	t.Setenv("QUORUM_CORS_ORIGINS", "https://a.example, https://b.example")
	c := server.ConfigFromEnv()
	if len(c.APIKeys) != 2 || c.APIKeys[0] != "a" || c.APIKeys[1] != "b" {
		t.Fatalf("keys: %+v", c.APIKeys)
	}
	if c.RateLimitRPS != 50 || c.RateBurst != 7 {
		t.Fatalf("rate: %+v", c)
	}
	if len(c.CORSOrigins) != 2 {
		t.Fatalf("origins: %+v", c.CORSOrigins)
	}
}
