# Quorum: Don't Trust the Binary. Trust the Builders.

Independent build verification with quorum policy, signed DSSE/in-toto
attestations (SLSA v1.2), tamper-evident audit, and optional EVM anchoring.

> `VERIFIED` means "N independent builders reproduced this digest from this
> pinned commit under this policy" — never "safe". See `docs/limitations.md`.

## Status

Working: quorum engine (Go source of truth + Node mirror, conformance-locked),
Cobra CLI, REST API (memory + Postgres), worker + job queue with signed
attestations, DSSE/local P-256 + real Cosign/Rekor paths, OSS Rebuild
fixture + live adapters, content-addressed storage (fs + S3-compat),
`QuorumAnchor.sol` (hash-only) + Anvil flow, audit hash chain, Next.js
dashboard (overview only), Docker Compose stack, CI.

Not yet: full dashboard (release/builder/policy/audit detail pages,
Playwright), remote/fleet builders, SBOM/signing/scans of Quorum itself,
public testnet deployment, `make demo` full E2E. See "What's lacking" below
and `docs/limitations.md`.

## Architecture

```text
CLI (Go/Cobra) ──┐
                 ├─► API (:8080, Postgres, S3-compat, DB job queue) ─► workers (isolated rebuild, signed digest)
Web (Next.js) ───┘         │                    │                    └─► quorum engine ─► decision + conflicts
                           │                    ├─► OSS Rebuild adapter (fixture default, live opt-in)
                           │                    ├─► DSSE / in-toto+SLSA verify (local P-256 or cosign)
                           │                    ├─► audit hash chain ─► object storage (sha256/<digest>)
                           │                    └─► QuorumAnchor.sol (hash-only + event, Anvil local)
```

Full flow: pinned `repo@commit` → builders A/B/C rebuild in isolation
(`independence_group` each) → SHA-256 + provenance → signed attestation →
quorum policy eval (`VERIFIED` / `VERIFIED_WITH_CONFLICT` / `REJECTED` /
`INSUFFICIENT_EVIDENCE`) → audit chain append → optional chain anchor.

## Layout

- `apps/cli/` — Cobra CLI (`docs/cli.md`), entry `main.go`, commands in `cmd/`
- `apps/api/` — REST API `:8080` (`docs/api.md`); `main.go` + `internal/server/` + `internal/store/` (memory/postgres + migrations)
- `apps/worker/` — rebuild daemon: claim job → Docker `--network none` rebuild → signed report (`docs/workers.md`)
- `apps/web/` — Next.js 14 dashboard (`src/app/page.tsx`, single overview page)
- `services/policy/` — quorum engine: `quorum.go` (truth, Go) + `quorum.mjs` (mirror, locked via `packages/fixtures/conformance.json`)
- `services/canonical/` — canonical JSON for hashing
- `services/attestation/` + `services/sigstore/` — DSSE + in-toto + SLSA v1.2 sign/verify (test P-256 keys; Cosign later/live)
- `services/cosign/` — real `cosign` CLI + Rekor adapter (stub default, live via `QUORUM_LIVE_COSIGN=1`)
- `services/ossrebuild/` (+ `services/oss-rebuild/adapter.mjs`) — OSS Rebuild CLI adapter, fixture default, live opt-in
- `services/audit/` (`chain.mjs`) + `internal/runner/audit.go` — append-only hash chain `H(prev||canonical(payload))`
- `services/storage/` — fs backend + S3-compat backend (in-file SigV4)
- `services/verification/guards.mjs` + `internal/runner/guards.go` — traversal/SSRF/size guards
- `internal/runner/` — shared verify/policy/audit/builders/anchor logic (CLI + API)
- `internal/exitcodes/` — exit-code contract 0–5
- `contracts/src/QuorumAnchor.sol` — hash-only anchor + `VerificationAnchored` event; `test/QuorumAnchor.t.sol`
- `packages/schemas/` + `packages/fixtures/conformance.json` — JSON schemas + Go/Node vectors
- `infra/compose/docker-compose.yml` — postgres + S3 (LocalStack) + Anvil; `infra/docker/builder.Dockerfile`
- `scripts/` — `doctor.js` (detect-only), `slice1-demo.mjs` (3 scenarios), `coverage.mjs` (90/95 gates)
- `fixtures/` — `tiny-package/`, `conflicting-builders/`, `mismatch/`
- `docs/` — architecture, api, cli, blockchain, demo, limitations, testing matrices, HANDOFF

