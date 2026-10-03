package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/quorum/quorum/services/policy"
)

func baseOpts() runner.VerifyOptions {
	return runner.VerifyOptions{
		Repo: "https://github.com/example/tiny", Commit: "abc123",
		FixtureTiny: true, ConflictTolerance: -1,
	}
}

// Exit-code matrix for verify (prompt section 40).
func TestVerifyExitMatrix(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*runner.VerifyOptions)
		decision string
		code     int
	}{
		{"valid verification", func(o *runner.VerifyOptions) {}, policy.DecisionVerified, 0},
		{"tampered artifact", func(o *runner.VerifyOptions) { o.InjectTamper = true; o.ExpectedDigest = "sha256:" + strings.Repeat("0", 64) }, policy.DecisionRejected, 1},
		{"conflicting builder", func(o *runner.VerifyOptions) { o.InjectConflictBuilder = "builder-c" }, policy.DecisionVerifiedConflict, 3},
		{"insufficient quorum", func(o *runner.VerifyOptions) { o.MinBuilders = 5 }, policy.DecisionInsufficient, 2},
		{"strict policy + conflict, tolerance 0", func(o *runner.VerifyOptions) {
			o.PolicyFile = "../../apps/cli/testdata/policy-strict.json"
			o.InjectConflictBuilder = "builder-c"
		}, policy.DecisionInvestigate, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := baseOpts()
			tc.mutate(&o)
			res, code, human, err := runner.RunVerify(o)
			if err != nil {
				t.Fatalf("unexpected operational error: %v", err)
			}
			if res.Decision != tc.decision {
				t.Fatalf("want %s, got %s (%v)", tc.decision, res.Decision, res.Reasons)
			}
			if code != tc.code {
				t.Fatalf("want exit %d, got %d", tc.code, code)
			}
			if !strings.Contains(human, tc.decision) {
				t.Fatalf("human output must name the decision")
			}
			if strings.Contains(human, "100% safe") {
				t.Fatalf("must never claim absolute safety")
			}
		})
	}
}

func TestVerifyInvalidInputs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*runner.VerifyOptions)
	}{
		{"missing repo", func(o *runner.VerifyOptions) { o.Repo = "" }},
		{"missing commit", func(o *runner.VerifyOptions) { o.Commit = "" }},
		{"http repo", func(o *runner.VerifyOptions) { o.Repo = "http://github.com/example/tiny" }},
		{"evil host", func(o *runner.VerifyOptions) { o.Repo = "https://evil.example/x" }},
		{"metadata IP", func(o *runner.VerifyOptions) { o.Repo = "https://169.254.169.254/x" }},
		{"localhost", func(o *runner.VerifyOptions) { o.Repo = "https://localhost:8080/x" }},
		{"credentialed url", func(o *runner.VerifyOptions) { o.Repo = "https://user:pass@github.com/example/tiny" }},
		{"missing artifact file", func(o *runner.VerifyOptions) { o.FixtureTiny = false; o.Artifact = "no-such-file.tar.gz" }},
		{"metachar artifact", func(o *runner.VerifyOptions) { o.FixtureTiny = false; o.Artifact = "a;b.tar.gz" }},
		{"bad package ref", func(o *runner.VerifyOptions) { o.Repo = ""; o.Commit = ""; o.PkgRef = "docker:nginx" }},
		{"bad policy file", func(o *runner.VerifyOptions) { o.PolicyFile = "nope.json" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := baseOpts()
			tc.mutate(&o)
			_, code, _, err := runner.RunVerify(o)
			if err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
			if code != exitcodes.InvalidInput {
				t.Fatalf("want exit 5, got %d (%v)", code, err)
			}
		})
	}
}

