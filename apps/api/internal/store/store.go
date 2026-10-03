// Package store defines persistence models and the Store interface.
// Postgres is the real backend; memoryStore serves unit tests and local dev.
package store

import (
	"context"
	"time"

	"github.com/quorum/quorum/services/policy"
)

type Release struct {
	ID             string `json:"id"`
	Package        string `json:"package"`
	Ecosystem      string `json:"ecosystem"`
	Name           string `json:"name"`
	Version        string `json:"version"`
	Repo           string `json:"repo"`
	Commit         string `json:"commit"`
	ExpectedDigest string `json:"expectedDigest"`
	Status         string `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Verification struct {
	ID           string            `json:"id"`
	ReleaseID    string            `json:"releaseId"`
	PolicyID     string            `json:"policyId"`
	Decision     string            `json:"decision"`
	Required     int               `json:"required"`
	Satisfied    int               `json:"satisfied"`
	Result       policy.Result     `json:"result"`
	Evidence     []policy.Evidence `json:"evidence"`
	ExpectedSrc  string            `json:"expectedSource"`
	CreatedAt    time.Time         `json:"createdAt"`
}

type Builder struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	IndependenceGroup string `json:"independenceGroup"`
	Endpoint         string `json:"endpoint"`
	Enabled          bool   `json:"enabled"`
}

type PolicyRecord struct {
	ID         string        `json:"id"`
	Version    string        `json:"version"`
	Body       policy.Policy `json:"body"`
	PolicyHash string        `json:"policyHash"`
}

type AuditRecord struct {
	ID             int64          `json:"id"`
	EventType      string         `json:"eventType"`
	Payload        map[string]any  `json:"payload"`
	VerificationID string         `json:"verificationId"`
	PreviousHash   string         `json:"previousHash"`
	RecordHash     string         `json:"recordHash"`
	CreatedAt      time.Time      `json:"createdAt"`
}

type Anchor struct {
	VerificationID  string `json:"verificationId"`
	TxHash          string `json:"txHash"`
	BlockNumber     string `json:"blockNumber"`
	ChainID         string `json:"chainId"`
	ContractAddress string `json:"contractAddress"`
	EventFound      bool   `json:"eventFound"`
}

type EvidenceObject struct {
	ID             string `json:"id"`
	VerificationID string `json:"verificationId"`
	Kind           string `json:"kind"`
	StorageKey     string `json:"storageKey"`
	SHA256         string `json:"sha256"`
}

// Store is the persistence boundary. All multi-record mutations that must be
// atomic are single methods so backends can wrap them in transactions.
type Store interface {
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) error

	CreateRelease(ctx context.Context, r Release, idempotencyKey string) (Release, bool, error)
	ListReleases(ctx context.Context, limit int) ([]Release, error)
	GetRelease(ctx context.Context, id string) (Release, error)

	CreateVerification(ctx context.Context, v Verification, idempotencyKey string) (Verification, bool, error)
	GetVerification(ctx context.Context, id string) (Verification, error)

	UpsertBuilder(ctx context.Context, b Builder) (Builder, error)
	ListBuilders(ctx context.Context) ([]Builder, error)
	PatchBuilder(ctx context.Context, id string, enabled *bool) (Builder, error)

	CreatePolicy(ctx context.Context, p PolicyRecord) (PolicyRecord, error)
	ListPolicies(ctx context.Context) ([]PolicyRecord, error)
	GetPolicy(ctx context.Context, id string) (PolicyRecord, error)

	AppendAudit(ctx context.Context, eventType string, payload map[string]any, verificationID string) (AuditRecord, error)
	ListAudit(ctx context.Context, limit int) ([]AuditRecord, error)

	PutAnchor(ctx context.Context, a Anchor) (Anchor, error)
	GetAnchor(ctx context.Context, verificationID string) (Anchor, error)

	PutEvidence(ctx context.Context, e EvidenceObject) (EvidenceObject, error)
	GetEvidence(ctx context.Context, id string) (EvidenceObject, error)
}