## Prerequisites

- Go 1.27.1 (pinned in `go.mod`), Node 20+, Docker + Compose, Git
- Optional: `forge`/`cast`/`anvil` (or Docker Foundry image), `cosign`, `oss-rebuild` CLI + creds (live paths only)

```bash
node scripts/doctor.js   # detection only; prints install hints, installs nothing
```

## Quickstart (no Docker, no creds)

```bash
export PATH="$HOME/.nvm/versions/node/v20.20.0/bin:$PATH"  # if node is via nvm
npm test                        # 21 unit/security/conformance tests
node scripts/slice1-demo.mjs    # VERIFIED / REJECTED / VERIFIED_WITH_CONFLICT + audit OK
go test ./...                   # full Go suite
go build -o quorum ./apps/cli && ./quorum verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny
```

`make` wraps the same: `make doctor/test/demo/test-all/coverage/lint`.
`make setup` installs project deps only — never your OS toolchain.

## Full stack with Docker (one command)

```bash
docker compose -f infra/compose/docker-compose.yml up --build -d
```

That builds and starts **postgres + anvil + api + web**. No exports needed —
defaults are baked into the compose file (Postgres on host **5433**, API
`:8080`, web `:3000`, filesystem blob store, CORS for `:3000`).

```bash
docker compose -f infra/compose/docker-compose.yml ps
curl localhost:8080/api/v1/health && curl localhost:8080/api/v1/ready
# open http://localhost:3000
docker compose -f infra/compose/docker-compose.yml down        # keep data
docker compose -f infra/compose/docker-compose.yml down -v    # wipe pg + blobs
```

Compose services:

| service  | image                        | host port | notes                              |
|----------|------------------------------|-----------|------------------------------------|
| postgres | `postgres:16-alpine`        | **5433**→5432 | 5432 often taken locally         |
| anvil    | `ghcr.io/foundry-rs/foundry` | 8545→8545 | local chain, chainId 31337        |
| api      | built (`infra/docker/api.Dockerfile`) | 8080→8080 | Postgres + fs blobs by default |
| web      | built (`infra/docker/web.Dockerfile`) | 3000→3000 | `NEXT_PUBLIC_API_BASE` baked at build |
| minio    | `minio/minio` (**opt-in**)   | 4566→4566 | S3-compat, `--profile s3` only   |
| worker   | built (**opt-in**)           | —         | `--profile worker`, needs Docker socket |

Port clashes? Override host ports (container ports stay):

```bash
QUORUM_API_PORT=18080 QUORUM_WEB_PORT=13000 \
NEXT_PUBLIC_API_BASE=http://localhost:18080 \
docker compose -f infra/compose/docker-compose.yml up --build -d
# then use http://localhost:18080 for the API (rebuild web so it points there)
```

Opt-ins:

```bash
# S3-compat blobs instead of filesystem (same key/error contract)
docker compose -f infra/compose/docker-compose.yml --profile s3 up --build -d
QUORUM_S3_ENDPOINT=http://minio:4566 QUORUM_API_PORT=8080 \
docker compose -f infra/compose/docker-compose.yml up --build -d api
# default MinIO creds: test / test12345 (override QUORUM_S3_SECRET_KEY)

# builder worker (mounts /var/run/docker.sock for isolated rebuilds)
docker compose -f infra/compose/docker-compose.yml --profile worker up --build -d worker
```

`make dev` is the same one-command start. The old manual flow (separate `go
build`, exports, `npm run dev`) still works for live-code dev; see git history
for the exact exports.

## Manual test: web dashboard

Requires API running (above). The page fetches `GET /api/v1/releases?limit=25`,
`/builders`, `/policies`, `/audit` from `NEXT_PUBLIC_API_BASE` (default
`http://localhost:8080`).

