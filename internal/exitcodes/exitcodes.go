// Package exitcodes defines the documented CLI exit-code contract.
// 0 = verified, 1 = rejected, 2 = insufficient evidence,
// 3 = conflict/investigation, 4 = operational error, 5 = invalid input.
package exitcodes

const (
	Verified      = 0
	Rejected      = 1
	Insufficient  = 2
	Conflict      = 3
	Operational   = 4
	InvalidInput  = 5
)

// ForDecision maps a quorum decision string to its exit code.
// ERROR is an operational failure, never a rejection.
func ForDecision(decision string) int {
	switch decision {
	case "VERIFIED":
		return Verified
	case "REJECTED":
		return Rejected
	case "INSUFFICIENT_EVIDENCE":
		return Insufficient
	case "VERIFIED_WITH_CONFLICT", "INVESTIGATE":
		return Conflict
	case "ERROR":
		return Operational
	default:
		return Operational
	}
}
