package store

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var migrationOrder = []string{"0001_init.sql", "0002_jobs.sql"}

// Postgres is the production Store.
type Postgres struct {
	pool *pgxpool.Pool
}

// Connect opens the pool (QUORUM_DATABASE_URL, e.g. postgres://quorum:quorum@localhost:5432/quorum).
func Connect(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("OPERATIONAL: bad database URL: %v", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("OPERATIONAL: db unreachable: %v", err)
	}
	return nil
}

func (p *Postgres) Close() { p.pool.Close() }

// Migrate applies pending migrations in order, idempotently.
func (p *Postgres) Migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("OPERATIONAL: cannot init migration tracking: %v", err)
	}
	for _, name := range migrationOrder {
		version := strings.TrimSuffix(name, ".sql")
		var applied bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("OPERATIONAL: missing embedded migration %s: %v", name, err)
		}
		if err := p.applyMigration(ctx, version, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) applyMigration(ctx context.Context, version, sql string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("OPERATIONAL: migration %s failed: %v", version, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1) ON CONFLICT DO NOTHING`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFoundError(err error, code, id string) error {
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%s: %s", code, id)
	}
	return err
}

func (p *Postgres) CreateRelease(ctx context.Context, r Release, key string) (Release, bool, error) {
	if key != "" {
		var existing Release
		err := p.pool.QueryRow(ctx, `SELECT id, package, ecosystem, name, version, repo, commit, expected_digest, status, created_at FROM releases WHERE idempotency_key=$1`, key).
			Scan(&existing.ID, &existing.Package, &existing.Ecosystem, &existing.Name, &existing.Version, &existing.Repo, &existing.Commit, &existing.ExpectedDigest, &existing.Status, &existing.CreatedAt)
		if err == nil {
			return existing, true, nil
		}
	}
	r.ID = newID("rel")
	r.CreatedAt = time.Now().UTC()
	if r.Status == "" {
		r.Status = "PENDING"
	}
	var k *string
	if key != "" {
		k = &key
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO releases(id, package, ecosystem, name, version, repo, commit, expected_digest, status, idempotency_key, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		r.ID, r.Package, r.Ecosystem, r.Name, r.Version, r.Repo, r.Commit, r.ExpectedDigest, r.Status, k, r.CreatedAt)
	if err != nil {
		return Release{}, false, fmt.Errorf("OPERATIONAL: insert release: %v", err)
	}
	return r, false, nil
}

func (p *Postgres) ListReleases(ctx context.Context, limit int) ([]Release, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := p.pool.Query(ctx, `SELECT id, package, ecosystem, name, version, repo, commit, expected_digest, status, created_at FROM releases ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		var r Release
		if err := rows.Scan(&r.ID, &r.Package, &r.Ecosystem, &r.Name, &r.Version, &r.Repo, &r.Commit, &r.ExpectedDigest, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (p *Postgres) GetRelease(ctx context.Context, id string) (Release, error) {
	var r Release
	err := p.pool.QueryRow(ctx, `SELECT id, package, ecosystem, name, version, repo, commit, expected_digest, status, created_at FROM releases WHERE id=$1`, id).
		Scan(&r.ID, &r.Package, &r.Ecosystem, &r.Name, &r.Version, &r.Repo, &r.Commit, &r.ExpectedDigest, &r.Status, &r.CreatedAt)
	return r, notFoundError(err, "RELEASE_NOT_FOUND", id)
}

func (p *Postgres) CreateVerification(ctx context.Context, v Verification, key string) (Verification, bool, error) {
	if key != "" {
		var id string
		err := p.pool.QueryRow(ctx, `SELECT id FROM verifications WHERE idempotency_key=$1`, key).Scan(&id)
		if err == nil {
			v, gerr := p.GetVerification(ctx, id)
			return v, true, gerr
		}
		if err != pgx.ErrNoRows {
			return Verification{}, false, err
		}
	}
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM releases WHERE id=$1)`, v.ReleaseID).Scan(&exists); err != nil || !exists {
		if err == nil {
			return Verification{}, false, fmt.Errorf("RELEASE_NOT_FOUND: %s", v.ReleaseID)
		}
		return Verification{}, false, err
	}
	v.ID = newID("ver")
	v.CreatedAt = time.Now().UTC()
	body, _ := json.Marshal(v.Result)
	var k *string
	if key != "" {
		k = &key
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Verification{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO verifications(id, release_id, policy_id, decision, required, satisfied, body, idempotency_key, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		v.ID, v.ReleaseID, v.PolicyID, v.Decision, v.Required, v.Satisfied, body, k, v.CreatedAt); err != nil {
		return Verification{}, false, fmt.Errorf("OPERATIONAL: insert verification: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Verification{}, false, err
	}
	return v, false, nil
}

func scanVerification(row pgx.Row) (Verification, error) {
	var v Verification
	var body []byte
	err := row.Scan(&v.ID, &v.ReleaseID, &v.PolicyID, &v.Decision, &v.Required, &v.Satisfied, &body, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	_ = json.Unmarshal(body, &v.Result)
	v.Evidence = v.Result.CountedEvidence
	return v, nil
}

func (p *Postgres) GetVerification(ctx context.Context, id string) (Verification, error) {
	v, err := scanVerification(p.pool.QueryRow(ctx, `SELECT id, release_id, policy_id, decision, required, satisfied, body, created_at FROM verifications WHERE id=$1`, id))
	return v, notFoundError(err, "VERIFICATION_NOT_FOUND", id)
}

func (p *Postgres) UpsertBuilder(ctx context.Context, b Builder) (Builder, error) {
	if b.ID == "" {
		return Builder{}, fmt.Errorf("INVALID_INPUT: builder id required")
	}
	if b.IndependenceGroup == "" {
		return Builder{}, fmt.Errorf("INVALID_INPUT: independence_group required")
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO builders(id, display_name, independence_group, endpoint, enabled) VALUES ($1,$2,$3,$4,TRUE)
		ON CONFLICT (id) DO UPDATE SET display_name=EXCLUDED.display_name, independence_group=EXCLUDED.independence_group, endpoint=EXCLUDED.endpoint`,
		b.ID, b.DisplayName, b.IndependenceGroup, b.Endpoint)
	if err != nil {
		return Builder{}, err
	}
	var out Builder
	var enabled bool
	err = p.pool.QueryRow(ctx, `SELECT id, display_name, independence_group, endpoint, enabled FROM builders WHERE id=$1`, b.ID).
		Scan(&out.ID, &out.DisplayName, &out.IndependenceGroup, &out.Endpoint, &enabled)
	out.Enabled = enabled
	return out, err
}