```bash
# seed something to look at (needs X-Api-Key when QUORUM_API_KEYS is set)
curl -X POST localhost:8080/api/v1/builders -H 'X-Api-Key: dev-admin-key' -H 'Content-Type: application/json' \
  -d '{"id":"builder-a","displayName":"demo A","independenceGroup":"operator-a"}'
curl -X POST localhost:8080/api/v1/releases -H 'X-Api-Key: dev-admin-key' -H 'Content-Type: application/json' \
  -d '{"package":"tiny","ecosystem":"demo","name":"tiny","version":"1","repo":"https://github.com/example/tiny","commit":"abc123"}'
curl 'localhost:8080/api/v1/audit?limit=20'

# then open http://localhost:3000 — hero cards + releases/builders/policies/audit sections
```

CORS: API defaults to allowing `http://localhost:3000` and
`http://127.0.0.1:3000` when `QUORUM_CORS_ORIGINS` is unset.

## Manual test: blockchain (Anvil, no testnet needed)

Contract stores hashes only, never artifacts/secrets. Owner-only
`recordVerification(...)`, duplicate-protected, emits `VerificationAnchored`.

```bash
make test-contracts   # forge tests (host forge or ghcr.io/foundry-rs/foundry via docker)

# live anchor round-trip against compose anvil (needs docker anvil on :8545)
QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive ./internal/runner/ -v

# manual: deploy + anchor + read back with cast inside docker foundry
docker run --rm --network host -v "$PWD/contracts:/project" -w /project \
  --entrypoint cast ghcr.io/foundry-rs/foundry:latest \
  send --private-key 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7b2dc3c --rpc-url http://localhost:8545 --create "$(cat src/QuorumAnchor.sol)"
# note: foundry ≥1.8 uses a new default mnemonic — prefer the key printed by `docker logs <anvil-container>`
# anchor + verify event per docs/blockchain.md; backend must re-read the chain, never trust client tx metadata
```

Deployed local example in `docs/blockchain.md` (contract
`0x5FbDB2315678afecb367f032d93F642f64180aa3`, chainId 31337).
Unreachable chain with default `BLOCKCHAIN_REQUIRED=false` → `SKIPPED`,
not a silent success; `--required` makes it an operational failure (exit 4).

## Manual test: CLI verify + worker end-to-end

```bash
# deterministic fixture: valid / tripwire / conflict / audit negative control
./quorum verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny
./quorum verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny --expected-digest sha256:0000000000000000000000000000000000000000000000000000000000000000
./quorum verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny --inject-conflict builder-c --verbose
./quorum policy test --policy-file apps/cli/testdata/policy-a.json --evidence-file fixtures/conflicting-builders/case.json
./quorum audit verify --demo && ./quorum audit verify --demo --tamper

# signed provenance (disposable keys only)
./quorum attest genkey --private /tmp/b.pem --public /tmp/b.pub.pem
echo -n demo-artifact > /tmp/a.bin
./quorum attest sign --artifact /tmp/a.bin --commit abc123 --builder builder-a --key /tmp/b.pem --output /tmp/a.dsse.json
./quorum attest verify --envelope /tmp/a.dsse.json --key /tmp/b.pub.pem --expect-commit abc123 --allow-builder builder-a

# worker loop (API must run; one worker per operator host in prod — same-machine demo proves protocol, not independence)
./quorum attest genkey --private builder-a.pem --public builder-a.pub.pem
./quorum attest genkey --private builder-b.pem --public builder-b.pub.pem
# register builders, create release + verification, enqueue jobs (see docs/demo.md judge script), then:
./quorum-worker --api http://localhost:8080 --owner operator-a --signing-key builder-a.pem --once
./quorum-worker --api http://localhost:8080 --owner operator-b --signing-key builder-b.pem --once
```

Exit codes: `0` verified · `1` rejected · `2` insufficient evidence ·
`3` conflict/investigation · `4` operational · `5` invalid input.
`audit verify`: 0 valid / 1 tampered / 5 bad input. `doctor`: 0 / 4.

## Backend API (summary)

Base `/api/v1`, JSON, every response carries `requestId`;
errors `{code,message,requestId}`. Full reference: `docs/api.md`.

