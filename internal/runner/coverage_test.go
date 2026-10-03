package runner_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quorum/quorum/internal/runner"
	"golang.org/x/crypto/sha3"
)

func keccakHex(s string) string {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(s))
	return "0x" + string(bytes2hex(h.Sum(nil)))
}

func bytes2hex(b []byte) []byte {
	const hexd = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexd[c>>4], hexd[c&0xf])
	}
	return out
}

// fakeChain serves canned JSON-RPC for anchor tests. receiptFirstNull makes
// the first receipt poll return null (covers the wait loop) before succeeding.
func fakeChain(t *testing.T, contract, eventSig string, status string, receiptFirstNull bool, rpcErr bool) *httptest.Server {
	t.Helper()
	var polls atomic.Int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		reply := func(result any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
		}
		switch req.Method {
		case "eth_chainId":
			if rpcErr {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -32000, "message": "boom"}})
				return
			}
			reply("0x7a69")
		case "eth_sendTransaction":
			reply("0xtxhash")
		case "eth_getTransactionReceipt":
			if receiptFirstNull && polls.Add(1) == 1 {
				reply(nil)
				return
			}
			logs := []any{
				// Decoys: wrong address, and right address with no topics.
				// Both must be skipped without affecting event detection.
				map[string]any{"address": "0x0000000000000000000000000000000000000000", "topics": []string{eventSig}},
				map[string]any{"address": contract, "topics": []string{}},
			}
			if eventSig != "" {
				logs = append(logs, map[string]any{"address": contract, "topics": []string{eventSig, "0xother"}})
			}
			reply(map[string]any{"status": status, "blockNumber": "0x5", "contractAddress": contract, "logs": logs})
		default:
			reply(nil)
		}
	}))
}

const testContract = "0x5FbDB2315678afecb367f032d93F642f64180aa3"

var hex1 = "0x" + strings.Repeat("1", 64)

func anchorOpts(rpc string) runner.AnchorOptions {
	return runner.AnchorOptions{
		RPC: rpc, Contract: testContract,
		VerificationID: hex1, EvidenceHash: hex1, ArtifactDigest: hex1,
		SourceCommit: hex1, PolicyHash: hex1, Decision: "VERIFIED",
	}
}

func TestDecisionToUint8All(t *testing.T) {
	for d, want := range map[string]int{
		"VERIFIED": 0, "VERIFIED_WITH_CONFLICT": 1, "INSUFFICIENT_EVIDENCE": 2,
		"REJECTED": 3, "INVESTIGATE": 4, "ERROR": 5,
	} {
		if got, err := runner.DecisionToUint8(d); err != nil || got != want {
			t.Fatalf("%s: %d %v", d, got, err)
		}
	}
	if _, err := runner.DecisionToUint8("BOGUS"); err == nil {
		t.Fatal("bogus decision must fail")
	}
}

func TestAnchorInputValidation(t *testing.T) {
	base := anchorOpts("http://127.0.0.1:9")
	for _, mutate := range []func(*runner.AnchorOptions){
		func(o *runner.AnchorOptions) { o.Contract = "nope" },
		func(o *runner.AnchorOptions) { o.From = "nope" },
		func(o *runner.AnchorOptions) { o.VerificationID = "0x123" },
		func(o *runner.AnchorOptions) { o.EvidenceHash = "0x" + strings.Repeat("z", 64) },
		func(o *runner.AnchorOptions) { o.ArtifactDigest = "0x" + strings.Repeat("q", 64) },
		func(o *runner.AnchorOptions) { o.SourceCommit = "0x123" },
		func(o *runner.AnchorOptions) { o.PolicyHash = "0x" + strings.Repeat("g", 64) },
		func(o *runner.AnchorOptions) { o.Decision = "BOGUS" },
	} {
		o := base
		mutate(&o)
		if _, err := runner.RunAnchor(o); err == nil {
			t.Fatalf("bad anchor input must fail: %+v", o)
		}
	}
	// Unreachable RPC, not required -> skipped (never faked).
	res, err := runner.RunAnchor(base)
	if err != nil || !res.Skipped || res.Anchored {
		t.Fatalf("unreachable non-required must skip: %+v %v", res, err)
	}
	// Required -> operational error.
	o := base
	o.Require = true
	if _, err := runner.RunAnchor(o); err == nil {
		t.Fatal("required unreachable must fail")
	}
}

func TestAnchorFullLoop(t *testing.T) {
	eventSig := keccakHex("VerificationAnchored(bytes32,bytes32,bytes32,bytes32,bytes32,uint8,uint256)")
	srv := fakeChain(t, testContract, eventSig, "0x1", true, false)
	defer srv.Close()
	res, err := runner.RunAnchor(anchorOpts(srv.URL))
	if err != nil {
		t.Fatalf("fake chain must anchor: %v", err)
	}
	if !res.Anchored || !res.EventFound || res.TxHash != "0xtxhash" || res.ChainID != "0x7a69" {
		t.Fatalf("bad result: %+v", res)
	}
}

