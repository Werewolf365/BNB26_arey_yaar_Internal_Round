-- Builder worker job queue (migration 0002).
-- One row per (verification, builder). Workers claim with leases so a dead
-- worker's job becomes visible again instead of stuck.

CREATE TABLE IF NOT EXISTS build_jobs (
  id TEXT PRIMARY KEY,
  verification_id TEXT NOT NULL REFERENCES verifications(id) ON DELETE RESTRICT,
  builder_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'QUEUED',
  attempts INT NOT NULL DEFAULT 0,
  max_attempts INT NOT NULL DEFAULT 3,
  result_digest TEXT NOT NULL DEFAULT '',
  result_commit TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_detail TEXT NOT NULL DEFAULT '',
  lease_owner TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  idempotency_key TEXT UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (verification_id, builder_id)
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON build_jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_verification ON build_jobs(verification_id);
CREATE INDEX IF NOT EXISTS idx_jobs_lease ON build_jobs(lease_expires_at) WHERE status IN ('CLAIMED', 'RUNNING');