```text
POST/GET /releases  GET /releases/:id
POST /verifications  GET /verifications/:id  POST /verifications/:id/reverify
POST /verifications/:id/jobs  GET /verifications/:id/jobs
POST /jobs/claim  GET /jobs/:id  POST /jobs/:id/complete
POST /evidence  GET /evidence/:id  GET /evidence/:id/blob  GET /attestations/:id
GET/POST /builders  PATCH /builders/:id   (admin: X-Api-Key)
GET/POST /policies  POST /policies/validate
GET /audit  GET /audit/:id  POST /audit/:id/verify-chain
POST /blockchain/anchor  GET /blockchain/:verificationId
GET /health  GET /ready
```

Env: `QUORUM_ADDR` (:8080), `QUORUM_DATABASE_URL`, `QUORUM_BLOB_DIR`
(`/data/evidence` in compose), `QUORUM_S3_ENDPOINT/BUCKET/REGION/ACCESS_KEY/SECRET_KEY/PATH_STYLE`
(S3 opt-in, `--profile s3`), `QUORUM_API_KEYS`, `QUORUM_CORS_ORIGINS`,
`QUORUM_RATE_LIMIT_RPS/BURST`, `QUORUM_TEST_POSTGRES`, `QUORUM_LIVE_ANVIL`,
`QUORUM_LIVE_COSIGN`, `QUORUM_LIVE_S3`. Host-port overrides:
`QUORUM_API_PORT` / `QUORUM_WEB_PORT`; web API target:
`NEXT_PUBLIC_API_BASE` (build arg).

## Testing

```bash
npm test                                        # deterministic Node suite (21 tests)
go test ./...                                   # full Go suite
go vet ./... && node scripts/coverage.mjs       # gates: 90% unit scope, 95% criticals
make test-contracts                             # forge (skips gracefully if absent)
QUORUM_TEST_POSTGRES=1 go test -run TestPostgresIntegration ./apps/api/...
QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive ./internal/runner/
QUORUM_LIVE_COSIGN=1 go test -run TestLive ./services/cosign/
QUORUM_LIVE_S3=1 go test -run TestLiveS3 ./services/storage/
OSS_REBUILD_MODE=live ...                       # real OSS Rebuild — make test-external only, needs creds
make test-all                                   # deterministic gate (never live/creds)
```

Matrices: `docs/testing/TEST-MATRIX.md`, `docs/testing/REQUIREMENT-MATRIX.md`
(REQ-001…019). Coverage measured 2026-10-04: unit 93.6%, policy 100 /
cosign 98.6 / sigstore 96.0 / runner 96.0 / server 97.8.

## Security model (short)

Signature valid ≠ safe. Rebuild match ≠ benign source. 2-agree ≠ absolute.
Every decision exposes predicates, counted/excluded evidence, conflicts.
API: keyed admin writes, per-IP rate limits, security headers, allowlisted
CORS, 1 MiB caps, timeouts, no secret logging. Builders run Docker-isolated,
`--network none`, never on the API server. See `SECURITY.md`,
`docs/threat-model.md`, `docs/trust-model.md`, `docs/security-model.md`.

## What's lacking (honest)

- Web is one overview page: no release/builder/policy/audit detail routes,
  evidence section hard-coded empty, no Playwright suite yet.
- Same-host Docker builders prove the protocol, not infrastructure
  independence (needs separate clouds/operators/creds/monitoring).
- Local P-256 signing is protocol proof; prod needs Cosign/Fulcio/Rekor
  trust-root governance (live adapters exist, env-gated).
- S3 impl done; bucket policy/retention/backups/creds are deployment work.
- Anvil demo only; public chain needs key custody, RPC resilience, fee policy.
- Quorum's own SBOM/signing/scans, quotas beyond caps, k8s manifests, full
  `make demo` E2E + FINAL-REPORT + mutation/fuzz/chaos: still roadmap
  (`prompt.md` §§26–33, `CONTEXT.md` "To achieve").

## Docs index

`docs/architecture.md` · `api.md` · `cli.md` · `blockchain.md` · `demo.md` ·
`builders.md` · `workers.md` · `attestations.md` · `provenance.md` ·
`oss-rebuild.md` · `quorum-policies.md` · `audit.md` · `trust-model.md` ·
`threat-model.md` · `security-model.md` · `limitations.md` · `testing.md` ·
`testing/{TEST-MATRIX,REQUIREMENT-MATRIX}.md` · `HANDOFF.md` ·
`../demo_time.MD` (judge runbook) · `../CONTEXT.md` (achieved vs to-achieve).
