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
	srv := &http.Server{Addr: addr, Handler: server.New(st), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("quorum api listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
