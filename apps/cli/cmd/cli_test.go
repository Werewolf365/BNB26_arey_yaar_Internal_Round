package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execute(args ...string) (code int, stdout, stderr string) {
	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	if e, ok := err.(*exitErr); ok {
		return e.code, outBuf.String(), errBuf.String()
	}
	return 4, outBuf.String(), errBuf.String()
}

func TestVerifyMatrix(t *testing.T) {
	base := []string{"verify", "--repo", "https://github.com/example/tiny", "--commit", "abc123", "--fixture-tiny"}
	if code, out, _ := execute(base...); code != 0 || !strings.Contains(out, "VERIFIED") {
		t.Fatalf("valid must exit 0 VERIFIED: %d %s", code, out)
	}
	if code, out, _ := execute(append(append([]string{}, base...), "--json")...); code != 0 {
		t.Fatalf("json valid must exit 0: %d", code)
	} else {
		var v map[string]any
		if err := json.Unmarshal([]byte(out), &v); err != nil || v["decision"] != "VERIFIED" {
			t.Fatalf("json must carry decision VERIFIED: %v %s", err, out)
		}
	}
	if code, out, _ := execute(append(append([]string{}, base...), "--quiet")...); code != 0 || out != "" {
		t.Fatalf("quiet must be silent with exit 0: %d %q", code, out)
	}
	// Quiet suppresses stdout reports but never swallows stderr errors.
	if code, out, errOut := execute("verify", "--repo", "https://evil.example/x", "--commit", "c", "--fixture-tiny", "--quiet"); code != 5 || out != "" || !strings.Contains(errOut, "INVALID_INPUT") {
		t.Fatalf("quiet errors must reach stderr with exit 5: %d %q %q", code, out, errOut)
	}
	// JSON mode reports errors as envelopes on stderr, never silence.
	if code, out, errOut := execute("verify", "--repo", "https://evil.example/x", "--commit", "c", "--fixture-tiny", "--json"); code != 5 || out != "" {
		t.Fatalf("json errors must not pollute stdout: %d %q", code, out)
	} else {
		var env map[string]string
		if err := json.Unmarshal([]byte(errOut), &env); err != nil || !strings.Contains(env["error"], "INVALID_INPUT") {
			t.Fatalf("json error envelope: %v %q", err, errOut)
		}
	}
	// Live OSS mode without a package ref is invalid input.
	if code, _, _ := execute("verify", "--repo", "https://github.com/example/tiny", "--commit", "c", "--fixture-tiny", "--oss-mode", "live"); code != 5 {
		t.Fatalf("live without ref must exit 5, got %d", code)
	}
	if code, _, _ := execute(append(append([]string{}, base...), "--inject-conflict", "builder-c")...); code != 3 {
		t.Fatalf("conflict must exit 3, got %d", code)
	}
	if code, out, _ := execute(append(append([]string{}, base...), "--inject-tamper", "--expected-digest", "sha256:"+strings.Repeat("0", 64))...); code != 1 || !strings.Contains(out, "ARTIFACT_HASH_MISMATCH") {
		t.Fatalf("tamper must exit 1 with mismatch: %d %s", code, out)
	}
	if code, _, _ := execute(append(append([]string{}, base...), "--min-builders", "5")...); code != 2 {
		t.Fatalf("insufficient must exit 2, got %d", code)
	}
	// Package-ref form.
	if code, out, _ := execute("verify", "pypi:quorum-tiny@1.0.0", "--json"); code != 0 || !strings.Contains(out, "SUPPORTED_AND_VERIFIED") {
		t.Fatalf("pkg ref must verify with OSS fixture: %d %s", code, out)
	}
	// Invalid inputs -> 5.
	for _, args := range [][]string{
		{"verify", "--commit", "abc123", "--fixture-tiny"},                         // no repo
		{"verify", "--repo", "http://github.com/e/t", "--commit", "c", "--fixture-tiny"},
		{"verify", "--repo", "https://evil.example/x", "--commit", "c", "--fixture-tiny"},
		{"verify", "--repo", "https://github.com/e/t", "--commit", "c"},             // no artifact source
		{"verify", "docker:nginx"},                                                  // bad ref
		{"verify", "--repo", "https://github.com/e/t", "--commit", "c", "--artifact", "missing.bin"},
		{"verify", "a", "b"}, // too many positionals
		{"policy", "validate"}, // missing flag
	} {
		if code, _, _ := execute(args...); code != 5 {
			t.Fatalf("%v must exit 5, got %d", args, code)
		}
	}
}

