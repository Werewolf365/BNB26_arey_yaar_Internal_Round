# Requirement matrix (seed — Slice 1)

| REQ | Requirement | Implementation | Unit | Integration | E2E | Negative | Security | Status |
|-----|-------------|----------------|------|-------------|-----|----------|----------|--------|
| REQ-001 | Deterministic quorum (same evidence+policy=same decision) | services/policy/quorum.go + quorum.mjs | test/quorum.test.mjs, quorum_test.go | — | scripts/slice1-demo.mjs | invalid policy -> ERROR | — | PASS (node), PENDING CI (go) |
| REQ-002 | independence_group uniqueness (same group != independent) | quorum.go Evaluate | quorum tests | — | demo scn 1 | aws/aws must not verify | — | PASS |
| REQ-003 | Conflict visibility (minority never hidden) | quorum.go conflicts | quorum tests | — | demo scn 3 VERIFIED_WITH_CONFLICT | tolerance exceeded -> INVESTIGATE | — | PASS |
| REQ-004 | Standard attestation (DSSE+in-toto+SLSA v1.2), no custom crypto | services/attestation/dsse.mjs | test/attestation.test.mjs | cosign (later) | demo scn 1 | tamper->MISMATCH, replay->reject | signature/tamper/replay | PASS |
| REQ-005 | 1-byte compromise cannot pass | guards + attestation binding + demo scn 2 | attestation tests | — | demo scn 2 REJECTED | wrong digest rejected | security suite | PASS |
| REQ-006 | Tamper-evident audit (H(prev\|\|canonical)) | services/audit/chain.mjs | test/audit.test.mjs | — | demo audit check | modify record 2 -> brokenAt=1 | tamper test | PASS |
| REQ-007 | Input guards (traversal/SSRF/size) | services/verification/guards.mjs | test/security.guards.test.mjs | API (later) | — | traversal/SSRF/oversize | security suite | PASS |
| REQ-008 | OSS Rebuild fixture/live split (no creds in test-all) | services/oss-rebuild/adapter.mjs | fixtureLookup | test-external (live) | demo label | UNSUPPORTED/NOT_FOUND explicit | — | PASS (fixture) |
| REQ-009 | Go pinned toolchain + CI verify | go.mod (go1.27.1) | go test ./... PASS locally (go1.27.1 windows/amd64) + CI | compose stack live | — | — | PASS |
| REQ-010 | doctor detects, never installs; setup = project deps only | scripts/doctor.js, Makefile | manual | compose postgres+s3+anvil live | — | — | PASS |
| REQ-011 | Real isolated builder reproduces artifact digest | infra/docker/builder.Dockerfile + compose | docker run --network none reproduced sha256:125e91d8… (matches demo) | Anvil live chainId 0x7a69 | — | — | PASS |
| REQ-012 | Contract tests via Dockerized forge (no host install) | contracts/test/QuorumAnchor.t.sol | forge test: 5/5 PASS (happy/duplicate/zero-id/zero-evidence/all-decisions) | — | — | zero-value + duplicate reverts | PASS |
| REQ-013 | Live anchor: deploy + record + re-read + event verify | docs/blockchain.md | cast re-read anchored()==true, receipt event matches | Anvil 31337, contract 0x5FbD…80aa3, tx 0x8f27…f721 | — | forged-metadata rejected by re-read rule | PASS |
| REQ-014 | CLI: init/verify/inspect/builders/policy/evidence/audit/blockchain/doctor/version | apps/cli/{cmd,internal/runner} | cmd/cli_test.go + runner/runner_test.go (exit matrix 0-5, guards, registry, chains) | binary runs: exits 0/1/2/3/5 + live anchor tx 0xa58a…b87d event=true | tamper/conflict/oversize/traversal-metachar/SSRF/duplicate/replay-audit | input-guard suite | PASS |
| REQ-015 | CLI exit-code contract (prompt section 40) | exitcodes pkg + docs/cli.md | every command exit-tested incl --json/--quiet | binary $LASTEXITCODE battery | operational-vs-input separation | — | PASS |
| REQ-016 | Coverage gate (>=90% overall, >=95% critical) | — | cmd 85.8%, runner 70.2%, policy 79.6% (measured) | — | — | — | PARTIAL (gate applies at full test-all; CLI phase honest gap, more tests land with backend/frontend) |

Full matrix (all 62 prompt sections) grows in docs/testing/TEST-MATRIX.md as phases land.
