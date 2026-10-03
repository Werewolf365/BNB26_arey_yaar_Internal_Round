# QUORUM — Agent handoff / mid-refactor state

> Read this before touching code in this working tree.
> Last stable commit: `ead8dca` (all suites green; `go build ./...` clean).

## What the previous session was doing

Continuing the master plan in order: workers done (`23ffdc0`), OSS Rebuild
live path done (`ead8dca`), then **Sigstore/Cosign phase** and **object
storage wiring** were in progress when the session stopped. It also picked up
two hygiene items en route (JSON error envelopes on stderr, enforced by
`emitErr`; and this handoff doc).

## Precise state of the uncommitted leftovers

All below is **uncommitted**. `git status` shows:
`M server.go, M root.go, ?? attest.go, ?? attest_test.go, ?? services/sigstore/, ?? services/storage/`.

| Piece | Status | Evidence |
|---|---|---|
| `services/sigstore/sigstore.go` + transparency log | **Complete, tested** | `go test ./services/sigstore/` ok — P-256 keys, DSSE sign/verify, policy checks (digest/commit prefix-normalized, builder allowlist), PEM round-trips, file transparency log tamper tests |
| `apps/cli/cmd/attest.go` (+`root.go` wiring) + test | **Complete, tested** | `go test ./apps/cli/cmd/` ok — `attest genkey`/`sign`/`verify` round-trip plus wrong-key/digest/commit/builder/missing-flags/malformed cases; binary path verified manually |
| `services/storage/storage.go` + tests | **Complete, tested** | `go test ./services/storage/` ok — content-addressed FS store, traversal/shape caps, O_EXCL+rename, concurrent-put retry (Windows), on-disk tamper detection |
| `apps/api/internal/server/server.go` storage wiring | **INCOMPLETE, breaks build** | `New` signature changed to `(st, blobs)`, `blobs` field + storage import added, but callers (`apps/api/main.go`, `server_test.go`) still pass one arg; also a stale import of `github.com/quorum/quorum/apps/cli/internal/runner` (moved to `internal/runner`) remains. `go build ./...` currently fails only here. |

## Intended (not done) — resume checklist

1. Finish the storage wiring: fix/replace the stale runner import in
   `server.go`; construct the `storage.Filesystem` backend in `apps/api/main.go`
   (default `./data/evidence`); pass it to `server.New(st, blobs)`; update
   `server_test.go`'`newTestServer()`; persist submitted evidence blobs via
   `POST /api/v1/evidence` and serve them back hash-verified via
   `GET /api/v1/evidence/:id` (+ an HTTP blob download route if wanted).
2. Remove the `services/attestation/dsse.mjs` test-key scaffolding from the
   trust path once sigstore is the single implementation; keep the Node mirror
   only if it stays conformance-locked.
3. Remaining roadmap unchanged (Sigstore transparency-in-production,
   dashboard, hardening, coverage gate, final demo/report).

## Verify a fresh checkout at HEAD (no leftover code)

```powershell
git stash -u            # or: git checkout -- . + rm untracked dirs
node scripts/doctor.js
npm test                # 21/21
go build ./...          # clean at ead8dca
go test ./...           # all ok
```

The leftover Sigstore + storage code passes its own tests standalone; it was
only the half-edited `server.go` that could not compile. Nothing was pushed.
