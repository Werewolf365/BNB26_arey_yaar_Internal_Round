package runner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quorum/quorum/internal/runner"
)

// TestAnchorLive is env-gated (QUORUM_LIVE_ANVIL=1): it deploys the real
// compiled QuorumAnchor artifact to the compose Anvil, anchors evidence,
// and re-reads chain state. Never runs in default `go test`.
func TestAnchorLive(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_ANVIL") != "1" {
		t.Skip("set QUORUM_LIVE_ANVIL=1 with compose anvil running")
	}
	rpc := os.Getenv("QUORUM_RPC_URL")
	if rpc == "" {
		rpc = "http://127.0.0.1:8545"
	}
	from := "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	artifactPath := filepath.Join("..", "..", "contracts", "out", "QuorumAnchor.sol", "QuorumAnchor.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Skipf("forge artifact absent (run forge build first): %v", err)
	}
	var artifact struct {
		Bytecode struct {
			Object string `json:"object"`
		} `json:"bytecode"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil || artifact.Bytecode.Object == "" {
		t.Fatalf("bad forge artifact: %v", err)
	}
	contract, err := runner.DeployContract(rpc, from, artifact.Bytecode.Object, 10*time.Second)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	t.Logf("deployed to %s", contract)
	vid := "0x" + strings.Repeat("a", 64)
	res, err := runner.RunAnchor(runner.AnchorOptions{
		RPC: rpc, Contract: contract, VerificationID: vid,
		EvidenceHash: "0x" + strings.Repeat("b", 64),
		ArtifactDigest: "0x" + strings.Repeat("c", 64),
		SourceCommit: "0x" + strings.Repeat("d", 64),
		PolicyHash: "0x" + strings.Repeat("e", 64),
		Decision:   "VERIFIED", From: from, Require: true,
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("anchor failed: %v", err)
	}
	if !res.Anchored || !res.EventFound || res.ChainID == "" {
		t.Fatalf("bad anchor result: %+v", res)
	}
	// Duplicate anchor of the same verification id must revert (no silent overwrite).
	if _, err := runner.RunAnchor(runner.AnchorOptions{
		RPC: rpc, Contract: contract, VerificationID: vid,
		EvidenceHash: "0x" + strings.Repeat("b", 64),
		ArtifactDigest: "0x" + strings.Repeat("c", 64),
		SourceCommit: "0x" + strings.Repeat("d", 64),
		PolicyHash: "0x" + strings.Repeat("e", 64),
		Decision:   "VERIFIED", From: from, Require: true,
		Timeout:    10 * time.Second,
	}); err == nil {
		t.Fatal("duplicate anchor must revert")
	}
}

// TestReadAnchorLive re-reads a fresh anchor without submitting anything.
func TestReadAnchorLive(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_ANVIL") != "1" {
		t.Skip("set QUORUM_LIVE_ANVIL=1 with compose anvil running")
	}
	rpc := os.Getenv("QUORUM_RPC_URL")
	if rpc == "" {
		rpc = "http://127.0.0.1:8545"
	}
	from := "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	artifactPath := filepath.Join("..", "..", "contracts", "out", "QuorumAnchor.sol", "QuorumAnchor.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Skipf("forge artifact absent (run forge build first): %v", err)
	}
	var artifact struct {
		Bytecode struct {
			Object string `json:"object"`
		} `json:"bytecode"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil || artifact.Bytecode.Object == "" {
		t.Fatalf("bad forge artifact: %v", err)
	}
	contract, err := runner.DeployContract(rpc, from, artifact.Bytecode.Object, 10*time.Second)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	vid := "0x" + strings.Repeat("c", 64)
	res, err := runner.RunAnchor(runner.AnchorOptions{
		RPC: rpc, Contract: contract, VerificationID: vid,
		EvidenceHash: "0x" + strings.Repeat("b", 64),
		ArtifactDigest: "0x" + strings.Repeat("c", 64),
		SourceCommit: "0x" + strings.Repeat("d", 64),
		PolicyHash: "0x" + strings.Repeat("e", 64),
		Decision:   "VERIFIED", From: from, Require: true,
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("anchor failed: %v", err)
	}
	read, err := runner.ReadAnchor(rpc, contract, vid, 10*time.Second)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !read.Anchored || !read.EventFound || read.TxHash != res.TxHash || read.BlockNumber != res.BlockNumber {
		t.Fatalf("read must match submitted anchor: %+v vs %+v", read, res)
	}
	// Unknown id reads back unanchored (exit-1 path for the CLI), no error.
	empty, err := runner.ReadAnchor(rpc, contract, "0x"+strings.Repeat("0", 63)+"1", 10*time.Second)
	if err != nil || empty.Anchored {
		t.Fatalf("unknown id must read unanchored: %+v %v", empty, err)
	}
}
