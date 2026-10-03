package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore is the in-process Store: unit tests + local dev without Postgres.
type MemoryStore struct {
	mu       sync.Mutex
	sequence int64
	releases map[string]Release
	relKeys  map[string]string // idempotency key -> release id
	verifs   map[string]Verification
	verKeys  map[string]string
	builders map[string]Builder
	policies map[string]PolicyRecord
	audit    []AuditRecord
	anchors  map[string]Anchor
	evidence map[string]EvidenceObject
	jobs     map[string]BuildJob
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		releases: map[string]Release{},
		relKeys:  map[string]string{},
		verifs:   map[string]Verification{},
		verKeys:  map[string]string{},
		builders: map[string]Builder{},
		policies: map[string]PolicyRecord{},
		anchors:  map[string]Anchor{},
		evidence: map[string]EvidenceObject{},
		jobs:     map[string]BuildJob{},
	}
}

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

func (m *MemoryStore) Migrate(ctx context.Context) error { return nil }
func (m *MemoryStore) Ping(ctx context.Context) error    { return nil }

func (m *MemoryStore) CreateRelease(ctx context.Context, r Release, key string) (Release, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key != "" {
		if id, ok := m.relKeys[key]; ok {
			return m.releases[id], true, nil
		}
	}
	r.ID = newID("rel")
	r.CreatedAt = time.Now().UTC()
	if r.Status == "" {
		r.Status = "PENDING"
	}
	m.releases[r.ID] = r
	if key != "" {
		m.relKeys[key] = r.ID
	}
	return r, false, nil
}

func (m *MemoryStore) ListReleases(ctx context.Context, limit int) ([]Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Release, 0, len(m.releases))
	for _, r := range m.releases {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryStore) GetRelease(ctx context.Context, id string) (Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.releases[id]
	if !ok {
		return Release{}, fmt.Errorf("RELEASE_NOT_FOUND: %s", id)
	}
	return r, nil
}

func (m *MemoryStore) CreateVerification(ctx context.Context, v Verification, key string) (Verification, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key != "" {
		if id, ok := m.verKeys[key]; ok {
			if v, found := m.verifs[id]; found {
				return v, true, nil
			}
		}
	}
	if _, ok := m.releases[v.ReleaseID]; !ok {
		return Verification{}, false, fmt.Errorf("RELEASE_NOT_FOUND: %s", v.ReleaseID)
	}
	v.ID = newID("ver")
	v.CreatedAt = time.Now().UTC()
	m.verifs[v.ID] = v
	if rel, ok := m.releases[v.ReleaseID]; ok {
		rel.Status = v.Decision
		m.releases[v.ReleaseID] = rel
	}
	if key != "" {
		m.verKeys[key] = v.ID
	}
	return v, false, nil
}

func (m *MemoryStore) GetVerification(ctx context.Context, id string) (Verification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.verifs[id]
	if !ok {
		return Verification{}, fmt.Errorf("VERIFICATION_NOT_FOUND: %s", id)
	}
	return v, nil
}

func (m *MemoryStore) UpsertBuilder(ctx context.Context, b Builder) (Builder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b.ID == "" {
		return Builder{}, fmt.Errorf("INVALID_INPUT: builder id required")
	}
	if b.IndependenceGroup == "" {
		return Builder{}, fmt.Errorf("INVALID_INPUT: independence_group required")
	}
	b.Enabled = true
	if old, ok := m.builders[b.ID]; ok {
		b.Enabled = old.Enabled
	}
	m.builders[b.ID] = b
	return b, nil
}

func (m *MemoryStore) ListBuilders(ctx context.Context) ([]Builder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Builder, 0, len(m.builders))
	for _, b := range m.builders {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MemoryStore) PatchBuilder(ctx context.Context, id string, enabled *bool) (Builder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.builders[id]
	if !ok {
		return Builder{}, fmt.Errorf("BUILDER_NOT_FOUND: %s", id)
	}
	if enabled != nil {
		b.Enabled = *enabled
	}
	m.builders[id] = b
	return b, nil
}

func (m *MemoryStore) CreatePolicy(ctx context.Context, p PolicyRecord) (PolicyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == "" {
		return PolicyRecord{}, fmt.Errorf("INVALID_INPUT: policy id required")
	}
	if _, ok := m.policies[p.ID]; ok {
		return PolicyRecord{}, fmt.Errorf("POLICY_CONFLICT: %s exists", p.ID)
	}
	m.policies[p.ID] = p
	return p, nil
}

func (m *MemoryStore) ListPolicies(ctx context.Context) ([]PolicyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PolicyRecord, 0, len(m.policies))
	for _, p := range m.policies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MemoryStore) GetPolicy(ctx context.Context, id string) (PolicyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.policies[id]
	if !ok {
		return PolicyRecord{}, fmt.Errorf("POLICY_NOT_FOUND: %s", id)
	}
	return p, nil
}

