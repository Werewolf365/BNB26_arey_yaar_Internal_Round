package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
)

// AnchorOptions for `blockchain anchor`.
type AnchorOptions struct {
	RPC            string
	Contract       string
	VerificationID string // 0x + 64 hex
	EvidenceHash   string
	ArtifactDigest string
	SourceCommit   string
	PolicyHash     string
	Decision       string // VERIFIED | VERIFIED_WITH_CONFLICT | ...
	From           string // sender; default: Anvil account 0
	Require        bool   // BLOCKCHAIN_REQUIRED=true semantics
	Timeout        time.Duration
}

// AnchorResult is the machine-readable anchor outcome.
type AnchorResult struct {
	Anchored    bool   `json:"anchored"`
	Skipped     bool   `json:"skipped,omitempty"`
	TxHash      string `json:"txHash,omitempty"`
	BlockNumber string `json:"blockNumber,omitempty"`
	ChainID     string `json:"chainId,omitempty"`
	Contract    string `json:"contract,omitempty"`
	EventFound  bool   `json:"eventFound,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func keccak256Hex(data []byte) string {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	sum := h.Sum(nil)
	return fmt.Sprintf("%x", sum)
}

// DecisionToUint8 follows the contract enum order.
func DecisionToUint8(d string) (int, error) {
	switch d {
	case "VERIFIED":
		return 0, nil
	case "VERIFIED_WITH_CONFLICT":
		return 1, nil
	case "INSUFFICIENT_EVIDENCE":
		return 2, nil
	case "REJECTED":
		return 3, nil
	case "INVESTIGATE":
		return 4, nil
	case "ERROR":
		return 5, nil
	default:
		return 0, fmt.Errorf("INVALID_INPUT: unknown decision %q", d)
	}
}

func parseBytes32(s, name string) ([]byte, error) {
	t := strings.TrimPrefix(s, "0x")
	if len(t) != 64 {
		return nil, fmt.Errorf("INVALID_INPUT: %s must be 0x + 64 hex chars", name)
	}
	var b [32]byte
	for i := 0; i < 32; i++ {
		var v byte
		_, err := fmt.Sscanf(t[i*2:i*2+2], "%02x", &v)
		if err != nil {
			return nil, fmt.Errorf("INVALID_INPUT: %s has non-hex characters", name)
		}
		b[i] = v
	}
	return b[:], nil
}

func parseAddress(s, name string) (string, error) {
	t := s
	if !strings.HasPrefix(t, "0x") || len(t) != 42 {
		return "", fmt.Errorf("INVALID_INPUT: %s must be a 0x address", name)
	}
	return t, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

func rpcCall(rpcURL, method string, params []any, timeout time.Duration) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(rpcURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("BLOCKCHAIN_UNAVAILABLE: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("BLOCKCHAIN_UNAVAILABLE: %v", err)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("BLOCKCHAIN_UNAVAILABLE: bad RPC response: %v", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("BLOCKCHAIN_REVERTED: %s (code %d)", envelope.Error.Message, envelope.Error.Code)
	}
	return envelope.Result, nil
}

// RunAnchor submits the anchor and independently verifies it on-chain.
func RunAnchor(o AnchorOptions) (*AnchorResult, error) {
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.RPC == "" {
		o.RPC = "http://127.0.0.1:8545"
	}
	contract, err := parseAddress(o.Contract, "contract")
	if err != nil {
		return nil, err
	}
	from := o.From
	if from == "" {
		from = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" // Anvil account 0
	}
	if _, err := parseAddress(from, "from"); err != nil {
		return nil, err
	}
	vid, err := parseBytes32(o.VerificationID, "verification-id")
	if err != nil {
		return nil, err
	}
	ev, err := parseBytes32(o.EvidenceHash, "evidence-hash")
	if err != nil {
		return nil, err
	}
	art, err := parseBytes32(o.ArtifactDigest, "artifact-digest")
	if err != nil {
		return nil, err
	}
	src, err := parseBytes32(o.SourceCommit, "source-commit")
	if err != nil {
		return nil, err
	}
	pol, err := parseBytes32(o.PolicyHash, "policy-hash")
	if err != nil {
		return nil, err
	}
	dec, err := DecisionToUint8(o.Decision)
	if err != nil {
		return nil, err
	}

	// ABI-encode recordVerification(bytes32,bytes32,bytes32,bytes32,bytes32,uint8).
	sel := keccak256Hex([]byte("recordVerification(bytes32,bytes32,bytes32,bytes32,bytes32,uint8)"))[:8]
	var data strings.Builder
	data.WriteString("0x" + sel)
	for _, w := range [][]byte{vid, ev, art, src, pol} {
		data.WriteString(fmt.Sprintf("%x", w))
	}
	data.WriteString(fmt.Sprintf("%064x", dec))

	// eth_sendTransaction works against Anvil's unlocked accounts; remote
	// networks without unlocking fail here with an explicit error (never faked).
	chainRaw, err := rpcCall(o.RPC, "eth_chainId", []any{}, o.Timeout)
	if err != nil {
		if !o.Require {
			return &AnchorResult{Anchored: false, Skipped: true, Reason: "chain unreachable; BLOCKCHAIN_REQUIRED=false so continuing unlabeled-anchored: " + err.Error()}, nil
		}
		return nil, err
	}
	var chainID string
	_ = json.Unmarshal(chainRaw, &chainID)

	txRaw, err := rpcCall(o.RPC, "eth_sendTransaction", []any{map[string]string{
		"from": from, "to": contract, "data": data.String(),
	}}, o.Timeout)
	if err != nil {
		return nil, err
	}
	var txHash string
	_ = json.Unmarshal(txRaw, &txHash)

	// Wait for receipt (Anvil mines instantly; poll briefly for generality).
	receipt, err := waitReceipt(o.RPC, txHash, o.Timeout)
	if err != nil {
		return nil, err
	}
	if receipt.Status != "0x1" {
		return nil, fmt.Errorf("BLOCKCHAIN_REVERTED: tx %s failed (status %s)", txHash, receipt.Status)
	}
	// Independently verify the emitted event (never trust the send alone).
	eventSig := "0x" + keccak256Hex([]byte("VerificationAnchored(bytes32,bytes32,bytes32,bytes32,bytes32,uint8,uint256)"))
	found := false
	for _, l := range receipt.Logs {
		if !strings.EqualFold(l.Address, contract) || len(l.Topics) == 0 {
			continue
		}
		if strings.EqualFold(l.Topics[0], eventSig) {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("BLOCKCHAIN_REVERTED: no VerificationAnchored event in %s", txHash)
	}
	return &AnchorResult{
		Anchored: true, TxHash: txHash, BlockNumber: receipt.BlockNumber,
		ChainID: chainID, Contract: contract, EventFound: true,
	}, nil
}

// TxReceipt is the subset of an Ethereum receipt Quorum verifies.
type TxReceipt struct {
	Status          string `json:"status"`
	BlockNumber     string `json:"blockNumber"`
	ContractAddress string `json:"contractAddress"`
	Logs            []struct {
		Address string   `json:"address"`
		Topics  []string `json:"topics"`
	} `json:"logs"`
}

func waitReceipt(rpcURL, txHash string, timeout time.Duration) (*TxReceipt, error) {
	var receipt TxReceipt
	deadline := time.Now().Add(30 * time.Second)
	for {
		r, err := rpcCall(rpcURL, "eth_getTransactionReceipt", []any{txHash}, timeout)
		if err != nil {
			return nil, err
		}
		if string(r) != "null" {
			if err := json.Unmarshal(r, &receipt); err != nil {
				return nil, fmt.Errorf("BLOCKCHAIN_UNAVAILABLE: bad receipt: %v", err)
			}
			return &receipt, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("BLOCKCHAIN_UNAVAILABLE: receipt timeout for %s", txHash)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// DeployContract deploys raw init bytecode (e.g. from the forge artifact) and
// returns the contract address. Used by env-gated integration tests and tooling.
func DeployContract(rpcURL, from, bytecodeHex string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if _, err := parseAddress(from, "from"); err != nil {
		return "", err
	}
	code := strings.TrimPrefix(bytecodeHex, "0x")
	if len(code) == 0 || len(code)%2 != 0 {
		return "", fmt.Errorf("INVALID_INPUT: bad init bytecode")
	}
	txRaw, err := rpcCall(rpcURL, "eth_sendTransaction", []any{map[string]string{
		"from": from, "data": "0x" + code,
	}}, timeout)
	if err != nil {
		return "", err
	}
	var txHash string
	_ = json.Unmarshal(txRaw, &txHash)
	receipt, err := waitReceipt(rpcURL, txHash, timeout)
	if err != nil {
		return "", err
	}
	if receipt.Status != "0x1" || receipt.ContractAddress == "" {
		return "", fmt.Errorf("BLOCKCHAIN_REVERTED: deploy tx %s failed", txHash)
	}
	return receipt.ContractAddress, nil
}