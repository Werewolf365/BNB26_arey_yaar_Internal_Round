// Package sigstore provides standard DSSE signing/verification for Quorum
// attestations (SLSA envelope guidance: ECDSA P-256 + SHA-256).
//
// Keys here are operator-managed PEM (dev/test generate ephemeral ones).
// Production trust comes from Sigstore Fulcio/Rekor; this package defines the
// TransparencyLog interface with a file-backed dev implementation that is
// NEVER presented as production transparency (see docs/attestations.md).
package sigstore

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

const (
	// StatementType is the in-toto statement envelope content.
	StatementType = "https://in-toto.io/Statement/v1"
	// ProvenancePredicate is the SLSA v1.2 provenance predicate (current).
	ProvenancePredicate = "https://slsa.dev/provenance/v1"
	// PayloadType is the DSSE payload type for in-toto statements.
	PayloadType = "application/vnd.in-toto+json"
)

// Envelope is a standard DSSE envelope (JSON serialization).
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"` // base64
	Signatures  []Signature `json:"signatures"`
}

// Signature binds one signature to a key id.
type Signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"` // base64
}

// Statement is the in-toto/SLSA provenance subset Quorum produces.
type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Predicate `json:"predicate"`
}

// Subject names the artifact under attestation.
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Predicate carries the SLSA build definition + run details.
type Predicate struct {
	BuildDefinition map[string]any `json:"buildDefinition"`
	RunDetails      map[string]any `json:"runDetails"`
}

// GenerateKey creates a fresh P-256 signing key.
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// KeyID identifies a key as hex(sha256(DER(public))).
func KeyID(pub *ecdsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// PrivateToPEM encodes a key (0600 when written to disk by callers).
func PrivateToPEM(priv *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}

// ParsePrivatePEM decodes a PEM elliptic-curve private key.
func ParsePrivatePEM(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("INVALID_INPUT: not a PEM block")
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if ec, ok := k.(*ecdsa.PrivateKey); ok {
			return ec, nil
		}
	}
	return nil, fmt.Errorf("INVALID_INPUT: no ECDSA private key in PEM")
}

// PublicToPEM encodes a public key.
func PublicToPEM(pub *ecdsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// ParsePublicPEM decodes a PEM public key (P-256 only).
func ParsePublicPEM(raw []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("INVALID_INPUT: not a PEM block")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("INVALID_INPUT: bad public key: %v", err)
	}
	ec, ok := k.(*ecdsa.PublicKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, fmt.Errorf("INVALID_INPUT: only P-256 keys accepted")
	}
	return ec, nil
}

// preAuth builds the DSSE Pre-Auth Encoding for payloadType + body.
func preAuth(payloadType string, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString("DSSEv1 ")
	b.WriteString(strconv.Itoa(len(payloadType)))
	b.WriteByte(' ')
	b.WriteString(payloadType)
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(len(body)))
	b.WriteByte(' ')
	b.Write(body)
	b.WriteByte(' ')
	b.WriteString("0 ")
	return b.Bytes()
}

// SignStatement canonical-signs a statement into a DSSE envelope.
func SignStatement(st Statement, priv *ecdsa.PrivateKey) (Envelope, error) {
	body, err := json.Marshal(st)
	if err != nil {
		return Envelope{}, err
	}
	pae := preAuth(PayloadType, body)
	digest := sha256.Sum256(pae)
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return Envelope{}, err
	}
	sig := append(pad32(r.Bytes()), pad32(s.Bytes())...)
	kid, err := KeyID(&priv.PublicKey)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		PayloadType: PayloadType,
		Payload:     base64.StdEncoding.EncodeToString(body),
		Signatures:  []Signature{{KeyID: kid, Sig: base64.StdEncoding.EncodeToString(sig)}},
	}, nil
}

// digestEqual compares digests with or without the "sha256:" scheme prefix.
func digestEqual(got, want string) bool {
	got = strings.TrimPrefix(strings.ToLower(got), "sha256:")
	want = strings.TrimPrefix(strings.ToLower(want), "sha256:")
	return got != "" && got == want
}

