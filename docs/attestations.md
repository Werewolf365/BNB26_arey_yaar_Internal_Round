# Attestations

- Envelope: **DSSE** (`application/vnd.in-toto+json`, ECDSA P-256 + SHA-256).
- Statement: **in-toto Statement v1** (`https://in-toto.io/Statement/v1`).
- Predicate: **SLSA provenance v1** (`https://slsa.dev/provenance/v1`).
  SLSA **v1.2 is current; v1.0 pages are retired** — do not hard-code v1.0 schemas;
  re-check https://slsa.dev at implementation time (done Oct 2026).
- Slice 1 uses **local test keys** with real signing/verification (no custom crypto,
  no custom envelope). Production integrates **Sigstore/Cosign** (blob sign/verify,
  in-toto attestation support) with signer-identity + transparency checks.
- Verification binds: source commit + artifact digest + builder id + build procedure +
  timestamp. Preserve raw envelope; never mutate signed bytes. Replay onto a different
  artifact/commit/release must fail with explicit reason.
