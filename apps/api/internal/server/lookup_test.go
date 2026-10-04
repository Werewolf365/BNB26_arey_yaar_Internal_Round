package server_test

import (
	"testing"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/storage"
)

// Lookup is read-only: bad input is 400, unreachable chain is 502, and
// nothing is ever submitted. No chain needed for these cases.
func TestLookupAnchorValidation(t *testing.T) {
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(store.NewMemoryStore(), fs)
	vid := "0x" + repeat("1", 64)
	contract := "0x5FbDB2315678afecb367f032d93F642f64180aa3"

	if code, env := do(t, srv, "GET", "/api/v1/blockchain/lookup/"+vid+"?contract="+contract, "", nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("missing rpc must be 400 INVALID_INPUT, got %d %+v", code, env.Error)
	}
	if code, env := do(t, srv, "GET", "/api/v1/blockchain/lookup/"+vid+"?rpc=http://127.0.0.1:9", "", nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("missing contract must be 400 INVALID_INPUT, got %d %+v", code, env.Error)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/blockchain/lookup/nope?rpc=http://127.0.0.1:9&contract="+contract, "", nil); code != 400 {
		t.Fatalf("malformed id must be 400, got %d", code)
	}
	if code, env := do(t, srv, "GET", "/api/v1/blockchain/lookup/"+vid+"?rpc=http://127.0.0.1:9&contract="+contract, "", nil); code != 502 || env.Error.Code != "BLOCKCHAIN_UNAVAILABLE" {
		t.Fatalf("unreachable chain must be 502 BLOCKCHAIN_UNAVAILABLE, got %d %+v", code, env.Error)
	}
}

func repeat(s string, n int) string {
	out := ""
	for len(out) < n {
		out += s
	}
	return out[:n]
}