func (p *Postgres) ListBuilders(ctx context.Context) ([]Builder, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, display_name, independence_group, endpoint, enabled FROM builders ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Builder{}
	for rows.Next() {
		var b Builder
		if err := rows.Scan(&b.ID, &b.DisplayName, &b.IndependenceGroup, &b.Endpoint, &b.Enabled); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (p *Postgres) PatchBuilder(ctx context.Context, id string, enabled *bool) (Builder, error) {
	if enabled != nil {
		tag, err := p.pool.Exec(ctx, `UPDATE builders SET enabled=$1 WHERE id=$2`, *enabled, id)
		if err != nil {
			return Builder{}, err
		}
		if tag.RowsAffected() == 0 {
			return Builder{}, fmt.Errorf("BUILDER_NOT_FOUND: %s", id)
		}
	}
	var b Builder
	err := p.pool.QueryRow(ctx, `SELECT id, display_name, independence_group, endpoint, enabled FROM builders WHERE id=$1`, id).
		Scan(&b.ID, &b.DisplayName, &b.IndependenceGroup, &b.Endpoint, &b.Enabled)
	return b, notFoundError(err, "BUILDER_NOT_FOUND", id)
}

func (p *Postgres) CreatePolicy(ctx context.Context, pr PolicyRecord) (PolicyRecord, error) {
	if pr.ID == "" {
		return PolicyRecord{}, fmt.Errorf("INVALID_INPUT: policy id required")
	}
	body, _ := json.Marshal(pr.Body)
	_, err := p.pool.Exec(ctx, `INSERT INTO policies(id, version, body, policy_hash) VALUES ($1,$2,$3,$4)`, pr.ID, pr.Version, body, pr.PolicyHash)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return PolicyRecord{}, fmt.Errorf("POLICY_CONFLICT: %s exists", pr.ID)
		}
		return PolicyRecord{}, err
	}
	return pr, nil
}

