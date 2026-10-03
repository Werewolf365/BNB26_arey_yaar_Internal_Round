# QUORUM — Context: achieved vs to-achieve

> "Don't Trust the Binary. Trust the Builders." Independent build verification
> with quorum policy, signed attestations, tamper-evident audit, EVM anchoring.
> Full vision: `prompt.md` (62 sections). Traceability: `docs/testing/REQUIREMENT-MATRIX.md`.

## Achieved (all executed, not claimed)

- **Quorum/policy engine** — deterministic, independence-group uniqueness, conflict
  visibility; Go source of truth + Node mirror locked by shared
  `packages/fixtures/conformance.json` (both suites fail on drift).
- **CLI** (`apps/cli`, Cobra): init/verify/inspect/builders/policy/evidence/audit/
  blockchain/doctor/version; exit contract 0–5 tested per command incl.
  `--json`/`--quiet`; errors always reach stderr.
- **Attestations**: standard DSSE + in-toto + SLSA v1.2, real sign/verify,
  tamper + replay rejection proven.
- **Builders**: 3 profiles, Docker-isolated network-less rebuild reproduced the
  tiny digest byte-for-byte; registry via CLI + API.
- **Backend API** (`apps/api`): all `docs/api.md` route groups on Postgres
  (migrations, FK/uniques, idempotency keys, concurrent-duplicate convergence,
  typed errors incl. a suffix-rule fix found by tests) + live release→VERIFIED→audit-ok loop.
- **Audit**: hash chain identical across CLI/API/Node; tamper pinpoints the record.
- **Blockchain**: `QuorumAnchor.sol` (5/5 forge tests), Dockerized Foundry,
  live Anvil deploys + anchors + event re-verification (txs recorded in `docs/blockchain.md`).
- **Stack**: compose postgres (host 5433 — 5432 squatted locally), S3-compat, Anvil 31337.
- **Docs**: README, SECURITY, architecture, trust/threat/security models, policies,
  builders, attestations, provenance, oss-rebuild, blockchain, audit, api, cli,
  testing, demo, limitations, TEST-MATRIX + REQUIREMENT-MATRIX (REQ-001…019).

## To achieve (ordered)

1. **Builder workers** — DONE: orchestrator queue + `quorum-worker`, real rebuild
   jobs end-to-end via API (live VERIFIED on `golang/example`). Remote/fleet
   builders remain future work.
2. **OSS Rebuild live path** — DONE: real CLI adapter (`services/ossrebuild`,
   all six states) + `verify --oss-mode live` (live 4-group VERIFIED on
   `absl-py 2.0.0`). Fixture mode stays default for deterministic CI.
3. **Sigstore/Cosign** — DONE (Slice): local ECDSA-P256 DSSE signing/verification,
   provenance statement, identity/builder policy checks, dev transparency log,
   `quorum attest` CLI + API metadata; **plus live adapter**: `services/cosign`
   shells the real CLI (`sign-blob`/`verify-blob` with explicit keys,
   bundle-preserving; `attest-blob`/`verify-blob-attestation` DSSE path with
   explicit-key or keyless-Fulcio trust anchor, keyless signing passed through
   as interactive OIDC) and `RekorClient` reads the public-good log
   (`PublicKey`/`Entry(uuid)`);
   `attest cosign-sign|cosign-verify|rekor-get|cosign-attest-blob|cosign-verify-attestation`
   wired with exit contract 0/1/4/5;
   deterministic stub+httptest suite, live paths env-gated (`QUORUM_LIVE_COSIGN=1`).
4. **Web dashboard** — Next.js: releases/builders/policy/audit/evidence pages + Playwright suite.
5. **Object storage wiring** — DONE: content-addressed filesystem backend mounted
   into the API (`POST /evidence` base64 upload, metadata rows in Postgres,
   `GET /evidence/:id/blob` download hash-verified on every read), plus an
   S3-compatible backend (`services/storage/s3.go`, in-file SigV4, same
   key/error contract) selected via `QUORUM_S3_ENDPOINT` with fail-fast
   startup; LocalStack compose service on `:4566` is the dev target
   (see `docs/api.md` "Object storage").
6. **Security hardening** — DONE (API slice): keyed auth on admin writes
   (`QUORUM_API_KEYS`, 401/403, constant-time compare), per-IP rate limits
   (429), security headers, allowlisted CORS, 1 MiB body caps, server
   timeouts; secrets never logged. Remaining: SBOM/signing/scans of Quorum
   itself, quotas beyond caps. Archive sandbox N/A: no extraction paths
   server-side (verified by grep); traversal/SSRF/size guards live in
   `services/verification/guards.mjs` + runner validators.
7. **Coverage gate** — DONE (measured 2026-10-04, `node scripts/coverage.mjs`):
   unit scope 93.6% (>=90 gate), criticals policy 100 / cosign 98.6 /
   sigstore 96.0 / runner 96.0 / server 97.8 (all >=95 gate). Locked
   per-package floors in the script + `make coverage` + CI slice1 enforce it.
   Integration-gated files (`postgres.go`, docker rebuild paths, live
   network paths) are covered by their own suites, excluded by design.
   REQ-016 PASS (unit scope; full 90-incl-gated needs docker+live in CI).
8. **Full demo + FINAL-REPORT** — 5 prompt scenarios, mutation/fuzz/chaos suites, `make test-all` green.

## Verify right now

```powershell
node scripts/doctor.js
npm test                                   # 21/21
node scripts/slice1-demo.mjs               # 3/3 scenarios
go test ./...                              # all pkgs ok
go build -o quorum.exe ./apps/cli; .\quorum.exe verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny
QUORUM_TEST_POSTGRES=1 go test -run TestPostgresIntegration ./apps/api/...
QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive ./internal/runner/
```

## Handoff / stop point (2026-10-03)

Last stable: `ead8dca` (green build/tests). Uncommitted leftovers are
documented field-by-field in `docs/HANDOFF.md`: Sigstore package +
`attest` CLI complete with tests; storage backend complete with tests;
API storage wiring intentionally left half-edited (only breakage, by design
of the pause). Every new capability above has focused tests; the
half-implemented server wiring is exactly where work resumes.

## Map

`apps/cli` `apps/api` `services/{policy,canonical,attestation,audit,verification,oss-rebuild,builders,blockchain}`
`internal/{runner,exitcodes}` `packages/{schemas,fixtures}` `contracts/` `infra/compose` `docs/` `fixtures/` `scripts/`
