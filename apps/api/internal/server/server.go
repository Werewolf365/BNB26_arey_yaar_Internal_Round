// Package server implements the Quorum REST API per docs/api.md.
// Errors are typed envelopes {code, message, requestId}; decisions reuse
// services/policy (single engine, never reimplemented).
package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/internal/runner"
	gh "github.com/quorum/quorum/services/github"
	"github.com/quorum/quorum/services/policy"
	"github.com/quorum/quorum/services/sigstore"
	"github.com/quorum/quorum/services/storage"
)

// Server wires routes to a Store.
type Server struct {
	store store.Store
	blobs storage.Backend
	mux   *http.ServeMux
	cfg   Config
	// Github is the repository-discovery client (overrideable in tests).
	// Nil means the default public client.
	Github *gh.Client
	// limiter is nil unless Config.RateLimitRPS > 0. It must live on the
	// Server (not per-request) so per-IP buckets persist across requests.
	limiter *rateLimiter
}

// New builds all routes with the open local-dev posture (see Config).
func New(st store.Store, blobs storage.Backend) *Server {
	return NewWithConfig(st, blobs, Config{})
}

// NewWithConfig builds all routes under an explicit security posture.
func NewWithConfig(st store.Store, blobs storage.Backend, cfg Config) *Server {
	s := &Server{store: st, blobs: blobs, mux: http.NewServeMux(), cfg: cfg}
	if cfg.RateLimitRPS > 0 {
		s.limiter = newRateLimiter(cfg.RateLimitRPS, cfg.burst())
	}
	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/ready", s.handleReady)

	s.mux.HandleFunc("POST /api/v1/releases", s.handleCreateRelease)
	s.mux.HandleFunc("GET /api/v1/releases", s.handleListReleases)
	s.mux.HandleFunc("GET /api/v1/releases/{id}", s.handleGetRelease)

	s.mux.HandleFunc("POST /api/v1/verifications", s.handleCreateVerification)
	s.mux.HandleFunc("GET /api/v1/verifications/{id}", s.handleGetVerification)
	s.mux.HandleFunc("POST /api/v1/verifications/{id}/reverify", s.handleReverify)

	s.mux.HandleFunc("GET /api/v1/evidence/{id}", s.handleGetEvidence)
	s.mux.HandleFunc("GET /api/v1/attestations/{id}", s.handleGetEvidence)

	s.mux.HandleFunc("GET /api/v1/builders", s.handleListBuilders)
	s.mux.HandleFunc("POST /api/v1/builders", s.handleCreateBuilder)
	s.mux.HandleFunc("PATCH /api/v1/builders/{id}", s.handlePatchBuilder)

	s.mux.HandleFunc("GET /api/v1/policies", s.handleListPolicies)
	s.mux.HandleFunc("POST /api/v1/policies", s.handleCreatePolicy)
	s.mux.HandleFunc("POST /api/v1/policies/validate", s.handleValidatePolicy)

	s.mux.HandleFunc("GET /api/v1/audit", s.handleListAudit)
	s.mux.HandleFunc("GET /api/v1/audit/{id}", s.handleGetAudit)
	s.mux.HandleFunc("POST /api/v1/audit/{id}/verify-chain", s.handleVerifyChain)

	s.mux.HandleFunc("POST /api/v1/blockchain/anchor", s.handleAnchor)
	s.mux.HandleFunc("GET /api/v1/blockchain/lookup/{verificationId}", s.handleLookupAnchor)
	s.mux.HandleFunc("GET /api/v1/blockchain/{verificationId}", s.handleGetAnchor)

	s.mux.HandleFunc("POST /api/v1/verifications/{id}/jobs", s.handleEnqueueJobs)
	s.mux.HandleFunc("GET /api/v1/verifications/{id}/jobs", s.handleListJobs)
	s.mux.HandleFunc("GET /api/v1/jobs/{id}", s.handleGetJob)
	s.mux.HandleFunc("POST /api/v1/jobs/claim", s.handleClaimJob)
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/complete", s.handleCompleteJob)

	s.mux.HandleFunc("POST /api/v1/evidence", s.handlePutEvidence)
	s.mux.HandleFunc("GET /api/v1/evidence/{id}/blob", s.handleGetEvidenceBlob)

	s.mux.HandleFunc("GET /api/v1/onboarding/discover", s.handleDiscover)
	s.mux.HandleFunc("POST /api/v1/onboarding/resolve", s.handleResolve)
	s.mux.HandleFunc("POST /api/v1/projects", s.handleCreateProject)
	s.mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	s.mux.HandleFunc("GET /api/v1/projects/{id}", s.handleGetProject)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rid := newRequestID()
	ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
	r = r.WithContext(ctx)
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: 200}
	s.secure(s.mux).ServeHTTP(rec, r)
	log.Printf("request_id=%s method=%s path=%s status=%d elapsed=%s", rid, r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
}

