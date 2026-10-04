// Command api serves the Quorum REST API.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	gh "github.com/quorum/quorum/services/github"
	"github.com/quorum/quorum/services/storage"
)

func main() {
	addr := os.Getenv("QUORUM_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var st store.Store = store.NewMemoryStore()
	if url := os.Getenv("QUORUM_DATABASE_URL"); url != "" {
		pg, err := store.Connect(ctx, url)
		if err != nil {
			log.Fatalf("db connect: %v", err)
		}
		defer pg.Close()
		if err := pg.Migrate(ctx); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		st = pg
		log.Printf("using postgres backend")
	} else {
		log.Printf("QUORUM_DATABASE_URL unset: using in-memory store (dev/test only)")
	}
	var blobs storage.Backend
	if ep := os.Getenv("QUORUM_S3_ENDPOINT"); ep != "" {
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
		pathStyle := true
		if v := os.Getenv("QUORUM_S3_PATH_STYLE"); v == "false" || v == "0" {
			pathStyle = false
		}
		s3blobs, err := storage.NewS3(storage.S3Config{
			Endpoint: ep, Bucket: bucket, Region: region,
			AccessKey: access, SecretKey: secret, UsePathStyle: pathStyle,
		})
		if err != nil {
			log.Fatalf("storage (s3): %v", err)
		}
		// Fail fast: never silently fall back to local disk when S3 is
		// configured but unreachable (that would split the blob store).
		if err := s3blobs.EnsureBucket(ctx); err != nil {
			log.Fatalf("storage (s3): %v", err)
		}
		blobs = s3blobs
		log.Printf("using s3-compatible storage endpoint=%s bucket=%s region=%s pathStyle=%v", ep, bucket, region, pathStyle)
	} else {
		blobDir := os.Getenv("QUORUM_BLOB_DIR")
		if blobDir == "" {
			blobDir = "./data/evidence"
		}
		fsblobs, err := storage.NewFilesystem(blobDir)
		if err != nil {
			log.Fatalf("storage: %v", err)
		}
		blobs = fsblobs
	}
	cfg := server.ConfigFromEnv()
	if len(cfg.CORSOrigins) == 0 {
		// Local dashboard runs on Next.js (:3000); allow it for browser calls.
		cfg.CORSOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
		log.Printf("QUORUM_CORS_ORIGINS unset: allowing http://localhost:3000 and http://127.0.0.1:3000")
	}
	if len(cfg.APIKeys) == 0 {
		log.Printf("QUORUM_API_KEYS unset: admin writes are OPEN (local dev only — set keys before exposing)")
	}
	srv := &http.Server{
		Addr: addr, Handler: newAPIHandler(st, blobs, cfg),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
	}
	log.Printf("quorum api listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// newAPIHandler builds the router. GITHUB_TOKEN (optional, never logged) is
// a rate-limit-only bearer for public GitHub API reads: unauthenticated
// quota is 60 req/hour per egress IP and shared networks exhaust it. Empty
// scope is sufficient; private repositories are never accessed.
func newAPIHandler(st store.Store, blobs storage.Backend, cfg server.Config) http.Handler {
	s := server.NewWithConfig(st, blobs, cfg)
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		s.Github = &gh.Client{Token: tok}
		log.Printf("github onboarding: token-backed quota enabled")
	} else {
		log.Printf("GITHUB_TOKEN unset: onboarding uses unauthenticated quota (60 req/hour)")
	}
	return s
}
