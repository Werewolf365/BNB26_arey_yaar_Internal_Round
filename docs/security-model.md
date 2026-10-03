# Security model (controls — prompt section 23)

Enforced now: strict input validation (repo allowlist, path metachar guards,
1 MiB API bodies, 50 MiB artifact cap), typed errors without internals,
0600 registry/bundle files, secret-free logs (request/verification/release/
builder ids only), isolated builder containers, hash-only chain anchors,
hash-chained audit, idempotent mutations, rate-limit-ready handlers.

Scheduled with later phases: API authN/Z for admin writes, rate limiting +
CORS/CSRF/secure headers, TLS deployment, archive-extraction sandbox
(traversal/symlink/bomb), builder resource quotas + timeouts, Sigstore
transparency checks, SBOM/provenance/signing of Quorum's own releases,
dependency + container scanning in CI. Never: custom crypto, client-trusted
verification results, silent anchor claims, unverified "safe" labels.