func (m *MemoryStore) AppendAudit(ctx context.Context, eventType string, payload map[string]any, verificationID string) (AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if eventType == "" {
		return AuditRecord{}, fmt.Errorf("INVALID_INPUT: event_type required")
	}
	prev := "GENESIS"
	if len(m.audit) > 0 {
		prev = m.audit[len(m.audit)-1].RecordHash
	}
	h, err := hashRecord(prev, eventType, payload)
	if err != nil {
		return AuditRecord{}, err
	}
	m.sequence++
	rec := AuditRecord{ID: m.sequence, EventType: eventType, Payload: payload, VerificationID: verificationID, PreviousHash: prev, RecordHash: h, CreatedAt: time.Now().UTC()}
	m.audit = append(m.audit, rec)
	return rec, nil
}

func (m *MemoryStore) ListAudit(ctx context.Context, limit int) ([]AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]AuditRecord(nil), m.audit...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (m *MemoryStore) PutAnchor(ctx context.Context, a Anchor) (Anchor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.verifs[a.VerificationID]; !ok {
		return Anchor{}, fmt.Errorf("VERIFICATION_NOT_FOUND: %s", a.VerificationID)
	}
	if _, dup := m.anchors[a.VerificationID]; dup {
		return Anchor{}, fmt.Errorf("ANCHOR_CONFLICT: %s already anchored", a.VerificationID)
	}
	m.anchors[a.VerificationID] = a
	return a, nil
}

func (m *MemoryStore) GetAnchor(ctx context.Context, verificationID string) (Anchor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.anchors[verificationID]
	if !ok {
		return Anchor{}, fmt.Errorf("ANCHOR_NOT_FOUND: %s", verificationID)
	}
	return a, nil
}

func (m *MemoryStore) PutEvidence(ctx context.Context, e EvidenceObject) (EvidenceObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ID == "" {
		e.ID = newID("ev")
	}
	m.evidence[e.ID] = e
	return e, nil
}

func (m *MemoryStore) GetEvidence(ctx context.Context, id string) (EvidenceObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.evidence[id]
	if !ok {
		return EvidenceObject{}, fmt.Errorf("EVIDENCE_NOT_FOUND: %s", id)
	}
	return e, nil
}

func (m *MemoryStore) EnqueueJobs(ctx context.Context, verificationID string, builderIDs []string) ([]BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.verifs[verificationID]; !ok {
		return nil, fmt.Errorf("VERIFICATION_NOT_FOUND: %s", verificationID)
	}
	out := make([]BuildJob, 0, len(builderIDs))
	for _, b := range builderIDs {
		dup := false
		for _, j := range m.jobs {
			if j.VerificationID == verificationID && j.BuilderID == b {
				out = append(out, j)
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		now := time.Now().UTC()
		j := BuildJob{ID: newID("job"), VerificationID: verificationID, BuilderID: b, Status: "QUEUED", MaxAttempts: 3, CreatedAt: now, UpdatedAt: now}
		m.jobs[j.ID] = j
		out = append(out, j)
	}
	return out, nil
}

func (m *MemoryStore) ListJobs(ctx context.Context, verificationID string) ([]BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []BuildJob{}
	for _, j := range m.jobs {
		if verificationID == "" || j.VerificationID == verificationID {
			out = append(out, j)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemoryStore) GetJob(ctx context.Context, id string) (BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return BuildJob{}, fmt.Errorf("JOB_NOT_FOUND: %s", id)
	}
	return j, nil
}

func (m *MemoryStore) ClaimJob(ctx context.Context, owner string, lease time.Duration) (BuildJob, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner == "" {
		return BuildJob{}, false, fmt.Errorf("INVALID_INPUT: lease owner required")
	}
	now := time.Now().UTC()
	var best *BuildJob
	for _, j := range m.jobs {
		if j.Status == "QUEUED" || ((j.Status == "CLAIMED" || j.Status == "RUNNING") && !j.LeaseExpiresAt.After(now)) {
			c := j
			if best == nil || c.CreatedAt.Before(best.CreatedAt) {
				best = &c
			}
		}
	}
	if best == nil {
		return BuildJob{}, false, nil
	}
	best.Status = "CLAIMED"
	best.Attempts++
	best.LeaseOwner = owner
	best.LeaseExpiresAt = now.Add(lease)
	best.UpdatedAt = now
	m.jobs[best.ID] = *best
	return *best, true, nil
}

func (m *MemoryStore) CompleteJob(ctx context.Context, id string, ok bool, digest, commit, errCode, errDetail, attestation string, signatureValid bool) (BuildJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, found := m.jobs[id]
	if !found {
		return BuildJob{}, fmt.Errorf("JOB_NOT_FOUND: %s", id)
	}
	if j.Status == "SUCCEEDED" || j.Status == "FAILED" {
		return BuildJob{}, fmt.Errorf("JOB_CONFLICT: %s already terminal (%s)", id, j.Status)
	}
	if ok {
		j.Status = "SUCCEEDED"
		j.ResultDigest = digest
		j.ResultCommit = commit
		j.Attestation = attestation
		j.SignatureValid = signatureValid
	} else if j.Attempts >= j.MaxAttempts {
		j.Status = "FAILED"
		j.ErrorCode = errCode
		j.ErrorDetail = errDetail
	} else {
		j.Status = "QUEUED" // retry: lease released, attempts kept
		j.ErrorCode = errCode
		j.ErrorDetail = errDetail
	}
	j.LeaseOwner = ""
	j.UpdatedAt = time.Now().UTC()
	m.jobs[id] = j
	return j, nil
}
