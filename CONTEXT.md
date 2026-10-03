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
- **CORS default origins for dev API**: when `QUORUM_CORS_ORIGINS` is not set the API now defaults to `http://localhost:3000` and `http://127.0.0.1:3000`, allowing the Next.js front‑end to fetch `/api/v1/*` without extra configuration.
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
go test ./...                              # all pkgs ok (incl. CORS change)
go build -o quorum.exe ./apps/cli; .\quorum.exe verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny
QUORUM_TEST_POSTGRES=1 go test -run TestPostgresIntegration ./apps/api/...
QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive ./internal/runner/
```

## Handoff / stop point (2026-10-03)

Last stable: `ead8dca` (green build/tests). The repo now includes the
CORS‑default change (`apps/api/main.go`) and the accompanying store
adjustments (`memory.go`, `postgres.go`). Uncommitted leftovers are
documented field-by-field in `docs/HANDOFF.md`: Sigstore package +
`attest` CLI complete with tests; storage backend complete with tests;
the API now has default `QUORUM_CORS_ORIGINS` for dev, enabling the
Next.js front‑end to reach `/api/v1/*` from `http://localhost:3000`.
Every new capability above has focused tests; work may continue from
this point.

## Map

`apps/cli` `apps/api` `services/{policy,canonical,attestation,audit,verification,oss-rebuild,builders,blockchain}`
`internal/{runner,exitcodes}` `packages/{schemas,fixtures}` `contracts/` `infra/compose` `docs/` `fixtures/` `scripts/`

## Change log — 2026-10-04: README rewrite + full-codebase review

- Read all root `.md` files (`README`, `CONTEXT`, `DESIGN`, `PRODUCT`,
  `CONTRIBUTING`, `SECURITY`, `demo_time`, `prompt.md` §§0–41) plus
  `docs/` (api, cli, architecture, blockchain, demo, limitations, HANDOFF),
  `Makefile`, `package.json`, `go.mod`, `infra/compose/docker-compose.yml`,
  `apps/api/main.go`, `apps/web/src/app/page.tsx`,
  `contracts/src/QuorumAnchor.sol`.
- Verified live: `npm test` 21/21 PASS, `go test ./services/policy/
  ./internal/runner/` ok, `node scripts/slice1-demo.mjs` 3/3 PASS
  (VERIFIED / REJECTED+ARTIFACT_HASH_MISMATCH / VERIFIED_WITH_CONFLICT,
  audit OK, anchor SKIPPED). Docker daemon up (29.1.3); compose services
  confirmed: `postgres`, `minio`, `anvil`.
- Rewrote `README.md`: was Slice-1-only (PowerShell, `quorum-api.exe`,
  no worker/web/cosign/storage/security/coverage). Now documents actual
  tree (cli/api/worker/web, sigstore/cosign/ossrebuild/storage, runner,
  contracts, compose ports 5433/4566/8545 + :8080/:3000), full Docker
  bring-up command, manual web test (seed builders/releases, open
  :3000), manual blockchain test (make test-contracts, TestAnchorLive,
  cast via docker foundry), CLI/worker loops, exit codes 0–5, API route
  table + env vars, testing gates, honest "What's lacking" section.
- No code changed; docs only (README.md + this CONTEXT.md entry).

## Change log — 2026-10-04: one-command Docker stack

- `docker compose up --build -d` now starts everything (postgres + anvil +
  api + web); previously only infra (postgres/minio/anvil) was composed and
  api/web/worker needed manual `go build` + exports + `npm run dev`.
- Added `infra/docker/api.Dockerfile` (Go multi-stage, non-root, curl
  healthcheck on `/api/v1/health`), `infra/docker/web.Dockerfile`
  (Node 20 multi-stage, `NEXT_PUBLIC_API_BASE` build arg, serves `:3000`),
  `infra/docker/worker.Dockerfile` (needs `/var/run/docker.sock`,
  `--profile worker` opt-in).
- Compose: api (Postgres + filesystem blobs by default, `restart:
  unless-stopped`, `depends_on` postgres healthy), web, port overrides
  (`QUORUM_API_PORT`/`QUORUM_WEB_PORT`), S3/MinIO moved to `--profile s3`
  opt-in (LocalStack `latest` now demands a license; `minio/minio` is the
  S3 target; empty `QUORUM_S3_ENDPOINT` = fs backend, same contract).
- Verified: full `up --build -d` on alternate ports (8080/3000 were taken
  by another project here), api `/health` + `/ready` ok, `POST /releases`
  201 persisted to Postgres, audit chain appended, web `/` 200 with title.
  Test api/web containers removed afterwards; postgres/anvil left running.
