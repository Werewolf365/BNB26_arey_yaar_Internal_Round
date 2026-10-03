package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/quorum/quorum/services/canonical"
	"github.com/quorum/quorum/services/policy"
)

// LoadPolicyFile reads, validates, and returns a policy plus its content hash.
func LoadPolicyFile(path string) (policy.Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return policy.Policy{}, fmt.Errorf("INVALID_INPUT: cannot read policy file: %v", err)
	}
	var p policy.Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return policy.Policy{}, fmt.Errorf("INVALID_INPUT: malformed policy JSON: %v", err)
	}
	if err := p.Validate(); err != nil {
		return policy.Policy{}, err
	}
	return p, nil
}

// PolicyHash returns sha256:<hex> of the canonical policy encoding.
func PolicyHash(p policy.Policy) (string, error) {
	canon, err := canonical.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// PolicyTestInput mirrors the JSON file for `policy test`.
type PolicyTestInput struct {
	ExpectedSource string            `json:"expectedSource"`
	Evidence       []policy.Evidence `json:"evidence"`
}

// RunPolicyTest evaluates a policy file against an evidence file.
func RunPolicyTest(policyPath, evidencePath string) (policy.Result, policy.Policy, error) {
	pol, err := LoadPolicyFile(policyPath)
	if err != nil {
		return policy.Result{}, pol, err
	}
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		return policy.Result{}, pol, fmt.Errorf("INVALID_INPUT: cannot read evidence file: %v", err)
	}
	var in PolicyTestInput
	if err := json.Unmarshal(data, &in); err != nil {
		return policy.Result{}, pol, fmt.Errorf("INVALID_INPUT: malformed evidence JSON: %v", err)
	}
	if in.ExpectedSource == "" {
		return policy.Result{}, pol, fmt.Errorf("INVALID_INPUT: evidence file needs expectedSource")
	}
	return policy.Evaluate(pol, in.ExpectedSource, in.Evidence), pol, nil
}