func TestPolicyCmds(t *testing.T) {
	if code, out, _ := execute("policy", "validate", "--policy-file", "../testdata/policy-a.json"); code != 0 || !strings.Contains(out, "valid") {
		t.Fatalf("valid policy must exit 0: %d %s", code, out)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"policyId":"x","minBuilders":0}`), 0o600)
	if code, _, _ := execute("policy", "validate", "--policy-file", bad); code != 5 {
		t.Fatalf("invalid policy must exit 5, got %d", code)
	}
	if code, _, _ := execute("policy", "validate", "--policy-file", filepath.Join(dir, "nope.json")); code != 5 {
		t.Fatalf("missing policy must exit 5, got %d", code)
	}
	if code, _, _ := execute("policy", "test", "--policy-file", "../testdata/policy-a.json", "--evidence-file", "../testdata/evidence-agree.json"); code != 0 {
		t.Fatalf("agree must exit 0, got %d", code)
	}
	if code, out, _ := execute("policy", "test", "--policy-file", "../testdata/policy-a.json", "--evidence-file", "../testdata/evidence-conflict.json", "--json"); code != 3 || !strings.Contains(out, "VERIFIED_WITH_CONFLICT") {
		t.Fatalf("conflict must exit 3: %d %s", code, out)
	}
	if code, _, _ := execute("policy", "test", "--policy-file", "../testdata/policy-a.json"); code != 5 {
		t.Fatalf("missing evidence flag must exit 5, got %d", code)
	}
}

func TestBuildersCmds(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "builders.json")
	if code, _, _ := execute("builders", "register", "--registry", reg, "--id", "builder-a", "--group", "cloud-a"); code != 0 {
		t.Fatalf("register must exit 0, got %d", code)
	}
	if code, _, _ := execute("builders", "register", "--registry", reg, "--id", "builder-a", "--group", "cloud-b"); code != 5 {
		t.Fatalf("duplicate must exit 5, got %d", code)
	}
	if code, _, _ := execute("builders", "register", "--registry", reg, "--id", "builder-b"); code != 5 {
		t.Fatalf("missing group must exit 5, got %d", code)
	}
	if code, out, _ := execute("builders", "list", "--registry", reg, "--json"); code != 0 || !strings.Contains(out, "cloud-a") {
		t.Fatalf("list must show group: %d %s", code, out)
	}
}

func TestInitInspectEvidenceAudit(t *testing.T) {
	dir := t.TempDir()
	if code, _, _ := execute("init", "--dir", dir); code != 0 {
		t.Fatalf("init must exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "quorum.yaml")); err != nil {
		t.Fatal("quorum.yaml must exist")
	}
	if code, _, _ := execute("init", "--dir", dir); code != 5 {
		t.Fatalf("second init must exit 5, got %d", code)
	}
	if code, _, _ := execute("init", "--dir", dir, "--force"); code != 0 {
		t.Fatalf("forced init must exit 0, got %d", code)
	}

	bundle := filepath.Join(dir, "bundle.json")
	verifyArgs := []string{"evidence", "export", "--repo", "https://github.com/example/tiny", "--commit", "abc123", "--fixture-tiny", "--output", bundle}
	if code, _, _ := execute(verifyArgs...); code != 0 {
		t.Fatalf("evidence export must exit 0, got %d", code)
	}
	data, _ := os.ReadFile(bundle)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil || parsed["result"] == nil {
		t.Fatal("bundle must contain result")
	}
	// inspect reads the result back (write result slice for inspect).
	resOnly, _ := json.Marshal(parsed["result"])
	resPath := filepath.Join(dir, "result.json")
	os.WriteFile(resPath, resOnly, 0o600)
	if code, out, _ := execute("inspect", resPath); code != 0 || !strings.Contains(out, "VERIFIED") {
		t.Fatalf("inspect must exit 0: %d %s", code, out)
	}
	if code, _, _ := execute("inspect", filepath.Join(dir, "missing.json")); code != 5 {
		t.Fatalf("inspect missing must exit 5, got %d", code)
	}

	if code, out, _ := execute("audit", "verify", "--demo"); code != 0 || !strings.Contains(out, "valid") {
		t.Fatalf("demo audit must verify: %d %s", code, out)
	}
	if code, out, _ := execute("audit", "verify", "--demo", "--tamper"); code != 1 || !strings.Contains(out, "INVALID") {
		t.Fatalf("tampered audit must exit 1: %d %s", code, out)
	}
	if code, _, _ := execute("audit", "verify"); code != 5 {
		t.Fatalf("audit without source must exit 5, got %d", code)
	}
}

func TestBlockchainCmdValidation(t *testing.T) {
	// Missing flags -> 5.
	if code, _, _ := execute("blockchain", "anchor", "--contract", "0x5FbDB2315678afecb367f032d93F642f64180aa3"); code != 5 {
		t.Fatalf("partial anchor flags must exit 5, got %d", code)
	}
	// Unreachable RPC, not required -> 0 + SKIPPED (never fake success).
	args := []string{"blockchain", "anchor", "--rpc", "http://127.0.0.1:9",
		"--contract", "0x5FbDB2315678afecb367f032d93F642f64180aa3",
		"--verification-id", "0x" + strings.Repeat("1", 64),
		"--evidence-hash", "0x" + strings.Repeat("2", 64),
		"--artifact-digest", "0x" + strings.Repeat("3", 64),
		"--source-commit", "0x" + strings.Repeat("4", 64),
		"--policy-hash", "0x" + strings.Repeat("5", 64),
		"--decision", "VERIFIED"}
	if code, out, _ := execute(args...); code != 0 || !strings.Contains(out, "SKIPPED") {
		t.Fatalf("unreachable non-required anchor must skip with 0: %d %s", code, out)
	}
	// Same with --required -> operational failure.
	if code, _, _ := execute(append(args, "--required")...); code != 4 {
		t.Fatalf("required unreachable anchor must exit 4, got %d", code)
	}
	// Bad decision -> 5.
	bad := append(append([]string{}, args...), "--decision", "BOGUS")
	bad[1] = "http://127.0.0.1:9"
	if code, _, _ := execute(bad...); code != 5 && code != 4 {
		t.Fatalf("bogus decision must fail, got %d", code)
	}
}

func TestVersionAndDoctor(t *testing.T) {
	if code, out, _ := execute("version"); code != 0 || !strings.Contains(out, "quorum") {
		t.Fatalf("version must exit 0: %d %s", code, out)
	}
	code, out, _ := execute("doctor")
	if code != 0 && code != 4 {
		t.Fatalf("doctor must exit 0 or 4, got %d", code)
	}
	if !strings.Contains(out, "PASS") && !strings.Contains(out, "MISS") {
		t.Fatalf("doctor must report checks: %s", out)
	}
}