func (p *Postgres) ListPolicies(ctx context.Context) ([]PolicyRecord, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, version, body, policy_hash FROM policies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PolicyRecord{}
	for rows.Next() {
		var pr PolicyRecord
		var body []byte
		if err := rows.Scan(&pr.ID, &pr.Version, &body, &pr.PolicyHash); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(body, &pr.Body)
		out = append(out, pr)
	}
	return out, nil
}

func (p *Postgres) GetPolicy(ctx context.Context, id string) (PolicyRecord, error) {
	var pr PolicyRecord
	var body []byte
	err := p.pool.QueryRow(ctx, `SELECT id, version, body, policy_hash FROM policies WHERE id=$1`, id).Scan(&pr.ID, &pr.Version, &body, &pr.PolicyHash)
	if err != nil {
		return pr, notFoundError(err, "POLICY_NOT_FOUND", id)
	}
	_ = json.Unmarshal(body, &pr.Body)
	return pr, nil
}

func (p *Postgres) AppendAudit(ctx context.Context, eventType string, payload map[string]any, verificationID string) (AuditRecord, error) {
	if eventType == "" {
		return AuditRecord{}, fmt.Errorf("INVALID_INPUT: event_type required")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return AuditRecord{}, err
	}
	defer tx.Rollback(ctx)
	var prev string
	err = tx.QueryRow(ctx, `SELECT record_hash FROM audit_records ORDER BY id DESC LIMIT 1`).Scan(&prev)
	if err == pgx.ErrNoRows {
		prev = "GENESIS"
	} else if err != nil {
		return AuditRecord{}, err
	}
	h, err := hashRecord(prev, eventType, payload)
	if err != nil {
		return AuditRecord{}, err
	}
	payloadJSON, _ := json.Marshal(payload)
	var rec AuditRecord
	err = tx.QueryRow(ctx, `INSERT INTO audit_records(event_type, payload, verification_id, previous_hash, record_hash) VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`,
		eventType, payloadJSON, verificationID, prev, h).Scan(&rec.ID, &rec.CreatedAt)
	if err != nil {
		return AuditRecord{}, err
	}
	rec.EventType, rec.Payload, rec.VerificationID, rec.PreviousHash, rec.RecordHash = eventType, payload, verificationID, prev, h
	if err := tx.Commit(ctx); err != nil {
		return AuditRecord{}, err
	}
	return rec, nil
}

func (p *Postgres) ListAudit(ctx context.Context, limit int) ([]AuditRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT id, event_type, payload, verification_id, previous_hash, record_hash, created_at FROM audit_records ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRecord{}
	for rows.Next() {
		var r AuditRecord
		var payload []byte
		if err := rows.Scan(&r.ID, &r.EventType, &payload, &r.VerificationID, &r.PreviousHash, &r.RecordHash, &r.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(payload, &r.Payload)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (p *Postgres) PutAnchor(ctx context.Context, a Anchor) (Anchor, error) {
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM verifications WHERE id=$1)`, a.VerificationID).Scan(&exists); err != nil || !exists {
		if err == nil {
			return Anchor{}, fmt.Errorf("VERIFICATION_NOT_FOUND: %s", a.VerificationID)
		}
		return Anchor{}, err
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO anchors(verification_id, tx_hash, block_number, chain_id, contract_address, event_found) VALUES ($1,$2,$3,$4,$5,$6)`,
		a.VerificationID, a.TxHash, a.BlockNumber, a.ChainID, a.ContractAddress, a.EventFound)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return Anchor{}, fmt.Errorf("ANCHOR_CONFLICT: %s already anchored", a.VerificationID)
		}
		return Anchor{}, err
	}
	return a, nil
}