func TestAnchorRejections(t *testing.T) {
	eventSig := keccakHex("VerificationAnchored(bytes32,bytes32,bytes32,bytes32,bytes32,uint8,uint256)")
	// Reverted tx.
	srv := fakeChain(t, testContract, eventSig, "0x0", false, false)
	defer srv.Close()
	if _, err := runner.RunAnchor(anchorOpts(srv.URL)); err == nil {
		t.Fatal("reverted tx must fail")
	}
	// No event in receipt.
	srv2 := fakeChain(t, testContract, "", "0x1", false, false)
	defer srv2.Close()
	if _, err := runner.RunAnchor(anchorOpts(srv2.URL)); err == nil {
		t.Fatal("missing event must fail")
	}
	// RPC-level error on chainId.
	srv3 := fakeChain(t, testContract, eventSig, "0x1", false, true)
	defer srv3.Close()
	o := anchorOpts(srv3.URL)
	o.Require = true
	if _, err := runner.RunAnchor(o); err == nil {
		t.Fatal("rpc error must fail when required")
	}
	// sendTransaction error envelope propagates as BLOCKCHAIN_REVERTED.
	sendErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "eth_sendTransaction" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -32000, "message": "reverted"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0x1"})
	}))
	defer sendErr.Close()
	so := anchorOpts(sendErr.URL)
	so.Require = true
	if _, err := runner.RunAnchor(so); err == nil {
		t.Fatal("send revert must fail")
	}
	// Empty RPC defaults to localhost (unreachable here -> skip, not fake).
	// Guard: if a dev chain actually listens on :8545, skip instead of
	// touching it.
	if conn, derr := net.DialTimeout("tcp", "127.0.0.1:8545", 200*time.Millisecond); derr == nil {
		conn.Close()
		t.Skip("dev chain on :8545; default-RPC branch needs a closed port")
	}
	do := anchorOpts("")
	if res, err := runner.RunAnchor(do); err != nil || !res.Skipped {
		t.Fatalf("default-rpc unreachable must skip: %+v %v", res, err)
	}
}

