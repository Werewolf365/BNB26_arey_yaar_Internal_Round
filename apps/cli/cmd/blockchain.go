package cmd

import (
	"fmt"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newBlockchainCmd() *cobra.Command {
	b := &cobra.Command{Use: "blockchain", Short: "Anchor verification evidence on an EVM chain"}
	anchor := &cobra.Command{
		Use:   "anchor --contract ADDR --verification-id H --evidence-hash H --artifact-digest H --source-commit H --policy-hash H --decision D",
		Short: "Submit an evidence anchor and verify it on-chain",
		Long: `Submit recordVerification to the anchor contract, wait for the receipt,
and independently verify the VerificationAnchored event. With --required=false
(the default) an unreachable chain is reported as SKIPPED, never as anchored.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var o runner.AnchorOptions
			o.RPC, _ = cmd.Flags().GetString("rpc")
			o.Contract, _ = cmd.Flags().GetString("contract")
			o.VerificationID, _ = cmd.Flags().GetString("verification-id")
			o.EvidenceHash, _ = cmd.Flags().GetString("evidence-hash")
			o.ArtifactDigest, _ = cmd.Flags().GetString("artifact-digest")
			o.SourceCommit, _ = cmd.Flags().GetString("source-commit")
			o.PolicyHash, _ = cmd.Flags().GetString("policy-hash")
			o.Decision, _ = cmd.Flags().GetString("decision")
			o.From, _ = cmd.Flags().GetString("from")
			o.Require, _ = cmd.Flags().GetBool("required")
			if o.Contract == "" || o.VerificationID == "" || o.EvidenceHash == "" ||
				o.ArtifactDigest == "" || o.SourceCommit == "" || o.PolicyHash == "" || o.Decision == "" {
			emitErr(cmd, "all of --contract/--verification-id/--evidence-hash/--artifact-digest/--source-commit/--policy-hash/--decision are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			res, err := runner.RunAnchor(o)
			if err != nil {
			emitErr(cmd, err.Error())
				if isInputErr(err) {
					return &exitErr{code: exitcodes.InvalidInput}
				}
				return &exitErr{code: exitcodes.Operational}
			}
			human := fmt.Sprintf("anchored: tx=%s block=%s chain=%s event=%v\n", res.TxHash, res.BlockNumber, res.ChainID, res.EventFound)
			if res.Skipped {
				human = fmt.Sprintf("anchor SKIPPED: %s\n", res.Reason)
			}
			if err := emit(cmd, human, res); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			return nil
		},
	}
	anchor.Flags().String("rpc", "http://127.0.0.1:8545", "EVM RPC endpoint")
	anchor.Flags().String("contract", "", "anchor contract address")
	anchor.Flags().String("verification-id", "", "bytes32 verification id (0x..)")
	anchor.Flags().String("evidence-hash", "", "bytes32 evidence hash (0x..)")
	anchor.Flags().String("artifact-digest", "", "bytes32 artifact digest (0x..)")
	anchor.Flags().String("source-commit", "", "bytes32 source commit (0x..)")
	anchor.Flags().String("policy-hash", "", "bytes32 policy hash (0x..)")
	anchor.Flags().String("decision", "VERIFIED", "VERIFIED|VERIFIED_WITH_CONFLICT|INSUFFICIENT_EVIDENCE|REJECTED|INVESTIGATE|ERROR")
	anchor.Flags().String("from", "", "sender (default: Anvil account 0)")
	anchor.Flags().Bool("required", false, "BLOCKCHAIN_REQUIRED=true: fail when chain is unreachable")
	verify := &cobra.Command{
		Use:   "verify --contract ADDR --verification-id H",
		Short: "Independently re-read an anchor (no transaction submitted)",
		Long: `Read-only: eth_call anchored(id) for contract state plus eth_getLogs
for the VerificationAnchored event (tx hash and block). Exit 0 when anchored,
exit 1 when the id is not anchored, 4 on operational failure.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rpc, _ := cmd.Flags().GetString("rpc")
			contract, _ := cmd.Flags().GetString("contract")
			vid, _ := cmd.Flags().GetString("verification-id")
			if contract == "" || vid == "" {
				emitErr(cmd, "--contract and --verification-id are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			read, err := runner.ReadAnchor(rpc, contract, vid, 0)
			if err != nil {
				emitErr(cmd, err.Error())
				if isInputErr(err) {
					return &exitErr{code: exitcodes.InvalidInput}
				}
				return &exitErr{code: exitcodes.Operational}
			}
			human := fmt.Sprintf("anchored: %v chain=%s tx=%s block=%s event=%v\n",
				read.Anchored, read.ChainID, read.TxHash, read.BlockNumber, read.EventFound)
			if err := emit(cmd, human, read); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if !read.Anchored {
				return &exitErr{code: exitcodes.Rejected}
			}
			return nil
		},
	}
	verify.Flags().String("rpc", "http://127.0.0.1:8545", "EVM RPC endpoint")
	verify.Flags().String("contract", "", "anchor contract address")
	verify.Flags().String("verification-id", "", "bytes32 verification id (0x..)")
	b.AddCommand(anchor, verify)
	return b
}

func isInputErr(err error) bool {
	s := err.Error()
	return len(s) > 13 && s[:13] == "INVALID_INPUT"
}
