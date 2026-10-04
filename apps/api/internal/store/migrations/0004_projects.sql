-- Onboarded GitHub projects (UI onboarding flow). One row per immutable
-- source: UNIQUE(repo, commit) prevents duplicate projects for one source.
-- Mirrors projects/*.json (demo-script format) field for field.

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  repo TEXT NOT NULL,
  tag TEXT NOT NULL DEFAULT '',
  commit TEXT NOT NULL,
  package TEXT NOT NULL DEFAULT '',
  ecosystem TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  build_kind TEXT NOT NULL DEFAULT 'git-archive',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(repo, commit)
);
CREATE INDEX IF NOT EXISTS idx_projects_repo ON projects(repo);
CREATE INDEX IF NOT EXISTS idx_projects_created ON projects(created_at);
