# Security policy (seed)

- Report issues privately; do not open public proof-of-concept issues.
- Never commit secrets/private keys. Test keys live in-memory only (Slice 1 generates ephemeral P-256 keys per run).
- Builders run isolated (Docker now, separate infra later); never execute untrusted build code on the API server.
- Logs redact keys/credentials/tokens. Errors are typed (`ARTIFACT_HASH_MISMATCH`, `SIGNATURE_INVALID`, ...) and human-readable.
- Supply chain of Quorum itself: pinned Go toolchain (go.mod `go1.27.1`, CI-enforced), lockfiles, SBOM/provenance/signing in release CI (phases to come).
