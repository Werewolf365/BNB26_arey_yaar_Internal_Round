package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/quorum/quorum/services/storage"
)

// fakeS3 is a minimal S3-compat server: path-style URLs, SigV4 header
// assertions, PUT/GET/HEAD incl. the 404 path. Failures use t.Errorf
// (safe from the server goroutine).
type fakeS3 struct {
	t        *testing.T
	bucket   string
	access   string
	blobs    map[string][]byte
	tamper   map[string][]byte
	lastPath string
}

func newFakeS3(t *testing.T, bucket, access string) *fakeS3 {
	t.Helper()
	return &fakeS3{t: t, bucket: bucket, access: access,
		blobs: map[string][]byte{}, tamper: map[string][]byte{}}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		f.t.Errorf("missing SigV4 auth scheme: %q", auth)
	}
	if !strings.Contains(auth, "Credential="+f.access+"/") {
		f.t.Errorf("auth missing credential scope for %q: %q", f.access, auth)
	}
	if !strings.Contains(auth, "/s3/aws4_request") {
		f.t.Errorf("auth missing s3 service scope: %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		f.t.Errorf("auth missing signed headers: %q", auth)
	}
	sig := ""
	if i := strings.Index(auth, "Signature="); i >= 0 {
		sig = auth[i+len("Signature="):]
		if j := strings.IndexAny(sig, ", "); j >= 0 {
			sig = sig[:j]
		}
	}
	if len(sig) != 64 {
		f.t.Errorf("auth signature must be 64 hex chars: %q", auth)
	} else {
		for _, c := range sig {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				f.t.Errorf("auth signature must be lowercase hex: %q", auth)
				break
			}
		}
	}
	if r.Header.Get("x-amz-date") == "" || r.Header.Get("x-amz-content-sha256") == "" {
		f.t.Errorf("missing amz signing headers")
	}

	p := strings.TrimPrefix(r.URL.EscapedPath(), "/")
	if p == f.bucket {
		// Bucket-level op (HEAD bucket / PUT bucket).
		w.WriteHeader(200)
		return
	}
	key := strings.TrimPrefix(p, f.bucket+"/")
	if key == p {
		http.Error(w, "wrong bucket path", http.StatusBadRequest)
		return
	}
	f.lastPath = "/" + p
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", http.StatusBadRequest)
			return
		}
		f.blobs[key] = body
		w.WriteHeader(200)
	case http.MethodGet:
		if tb, ok := f.tamper[key]; ok {
			_, _ = w.Write(tb)
			return
		}
		b, ok := f.blobs[key]
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>NoSuchKey</Code></Error>`))
			return
		}
		_, _ = w.Write(b)
	case http.MethodHead:
		if _, ok := f.blobs[key]; !ok {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(200)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func newTestS3(t *testing.T, f *fakeS3, srv *httptest.Server) *storage.S3 {
	t.Helper()
	s, err := storage.NewS3(storage.S3Config{
		Endpoint: srv.URL, Bucket: f.bucket, Region: "us-east-1",
		AccessKey: f.access, SecretKey: "sekret-for-tests-only",
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	return s
}

func TestS3PutGetRoundTrip(t *testing.T) {
	f := newFakeS3(t, "quorum-evidence", "test-access")
	srv := httptest.NewServer(f)
	defer srv.Close()
	s := newTestS3(t, f, srv)

	key, err := s.Put([]byte("evidence-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "sha256/") {
		t.Fatalf("content key: %q", key)
	}
	if f.lastPath != "/quorum-evidence/"+key {
		t.Fatalf("object key path: %q", f.lastPath)
	}
	got, err := s.Get(key)
	if err != nil || string(got) != "evidence-bytes" {
		t.Fatalf("round-trip: %v", err)
	}
	if key2, err := s.Put([]byte("evidence-bytes")); err != nil || key2 != key {
		t.Fatal("dedup")
	}
	ok, err := s.Stat(key)
	if err != nil || !ok {
		t.Fatal("stat")
	}
	if ok, err := s.Stat("sha256/" + strings.Repeat("0", 64)); err != nil || ok {
		t.Fatal("missing must stat false")
	}
	if _, err := s.Get("sha256/" + strings.Repeat("0", 64)); err == nil ||
		!strings.Contains(err.Error(), "EVIDENCE_NOT_FOUND") {
		t.Fatalf("missing must be EVIDENCE_NOT_FOUND: %v", err)
	}
}

func TestS3TamperDetected(t *testing.T) {
	f := newFakeS3(t, "quorum-evidence", "test-access")
	srv := httptest.NewServer(f)
	defer srv.Close()
	s := newTestS3(t, f, srv)

	key, err := s.Put([]byte("pristine"))
	if err != nil {
		t.Fatal(err)
	}
	f.tamper[key] = []byte("corrupted!!")
	if _, err := s.Get(key); err == nil || !strings.Contains(err.Error(), "AUDIT_TAMPERED") {
		t.Fatalf("tampered blob must fail hash check: %v", err)
	}
}

func TestS3LimitsAndBadKeys(t *testing.T) {
	f := newFakeS3(t, "quorum-evidence", "test-access")
	srv := httptest.NewServer(f)
	defer srv.Close()
	s := newTestS3(t, f, srv)

	if _, err := s.Put(make([]byte, storage.MaxBlobBytes+1)); err == nil ||
		!strings.Contains(err.Error(), "ARTIFACT_OVERSIZED") {
		t.Fatalf("oversize must be rejected: %v", err)
	}
	if _, err := s.Put(nil); err == nil {
		t.Fatal("empty blob must fail")
	}
	for _, bad := range []string{
		"", "../../etc/passwd", "sha256/../../x", "sha256/xyz",
		"sha256/" + strings.Repeat("g", 64), "md5/abc", "sha256/abc/extra",
		"sha256/" + strings.Repeat("A", 64),
	} {
		if _, err := s.Get(bad); err == nil {
			t.Fatalf("Get(%q) must fail", bad)
		}
		if _, err := s.Stat(bad); err == nil {
			t.Fatalf("Stat(%q) must fail", bad)
		}
	}
	for _, cfg := range []storage.S3Config{
		{Bucket: "b", AccessKey: "a", SecretKey: "s"},                      // no endpoint
		{Endpoint: "://bad", Bucket: "b", AccessKey: "a", SecretKey: "s"},  // bad endpoint
		{Endpoint: srv.URL, AccessKey: "a", SecretKey: "s"},                // no bucket
		{Endpoint: srv.URL, Bucket: "b/c", AccessKey: "a", SecretKey: "s"}, // bad bucket
		{Endpoint: srv.URL, Bucket: "b", SecretKey: "s"},                   // no access key
		{Endpoint: srv.URL, Bucket: "b", AccessKey: "a"},                   // no secret
		{Endpoint: srv.URL, Bucket: "b", AccessKey: "a", SecretKey: "s"},   // valid
	} {
		_, err := storage.NewS3(cfg)
		if cfg.Bucket == "b" && cfg.AccessKey == "a" && cfg.SecretKey == "s" &&
			strings.HasPrefix(cfg.Endpoint, "http") {
			if err != nil {
				t.Fatalf("valid config rejected: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("bad config accepted: %+v", cfg)
		}
	}
}

func TestS3UnreachableIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close() // nothing listens here now: connection refused
	s, err := storage.NewS3(storage.S3Config{
		Endpoint: deadURL, Bucket: "quorum-evidence", Region: "us-east-1",
		AccessKey: "test-access", SecretKey: "super-sekret-value-123",
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "sha256/" + strings.Repeat("0", 64)
	if _, err := s.Put([]byte("x")); err == nil ||
		!strings.Contains(err.Error(), "UNAVAILABLE") {
		t.Fatalf("unreachable put must be UNAVAILABLE: %v", err)
	} else if strings.Contains(err.Error(), "super-sekret") {
		t.Fatalf("error leaks secret: %v", err)
	}
	if _, err := s.Get(key); err == nil ||
		!strings.Contains(err.Error(), "UNAVAILABLE") {
		t.Fatalf("unreachable get must be UNAVAILABLE: %v", err)
	} else if strings.Contains(err.Error(), "super-sekret") {
		t.Fatalf("error leaks secret: %v", err)
	}
	if _, err := s.Stat(key); err == nil ||
		!strings.Contains(err.Error(), "UNAVAILABLE") {
		t.Fatalf("unreachable stat must be UNAVAILABLE: %v", err)
	} else if strings.Contains(err.Error(), "super-sekret") {
		t.Fatalf("error leaks secret: %v", err)
	}
	if err := s.EnsureBucket(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "UNAVAILABLE") {
		t.Fatalf("unreachable ensure-bucket must be UNAVAILABLE: %v", err)
	}
}

// TestLiveS3MinIO round-trips a real blob against S3-compat storage
// (compose LocalStack on :4566). Env-gated: skips without QUORUM_LIVE_S3=1,
// and skips (not fails) when the daemon is down.
func TestLiveS3MinIO(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_S3") != "1" {
		t.Skip("set QUORUM_LIVE_S3=1 with s3-compat storage reachable")
	}
	ep := os.Getenv("QUORUM_S3_ENDPOINT")
	if ep == "" {
		ep = "http://localhost:4566"
	}
	bucket := os.Getenv("QUORUM_S3_BUCKET")
	if bucket == "" {
		bucket = "quorum-evidence"
	}
	region := os.Getenv("QUORUM_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	access := os.Getenv("QUORUM_S3_ACCESS_KEY")
	if access == "" {
		access = "test"
	}
	secret := os.Getenv("QUORUM_S3_SECRET_KEY")
	if secret == "" {
		secret = "test"
	}
	s, err := storage.NewS3(storage.S3Config{
		Endpoint: ep, Bucket: bucket, Region: region,
		AccessKey: access, SecretKey: secret, UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.EnsureBucket(ctx); err != nil {
		t.Skipf("s3 unreachable: %v", err)
	}
	payload := []byte(fmt.Sprintf("quorum-live-s3-%d", time.Now().UnixNano()))
	key, err := s.Put(payload)
	if err != nil {
		t.Fatalf("live put: %v", err)
	}
	got, err := s.Get(key)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("live round-trip: %v", err)
	}
	ok, err := s.Stat(key)
	if err != nil || !ok {
		t.Fatalf("live stat: %v", err)
	}
}
