# Backend API contract (to be implemented; clients/CLI must follow this)

Base: `/api/v1`. JSON everywhere. Every response carries `requestId`.
Errors: `{ "code": "TYPED_CODE", "message": "human text", "requestId": "..." }`
with HTTP mapping: 400 INVALID_INPUT / MALFORMED_*, 401/403 auth, 404 *_NOT_FOUND,
409 conflicts/duplicates, 422 POLICY_VIOLATION / INSUFFICIENT_EVIDENCE,
500 INTERNAL_ERROR, 502 OSS_REBUILD_UNAVAILABLE / BLOCKCHAIN_UNAVAILABLE,
504 BUILDER_TIMEOUT.

## Typed error codes (prompt section 36 — shared with CLI)

```
INVALID_INPUT, SOURCE_NOT_FOUND, COMMIT_NOT_FOUND, ARTIFACT_NOT_FOUND,
ARTIFACT_HASH_MISMATCH, ARTIFACT_OVERSIZED, ATTESTATION_INVALID,
SIGNATURE_INVALID, SIGNER_NOT_TRUSTED, BUILDER_FAILED, BUILDER_TIMEOUT,
BUILDER_CONFLICT, OSS_REBUILD_UNAVAILABLE, OSS_REBUILD_UNSUPPORTED,
INSUFFICIENT_EVIDENCE, POLICY_VIOLATION, AUDIT_TAMPERED,
BLOCKCHAIN_UNAVAILABLE, BLOCKCHAIN_REVERTED, INTERNAL_ERROR
```

Codes are stable strings: CLI prints them, UI renders them, tests assert them.
New codes require a docs + matrix update. Never return raw internal errors.

## Routes (prompt section 17)

```
POST   /api/v1/releases
GET    /api/v1/releases
GET    /api/v1/releases/:id

POST   /api/v1/verifications                 # runs the pipeline (same runner semantics as CLI)
GET    /api/v1/verifications/:id
POST   /api/v1/verifications/:id/reverify
POST   /api/v1/verifications/:id/jobs      # enqueue builder rebuild jobs
GET    /api/v1/verifications/:id/jobs

POST   /api/v1/jobs/claim                  # workers poll: {owner, leaseSeconds}
GET    /api/v1/jobs/:id
POST   /api/v1/jobs/:id/complete           # {ok, digest, commit, errorCode, errorDetail};
                                           # when all siblings terminal, auto-evaluates quorum

POST   /api/v1/evidence                    # submit blob: {verificationId, kind, sha256?, data(base64)}
GET    /api/v1/evidence/:id                # metadata
GET    /api/v1/evidence/:id/blob           # raw blob, hash-verified on read
GET    /api/v1/attestations/:id

GET    /api/v1/builders
POST   /api/v1/builders
PATCH  /api/v1/builders/:id

GET    /api/v1/policies
POST   /api/v1/policies
POST   /api/v1/policies/validate

GET    /api/v1/audit
GET    /api/v1/audit/:id
POST   /api/v1/audit/:id/verify-chain

POST   /api/v1/blockchain/anchor             # submit + receipt + event verify (same as CLI)
GET    /api/v1/blockchain/:verificationId

GET    /api/v1/health
GET    /api/v1/ready                         # 200 only when DB reachable
```

## Rules (all endpoints)

- Request validation + response/error schemas (this doc is normative).
- Auth for administrative writes (builders PATCH, policies POST); reads open locally.
- Rate limiting on externally exposed routes; structured logs with
  `request_id + verification_id + release_id + builder_id`; no secrets in logs.
- Idempotency: POST releases/verifications accept `Idempotency-Key`; repeated
  delivery returns the original record, concurrent duplicates never corrupt state.
- Pagination on list endpoints (`?limit&cursor`); UUIDs for ids; RFC3339 timestamps.
- Persistence: PostgreSQL (migrations in `apps/api/migrations/`), large blobs
  (attestations, bundles, logs) in object storage under `sha256/<digest>`.
- Decisions reuse `services/policy` (single engine — no reimplementation).
- Empty evidence on verification create records `INSUFFICIENT_EVIDENCE`
  (orchestrator pending state for job-driven flows), not a 422.
- Build jobs: `QUEUED → CLAIMED → SUCCEEDED|FAILED`, leases with `SKIP LOCKED`
  claiming, retries to `maxAttempts`, terminal duplicates rejected; enqueue
  requires registered builders; completion triggers quorum auto-evaluation.
