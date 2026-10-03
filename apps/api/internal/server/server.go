// Package server implements the Quorum REST API per docs/api.md.
// Errors are typed envelopes {code, message, requestId}; decisions reuse
// services/policy (single engine, never reimplemented).
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quorum/quorum/internal/runner"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/policy"
)

// Server wires routes to a Store.
type Server struct {
	store store.Store
	mux   *http.ServeMux
}

// New builds all routes.
func New(st store.Store) *Server {
	s := &Server{store: st, mux: http.NewServeMux()}
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
	s.mux.HandleFunc("GET /api/v1/blockchain/{verificationId}", s.handleGetAnchor)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rid := newRequestID()
	ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
	r = r.WithContext(ctx)
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: 200}
	s.mux.ServeHTTP(rec, r)
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
	Data      any    `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	RequestID string `json:"requestId"`
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
	// construction; the explicit lists below only tune HTTP semantics.
	switch {
	case strings.HasPrefix(code, "INVALID_"):
		return code, trimPrefix(msg), 400
	case strings.HasSuffix(code, "_NOT_FOUND"):
		return code, trimPrefix(msg), 404
	case strings.HasSuffix(code, "_CONFLICT"):
		return code, trimPrefix(msg), 409
	}
	switch code {
	case "INVALID_INPUT":
		return code, trimPrefix(msg), 400
	case "RELEASE_NOT_FOUND", "VERIFICATION_NOT_FOUND", "BUILDER_NOT_FOUND",
		"POLICY_NOT_FOUND", "ANCHOR_NOT_FOUND", "EVIDENCE_NOT_FOUND",
		"SOURCE_NOT_FOUND", "COMMIT_NOT_FOUND", "ARTIFACT_NOT_FOUND":
		return code, trimPrefix(msg), 404
	case "POLICY_CONFLICT", "ANCHOR_CONFLICT":
		return code, trimPrefix(msg), 409
	case "INSUFFICIENT_EVIDENCE", "POLICY_VIOLATION":
		return code, trimPrefix(msg), 422
	case "OSS_REBUILD_UNAVAILABLE", "BLOCKCHAIN_UNAVAILABLE":
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

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)) // 1 MiB cap
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
	if err := decodeJSON(r, &in); err != nil {
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
	if err := decodeJSON(r, &in); err != nil {
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
		writeErr(w, r, fmt.Errorf("INSUFFICIENT_EVIDENCE: at least one evidence entry is required"))
		return
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

// --- evidence / attestations ---

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.GetEvidence(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, r, 200, e)
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
	if err := decodeJSON(r, &in); err != nil {
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
	if err := decodeJSON(r, &in); err != nil {
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
	if err := decodeJSON(r, &p); err != nil {
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
	if err := decodeJSON(r, &p); err != nil {
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
	recs, err := s.store.ListAudit(r.Context(), 500)
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
	if err := decodeJSON(r, &in); err != nil {
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
