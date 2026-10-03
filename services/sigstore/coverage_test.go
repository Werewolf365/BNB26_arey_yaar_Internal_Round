package sigstore_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quorum/quorum/services/sigstore"
)

func TestPKCS8PrivateKey(t *testing.T) {
	priv := mustKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	back, err := sigstore.ParsePrivatePEM(raw)
	if err != nil {
		t.Fatalf("PKCS8 PEM must parse: %v", err)
	}
	k1, _ := sigstore.KeyID(&priv.PublicKey)
	k2, _ := sigstore.KeyID(&back.PublicKey)
	if k1 != k2 {
		t.Fatal("PKCS8 round-trip must preserve identity")
	}
	// PKCS8 wrapping a non-ECDSA key must fail.
	rsakey, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsader, _ := x509.MarshalPKCS8PrivateKey(rsakey)
	if _, err := sigstore.ParsePrivatePEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsader})); err == nil {
		t.Fatal("PKCS8 RSA must fail")
	}
	// Public PEM for a non-P-256 key must fail.
	rsapub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustMarshalPKIX(t, &rsakey.PublicKey)})
	if _, err := sigstore.ParsePublicPEM(rsapub); err == nil {
		t.Fatal("RSA public key must fail (P-256 only)")
	}
}

func mustMarshalPKIX(t *testing.T, k any) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestMalformedEnvelopes(t *testing.T) {
	priv := mustKey(t)
	trusted := trustedOf(t, priv)
	st := sigstore.ProvenanceStatement("x", strings.Repeat("a", 64), "c", "builder-a", "type")
	env, err := sigstore.SignStatement(st, priv)
	if err != nil {
		t.Fatal(err)
	}
	// Bad base64 payload.
	bad := env
	bad.Payload = "!!!"
	if _, err := sigstore.VerifyPolicy(bad, trusted, "", "", nil); err == nil {
		t.Fatal("bad base64 must fail")
	}
	// No signatures.
	empty := env
	empty.Signatures = nil
	if _, err := sigstore.VerifyPolicy(empty, trusted, "", "", nil); err == nil {
		t.Fatal("unsigned envelope must fail")
	}
	// Payload that is not statement JSON (re-sign garbage statement bytes
	// through a tampered-but-resigned envelope is impossible without the key,
	// so craft: valid signature over non-JSON is unreachable; instead flip the
	// decoded statement type by signing a hand-built statement).
	odd := sigstore.Statement{
		Type: "https://example.invalid/Statement/v9", Subject: st.Subject,
		PredicateType: st.PredicateType, Predicate: st.Predicate,
	}
	oddEnv, _ := sigstore.SignStatement(odd, priv)
	if _, err := sigstore.VerifyPolicy(oddEnv, trusted, "", "", nil); err == nil {
		t.Fatal("unknown statement type must fail")
	}
	odd2 := st
	odd2.PredicateType = "https://example.invalid/provenance/v9"
	oddEnv2, _ := sigstore.SignStatement(odd2, priv)
	if _, err := sigstore.VerifyPolicy(oddEnv2, trusted, "", "", nil); err == nil {
		t.Fatal("unknown predicate must fail")
	}
	odd3 := st
	odd3.Subject = nil
	oddEnv3, _ := sigstore.SignStatement(odd3, priv)
	if _, err := sigstore.VerifyPolicy(oddEnv3, trusted, strings.Repeat("a", 64), "", nil); err == nil {
		t.Fatal("empty subject must fail")
	}
	// Wrong payload type.
	badType := env
	badType.PayloadType = "application/json"
	if _, err := sigstore.VerifyPolicy(badType, trusted, "", "", nil); err == nil {
		t.Fatal("wrong payload type must fail")
	}
	// Corrupt signature bytes (bad base64 / wrong length) are skipped, then
	// no trusted signature remains.
	corrupt := env
	corrupt.Signatures = []sigstore.Signature{{KeyID: env.Signatures[0].KeyID, Sig: base64.StdEncoding.EncodeToString([]byte("short"))}}
	if _, err := sigstore.VerifyPolicy(corrupt, trusted, "", "", nil); err == nil {
		t.Fatal("corrupt signature must fail")
	}
}

func TestTransparencyEdges(t *testing.T) {
	dir := t.TempDir()
	// Garbage lines are ignored; first append starts at genesis.
	logPath := filepath.Join(dir, "t.jsonl")
	os.WriteFile(logPath, []byte("garbage\n{}\r\n{\"nohash\":1}\n"), 0o600)
	l := sigstore.NewFileLog(logPath)
	e0, err := l.Append("stmt", "k")
	if err != nil || e0.Index != 0 || e0.PrevHash != "GENESIS" {
		t.Fatalf("genesis after garbage: %+v %v", e0, err)
	}
	// Bad genesis link.
	bad := e0
	bad.PrevHash = "forged"
	if err := l.Verify(bad); err == nil {
		t.Fatal("bad genesis link must fail")
	}
	// Predecessor missing.
	ghost := e0
	ghost.Index = 5
	if err := l.Verify(ghost); err == nil {
		t.Fatal("missing predecessor must fail")
	}
	// Unwritable log (directory as file).
	dl := sigstore.NewFileLog(dir)
	if _, err := dl.Append("stmt", "k"); err == nil {
		t.Fatal("unwritable log must fail append")
	}
	// Non-PEM public block.
	if _, err := sigstore.ParsePublicPEM([]byte("nope")); err == nil {
		t.Fatal("non-PEM public key must fail")
	}
}
