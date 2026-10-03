# Quorum: Don't Trust the Binary. Trust the Builders.

Independent build verification with quorum policy, signed DSSE/in-toto attestations
(SLSA v1.2), tamper-evident audit, and optional EVM anchoring.

## Status: Slice 1 COMPLETE (deterministic, no external creds)

- Tiny fixture -> Builder A -> SHA-256 -> DSSE/in-toto attestation -> quorum -> VERIFIED
- 1-byte tamper -> REJECTED (`ARTIFACT_HASH_MISMATCH`)
- A=B=abc, C=def under 2/3 policy -> VERIFIED_WITH_CONFLICT (minority surfaced)
- Audit hash chain + input guards + OSS fixture/live split + `doctor` (detect-only)

## Quickstart (Windows PowerShell friendly)

```powershell
node scripts/doctor.js   # detection only; prints install hints, installs nothing
npm test                 # 21 unit/security/conformance tests, no Docker, no creds
node scripts/slice1-demo.mjs   # the three fundamental behaviors
```

`make` targets wrap the same (`make doctor/test/demo/test-all`). `make setup`
installs project deps only — never your OS toolchain. Live external services
(real OSS Rebuild, testnets) are `make test-external` only (`OSS_REBUILD_MODE=live`,
needs ADC creds). Daemon-backed integration: `make test-integration`
(`QUORUM_TEST_POSTGRES=1`, `QUORUM_LIVE_ANVIL=1`; compose maps Postgres to host
**5433** because 5432 is often taken by a machine-local PostgreSQL).

## Backend API (new)

```powershell
make build-api
$env:QUORUM_DATABASE_URL="postgres://quorum:quorum@localhost:5433/quorum?sslmode=disable"
./quorum-api.exe   # :8080; memory store when the env var is unset (dev only)
```

Routes + typed error codes: `docs/api.md`. Live loop proven: release → verification
(`VERIFIED`) → audit `ok:true`, all against Postgres + Anvil.

## Layout

- `services/policy/` — quorum engine: `quorum.go` (source of truth, Go 1.27 pinned) + `quorum.mjs` (Node mirror, conformance-locked via `packages/fixtures/conformance.json`)
- `internal/runner/` — shared verify/policy/audit/builders/anchor logic (CLI + API)
- `internal/exitcodes/` — exit-code contract 0–5
- `apps/cli/` — Cobra CLI (`docs/cli.md`)
- `apps/api/` — REST API + Postgres (`docs/api.md`; migrations embedded in store)
- `services/attestation/` — standard DSSE + in-toto + SLSA provenance v1 (test keys; Cosign later)
- `services/audit/` — append-only hash chain
- `services/verification/guards.mjs` — traversal / SSRF / size guards
- `services/oss-rebuild/` — fixture-vs-live adapter boundary
- `contracts/src/QuorumAnchor.sol` — hash-only anchor + event
- `docs/` — architecture, attestations, oss-rebuild, limitations, testing matrices
- `fixtures/tiny-package/` — deterministic demo package

## Security model (short)

Signature valid != safe. Rebuild match != benign source. 2-agree != absolute.
Every decision exposes predicates, counted/excluded evidence, and conflicts.
See `docs/`, `SECURITY.md`, `docs/limitations.md`, `docs/HANDOFF.md`.