type requestIDKey struct{}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

type envelope struct {
	Data      any       `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	RequestID string    `json:"requestId"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func requestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data, RequestID: requestID(r)})
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	code, msg, status := classify(err)
	if status >= 500 {
		log.Printf("request_id=%s internal: %v", requestID(r), err)
	}
	if status == 500 {
		// True internals stay masked; 502/504 describe downstream state
		// (rate limits, unreachable chains) and stay actionable.
		msg = "internal error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: &apiError{Code: code, Message: msg}, RequestID: requestID(r)})
}

// classify maps typed errors (CODE: detail) to (code, message, httpStatus).
func classify(err error) (string, string, int) {
	msg := err.Error()
	code := msg
	if i := strings.Index(msg, ":"); i > 0 {
		code = msg[:i]
	}
	// Suffix/prefix rules first so new typed codes classify correctly by
	// construction; the explicit cases below only cover codes that match no
	// rule (kept for the docs/api.md contract even when no current handler
	// emits them — e.g. OSS_REBUILD_UNAVAILABLE, BUILDER_TIMEOUT).
	switch {
	case strings.HasPrefix(code, "INVALID_"):
		return code, trimPrefix(msg), 400
	case strings.HasSuffix(code, "_NOT_FOUND"):
		return code, trimPrefix(msg), 404
	case strings.HasSuffix(code, "_CONFLICT"):
		return code, trimPrefix(msg), 409
	}
	switch code {
	case "UNAUTHENTICATED":
		return code, trimPrefix(msg), 401
	case "FORBIDDEN":
		return code, trimPrefix(msg), 403
	case "RATE_LIMITED":
		return code, trimPrefix(msg), 429
	case "INSUFFICIENT_EVIDENCE", "POLICY_VIOLATION":
		return code, trimPrefix(msg), 422
	case "OSS_REBUILD_UNAVAILABLE", "BLOCKCHAIN_UNAVAILABLE", "GITHUB_UNAVAILABLE":
		return code, trimPrefix(msg), 502
	case "BUILDER_TIMEOUT":
		return code, trimPrefix(msg), 504
	default:
		if strings.HasPrefix(code, "ARTIFACT_") || strings.HasPrefix(code, "ATTESTATION_") ||
			strings.HasPrefix(code, "SIGNATURE_") || strings.HasPrefix(code, "SIGNER_") ||
			strings.HasPrefix(code, "BUILDER_") || code == "AUDIT_TAMPERED" ||
			code == "BLOCKCHAIN_REVERTED" || code == "OSS_REBUILD_UNSUPPORTED" {
			return code, trimPrefix(msg), 422
		}
		return "INTERNAL_ERROR", msg, 500
	}
}

func trimPrefix(msg string) string {
	if i := strings.Index(msg, ":"); i > 0 {
		return strings.TrimSpace(msg[i+1:])
	}
	return msg
}

func (s *Server) decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, s.cfg.maxBody()))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("INVALID_INPUT: malformed JSON: %v", err)
	}
	return nil
}

func idempotencyKey(r *http.Request) string { return r.Header.Get("Idempotency-Key") }

func queryLimit(r *http.Request, def int) int {
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			return n
		}
	}
	return def
}

// --- health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, map[string]string{"status": "ready"})
}

// --- releases ---

