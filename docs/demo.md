# Demo

## Full E2E (`make demo` -> `scripts/demo-e2e.mjs`)

Five phases, repeatable, temp keys/files cleaned up; API/Anvil phases SKIP
with reasons when the stack is down (never faked), `--strict` turns skips
into failures:

1. deterministic core (VERIFIED / conflict-surfaced / audit-ok, no binaries)
2. CLI matrix (verify exits 0/1/3, audit ok/tampered, attest round-trip +
   wrong-commit replay rejection)
3. API loop vs `QUORUM_API_BASE` (builders, release, VERIFIED verification,
   INVESTIGATE disagreement case, evidence blob hash round-trip, audit read)
4. job queue (enqueue, two worker claim/complete, unsigned completions settle
   INSUFFICIENT_EVIDENCE — signatures required, correctly)
5. chain anchor (live `TestAnchorLive` when Anvil is up, else SKIP)

`make demo-slice` keeps the original three-scenario Slice-1 script.

## Slice 1 (`node scripts/slice1-demo.mjs`)

```powershell
node scripts/slice1-demo.mjs
```

Expected output: Scenario 1 `VERIFIED`, Scenario 2 `REJECTED
(ARTIFACT_HASH_MISMATCH)`, Scenario 3 `VERIFIED_WITH_CONFLICT` with 1 conflict,
audit chain OK, anchor SKIPPED with explicit label (`BLOCKCHAIN_REQUIRED=false`).

Repeatable, no network, no credentials, cleans up after itself (in-memory).
Full `make demo` (infra + seed + OSS + builders + attest + quorum + persist +
anchor + dashboard URL + all 5 prompt scenarios) arrives with later phases.

## Real-repo run (executed 2026-10-03, `golang/example@7f05d21`, `hello/`)

- Clone pinned commit; `go build -trimpath` twice on host → identical
  `sha256:06d677b0…` ("Hello, world!" runs).
- Two network-less Docker rebuilds (`golang:1.27.1-bookworm`) → identical
  `sha256:57dab86a…`; differs from host (PE vs ELF) = honest
  EXPECTED_NONDETERMINISM across OS/ABI, reproducible within a platform.
- CLI: real repo+commit+file → `VERIFIED` (0); +1 byte → `REJECTED` (1);
  Linux bytes vs Windows digest → `REJECTED` (1); injected conflict →
  `VERIFIED_WITH_CONFLICT` (3).
- API: real release → host-vs-container evidence correctly yields
  `INVESTIGATE` with the minority surfaced (not a silent pass); agreeing
  evidence → `VERIFIED`; audit `ok:true`.
- Caveat: the VERIFIED API case reused one host digest under two group labels
  (same-host reruns, not true independence) — valid as a pipeline proof, not
  as an independence claim.

## Live OSS Rebuild run (executed 2026-10-03, `pypi:absl-py@2.0.0`)

- Downloaded the real wheel from files.pythonhosted.org; local sha256
  `9a28abb6…` equals the OSS Rebuild upstream digest exactly.
- `quorum verify pypi:absl-py@2.0.0 --repo https://github.com/abseil/abseil-py
  --commit 37dad4d3… --artifact <wheel> --oss-mode live` →
  **4 independent groups agree** (3 builders + oss-rebuild), source commit
  matches the rebuild's pinned commit, `SUPPORTED_AND_VERIFIED`, exit 0.
- Control case (pinned fixture vs live absl evidence): OSS row correctly
  **excluded with "source commit mismatch"** — visible in `excludedEvidence`,
  never counted, never hidden.
