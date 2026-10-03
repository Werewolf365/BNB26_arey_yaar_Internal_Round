// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title QuorumAnchor — tamper-evident public anchor for verification records.
/// @notice Stores compact deterministic evidence hashes only. Never artifacts/secrets.
contract QuorumAnchor {
    enum Decision { Verified, VerifiedWithConflict, InsufficientEvidence, Rejected, Investigate, Error }

    event VerificationAnchored(
        bytes32 indexed verificationId,
        bytes32 evidenceHash,
        bytes32 artifactDigest,
        bytes32 sourceCommit,
        bytes32 policyHash,
        Decision decision,
        uint256 timestamp
    );

    address public owner;
    mapping(bytes32 => bool) public anchored;

    error AlreadyAnchored();
    error ZeroValue();

    constructor() { owner = msg.sender; }

    function recordVerification(
        bytes32 verificationId,
        bytes32 evidenceHash,
        bytes32 artifactDigest,
        bytes32 sourceCommit,
        bytes32 policyHash,
        Decision decision
    ) external {
        if (msg.sender != owner) revert();
        if (verificationId == bytes32(0) || evidenceHash == bytes32(0)) revert ZeroValue();
        if (anchored[verificationId]) revert AlreadyAnchored();
        anchored[verificationId] = true;
        emit VerificationAnchored(verificationId, evidenceHash, artifactDigest, sourceCommit, policyHash, decision, block.timestamp);
    }
}