func (p *Postgres) GetAnchor(ctx context.Context, verificationID string) (Anchor, error) {
	var a Anchor
	err := p.pool.QueryRow(ctx, `SELECT verification_id, tx_hash, block_number, chain_id, contract_address, event_found FROM anchors WHERE verification_id=$1`, verificationID).
		Scan(&a.VerificationID, &a.TxHash, &a.BlockNumber, &a.ChainID, &a.ContractAddress, &a.EventFound)
	return a, notFoundError(err, "ANCHOR_NOT_FOUND", verificationID)
}

func (p *Postgres) PutEvidence(ctx context.Context, e EvidenceObject) (EvidenceObject, error) {
	if e.ID == "" {
		e.ID = newID("ev")
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO evidence_objects(id, verification_id, kind, storage_key, sha256) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`,
		e.ID, e.VerificationID, e.Kind, e.StorageKey, e.SHA256)
	return e, err
}

// DeleteAllJobs removes every queued/claimed job (test isolation helper).
func (p *Postgres) DeleteAllJobs(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM build_jobs`)
	return err
}

func (p *Postgres) GetEvidence(ctx context.Context, id string) (EvidenceObject, error) {
	var e EvidenceObject
	err := p.pool.QueryRow(ctx, `SELECT id, verification_id, kind, storage_key, sha256 FROM evidence_objects WHERE id=$1`, id).
		Scan(&e.ID, &e.VerificationID, &e.Kind, &e.StorageKey, &e.SHA256)
	return e, notFoundError(err, "EVIDENCE_NOT_FOUND", id)
}

func scanJob(row pgx.Row) (BuildJob, error) {
	var j BuildJob
	var lease pgtype.Timestamptz
	err := row.Scan(&j.ID, &j.VerificationID, &j.BuilderID, &j.Status, &j.Attempts, &j.MaxAttempts,
		&j.ResultDigest, &j.ResultCommit, &j.ErrorCode, &j.ErrorDetail, &j.LeaseOwner, &lease,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return j, err
	}
	// NULL lease (never claimed) reads as zero time, which is expired by
	// definition — the job is claimable. No special-casing needed.
	if lease.Valid {
		j.LeaseExpiresAt = lease.Time
	}
	return j, nil
}

const jobColumns = `id, verification_id, builder_id, status, attempts, max_attempts, result_digest, result_commit, error_code, error_detail, lease_owner, lease_expires_at, created_at, updated_at`

