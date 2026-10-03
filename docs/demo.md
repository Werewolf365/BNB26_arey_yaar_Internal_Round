# Demo (Slice 1)

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
