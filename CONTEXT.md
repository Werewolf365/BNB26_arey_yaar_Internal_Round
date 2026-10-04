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

## Change log — 2026-10-04: web detail routes + trust governance + E2E demo + supply-chain gates

- Web: shared `src/lib/api.ts` + `src/components/ui.tsx`; routes `/releases`,
  `/releases/[id]`, `/verifications/[id]` (decision, counted/excluded,
  conflicts, jobs, anchor), `/builders`, `/policies`, `/audit`, `/evidence`
  explorer (metadata + hash-verified blob links). Overview verification lookup
  replaces the hard-coded `setEvidence([])`. Playwright
  (`playwright.config.ts`, `tests/e2e/dashboard.spec.ts`, chromium + mobile):
  14/14 vs the compose prod build. Upgraded Next 14.2.23 -> 15.5.27
  (critical CVEs), React 19, postcss `overrides` pin; `npm audit` 0 vulns.
- Fulcio/Rekor governance: `services/cosign/trust.go` (TrustRoot JSON +
  `QUORUM_TRUST_ROOT`, exact/regexp identities, Rekor-required fail-closed),
  `attest cosign-verify-attestation --trust-root/--rekor-entry`,
  `fixtures/trust-root.example.json`, trust unit tests + CLI matrix. Keyless
  without a root passes through to cosign Fulcio validation (additive).
- Fuzz (`services/policy/quorum_fuzz_test.go`,
  `services/canonical/canonical_fuzz_test.go`, `make fuzz`, CI 60s runs):
  determinism, independence, conflict-visibility, canonical stability —
  6M+ execs clean AFTER fixing a real bug fuzz found: empty-digest evidence
  counted toward quorum (excluded with reason now, Go + Node mirror +
  `TestMalformedEvidenceExcluded` + Node mirror tests).
- Mutation (`scripts/mutate.mjs`, `make mutation`, test-all + CI): 6/6 killed;
  2 initial survivors led to new tiebreak + tolerance-zero tests.
- Supply chain: `scripts/sbom.mjs` (`make sbom`, syft-or-inventory),
  `make sign` (cosign, SKIP when absent), `scripts/scan.mjs` (`make scan`:
  vet + govulncheck + npm audit + secret-grep, all green),
  `.github/workflows/security.yml` (scan/mutation/fuzz), CI web job
  (typecheck/build/Playwright), `make test-web`.
- E2E: `scripts/demo-e2e.mjs` (`make demo`, old script kept as
  `make demo-slice`): deterministic core + CLI exit matrix + attest replay
  rejection + API VERIFIED/INVESTIGATE/blob/audit + jobs with unsigned-settles-
  INSUFFICIENT_EVIDENCE assertion + live anchor-or-SKIP. ALL PHASES PASS vs
  compose stack.
- Matrices: REQ-030…036 appended to `docs/testing/REQUIREMENT-MATRIX.md`;
  `docs/attestations.md` (trust roots), `docs/demo.md` (E2E), README
  rewritten sections.

## Change log — 2026-10-04: gate-verification pass (Makefile fix + live proofs)

- Fixed duplicate `test-integration` Makefile target (second definition was
  silently overriding the first and dropping the `go test` integration run;
  consolidated to one recipe: `compose up -d postgres anvil` + Postgres +
  Anvil live tests). `make -n test-integration` confirms a single recipe.
- Live anchor proof vs compose Anvil (`eth_blockNumber` 0xf, chain live):
  `QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive` deploys + anchors +
  re-reads + duplicate-revert check. Independent `cast` re-verification
  (deploy tx, `recordVerification` tx, receipt status/block, event logs)
  recorded below; foundry 1.8.1 needs `--mnemonic` (raw `--private-key`
  hex rejected by this build).
- Re-ran separate gates individually: `make sbom`, `make sign`, `make scan`,
  Playwright vs live stack, live cosign/Rekor probe results recorded below
  (env-gated paths report SKIP with reason when binaries/network absent —
  never faked).
- Live anchor receipts (compose Anvil, chainId 31337, foundry 1.8.1 —
  raw `--private-key` hex rejected by this build, `--mnemonic` used):
  deploy tx `0xc2c22f82dbe5f2a0e29cc905b83530e6a6a654ef4df2ab1a715de7d7c8477b27`
  status 1 -> contract `0x68B1D87F95878fE05B998F19b66F4baba5De1aed`;
  `recordVerification` tx
  `0x867eae1f7a38dfafe01e3feb9bca7ffb03451a3594758e87ad082800793259d5`
  block 20 status 1 with `VerificationAnchored` event (verificationId
  `0xaaaa…` as topic); independent re-read `anchored(0xaaaa…) == true`.
  `QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive` also green
  (deploy + anchor + event + duplicate-revert).
- Separate gates, run individually (not covered by `test-all` by design):
  `make sbom` -> 166-entry module inventory (syft absent, honest fallback);
  `make sign` -> explicit `[sign] SKIP: cosign absent` (fixed recipe that
  previously swallowed its own SKIP behind a pipe); `make scan` green
  (vet, govulncheck 0 vulns, npm audit web PASS, secret-grep PASS);
  Playwright 14/14 vs live compose stack; live Rekor
  `TestLiveRekorLiveness` PASS + CLI `rekor-get` bogus UUID -> live
  `NOT_FOUND` exit 1; live cosign sign/attest round-trips SKIP (no binary —
  deterministic stub path covers the logic in `test-all`).
