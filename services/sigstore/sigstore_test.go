package sigstore_test

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quorum/quorum/services/sigstore"
)

func mustKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := sigstore.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func trustedOf(t *testing.T, keys ...*ecdsa.PrivateKey) []*ecdsa.PublicKey {
	t.Helper()
	out := make([]*ecdsa.PublicKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, &k.PublicKey)
	}
	return out
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv := mustKey(t)
	st := sigstore.ProvenanceStatement("tiny.tar.gz", strings.Repeat("a", 64), "abc123", "https://quorum.example/builders/a", "https://quorum.example/buildtypes/tiny/v1")
	env, err := sigstore.SignStatement(st, priv)
	if err != nil {
		t.Fatal(err)
	}
	if env.PayloadType != "application/vnd.in-toto+json" || len(env.Signatures) != 1 {
		t.Fatalf("bad envelope: %+v", env)
	}
	if env.Signatures[0].KeyID == "" {
		t.Fatal("signature must carry a key id")
	}
	got, err := sigstore.VerifyPolicy(env, trustedOf(t, priv), strings.Repeat("a", 64), "abc123", []string{"https://quorum.example/builders/a"})
	if err != nil {
		t.Fatalf("valid envelope must verify: %v", err)
	}
	if got.PredicateType != sigstore.ProvenancePredicate {
		t.Fatal("predicate must survive round-trip")
	}
}

func TestWrongKeyFails(t *testing.T) {
	priv, other := mustKey(t), mustKey(t)
	st := sigstore.ProvenanceStatement("x", strings.Repeat("a", 64), "c", "builder-a", "type")
	env, _ := sigstore.SignStatement(st, priv)
	if _, err := sigstore.VerifyPolicy(env, trustedOf(t, other), strings.Repeat("a", 64), "c", nil); err == nil {
		t.Fatal("signature from untrusted key must fail")
	}
}

func TestPayloadTamperFails(t *testing.T) {
	priv := mustKey(t)
	st := sigstore.ProvenanceStatement("x", strings.Repeat("a", 64), "c", "builder-a", "type")
	env, _ := sigstore.SignStatement(st, priv)
	raw, _ := base64.StdEncoding.DecodeString(env.Payload)
	raw[len(raw)-10] ^= 0x01 // flip one byte of the signed statement
	env.Payload = base64.StdEncoding.EncodeToString(raw)
	if _, err := sigstore.VerifyPolicy(env, trustedOf(t, priv), strings.Repeat("a", 64), "c", nil); err == nil {
		t.Fatal("tampered payload must fail signature check")
	}
}

func TestReplayBindings(t *testing.T) {
	priv := mustKey(t)
	st := sigstore.ProvenanceStatement("x", strings.Repeat("a", 64), "commit-OLD", "builder-a", "type")
	env, _ := sigstore.SignStatement(st, priv)
	trusted := trustedOf(t, priv)
	if _, err := sigstore.VerifyPolicy(env, trusted, strings.Repeat("b", 64), "commit-OLD", nil); err == nil {
		t.Fatal("replay onto another artifact must fail")
	}
	if _, err := sigstore.VerifyPolicy(env, trusted, strings.Repeat("a", 64), "commit-NEW", nil); err == nil {
		t.Fatal("replay onto another commit must fail")
	}
	if _, err := sigstore.VerifyPolicy(env, trusted, strings.Repeat("a", 64), "commit-OLD", []string{"builder-allowed"}); err == nil {
		t.Fatal("non-allowlisted builder must fail")
	}
}

func TestKeyPEM(t *testing.T) {
	priv := mustKey(t)
	raw, err := sigstore.PrivateToPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	back, err := sigstore.ParsePrivatePEM(raw)
	if err != nil {
		t.Fatal(err)
	}
	k1, _ := sigstore.KeyID(&priv.PublicKey)
	k2, _ := sigstore.KeyID(&back.PublicKey)
	if k1 != k2 || k1 == "" {
		t.Fatal("PEM round-trip must preserve identity")
	}
	pubRaw, _ := sigstore.PublicToPEM(&priv.PublicKey)
	pub, err := sigstore.ParsePublicPEM(pubRaw)
	if err != nil {
		t.Fatal(err)
	}
	k3, _ := sigstore.KeyID(pub)
	if k3 != k1 {
		t.Fatal("public PEM must resolve to same key id")
	}
	if _, err := sigstore.ParsePrivatePEM([]byte("nope")); err == nil {
		t.Fatal("garbage PEM must fail")
	}
	if _, err := sigstore.ParsePublicPEM([]byte("-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----")); err == nil {
		t.Fatal("bad public key must fail")
	}
}

func TestTransparencyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transparency.jsonl")
	l := sigstore.NewFileLog(path)
	sum := sha256.Sum256([]byte("stmt"))
	sh := hex.EncodeToString(sum[:])
	e0, err := l.Append(sh, "key-1")
	if err != nil || e0.Index != 0 || e0.PrevHash != "GENESIS" {
		t.Fatalf("genesis: %v %+v", err, e0)
	}
	e1, err := l.Append(sh, "key-1")
	if err != nil || e1.PrevHash != e0.EntryHash {
		t.Fatal("chain linkage")
	}
	if err := l.Verify(e0); err != nil {
		t.Fatal(err)
	}
	if err := l.Verify(e1); err != nil {
		t.Fatal(err)
	}
	forged := e1
	forged.StatementHash = "forged"
	if err := l.Verify(forged); err == nil {
		t.Fatal("forged entry must fail")
	}
	broken := e1
	broken.PrevHash = "forged-link"
	if err := l.Verify(broken); err == nil {
		t.Fatal("broken link must fail")
	}
}

func TestDigestPrefixNormalization(t *testing.T) {
	priv := mustKey(t)
	hexDigest := strings.Repeat("a", 64)
	st := sigstore.ProvenanceStatement("x", hexDigest, "c", "builder-a", "type")
	env, _ := sigstore.SignStatement(st, priv)
	trusted := trustedOf(t, priv)
	for _, want := range []string{hexDigest, "sha256:" + hexDigest, "SHA256:" + strings.ToUpper(hexDigest)} {
		if _, err := sigstore.VerifyPolicy(env, trusted, want, "c", nil); err != nil {
			t.Fatalf("digest form %q must verify: %v", want, err)
		}
	}
	if _, err := sigstore.VerifyPolicy(env, trusted, "", "c", nil); err != nil {
		t.Fatalf("empty expectation must not constrain: %v", err)
	}
}
