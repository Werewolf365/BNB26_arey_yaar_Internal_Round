-- Quorum initial schema (apps/api/migrations/0001_init.sql).
-- PostgreSQL. Explicit constraints + indexes per prompt section 21.

CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS releases (
  id TEXT PRIMARY KEY,
  package TEXT NOT NULL,
  ecosystem TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  repo TEXT NOT NULL,
  commit TEXT NOT NULL,
  expected_digest TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'PENDING',
  idempotency_key TEXT UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_releases_commit ON releases(commit);
CREATE INDEX IF NOT EXISTS idx_releases_status ON releases(status);
CREATE INDEX IF NOT EXISTS idx_releases_created ON releases(created_at);

CREATE TABLE IF NOT EXISTS builders (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL DEFAULT '',
  independence_group TEXT NOT NULL,
  endpoint TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_builders_group ON builders(independence_group);

CREATE TABLE IF NOT EXISTS policies (
  id TEXT PRIMARY KEY,
  version TEXT NOT NULL,
  body JSONB NOT NULL,
  policy_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS verifications (
  id TEXT PRIMARY KEY,
  release_id TEXT NOT NULL REFERENCES releases(id) ON DELETE RESTRICT,
  policy_id TEXT NOT NULL DEFAULT '',
  decision TEXT NOT NULL,
  required INT NOT NULL DEFAULT 0,
  satisfied INT NOT NULL DEFAULT 0,
  body JSONB NOT NULL,
  idempotency_key TEXT UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_verifications_release ON verifications(release_id);
CREATE INDEX IF NOT EXISTS idx_verifications_decision ON verifications(decision);
CREATE INDEX IF NOT EXISTS idx_verifications_created ON verifications(created_at);

CREATE TABLE IF NOT EXISTS evidence_objects (
  id TEXT PRIMARY KEY,
  verification_id TEXT NOT NULL REFERENCES verifications(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_evidence_verification ON evidence_objects(verification_id);
CREATE INDEX IF NOT EXISTS idx_evidence_sha ON evidence_objects(sha256);

-- Append-only audit log. No UPDATE/DELETE allowed at the app layer;
-- record_hash chains via previous_hash (verified on read + on insert).
CREATE TABLE IF NOT EXISTS audit_records (
  id BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  verification_id TEXT NOT NULL DEFAULT '',
  previous_hash TEXT NOT NULL,
  record_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_verification ON audit_records(verification_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_records(created_at);

CREATE TABLE IF NOT EXISTS anchors (
  verification_id TEXT PRIMARY KEY REFERENCES verifications(id) ON DELETE RESTRICT,
  tx_hash TEXT NOT NULL,
  block_number TEXT NOT NULL DEFAULT '',
  chain_id TEXT NOT NULL,
  contract_address TEXT NOT NULL,
  event_found BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_anchors_tx ON anchors(tx_hash);
