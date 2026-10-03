package sigstore

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestPad32Shapes(t *testing.T) {
	if got := pad32([]byte{0x01}); len(got) != 32 || got[31] != 0x01 || got[0] != 0 {
		t.Fatalf("short input must left-pad to 32: %x", got)
	}
	full := make([]byte, 32)
	for i := range full {
		full[i] = byte(i)
	}
	if got := pad32(full); len(got) != 32 || got[0] != 0 {
		t.Fatal("32-byte input must pass through")
	}
	long := append([]byte{0xff}, full...)
	if got := pad32(long); len(got) != 32 || got[0] != 0 {
		t.Fatal("long input must keep the low 32 bytes")
	}
}

func TestVerifyReadError(t *testing.T) {
	dir := t.TempDir()
	l := NewFileLog(filepath.Join(dir, "t.jsonl"))
	e, err := l.Append("stmt", "k")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	// Point at an unreadable history: verification of a non-genesis entry
	// must surface the read failure instead of hanging or panicking.
	dirLog := NewFileLog(dir)
	chained := e
	chained.Index = 3
	if err := dirLog.Verify(chained); err == nil {
		t.Fatal("unreadable history must fail verify")
	}
}

func TestGenesisAndLinkPaths(t *testing.T) {
	// Bad genesis link with a MATCHING hash (index 0, non-GENESIS prev).
	sum := sha256.Sum256([]byte("forged\nstmt\nk"))
	badGen := Entry{Index: 0, StatementHash: "stmt", KeyID: "k", PrevHash: "forged",
		EntryHash: hex.EncodeToString(sum[:])}
	l := NewFileLog(filepath.Join(t.TempDir(), "t.jsonl"))
	if err := l.Verify(badGen); err == nil {
		t.Fatal("non-genesis prev at index 0 must fail")
	}
	// Broken link with a matching hash: predecessor exists, hash differs.
	dir := t.TempDir()
	l2 := NewFileLog(filepath.Join(dir, "t.jsonl"))
	e0, err := l2.Append("stmt", "k")
	if err != nil {
		t.Fatal(err)
	}
	sum2 := sha256.Sum256([]byte("other-prev\nstmt\nk"))
	broken := Entry{Index: 1, StatementHash: "stmt", KeyID: "k", PrevHash: "other-prev",
		EntryHash: hex.EncodeToString(sum2[:])}
	if err := l2.Verify(broken); err == nil {
		t.Fatal("diverged link must fail")
	}
	_ = e0
	// Unterminated last line still parses (splitLines flush branch).
	lp := filepath.Join(dir, "u.jsonl")
	os.WriteFile(lp, []byte("junk-without-newline"), 0o600)
	l3 := NewFileLog(lp)
	if _, err := l3.Append("stmt", "k"); err != nil {
		t.Fatalf("append after unterminated junk: %v", err)
	}
	// Append into a missing directory fails operationally.
	l4 := NewFileLog(filepath.Join(dir, "no-such-dir", "t.jsonl"))
	if _, err := l4.Append("stmt", "k"); err == nil {
		t.Fatal("append without parent dir must fail")
	}
}

func TestNonJSONPayloadFails(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	// Hand-sign bytes that are valid base64 but not statement JSON: the
	// signature verifies, then statement parsing must reject.
	body := []byte("not json{")
	pae := preAuth(PayloadType, body)
	digest := sha256.Sum256(pae)
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(pad32(r.Bytes()), pad32(s.Bytes())...)
	kid, _ := KeyID(&priv.PublicKey)
	env := Envelope{
		PayloadType: PayloadType,
		Payload:     base64.StdEncoding.EncodeToString(body),
		Signatures:  []Signature{{KeyID: kid, Sig: base64.StdEncoding.EncodeToString(sig)}},
	}
	if _, err := VerifyPolicy(env, []*ecdsa.PublicKey{&priv.PublicKey}, "", "", nil); err == nil {
		t.Fatal("non-JSON payload with valid signature must fail parsing")
	}
}
