# Testing (how to run everything)

Deterministic first: `go test ./...` and `npm test` need no daemon, no creds.
Live suites are env-gated and never block the default run.

| Suite | Command | Needs |
|---|---|---|
| Go unit (all pkgs) | `go test ./...` | Go 1.27.x |
| Node unit/security/conformance | `npm test` | Node 20+ |
| Slice demo (3 scenarios) | `node scripts/slice1-demo.mjs` / `make demo` | nothing |
| CLI binary battery | `go build -o quorum ./apps/cli` + `docs/cli.md` commands | nothing |
| Postgres integration | `QUORUM_TEST_POSTGRES=1 go test -run TestPostgresIntegration ./apps/api/...` | compose `postgres` (host **5433**) |
| Live Anvil anchor | `QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive ./internal/runner/` | compose `anvil` (8545) |
| Foundry contracts | `forge test` (host) or `make test-contracts` (Docker fallback) | forge or Docker image |
| Self-provisioned stack | `make test-integration` (starts postgres/minio/anvil, skips if daemon down) | Docker daemon |
| Real externals | `OSS_REBUILD_MODE=live ...` / `make test-external` | ADC creds, network — never in CI gate |
| Full gate | `make test-all` | above, minus externals |

Conventions: env-gated tests `Skip` (never fail) without their backend;
`Idempotency-Key` concurrency is tested, not assumed; every negative in the
44-fixture list (§25 prompt) lands with the phase that owns it — Slice/CLI/API
coverage is recorded in `docs/testing/REQUIREMENT-MATRIX.md` (REQ-001…019,
REQ-016 PARTIAL: coverage gate below 90/95 until the full platform lands).