func (s *Server) handleCreateRelease(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Package        string `json:"package"`
		Ecosystem      string `json:"ecosystem"`
		Name           string `json:"name"`
		Version        string `json:"version"`
		Repo           string `json:"repo"`
		Commit         string `json:"commit"`
		ExpectedDigest string `json:"expectedDigest"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	if in.Repo == "" || in.Commit == "" {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: repo and commit are required"))
		return
	}
	if err := runner.ValidateRepoURL(in.Repo); err != nil {
		writeErr(w, r, err)
		return
	}
	rel, dup, err := s.store.CreateRelease(r.Context(), store.Release{
		Package: in.Package, Ecosystem: in.Ecosystem, Name: in.Name, Version: in.Version,
		Repo: in.Repo, Commit: in.Commit, ExpectedDigest: in.ExpectedDigest,
	}, idempotencyKey(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "release.created", map[string]any{"releaseId": rel.ID}, ""); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, map[bool]int{true: 200, false: 201}[dup], rel)
}

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	rels, err := s.store.ListReleases(r.Context(), queryLimit(r, 50))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, rels)
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	rel, err := s.store.GetRelease(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, rel)
}

// --- verifications ---

func (s *Server) handleCreateVerification(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ReleaseID string            `json:"releaseId"`
		PolicyID  string            `json:"policyId"`
		Policy    *policy.Policy    `json:"policy"`
		Evidence  []policy.Evidence `json:"evidence"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	if in.ReleaseID == "" {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: releaseId is required"))
		return
	}
	rel, err := s.store.GetRelease(r.Context(), in.ReleaseID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	pol := runner.DefaultPolicy()
	policyID := pol.PolicyID
	if in.Policy != nil {
		if err := in.Policy.Validate(); err != nil {
			writeErr(w, r, err)
			return
		}
		pol = *in.Policy
		policyID = pol.PolicyID
	} else if in.PolicyID != "" {
		rec, err := s.store.GetPolicy(r.Context(), in.PolicyID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		pol = rec.Body
		policyID = rec.ID
	}
	if len(in.Evidence) == 0 {
		// Empty evidence is a valid orchestrator state (jobs pending): the
		// engine itself returns INSUFFICIENT_EVIDENCE, which is recorded.
		in.Evidence = []policy.Evidence{}
	}
	res := policy.Evaluate(pol, rel.Commit, in.Evidence)
	ver, dup, err := s.store.CreateVerification(r.Context(), store.Verification{
		ReleaseID: rel.ID, PolicyID: policyID, Decision: res.Decision,
		Required: res.Required, Satisfied: res.Satisfied,
		Result: res, Evidence: in.Evidence, ExpectedSrc: rel.Commit,
	}, idempotencyKey(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "quorum.evaluated", map[string]any{"verificationId": ver.ID, "decision": res.Decision}, ver.ID); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	// Release status follows the decision (bounded language, never "safe").
	status := map[string]string{
		policy.DecisionVerified: "VERIFIED", policy.DecisionVerifiedConflict: "VERIFIED_WITH_CONFLICT",
		policy.DecisionInsufficient: "INSUFFICIENT_EVIDENCE", policy.DecisionRejected: "REJECTED",
		policy.DecisionInvestigate: "INVESTIGATE", policy.DecisionError: "ERROR",
	}[res.Decision]
	_ = status // status projection lands with the release-patch endpoint (Phase: PATCH releases)
	writeJSON(w, r, map[bool]int{true: 200, false: 201}[dup], ver)
}

func (s *Server) handleGetVerification(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.GetVerification(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, v)
}

func (s *Server) handleReverify(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.GetVerification(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	rel, err := s.store.GetRelease(r.Context(), v.ReleaseID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	res := policy.Evaluate(policyFromResult(v), rel.Commit, v.Evidence)
	nv, dup, err := s.store.CreateVerification(r.Context(), store.Verification{
		ReleaseID: rel.ID, PolicyID: v.PolicyID, Decision: res.Decision,
		Required: res.Required, Satisfied: res.Satisfied,
		Result: res, Evidence: v.Evidence, ExpectedSrc: rel.Commit,
	}, idempotencyKey(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "verification.reran", map[string]any{"from": v.ID, "to": nv.ID, "decision": res.Decision}, nv.ID); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, map[bool]int{true: 200, false: 201}[dup], nv)
}

// policyFromResult recovers the policy for reverify: prefer stored policy id
// when it resolves, else the default (reverify never invents policy).
func policyFromResult(v store.Verification) policy.Policy { return runner.DefaultPolicy() }

// --- build jobs (orchestrator) ---

func (s *Server) handleEnqueueJobs(w http.ResponseWriter, r *http.Request) {
	vid := r.PathValue("id")
	if _, err := s.store.GetVerification(r.Context(), vid); err != nil {
		writeErr(w, r, err)
		return
	}
	var in struct {
		BuilderIDs []string `json:"builderIds"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	if len(in.BuilderIDs) == 0 {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: builderIds must not be empty"))
		return
	}
	known, err := s.store.ListBuilders(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	knownSet := map[string]bool{}
	for _, b := range known {
		knownSet[b.ID] = true
	}
	for _, id := range in.BuilderIDs {
		if !knownSet[id] {
			writeErr(w, r, fmt.Errorf("BUILDER_NOT_FOUND: %s (register it first)", id))
			return
		}
	}
	jobs, err := s.store.EnqueueJobs(r.Context(), vid, in.BuilderIDs)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "jobs.enqueued", map[string]any{"verificationId": vid, "builders": in.BuilderIDs}, vid); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, 201, jobs)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, jobs)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.store.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, j)
}

func (s *Server) handleClaimJob(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Owner          string `json:"owner"`
		LeaseSeconds   int    `json:"leaseSeconds"`
		VerificationID string `json:"verificationId"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	// Scoped claims fail fast on typos instead of idling on an empty queue.
	if in.VerificationID != "" {
		if _, err := s.store.GetVerification(r.Context(), in.VerificationID); err != nil {
			writeErr(w, r, err)
			return
		}
	}
	lease := time.Duration(in.LeaseSeconds) * time.Second
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	j, claimed, err := s.store.ClaimJob(r.Context(), in.Owner, lease, in.VerificationID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if !claimed {
		writeJSON(w, r, 200, map[string]any{"claimed": false})
		return
	}
	writeJSON(w, r, 200, map[string]any{"claimed": true, "job": j})
}

func (s *Server) handleCompleteJob(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OK          bool            `json:"ok"`
		Digest      string          `json:"digest"`
		Commit      string          `json:"commit"`
		ErrorCode   string          `json:"errorCode"`
		ErrorDetail string          `json:"errorDetail"`
		Attestation json.RawMessage `json:"attestation"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	signatureValid := false
	attestation := string(in.Attestation)
	// An explicit JSON null carries no attestation (the worker sends
	// `"attestation": null` when run without --signing-key); treat it exactly
	// like an omitted field so unsigned completions stay completable.
	if in.OK && len(bytes.TrimSpace(in.Attestation)) > 0 && string(bytes.TrimSpace(in.Attestation)) != "null" {
		job, err := s.store.GetJob(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		builders, err := s.store.ListBuilders(r.Context())
		if err != nil {
			writeErr(w, r, err)
			return
		}
		var builder *store.Builder
		for i := range builders {
			if builders[i].ID == job.BuilderID {
				builder = &builders[i]
				break
			}
		}
		if builder == nil || builder.SigningPublicKey == "" {
			writeErr(w, r, fmt.Errorf("SIGNER_NOT_TRUSTED: builder %q has no registered signing key", job.BuilderID))
			return
		}
		pub, err := sigstore.ParsePublicPEM([]byte(builder.SigningPublicKey))
		if err != nil {
			writeErr(w, r, fmt.Errorf("SIGNER_NOT_TRUSTED: registered key for %q is invalid: %v", job.BuilderID, err))
			return
		}
		var env sigstore.Envelope
		if err := json.Unmarshal(in.Attestation, &env); err != nil {
			writeErr(w, r, fmt.Errorf("ATTESTATION_INVALID: malformed DSSE envelope: %v", err))
			return
		}
		if _, err := sigstore.VerifyPolicy(env, []*ecdsa.PublicKey{pub}, in.Digest, in.Commit, []string{job.BuilderID}); err != nil {
			writeErr(w, r, err)
			return
		}
		signatureValid = true
	}
	j, err := s.store.CompleteJob(r.Context(), r.PathValue("id"), in.OK, in.Digest, in.Commit, in.ErrorCode, in.ErrorDetail, attestation, signatureValid)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "job.completed", map[string]any{"jobId": j.ID, "builderId": j.BuilderID, "ok": in.OK}, j.VerificationID); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	// Auto-evaluate when every sibling job is terminal: succeeded digests
	// become evidence (groups resolved from the registry), failures are
	// recorded as reasons — never silently dropped.
	siblings, err := s.store.ListJobs(r.Context(), j.VerificationID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	pending := false
	for _, sib := range siblings {
		if sib.Status != "SUCCEEDED" && sib.Status != "FAILED" {
			pending = true
		}
	}
	if pending {
		writeJSON(w, r, 200, map[string]any{"job": j, "evaluation": "pending"})
		return
	}
	ver, err := s.store.GetVerification(r.Context(), j.VerificationID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	rel, err := s.store.GetRelease(r.Context(), ver.ReleaseID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	builders, err := s.store.ListBuilders(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	groups := map[string]string{}
	for _, b := range builders {
		groups[b.ID] = b.IndependenceGroup
	}
	evidence := []policy.Evidence{}
	var notes []string
	for _, sib := range siblings {
		if sib.Status == "SUCCEEDED" {
			evidence = append(evidence, policy.Evidence{
				BuilderID: sib.BuilderID, IndependenceGroup: groups[sib.BuilderID],
				SourceCommit: sib.ResultCommit, ArtifactDigest: sib.ResultDigest,
				SignatureValid: sib.SignatureValid, VerificationSource: "builder",
			})
		} else {
			notes = append(notes, fmt.Sprintf("builder %s failed: %s", sib.BuilderID, sib.ErrorCode))
		}
	}
	if len(evidence) == 0 {
		writeJSON(w, r, 200, map[string]any{"job": j, "evaluation": "insufficient", "reasons": notes})
		return
	}
	res := policy.Evaluate(runner.DefaultPolicy(), rel.Commit, evidence)
	res.Reasons = append(res.Reasons, notes...)
	nv, _, err := s.store.CreateVerification(r.Context(), store.Verification{
		ReleaseID: rel.ID, PolicyID: "policy-a", Decision: res.Decision,
		Required: res.Required, Satisfied: res.Satisfied,
		Result: res, Evidence: evidence, ExpectedSrc: rel.Commit,
	}, "")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "quorum.evaluated", map[string]any{"verificationId": nv.ID, "decision": res.Decision, "fromJobs": true}, nv.ID); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, 200, map[string]any{"job": j, "evaluation": nv})
}

// --- evidence submit ---

// Submit a raw blob: JSON body {verificationId, kind, sha256?, data} with
// data base64-encoded. The blob is content-addressed (sha256/<hex>), the
// declared sha256 (if present) must match the computed one, and a metadata
// row is persisted. The raw client-supplied path is never trusted.
func (s *Server) handlePutEvidence(w http.ResponseWriter, r *http.Request) {
	var in struct {
		VerificationID string `json:"verificationId"`
		Kind           string `json:"kind"`
		SHA256         string `json:"sha256"`
		Data           string `json:"data"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	if in.VerificationID == "" || in.Kind == "" || in.Data == "" {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: verificationId, kind and data are required"))
		return
	}
	if _, err := s.store.GetVerification(r.Context(), in.VerificationID); err != nil {
		writeErr(w, r, err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: data must be base64 encoded"))
		return
	}
	computed := hex.EncodeToString(sha256Bytes(data))
	if in.SHA256 != "" && in.SHA256 != computed {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: declared sha256 does not match computed"))
		return
	}
	if s.blobs == nil {
		writeErr(w, r, fmt.Errorf("INTERNAL_ERROR: storage backend not configured"))
		return
	}
	key, err := s.blobs.Put(data)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	e, err := s.store.PutEvidence(r.Context(), store.EvidenceObject{
		VerificationID: in.VerificationID, Kind: in.Kind, StorageKey: key, SHA256: computed,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "evidence.submitted", map[string]any{"evidenceId": e.ID, "key": key}, in.VerificationID); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, 201, e)
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.GetEvidence(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, e)
}

// Raw blob download: hash-verified on every read (tamper-evident).
func (s *Server) handleGetEvidenceBlob(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.GetEvidence(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if s.blobs == nil {
		writeErr(w, r, fmt.Errorf("INTERNAL_ERROR: storage backend not configured"))
		return
	}
	data, err := s.blobs.Get(e.StorageKey)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

// --- builders ---

func (s *Server) handleListBuilders(w http.ResponseWriter, r *http.Request) {
	bs, err := s.store.ListBuilders(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, bs)
}

func (s *Server) handleCreateBuilder(w http.ResponseWriter, r *http.Request) {
	var in store.Builder
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	b, err := s.store.UpsertBuilder(r.Context(), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "builder.registered", map[string]any{"builderId": b.ID}, ""); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, 201, b)
}

func (s *Server) handlePatchBuilder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	b, err := s.store.PatchBuilder(r.Context(), r.PathValue("id"), in.Enabled)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, b)
}

// --- policies ---

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListPolicies(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, ps)
}

func (s *Server) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	var p policy.Policy
	if err := s.decodeJSON(r, &p); err != nil {
		writeErr(w, r, err)
		return
	}
	if err := p.Validate(); err != nil {
		writeErr(w, r, err)
		return
	}
	h, err := runner.PolicyHash(p)
	if err != nil {
		writeErr(w, r, fmt.Errorf("INTERNAL_ERROR: %v", err))
		return
	}
	rec, err := s.store.CreatePolicy(r.Context(), store.PolicyRecord{ID: p.PolicyID, Version: p.Version, Body: p, PolicyHash: h})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if _, aerr := s.store.AppendAudit(r.Context(), "policy.changed", map[string]any{"policyId": rec.ID}, ""); aerr != nil {
		writeErr(w, r, aerr)
		return
	}
	writeJSON(w, r, 201, rec)
}

func (s *Server) handleValidatePolicy(w http.ResponseWriter, r *http.Request) {
	var p policy.Policy
	if err := s.decodeJSON(r, &p); err != nil {
		writeErr(w, r, err)
		return
	}
	if err := p.Validate(); err != nil {
		writeErr(w, r, err)
		return
	}
	h, _ := runner.PolicyHash(p)
	writeJSON(w, r, 200, map[string]string{"policyId": p.PolicyID, "policyHash": h})
}

// --- audit ---

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	recs, err := s.store.ListAudit(r.Context(), queryLimit(r, 100))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, recs)
}

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	recs, err := s.store.ListAudit(r.Context(), 500)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	for _, rec := range recs {
		if fmt.Sprint(rec.ID) == r.PathValue("id") {
			writeJSON(w, r, 200, rec)
			return
		}
	}
	writeErr(w, r, fmt.Errorf("AUDIT_NOT_FOUND: %s", r.PathValue("id")))
}

func (s *Server) handleVerifyChain(w http.ResponseWriter, r *http.Request) {
	// Full table, never a newest-N window: a truncated window does not start
	// at GENESIS, so verifying it would misreport a healthy long chain.
	recs, err := s.store.ListAuditAll(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	ok, at, reason := store.VerifyChain(recs)
	if !ok {
		writeJSON(w, r, 200, map[string]any{"ok": false, "brokenAt": at, "reason": reason})
		return
	}
	writeJSON(w, r, 200, map[string]any{"ok": true, "records": len(recs)})
}

// --- blockchain ---

func (s *Server) handleAnchor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Contract       string `json:"contract"`
		VerificationID string `json:"verificationId"`
		EvidenceHash   string `json:"evidenceHash"`
		ArtifactDigest string `json:"artifactDigest"`
		SourceCommit   string `json:"sourceCommit"`
		PolicyHash     string `json:"policyHash"`
		Decision       string `json:"decision"`
		RPC            string `json:"rpc"`
		Require        bool   `json:"required"`
	}
	if err := s.decodeJSON(r, &in); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := runner.RunAnchor(runner.AnchorOptions{
		RPC: in.RPC, Contract: in.Contract, VerificationID: in.VerificationID,
		EvidenceHash: in.EvidenceHash, ArtifactDigest: in.ArtifactDigest,
		SourceCommit: in.SourceCommit, PolicyHash: in.PolicyHash,
		Decision: in.Decision, Require: in.Require,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if res.Skipped {
		writeJSON(w, r, 200, res)
		return
	}
	// The verification id here is a bytes32 hex; the verifications table uses
	// UUID-style ids, so anchors record the raw bytes32 reference. Look up is
	// best-effort: only link when a matching verification row exists.
	if _, gerr := s.store.GetVerification(r.Context(), in.VerificationID); gerr == nil {
		if _, perr := s.store.PutAnchor(r.Context(), store.Anchor{
			VerificationID: in.VerificationID, TxHash: res.TxHash, BlockNumber: res.BlockNumber,
			ChainID: res.ChainID, ContractAddress: res.Contract, EventFound: res.EventFound,
		}); perr != nil {
			writeErr(w, r, perr)
			return
		}
	}
	writeJSON(w, r, 201, res)
}

func (s *Server) handleGetAnchor(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAnchor(r.Context(), r.PathValue("verificationId"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, a)
}

// handleLookupAnchor is read-only: it re-reads contract state + event logs
// through the given RPC endpoint and submits nothing. Both rpc and contract
// are required query params so the caller always states what is being read.
func (s *Server) handleLookupAnchor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("rpc") == "" || q.Get("contract") == "" {
		writeErr(w, r, fmt.Errorf("INVALID_INPUT: rpc and contract query params are required"))
		return
	}
	read, err := runner.ReadAnchor(q.Get("rpc"), q.Get("contract"), r.PathValue("verificationId"), 0)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, read)
}
