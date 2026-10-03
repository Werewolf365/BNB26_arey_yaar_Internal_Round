# QUORUM — Agent handoff / current state

> Updated after the interrupted Sigstore/storage session was resumed and fixed.

## Stable points

- `ead8dca` — last full green before the Sigstore/storage work began.
- The working tree now ALSO contains a **complete and tested** Sigstore slice and
  a **complete and tested** storage slice (both below). Commit pending verification.

## Pieces landed since `ead8dca` (all tests green)

| Piece | Status | Evidence |
|---|---|---|
| `services/sigstore` (DSSE + in-toto + SLSA v1.2, P-256 keys, policy verify, dev transparency log) | Complete, tested | `go test ./services/sigstore/` ok |
| `quorum attest genkey|sign|verify` (+ root wiring, envelope JSON error codes) | Complete, tested | `go test ./apps/cli/cmd/` ok incl. `attest_test.go` matrix |
| `services/storage` (content-addressed FS: traversal guards, size caps, atomic EXCL rename, concurrent-put, on-read hash verify) | Complete, tested | `go test ./services/storage/` ok |
| Storage wired into API (`POST /api/v1/evidence` base64 upload, metadata in store, `GET /api/v1/evidence/:id/blob` hash-verified raw download, `PUT /evidence` rejects sha mismatch) | Complete, tested | `go test ./apps/api/...` ok incl. `TestEvidenceBlobStorage`; Postgres integration `QUORUM_TEST_POSTGRES=1` ok |

## Prior breakage, now resolved

`apps/api/internal/server/server.go` was left half-edited (stale
`apps/cli/internal/runner` import, `New(st,blobs)` signature without blobs
field assignment, 1-arg callers in main/tests, missing storage import for
`storage.NewFilesystem`). All fixed: stale import corrected, struct literal
now passes `blobs`, `main.go` builds a `storage.Filesystem` from
`QUORUM_BLOB_DIR` (default `./data/evidence`), tests use a temp-dir store
via `newTestServer(t)`, new `TestEvidenceBlobStorage` covers put → metadata →
blob round-trip + sha-mismatch + 400 paths.

Also hardened earlier: `--json` CLI errors now emit `{"error":...}` on stderr
(never stdout, never silence); `emitErr` centralizes this in `cmd/output.go`.

## Remaining from the roadmap

1. Web dashboard (Next.js + Playwright).
2. ~~Cosign keyless/Fulcio + DSSE `verify-blob-attestation` policy path~~ DONE:
   `services/cosign/attest.go` (`AttestBlob` incl. keyless-sign passthrough,
   `VerifyBlobAttestation` with explicit-key or Fulcio-identity trust anchor)
   + `attest cosign-attest-blob | cosign-verify-attestation` wired with exit
   contract 0/1/4/5 (explicit-key blob sign/verify + Rekor reads were already
   live; see `services/cosign/`).
3. S3-compatible object storage — DONE: `services/storage/s3.go` (in-file SigV4,
   same key/error contract, fake-S3 + `QUORUM_LIVE_S3` live tests) selected via
   `QUORUM_S3_ENDPOINT` with fail-fast `EnsureBucket`; LocalStack `:4566` dev target.
4. Production hardening — DONE (API slice): `server.Config` (`QUORUM_API_KEYS`,
   rate/CORS knobs), 401/403/429 mapping, headers, body caps, timeouts, full
   negative-test matrices. Remaining: SBOM/signing/scans of Quorum itself.
5. Coverage gate — DONE: `scripts/coverage.mjs` (+ `make coverage`, CI slice1):
   unit 93.6% (>=90), criticals 95–100 (policy 100, cosign 98.6, sigstore 96.0,
   runner 96.0, server 97.8); per-package floors lock it. Gated files
   (postgres/docker/live) keep their own suites by design.
6. Full demo + FINAL-REPORT + mutation/fuzz/chaos suites.