func TestVerifyRealFileAndTamper(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "tiny.tar.gz")
	if err := os.WriteFile(artifact, runner.TinyFixtureBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	o := baseOpts()
	o.FixtureTiny = false
	o.Artifact = artifact
	res, code, _, err := runner.RunVerify(o)
	if err != nil || code != 0 || res.Decision != policy.DecisionVerified {
		t.Fatalf("real file must verify: %v %d %v", err, code, res)
	}
	// Flip one byte on disk: rebuilt digest changes, expected pins the old one.
	evil := append(append([]byte(nil), runner.TinyFixtureBytes...), 'X')
	if err := os.WriteFile(artifact, evil, 0o600); err != nil {
		t.Fatal(err)
	}
	o2 := baseOpts()
	o2.FixtureTiny = false
	o2.Artifact = artifact
	o2.ExpectedDigest = res.Artifact
	res2, code2, _, err := runner.RunVerify(o2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Decision != policy.DecisionRejected || code2 != 1 {
		t.Fatalf("1-byte change must reject: %+v", res2)
	}
}

func TestOversizeRejected(t *testing.T) {
	o := baseOpts()
	o.MaxBytes = 4 // tiny bytes are 23 bytes
	res, code, _, err := runner.RunVerify(o)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != policy.DecisionRejected || code != 1 {
		t.Fatalf("oversize must reject: %+v", res)
	}
}

func TestGuards(t *testing.T) {
	if runner.ValidateRepoURL("https://github.com/o/r") != nil {
		t.Fatal("github must be allowed")
	}
	for _, bad := range []string{"http://github.com/o/r", "https://evil.example/x", "https://169.254.169.254/", "https://localhost:1/x", "not a url"} {
		if runner.ValidateRepoURL(bad) == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
	for _, bad := range []string{"", "a;b", "a|b", "a$b", "nul\x00x", "has\nnewline", strings.Repeat("a", 600)} {
		if runner.ValidateArtifactPath(bad) == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
	// Absolute paths and .. segments are usable (opened directly, no join).
	for _, ok := range []string{"tiny-1.0.0.tar.gz", "/tmp/x.tar.gz", `C:\tmp\x.tar.gz`, "a/../../b.tar.gz"} {
		if runner.ValidateArtifactPath(ok) != nil {
			t.Fatalf("%q must pass", ok)
		}
	}
	if eco, name, ver, ok := runner.ParsePackageRef("pypi:quorum-tiny@1.0.0"); !ok || eco != "pypi" || name != "quorum-tiny" || ver != "1.0.0" {
		t.Fatal("package ref parse failed")
	}
	if _, _, _, ok := runner.ParsePackageRef("docker:nginx"); ok {
		t.Fatal("unsupported ecosystem must not parse")
	}
	if st, _ := runner.OssFixture("pypi:quorum-tiny@1.0.0"); st != "SUPPORTED_AND_VERIFIED" {
		t.Fatalf("tiny fixture must verify, got %s", st)
	}
	if st, _ := runner.OssFixture("npm:express@4.0.0"); st != "UNSUPPORTED" {
		t.Fatalf("unknown package must be UNSUPPORTED, got %s", st)
	}
}

func TestPolicyLoadAndTest(t *testing.T) {
	pol, err := runner.LoadPolicyFile("../../apps/cli/testdata/policy-a.json")
	if err != nil {
		t.Fatal(err)
	}
	if pol.PolicyID != "policy-a" {
		t.Fatalf("wrong policy: %+v", pol)
	}
	h1, _ := runner.PolicyHash(pol)
	h2, _ := runner.PolicyHash(pol)
	if h1 != h2 || !strings.HasPrefix(h1, "sha256:") {
		t.Fatal("policy hash must be deterministic")
	}
	res, _, err := runner.RunPolicyTest("../../apps/cli/testdata/policy-a.json", "../../apps/cli/testdata/evidence-agree.json")
	if err != nil || res.Decision != policy.DecisionVerified {
		t.Fatalf("agree evidence must verify: %v %+v", err, res)
	}
	res, _, err = runner.RunPolicyTest("../../apps/cli/testdata/policy-a.json", "../../apps/cli/testdata/evidence-conflict.json")
	if err != nil || res.Decision != policy.DecisionVerifiedConflict || len(res.Conflicts) != 1 {
		t.Fatalf("conflict must surface: %v %+v", err, res)
	}
	if _, _, err := runner.RunPolicyTest("missing.json", "../../apps/cli/testdata/evidence-agree.json"); err == nil {
		t.Fatal("missing policy must error")
	}
	// Invalid policy content.
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"policyId":"x","minBuilders":0}`), 0o600)
	if _, err := runner.LoadPolicyFile(bad); err == nil {
		t.Fatal("minBuilders 0 must fail validation")
	}
	malformed := filepath.Join(dir, "mal.json")
	os.WriteFile(malformed, []byte(`{oops`), 0o600)
	if _, err := runner.LoadPolicyFile(malformed); err == nil {
		t.Fatal("malformed JSON must fail")
	}
}

func TestAuditChain(t *testing.T) {
	chain, err := runner.DemoChain()
	if err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := runner.VerifyChain(chain); !ok {
		t.Fatal("demo chain must verify")
	}
	chain[1].Payload["builderId"] = "builder-EVIL"
	ok, at, _ := runner.VerifyChain(chain)
	if ok || at != 1 {
		t.Fatalf("tamper must break at record 1, got ok=%v at=%d", ok, at)
	}
	// Broken previous-hash link.
	chain2, _ := runner.DemoChain()
	chain2[2].PreviousHash = "forged"
	if ok, _, _ := runner.VerifyChain(chain2); ok {
		t.Fatal("forged link must fail")
	}
}

func TestRegistry(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "builders.json")
	if regs, err := runner.LoadRegistry(reg); err != nil || len(regs) != 0 {
		t.Fatal("missing registry must load empty")
	}
	if err := runner.Register(reg, runner.BuilderRegistration{ID: "builder-a", IndependenceGroup: "cloud-a"}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Register(reg, runner.BuilderRegistration{ID: "builder-a", IndependenceGroup: "cloud-b"}); err == nil {
		t.Fatal("duplicate id must fail")
	}
	if err := runner.Register(reg, runner.BuilderRegistration{ID: "builder-b"}); err == nil {
		t.Fatal("missing group must fail")
	}
	regs, err := runner.LoadRegistry(reg)
	if err != nil || len(regs) != 1 || regs[0].IndependenceGroup != "cloud-a" {
		t.Fatalf("registry round-trip failed: %v %+v", err, regs)
	}
}

func TestExitMapping(t *testing.T) {
	cases := map[string]int{
		"VERIFIED": 0, "REJECTED": 1, "INSUFFICIENT_EVIDENCE": 2,
		"VERIFIED_WITH_CONFLICT": 3, "INVESTIGATE": 3, "ERROR": 4, "BOGUS": 4,
	}
	for decision, want := range cases {
		if got := exitcodes.ForDecision(decision); got != want {
			t.Fatalf("%s: want %d got %d", decision, want, got)
		}
	}
}

func TestBlockchainInputValidation(t *testing.T) {
	// Unreachable RPC with Require=false must SKIP, never claim anchored.
	res, err := runner.RunAnchor(runner.AnchorOptions{
		RPC: "http://127.0.0.1:9", Contract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		VerificationID: "0x" + strings.Repeat("1", 64), EvidenceHash: "0x" + strings.Repeat("2", 64),
		ArtifactDigest: "0x" + strings.Repeat("3", 64), SourceCommit: "0x" + strings.Repeat("4", 64),
		PolicyHash: "0x" + strings.Repeat("5", 64), Decision: "VERIFIED",
	})
	if err != nil {
		t.Fatalf("unrequired unreachable chain must skip, not error: %v", err)
	}
	if !res.Skipped || res.Anchored {
		t.Fatalf("must be skipped+unanchored: %+v", res)
	}
	// Same call with Require=true must fail loudly.
	if _, err := runner.RunAnchor(runner.AnchorOptions{
		RPC: "http://127.0.0.1:9", Require: true, Contract: "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		VerificationID: "0x" + strings.Repeat("1", 64), EvidenceHash: "0x" + strings.Repeat("2", 64),
		ArtifactDigest: "0x" + strings.Repeat("3", 64), SourceCommit: "0x" + strings.Repeat("4", 64),
		PolicyHash: "0x" + strings.Repeat("5", 64), Decision: "VERIFIED",
	}); err == nil {
		t.Fatal("required unreachable chain must error")
	}
	// Bad hex / address / decision rejected as input errors.
	bad := runner.AnchorOptions{
		RPC: "http://127.0.0.1:9", Contract: "not-an-address",
		VerificationID: "0x123", EvidenceHash: "0x" + strings.Repeat("2", 64),
		ArtifactDigest: "0x" + strings.Repeat("3", 64), SourceCommit: "0x" + strings.Repeat("4", 64),
		PolicyHash: "0x" + strings.Repeat("5", 64), Decision: "VERIFIED",
	}
	if _, err := runner.RunAnchor(bad); err == nil || !strings.HasPrefix(err.Error(), "INVALID_INPUT") {
		t.Fatalf("bad contract must be INVALID_INPUT: %v", err)
	}
	bad.Contract = "0x5FbDB2315678afecb367f032d93F642f64180aa3"
	bad.Decision = "BOGUS"
	if _, err := runner.RunAnchor(bad); err == nil {
		t.Fatal("bogus decision must error")
	}
	if n, _ := runner.DecisionToUint8("VERIFIED"); n != 0 {
		t.Fatal("VERIFIED must encode 0")
	}
	if n, _ := runner.DecisionToUint8("ERROR"); n != 5 {
		t.Fatal("ERROR must encode 5")
	}
}

func TestOssModeValidation(t *testing.T) {
	o := baseOpts()
	o.OssMode = "live" // no PkgRef: nothing to look up
	_, code, _, err := runner.RunVerify(o)
	if err == nil || code != exitcodes.InvalidInput {
		t.Fatalf("live mode without package ref must be invalid input: %v", code)
	}
}

// TestOssLiveWiring shells the real CLI (env-gated). The tiny fixture digest
// never equals the absl-py upstream digest, so the OSS row must appear as a
// surfaced conflict or a source exclusion -- never vanish, never flip the
// builders agreement.
func TestOssLiveWiring(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_OSS") != "1" {
		t.Skip("set QUORUM_LIVE_OSS=1 with oss-rebuild on PATH")
	}
	o := baseOpts()
	o.PkgRef = "pypi:absl-py@2.0.0"
	o.FixtureTiny = true
	o.Repo = "https://pypi.org/project/absl-py"
	o.Commit = "abc123"
	o.OssMode = "live"
	res, code, _, err := runner.RunVerify(o)
	if err != nil {
		t.Fatal(err)
	}
	if res.OssState != "SUPPORTED_AND_VERIFIED" {
		t.Fatalf("want live verified state, got %s (%v)", res.OssState, res.Reasons)
	}
	if code != 0 && code != 3 {
		t.Fatalf("builders agree, so decision must be 0 or 3, got %d (%s)", code, res.Decision)
	}
	seen := false
	for _, c := range res.Conflicts {
		for _, b := range c.Builders {
			if b == "oss-rebuild" {
				seen = true
			}
		}
	}
	for _, x := range res.Excluded {
		if x.Evidence.BuilderID == "oss-rebuild" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("oss-rebuild evidence must be visible (conflict or exclusion): %+v", res)
	}
}