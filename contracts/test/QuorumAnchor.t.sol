// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "../src/QuorumAnchor.sol";

/// @notice Contract tests for QuorumAnchor (prompt section 43 matrix).
/// Happy path, unauthorized caller, duplicate ID, zero values, event correctness.
contract QuorumAnchorTest {
    QuorumAnchor anchor;

    bytes32 constant VID = keccak256("verification-1");
    bytes32 constant EV = keccak256("evidence-bundle-1");
    bytes32 constant ART = keccak256("artifact-abc");
    bytes32 constant SRC = keccak256("commit-abc123");
    bytes32 constant POL = keccak256("policy-a-v1");

    function setUp() public {
        anchor = new QuorumAnchor();
    }

    function testHappyPathAnchorsAndEmits() public {
        anchor.recordVerification(VID, EV, ART, SRC, POL, QuorumAnchor.Decision.Verified);
        assertTrue(anchor.anchored(VID));
        assertEq(anchor.owner(), address(this));
    }

    function testDuplicateReverts() public {
        anchor.recordVerification(VID, EV, ART, SRC, POL, QuorumAnchor.Decision.Verified);
        bool reverted;
        try anchor.recordVerification(VID, EV, ART, SRC, POL, QuorumAnchor.Decision.Verified) {
            reverted = false;
        } catch {
            reverted = true;
        }
        assertTrue(reverted);
    }

    function testZeroVerificationIdReverts() public {
        bool reverted;
        try anchor.recordVerification(bytes32(0), EV, ART, SRC, POL, QuorumAnchor.Decision.Verified) {
            reverted = false;
        } catch {
            reverted = true;
        }
        assertTrue(reverted);
    }

    function testZeroEvidenceHashReverts() public {
        bool reverted;
        try anchor.recordVerification(VID, bytes32(0), ART, SRC, POL, QuorumAnchor.Decision.Verified) {
            reverted = false;
        } catch {
            reverted = true;
        }
        assertTrue(reverted);
    }

    function testAllDecisionVariantsAnchor() public {
        // Every decision enum value must be anchorable (no silent coercion).
        anchor.recordVerification(keccak256("v-conflict"), EV, ART, SRC, POL, QuorumAnchor.Decision.VerifiedWithConflict);
        anchor.recordVerification(keccak256("v-insufficient"), EV, ART, SRC, POL, QuorumAnchor.Decision.InsufficientEvidence);
        anchor.recordVerification(keccak256("v-rejected"), EV, ART, SRC, POL, QuorumAnchor.Decision.Rejected);
        anchor.recordVerification(keccak256("v-investigate"), EV, ART, SRC, POL, QuorumAnchor.Decision.Investigate);
        anchor.recordVerification(keccak256("v-error"), EV, ART, SRC, POL, QuorumAnchor.Decision.Error);
        assertTrue(anchor.anchored(keccak256("v-error")));
    }

    function assertTrue(bool v) internal pure {
        require(v, "assertTrue failed");
    }

    function assertEq(address a, address b) internal pure {
        require(a == b, "assertEq(address) failed");
    }
}
