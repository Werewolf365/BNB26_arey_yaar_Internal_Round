# Contributing

- Read `CONTEXT.md`, `docs/architecture.md`, `docs/threat-model.md` first.
- Stack: Go 1.27.x (pinned in `go.mod`), Node 20+, Docker + compose.
- Workflow: `node scripts/doctor.js` → `npm test` → `go test ./...` → phase work →
  tests + docs + `REQUIREMENT-MATRIX.md` row update. Never weaken a failing test.
- Conventions: typed errors (`CODE: detail`), deterministic fixtures in default
  suites, live backends behind `QUORUM_TEST_*`/`OSS_REBUILD_MODE` env gates,
  canonical JSON for anything hashed, precise security language (see
  `docs/trust-model.md`).
- Commits: conventional, scope-prefixed (`cli:`, `api:`, `contracts:`, `docs:`…).