func TestDeployContractPaths(t *testing.T) {
	if _, err := runner.DeployContract("http://127.0.0.1:9", "nope", "0x1234", 0); err == nil {
		t.Fatal("bad from must fail")
	}
	if _, err := runner.DeployContract("http://127.0.0.1:9", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x123", 0); err == nil {
		t.Fatal("odd bytecode must fail")
	}
	eventSig := keccakHex("VerificationAnchored(bytes32,bytes32,bytes32,bytes32,bytes32,uint8,uint256)")
	srv := fakeChain(t, testContract, eventSig, "0x1", false, false)
	defer srv.Close()
	addr, err := runner.DeployContract(srv.URL, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x6080", 0)
	if err != nil || addr != testContract {
		t.Fatalf("deploy: %q %v", addr, err)
	}
	// Deploy with reverted receipt.
	srv2 := fakeChain(t, testContract, eventSig, "0x0", false, false)
	defer srv2.Close()
	if _, err := runner.DeployContract(srv2.URL, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x6080", 0); err == nil {
		t.Fatal("reverted deploy must fail")
	}
}

func TestLiveOssEvidenceOffline(t *testing.T) {
	// Missing binary surfaces as an operational note, never silent.
	o := baseOpts()
	o.PkgRef = "pypi:quorum-tiny@1.0.0"
	o.OssMode = "live"
	o.OssBin = "oss-rebuild-definitely-missing"
	res, _, _, err := runner.RunVerify(o)
	if err != nil {
		t.Fatalf("live-unavailable must degrade, not error: %v", err)
	}
	if res.OssState != "UNAVAILABLE" {
		t.Fatalf("missing oss binary must be UNAVAILABLE, got %s", res.OssState)
	}
	// Unsupported ecosystem is a visible note with surviving verdict.
	o2 := baseOpts()
	o2.PkgRef = "docker:nginx@1.0"
	o2.OssMode = "live"
	res2, _, _, err := runner.RunVerify(o2)
	_ = res2
	_ = err
	// docker: ref is rejected at input validation (bad package ref).
	if err == nil {
		t.Fatal("docker ref must be invalid input")
	}
}

func TestRunVerifyBranches(t *testing.T) {
	// Oversize trips rejection, not operational failure.
	o := baseOpts()
	o.MaxBytes = 1
	if res, code, _, _ := runner.RunVerify(o); res.Decision != "REJECTED" || code != 1 {
		t.Fatalf("oversize: %s %d", res.Decision, code)
	}
	// Unreadable policy file is invalid input.
	o2 := baseOpts()
	o2.PolicyFile = "no-such-policy.json"
	if _, code, _, err := runner.RunVerify(o2); err == nil || code != 5 {
		t.Fatalf("bad policy file: %d %v", code, err)
	}
	// Min-builders above evidence -> INSUFFICIENT_EVIDENCE.
	o3 := baseOpts()
	o3.MinBuilders = 9
	o3.RequiredAgree = 2
	if res, code, _, _ := runner.RunVerify(o3); res.Decision != "INSUFFICIENT_EVIDENCE" || code != 2 {
		t.Fatalf("insufficient: %s %d", res.Decision, code)
	}
	// Short expected digest exercises the untruncated reason branch.
	o5 := baseOpts()
	o5.ExpectedDigest = "nope"
	if res, code, _, _ := runner.RunVerify(o5); res.Decision != "REJECTED" || code != 1 {
		t.Fatalf("short digest mismatch: %s %d", res.Decision, code)
	}
	// Tamper + conflict + package-ref fixture flows.
	ot := baseOpts()
	ot.InjectTamper = true
	ot.ExpectedDigest = "sha256:" + strings.Repeat("0", 64)
	if res, _, _, _ := runner.RunVerify(ot); res.Decision != "REJECTED" {
		t.Fatalf("tamper: %s", res.Decision)
	}
	oc := baseOpts()
	oc.InjectConflictBuilder = "builder-c"
	if res, code, _, _ := runner.RunVerify(oc); res.Decision != "VERIFIED_WITH_CONFLICT" || code != 3 {
		t.Fatalf("conflict: %s %d", res.Decision, code)
	}
	op := baseOpts()
	op.PkgRef = "pypi:quorum-tiny@1.0.0"
	if res, _, _, _ := runner.RunVerify(op); res.OssState != "SUPPORTED_AND_VERIFIED" {
		t.Fatalf("fixture oss: %s", res.OssState)
	}
	// Package ref without repo/commit takes the fixture defaults.
	op2 := runner.VerifyOptions{PkgRef: "pypi:quorum-tiny@1.0.0", FixtureTiny: true}
	if res, code, _, err := runner.RunVerify(op2); err != nil || code != 0 {
		t.Fatalf("pkgref defaults: %s %d %v", res.Decision, code, err)
	}
	// Overlong commit is invalid input.
	op3 := baseOpts()
	op3.Commit = strings.Repeat("c", 129)
	if _, code, _, err := runner.RunVerify(op3); err == nil || code != 5 {
		t.Fatalf("long commit: %d %v", code, err)
	}
	// Policy scalar overrides flow into evaluation.
	op4 := baseOpts()
	op4.MinBuilders = 3
	op4.RequiredAgree = 3
	op4.RequiredGroups = 3
	op4.ConflictTolerance = 0
	if res, _, _, _ := runner.RunVerify(op4); res.Decision != "VERIFIED" {
		t.Fatalf("3-of-3 override: %s", res.Decision)
	}
	// Verbose renders policy hash + elapsed.
	o4 := baseOpts()
	o4.Verbose = true
	if _, _, human, _ := runner.RunVerify(o4); !strings.Contains(human, "Policy hash") {
		t.Fatalf("verbose must show policy hash:\n%s", human)
	}
}

func TestRegistryAndPolicyFileEdges(t *testing.T) {
	dir := t.TempDir()
	// Malformed registry JSON.
	badReg := filepath.Join(dir, "reg.json")
	os.WriteFile(badReg, []byte("{oops"), 0o600)
	if _, err := runner.LoadRegistry(badReg); err == nil {
		t.Fatal("malformed registry must fail")
	}
	// Unreadable registry (directory as path).
	if _, err := runner.LoadRegistry(dir); err == nil {
		t.Fatal("unreadable registry must fail")
	}
	// Register onto a corrupt registry propagates the error.
	if err := runner.Register(badReg, runner.BuilderRegistration{ID: "a", IndependenceGroup: "g"}); err == nil {
		t.Fatal("register on corrupt registry must fail")
	}
	// Register where the file cannot be written.
	if err := runner.Register(filepath.Join(dir, "no-such-dir", "reg.json"), runner.BuilderRegistration{ID: "a", IndependenceGroup: "g"}); err == nil {
		t.Fatal("unwritable registry must fail")
	}
	// Policy file edges.
	if _, err := runner.LoadPolicyFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing policy file must fail")
	}
	badPol := filepath.Join(dir, "pol.json")
	os.WriteFile(badPol, []byte("{oops"), 0o600)
	if _, err := runner.LoadPolicyFile(badPol); err == nil {
		t.Fatal("malformed policy must fail")
	}
	invalidPol := filepath.Join(dir, "invalid.json")
	os.WriteFile(invalidPol, []byte(`{"policyId":"p","minBuilders":0}`), 0o600)
	if _, err := runner.LoadPolicyFile(invalidPol); err == nil {
		t.Fatal("invalid policy must fail")
	}
	// Policy test input edges (need a valid policy file).
	goodPol := filepath.Join(dir, "good.json")
	os.WriteFile(goodPol, []byte(`{"policyId":"p","version":"v1","minBuilders":1,"requiredAgreement":1,"requiredIndependentGroups":1,"conflictTolerance":0}`), 0o600)
	if _, _, err := runner.RunPolicyTest(filepath.Join(dir, "missing.json"), "x"); err == nil {
		t.Fatal("missing policy must fail policy test")
	}
	if _, _, err := runner.RunPolicyTest(goodPol, filepath.Join(dir, "missing-ev.json")); err == nil {
		t.Fatal("missing evidence must fail policy test")
	}
	badEv := filepath.Join(dir, "badev.json")
	os.WriteFile(badEv, []byte("{oops"), 0o600)
	if _, _, err := runner.RunPolicyTest(goodPol, badEv); err == nil {
		t.Fatal("malformed evidence must fail policy test")
	}
	noSrc := filepath.Join(dir, "nosrc.json")
	os.WriteFile(noSrc, []byte(`{"evidence":[]}`), 0o600)
	if _, _, err := runner.RunPolicyTest(goodPol, noSrc); err == nil {
		t.Fatal("evidence without source must fail policy test")
	}
	// Guard edges.
	if err := runner.CheckSize(-1, 100); err == nil {
		t.Fatal("negative size must fail")
	}
	if _, _, _, ok := runner.ParsePackageRef("pypi:noversion"); ok {
		t.Fatal("versionless ref must not parse")
	}
	if _, _, _, ok := runner.ParsePackageRef("docker:nginx"); ok {
		t.Fatal("unknown ecosystem must not parse")
	}
	if state, _ := runner.OssFixture("npm:something@1.0"); state != "UNSUPPORTED" {
		t.Fatalf("unknown fixture package must be UNSUPPORTED, got %s", state)
	}
	if state, _ := runner.OssFixture("docker:nginx"); state != "NOT_FOUND" {
		t.Fatalf("bad prefix must be NOT_FOUND, got %s", state)
	}
}

