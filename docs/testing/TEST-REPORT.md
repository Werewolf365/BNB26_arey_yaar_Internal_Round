# Quorum test report (executed, 2026-10-03)

Everything below was run, not asserted. Rerun with `docs/testing.md`.

## 1. Unit suites (no daemon, no creds)

| Suite | Command | Result |
|---|---|---|
| Go all packages | `go test ./...` | ok (policy, canonical, runner, cmd, server, store) + `go vet` clean |
| Node (quorum, attestation, audit, guards, conformance, external-opt-in) | `npm test` | **21/21 pass** |
| Foundry contracts | `forge test` (Docker image `ghcr.io/foundry-rs/foundry:latest`, forge 1.8.1) | **5/5 pass** (happy path, duplicate revert, zero-id revert, zero-evidence revert, all 6 decisions) |
| Slice demo | `node scripts/slice1-demo.mjs` | 3/3: `VERIFIED`, 1-byte `REJECTED`, A=B≠C `VERIFIED_WITH_CONFLICT`, audit OK |

Coverage (measured `go test -cover`): server 81.0%, cmd 83.3%, runner 69.6%,
canonical 85.0%, policy 80.6%, store 33.8% (memory paths; Postgres paths via
integration). Gate 90/95 stays PARTIAL until the full platform lands (REQ-016).

## 2. Conformance (drift-proof mirrors)

`packages/fixtures/conformance.json` asserted by **both** Go
(`canonical_test.go`, `policy/conformance_test.go`) and Node
(`test/conformance.test.mjs`): canonical encodings, 4 quorum scenarios incl.
same-group-not-independent and order-independence. Either side drifting fails.

## 3. CLI matrix (in-process `cmd/cli_test.go` + real `quorum.exe` battery)

Exit contract 0–5 verified per command: verify valid/conflict/tamper/
insufficient/invalid (0/3/1/2/5), `--json` shape, `--quiet` silent-stdout with
errors still on stderr, package-ref form, policy validate/test incl. strict
3-of-3, builders register/list/duplicate/no-group, init + refuse-overwrite,
inspect, evidence export, audit demo valid + tampered (1), blockchain
unreachable→SKIPPED (0) vs `--required` (4), doctor, version.

## 4. Negative / security (all must fail loudly, never silently)

SSRF (http/host/metadata-IP/localhost/credentialed URLs), path metacharacters
(NUL/controls/`"*?<>|;&$\``), oversize artifacts, malformed JSON everywhere,
duplicate builders/policies/anchors, orphan verifications/anchors (FK),
replayed attestations (commit/subject mismatch), 1-byte artifact mutation,
audit tamper pinpointed at the exact record, forged chain metadata rejected by
mandatory on-chain re-read.

## 5. Integration (env-gated, compose stack)

- Postgres 16 (host **5433**; 5432 squatted by a machine-local instance):
  `QUORUM_TEST_POSTGRES=1 go test -run TestPostgresIntegration` — **PASS**:
  idempotent migrations, idempotent replays, concurrent-duplicate convergence,
  FK/unique enforcement, persisted chain verifies.
- Anvil 31337: `QUORUM_LIVE_ANVIL=1 go test -run TestAnchorLive` — **PASS**:
  deploy from forge artifact → anchor → re-read `true` → duplicate reverts.
- Manual anchors: deploy tx `0xb917…f93a` @ `0x5FbD…80aa3`; anchor txs
  `0x8f27…f721` (block 2), CLI tx `0xa58a…b87d` (block 3) — all events verified.
- Builder isolation: network-less Docker rebuild reproduced the tiny digest
  byte-for-byte.
- Live API loop: release → verification `VERIFIED` → audit `ok:true` (6 records).

## 6. Real repository run (`golang/example@7f05d21`, `hello/`)

Host `go build -trimpath` ×2 → identical `sha256:06d677b0…` (runs "Hello,
world!"); Docker ×2 → identical `sha256:57dab86a…` (PE vs ELF = honest
cross-OS nondeterminism). CLI: real file `VERIFIED` (0), +1 byte `REJECTED`
(1), cross-platform `REJECTED` (1), conflict `VERIFIED_WITH_CONFLICT` (3).
API: host-vs-container evidence → `INVESTIGATE` with minority surfaced;
agreeing evidence → `VERIFIED`. Full detail: `docs/demo.md`.

## 7. Builder workers end-to-end (live, `golang/example@7f05d21`)

API on Postgres + `quorum-worker --once` ×2 (owners worker-1/2): claim →
fetch pinned commit → network-none sandbox archive → complete. Both workers
reported identical `sha256:39193228…`; API auto-evaluated `VERIFIED`
(`fromJobs:true`) with full audit trail (register → release → evaluated →
enqueued → completed ×2 → evaluated). Failure path tested (3× fail →
`FAILED` terminal, insufficient evaluation). Cross-toolchain archive drift
(host git 2.51 vs container 2.39.5) proven and documented — comparability
requires homogeneous builder images.

## 8. OSS Rebuild live path (executed 2026-10-03)

Real CLI installed (`go install .../cmd/oss-rebuild@latest`) and probed:
`list`/`get` need no credentials; `--verify` on by default. Adapter
(`services/ossrebuild`) tested live (`QUORUM_LIVE_OSS=1`): `pypi:absl-py@2.0.0`
→ `SUPPORTED_AND_VERIFIED` with pinned upstream digest
`sha256:9a28abb6…` + source commit `37dad4d…`, raw payload preserved
(first-document in-toto/SLSA validated); bogus version → `NOT_FOUND`;
`docker:` ref → `UNSUPPORTED`. Two real CLI quirks found by failing tests and
fixed: human markers on stderr, multi-document payload output.
CLI wiring (`--oss-mode live`): real wheel downloaded (local sha256 matches
upstream exactly) → **4 independent groups agree**, exit 0; drift control case
excludes the OSS row with "source commit mismatch". JSON-mode errors now emit
`{"error":…}` envelopes on stderr (tested).

## 10. Ten-target resilience battery (large-corpus proof)

`docs/testing/TARGET-BATTERY.md` (auto-regenerable via
`python scripts/target-battery.py`): every target was cloned, archived via
`git archive`, passed through the CLI **verify / tamper / conflict** triple,
and had a real OSS Rebuild Rebuild probe (`list` + first-version `get`).
All 10 repos: VERIFIED / REJECTED-with-digest-mismatch / conflict minority
surfaced. Test count, paths, commands, and sha prefixes are recorded there.`make test-all` = deterministic only. Live externals (real OSS Rebuild/ADC,
testnets) live in `make test-external` and never gate CI. CI runs unit +
`quorum verify`/`audit verify` smoke + a Postgres-service integration job.
