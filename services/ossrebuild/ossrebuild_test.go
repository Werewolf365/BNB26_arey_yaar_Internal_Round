package ossrebuild_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/quorum/quorum/services/ossrebuild"
)

func TestMapRef(t *testing.T) {
	cases := []struct {
		in             string
		eco, name, ver string
		wantErr        string
	}{
		{"pypi:absl-py@2.0.0", "pypi", "absl-py", "2.0.0", ""},
		{"npm:left-pad@1.3.0", "npm", "left-pad", "1.3.0", ""},
		{"cargo:serde@1.0.0", "cratesio", "serde", "1.0.0", ""},
		{"docker:nginx", "", "", "", "UNSUPPORTED"},
		{"pypi:noversion", "", "", "", "INVALID_INPUT"},
	}
	for _, tc := range cases {
		eco, name, ver, err := ossrebuild.MapRef(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s: want %s, got %v", tc.in, tc.wantErr, err)
			}
			continue
		}
		if err != nil || eco != tc.eco || name != tc.name || ver != tc.ver {
			t.Fatalf("%s: got %s/%s/%s err=%v", tc.in, eco, name, ver, err)
		}
	}
}

func TestIsProvenancePayload(t *testing.T) {
	good := `{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1"}`
	if !ossrebuild.IsProvenancePayload(good) {
		t.Fatal("valid envelope must pass")
	}
	for _, bad := range []string{
		`{}`,
		`{"_type":"https://in-toto.io/Statement/v1"}`,
		`{"_type":"x","predicateType":"https://slsa.dev/provenance/v1"}`,
		`not json`,
	} {
		if ossrebuild.IsProvenancePayload(bad) {
			t.Fatalf("must reject %q", bad)
		}
	}
}

func TestProbeMissingBinary(t *testing.T) {
	p := ossrebuild.Provider{Bin: "oss-rebuild-definitely-missing", Timeout: 5 * time.Second, MaxAttempts: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.Probe(ctx); err == nil {
		t.Fatal("missing binary must fail probe")
	}
	if _, err := p.Lookup(ctx, "docker:nginx"); err != nil {
		t.Fatalf("unsupported ref must return a state, not an error: %v", err)
	}
}

// TestLiveLookup hits the real service (env-gated, needs network + CLI).
// Pinned expectations: absl-py 2.0.0 is rebuilt with this upstream digest.
func TestLiveLookup(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_OSS") != "1" {
		t.Skip("set QUORUM_LIVE_OSS=1 with oss-rebuild on PATH")
	}
	p := ossrebuild.DefaultProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	if err := p.Probe(ctx); err != nil {
		t.Skipf("oss-rebuild CLI absent: %v", err)
	}
	got, err := p.Lookup(ctx, "pypi:absl-py@2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ossrebuild.StateVerified {
		t.Fatalf("want SUPPORTED_AND_VERIFIED, got %s (%s)", got.State, got.Detail)
	}
	if got.UpstreamDigest != "sha256:9a28abb62774ae4e8edbe2dd4c49ffcd45a6a848952a5eccc6a49f3f0fc1e2f3" {
		t.Fatalf("unexpected upstream digest %q", got.UpstreamDigest)
	}
	if got.SourceCommit != "37dad4d356ca9e13f1c533ad6309631b397a2b6b" {
		t.Fatalf("unexpected source commit %q", got.SourceCommit)
	}
	if len(got.Raw) == 0 {
		t.Fatal("raw payload must be preserved")
	}
	missing, err := p.Lookup(ctx, "pypi:absl-py@0.0.0-bogus-quorum")
	if err != nil {
		t.Fatal(err)
	}
	if missing.State != ossrebuild.StateNotFound {
		t.Fatalf("bogus version must be NOT_FOUND, got %s", missing.State)
	}
	unsup, err := p.Lookup(ctx, "docker:nginx")
	if err != nil || unsup.State != ossrebuild.StateUnsupported {
		t.Fatalf("docker ref must be UNSUPPORTED: %v %+v", err, unsup)
	}
}