func TestRPCErrorPaths(t *testing.T) {
	// Truncated JSON breaks the RPC envelope parse.
	trunc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":`))
	}))
	defer trunc.Close()
	o := anchorOpts(trunc.URL)
	o.Require = true
	if _, err := runner.RunAnchor(o); err == nil {
		t.Fatal("truncated RPC must fail")
	}
	// sendTransaction error envelope -> BLOCKCHAIN_REVERTED (deploy path).
	errEnv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -32000, "message": "reverted"}})
	}))
	defer errEnv.Close()
	if _, err := runner.DeployContract(errEnv.URL, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x6080", 0); err == nil {
		t.Fatal("reverted send must fail deploy")
	}
	// Malformed receipt JSON.
	badReceipt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "eth_getTransactionReceipt" {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "not-an-object"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0xtx"})
	}))
	defer badReceipt.Close()
	if _, err := runner.DeployContract(badReceipt.URL, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x6080", 0); err == nil {
		t.Fatal("malformed receipt must fail deploy")
	}
	// Receipt RPC error propagates out of the wait loop.
	receiptErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "eth_getTransactionReceipt" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -32000, "message": "gone"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "0xtx"})
	}))
	defer receiptErr.Close()
	if _, err := runner.DeployContract(receiptErr.URL, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x6080", 0); err == nil {
		t.Fatal("receipt RPC error must fail deploy")
	}
	// Truncated body breaks the RPC envelope parse.
	truncBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer truncBody.Close()
	to := anchorOpts(truncBody.URL)
	to.Require = true
	if _, err := runner.RunAnchor(to); err == nil {
		t.Fatal("truncated body must fail when required")
	} else if !strings.Contains(err.Error(), "BLOCKCHAIN_UNAVAILABLE") {
		t.Fatalf("want UNAVAILABLE, got %v", err)
	}
}

func TestAuditUnhashable(t *testing.T) {
	if _, err := runner.HashRecord("GENESIS", "ev", map[string]any{"fn": func() {}}); err == nil {
		t.Fatal("func payload must fail hashing")
	}
	chain, err := runner.DemoChain()
	if err != nil {
		t.Fatal(err)
	}
	chain[1].Payload = map[string]any{"fn": func() {}}
	if ok, at, _ := runner.VerifyChain(chain); ok || at != 1 {
		t.Fatalf("unhashable record must break at 1: %v %d", ok, at)
	}
}