func pad32(b []byte) []byte {
	if len(b) >= 32 {
		return b[len(b)-32:]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

// ProvenanceStatement builds the standard statement Quorum signs.
func ProvenanceStatement(artifactName, artifactDigestHex, sourceCommit, builderID, buildType string) Statement {
	return Statement{
		Type:          StatementType,
		Subject:       []Subject{{Name: artifactName, Digest: map[string]string{"sha256": artifactDigestHex}}},
		PredicateType: ProvenancePredicate,
		Predicate: Predicate{
			BuildDefinition: map[string]any{
				"buildType":            buildType,
				"externalParameters":   map[string]any{"sourceCommit": sourceCommit},
				"resolvedDependencies": []any{map[string]any{"uri": "git+https://example.invalid/repo@" + sourceCommit}},
			},
			RunDetails: map[string]any{
				"builder":  map[string]any{"id": builderID},
				"metadata": map[string]any{"invocationId": "quorum-" + time.Now().UTC().Format("20060102T150405Z")},
			},
		},
	}
}

// VerifyPolicy checks a DSSE envelope against trust policy. Every binding is
// explicit: signature by a trusted key, expected subject, expected commit,
// allowed builders. Anything else fails with a typed code.
func VerifyPolicy(env Envelope, trusted []*ecdsa.PublicKey, expectedDigest, expectedCommit string, allowedBuilders []string) (Statement, error) {
	if env.PayloadType != PayloadType {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: unexpected payload type %q", env.PayloadType)
	}
	body, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil || len(env.Signatures) == 0 {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: malformed envelope")
	}
	pae := preAuth(env.PayloadType, body)
	digest := sha256.Sum256(pae)
	sigOK := false
	for _, sig := range env.Signatures {
		raw, err := base64.StdEncoding.DecodeString(sig.Sig)
		if err != nil || len(raw) != 64 {
			continue
		}
		r := new(big.Int).SetBytes(raw[:32])
		s := new(big.Int).SetBytes(raw[32:])
		for _, pub := range trusted {
			kid, _ := KeyID(pub)
			if sig.KeyID != "" && !strings.EqualFold(sig.KeyID, kid) {
				continue
			}
			if ecdsa.Verify(pub, digest[:], r, s) {
				sigOK = true
				break
			}
		}
		if sigOK {
			break
		}
	}
	if !sigOK {
		return Statement{}, fmt.Errorf("SIGNATURE_INVALID: no trusted signature over this payload")
	}
	var st Statement
	if err := json.Unmarshal(body, &st); err != nil {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: bad statement JSON: %v", err)
	}
	if st.Type != StatementType {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: unknown statement type")
	}
	if st.PredicateType != ProvenancePredicate {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: unknown predicate")
	}
	if len(st.Subject) == 0 || (expectedDigest != "" && !digestEqual(st.Subject[0].Digest["sha256"], expectedDigest)) {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: SUBJECT_DIGEST_MISMATCH")
	}
	gotCommit := ""
	if ep, ok := st.Predicate.BuildDefinition["externalParameters"].(map[string]any); ok {
		gotCommit, _ = ep["sourceCommit"].(string)
	}
	if expectedCommit != "" && gotCommit != expectedCommit {
		return Statement{}, fmt.Errorf("ATTESTATION_INVALID: SOURCE_COMMIT_MISMATCH")
	}
	builder := ""
	if rd, ok := st.Predicate.RunDetails["builder"].(map[string]any); ok {
		builder, _ = rd["id"].(string)
	}
	if len(allowedBuilders) > 0 {
		allowed := false
		for _, b := range allowedBuilders {
			if b == builder {
				allowed = true
			}
		}
		if !allowed {
			return Statement{}, fmt.Errorf("SIGNER_NOT_TRUSTED: builder %q not allowlisted", builder)
		}
	}
	return st, nil
}
