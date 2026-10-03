# Blockchain anchoring (Slice 1: live on local Anvil)

- Contract: `contracts/src/QuorumAnchor.sol` — hash-only anchor + `VerificationAnchored` event. No artifacts, no secrets.
- Toolchain: Foundry **via Docker** (`ghcr.io/foundry-rs/foundry:latest`, proven `forge 1.8.1`).
  The image entrypoint is `/bin/sh -c`, so tools MUST run with
  `--entrypoint forge|cast|anvil` (else flags are swallowed and bare help prints).
  Wrapper: `scripts/forge-docker.ps1`. No host Foundry install required.
- Chain: compose `anvil` service, chainId `31337 (0x7a69)`.
- Gotcha (documented, hit live): Foundry 1.8.x uses a **new default mnemonic** —
  the memorized classic key `0xac09…b12d` is WRONG for it. Source deploy keys from
  `docker logs compose-anvil-1` (Private Keys section), never from memory.

## Live Slice-1 anchor (reproducible)

- Deployed: `0x5FbDB2315678afecb367f032d93F642f64180aa3`
  deploy tx `0xb917ced7a8527a69a6685deb3bfc945b8eec96a8b7cbdd03d608115b9f93a94a`
- Anchor tx: `0x8f276b5e2c85e0595e698b790cd4fffa7e4dc5e93bcc9c62c696fdbc7572f721` (block 2, status success)
  - verificationId `0x143c31990659a4e079fd1d1fca0ae64dda03d44b5ea808918990b6992098e161` (keccak "quorum-slice1-verification-1")
  - evidenceHash `0xfb273925b05ec633a92e9ed57bb262709c29eeaf405213c5b74bf4e9d9469129`
  - artifactDigest `0x125e91d8…ec91227` (the real tiny-artifact SHA-256, as bytes32)
  - decision `0` (Verified)
- Verified by re-reading chain: `anchored(VID) == true`, receipt log carries the
  `VerificationAnchored` event with matching topics/data. Backend must do the same —
  never trust client-provided tx metadata (prompt section 49).
- Policy: `BLOCKCHAIN_REQUIRED=false` — anchor attempted when chain is live,
  explicitly labeled SKIPPED otherwise. Never claim anchored when not.