func (p *Postgres) EnqueueJobs(ctx context.Context, verificationID string, builderIDs []string) ([]BuildJob, error) {
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM verifications WHERE id=$1)`, verificationID).Scan(&exists); err != nil || !exists {
		if err == nil {
			return nil, fmt.Errorf("VERIFICATION_NOT_FOUND: %s", verificationID)
		}
		return nil, err
	}
	out := make([]BuildJob, 0, len(builderIDs))
	for _, b := range builderIDs {
		if b == "" {
			return nil, fmt.Errorf("INVALID_INPUT: builder id required")
		}
		id := newID("job")
		_, err := p.pool.Exec(ctx, `INSERT INTO build_jobs(id, verification_id, builder_id) VALUES ($1,$2,$3) ON CONFLICT (verification_id, builder_id) DO NOTHING`, id, verificationID, b)
		if err != nil {
			return nil, err
		}
		j, err := scanJob(p.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM build_jobs WHERE verification_id=$1 AND builder_id=$2`, verificationID, b))
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (p *Postgres) ListJobs(ctx context.Context, verificationID string) ([]BuildJob, error) {
	q := `SELECT ` + jobColumns + ` FROM build_jobs ORDER BY created_at`
	args := []any{}
	if verificationID != "" {
		q = `SELECT ` + jobColumns + ` FROM build_jobs WHERE verification_id=$1 ORDER BY created_at`
		args = append(args, verificationID)
	}
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BuildJob{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (p *Postgres) GetJob(ctx context.Context, id string) (BuildJob, error) {
	j, err := scanJob(p.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM build_jobs WHERE id=$1`, id))
	return j, notFoundError(err, "JOB_NOT_FOUND", id)
}

func (p *Postgres) ClaimJob(ctx context.Context, owner string, lease time.Duration) (BuildJob, bool, error) {
	if owner == "" {
		return BuildJob{}, false, fmt.Errorf("INVALID_INPUT: lease owner required")
	}
	secs := int(lease.Seconds())
	if secs < 1 {
		secs = 60
	}
	var j BuildJob
	err := p.pool.QueryRow(ctx, `
		UPDATE build_jobs SET status='CLAIMED', lease_owner=$1,
			lease_expires_at=now()+make_interval(secs=>$2),
			attempts=attempts+1, updated_at=now()
		WHERE id = (
			SELECT id FROM build_jobs
			WHERE status='QUEUED'
			   OR (status IN ('CLAIMED','RUNNING') AND lease_expires_at < now())
			ORDER BY created_at LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+jobColumns, owner, secs).Scan(
		&j.ID, &j.VerificationID, &j.BuilderID, &j.Status, &j.Attempts, &j.MaxAttempts,
		&j.ResultDigest, &j.ResultCommit, &j.ErrorCode, &j.ErrorDetail, &j.LeaseOwner,
		&j.LeaseExpiresAt, &j.CreatedAt, &j.UpdatedAt)
	if err == pgx.ErrNoRows {
		return BuildJob{}, false, nil
	}
	if err != nil {
		return BuildJob{}, false, err
	}
	return j, true, nil
}

func (p *Postgres) CompleteJob(ctx context.Context, id string, ok bool, digest, commit, errCode, errDetail string) (BuildJob, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return BuildJob{}, err
	}
	defer tx.Rollback(ctx)
	var j BuildJob
	err = tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM build_jobs WHERE id=$1 FOR UPDATE`, id).Scan(
		&j.ID, &j.VerificationID, &j.BuilderID, &j.Status, &j.Attempts, &j.MaxAttempts,
		&j.ResultDigest, &j.ResultCommit, &j.ErrorCode, &j.ErrorDetail, &j.LeaseOwner,
		&j.LeaseExpiresAt, &j.CreatedAt, &j.UpdatedAt)
	if err == pgx.ErrNoRows {
		return BuildJob{}, fmt.Errorf("JOB_NOT_FOUND: %s", id)
	}
	if err != nil {
		return BuildJob{}, err
	}
	if j.Status == "SUCCEEDED" || j.Status == "FAILED" {
		return BuildJob{}, fmt.Errorf("JOB_CONFLICT: %s already terminal (%s)", id, j.Status)
	}
	next := "QUEUED"
	if ok {
		next = "SUCCEEDED"
	} else if j.Attempts >= j.MaxAttempts {
		next = "FAILED"
	}
	_, err = tx.Exec(ctx, `UPDATE build_jobs SET status=$1, result_digest=$2, result_commit=$3,
		error_code=$4, error_detail=$5, lease_owner='', updated_at=now() WHERE id=$6`,
		next, digest, commit, errCode, errDetail, id)
	if err != nil {
		return BuildJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BuildJob{}, err
	}
	return p.GetJob(ctx, id)
}
