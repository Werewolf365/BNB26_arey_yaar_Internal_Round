-- Bind each builder identity to a trust root and preserve the signed result
-- supplied by its worker. The API verifies the envelope before setting
-- signature_valid; it is never client-controlled.
ALTER TABLE builders ADD COLUMN IF NOT EXISTS signing_public_key TEXT NOT NULL DEFAULT '';
ALTER TABLE build_jobs ADD COLUMN IF NOT EXISTS attestation TEXT NOT NULL DEFAULT '';
ALTER TABLE build_jobs ADD COLUMN IF NOT EXISTS signature_valid BOOLEAN NOT NULL DEFAULT FALSE;
