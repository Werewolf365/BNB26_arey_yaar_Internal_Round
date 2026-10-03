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
