# Audit

Append-only, hash-chained: `record_hash = H(previous_hash || canonical_payload)`,
genesis `GENESIS`. Events recorded: release created, source resolved, artifact
registered, OSS Rebuild checked, builder started/completed, artifact hashed,
attestation created/verified, quorum evaluated, decision made, policy changed,
builder registered/disabled, evidence anchored, chain tx confirmed.

Rules: no UPDATE/DELETE at the app layer (Postgres enforces via API-only
inserts); linkage checked on insert (Postgres, in-transaction prev-hash read)
and on read (`verify-chain`); `quorum audit verify` and
`POST /api/v1/audit/:id/verify-chain` report the exact broken record.
Tampering demo: mutate record 2 → `brokenAt: 1`; chain head also comparable
against the blockchain anchor. CLI, API, and Node harness share the identical
hash algorithm (conformance-covered inputs).
