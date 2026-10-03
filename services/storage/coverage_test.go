package storage_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quorum/quorum/services/storage"
)

// TestS3EnsureBucketCreateFlow drives the 404 -> PUT-create path with a
// non-us-east-1 region (covers createBucket + LocationConstraint body).
func TestS3EnsureBucketCreateFlow(t *testing.T) {
	var mu sync.Mutex
	created := false
	var putBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodHead:
			if !created {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/quorum-evidence":
			putBody, _ = io.ReadAll(r.Body)
			created = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	s3, err := storage.NewS3(storage.S3Config{
		Endpoint: srv.URL, Bucket: "quorum-evidence", Region: "eu-west-1",
		AccessKey: "a", SecretKey: "s", UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s3.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("ensure with create: %v", err)
	}
	if !strings.Contains(string(putBody), "<LocationConstraint>eu-west-1</LocationConstraint>") {
		t.Fatalf("regional create must carry constraint: %q", putBody)
	}
}

// TestS3EnsureBucketErrors covers the 403/500 HEAD branches.
func TestS3EnsureBucketErrors(t *testing.T) {
	for status, want := range map[int]string{403: "access key", 500: "UNAVAILABLE"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		s3, err := storage.NewS3(storage.S3Config{
			Endpoint: srv.URL, Bucket: "b", Region: "us-east-1",
			AccessKey: "a", SecretKey: "s", UsePathStyle: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		err = s3.EnsureBucket(context.Background())
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("status %d must fail with %q: %v", status, want, err)
		}
	}
}

// TestS3VirtualHostStyle round-trips through bucket-in-Host addressing.
func TestS3VirtualHostStyle(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.Host, "vh-bucket.") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		obj := strings.TrimPrefix(r.URL.Path, "/")
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			objects[obj] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b, ok := objects[obj]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(b)
		case http.MethodHead:
			if _, ok := objects[obj]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	// Virtual-host style changes the DNS name, so dial the test listener
	// directly regardless of the Host header.
	vhClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
			},
		},
	}
	s3, err := storage.NewS3(storage.S3Config{
		Endpoint: srv.URL, Bucket: "vh-bucket", Region: "us-east-1",
		AccessKey: "a", SecretKey: "s", UsePathStyle: false, HTTPClient: vhClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s3.Put([]byte("vh-data"))
	if err != nil {
		t.Fatalf("virtual-host put: %v", err)
	}
	got, err := s3.Get(key)
	if err != nil || string(got) != "vh-data" {
		t.Fatalf("virtual-host get: %q %v", got, err)
	}
	if ok, err := s3.Stat(key); err != nil || !ok {
		t.Fatalf("virtual-host stat: %v %v", ok, err)
	}
	if ok, err := s3.Stat("sha256/" + strings.Repeat("0", 64)); err != nil || ok {
		t.Fatalf("missing stat must be false,nil: %v %v", ok, err)
	}
}
