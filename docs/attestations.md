# Attestations

- Envelope: **DSSE** (`application/vnd.in-toto+json`, ECDSA P-256 + SHA-256).
- Statement: **in-toto Statement v1** (`https://in-toto.io/Statement/v1`).
- Predicate: **SLSA provenance v1** (`https://slsa.dev/provenance/v1`).
  SLSA **v1.2 is current; v1.0 pages are retired** — do not hard-code v1.0 schemas;
  re-check https://slsa.dev at implementation time (done Oct 2026).
- Slice 1 uses **local test keys** with real signing/verification (no custom crypto,
  no custom envelope). Production integrates **Sigstore/Cosign** (blob sign/verify,
  in-toto attestation support) with signer-identity + transparency checks.
- **Cosign adapter (live): `services/cosign/`** shells the real `cosign` CLI with
  explicit keys only (no keyless/Fulcio): `SignBlob` (`sign-blob --key --output-signature
  [--output-bundle] --yes`), `VerifyBlob` (`verify-blob --key --signature [--bundle]`)
  with states `VERIFIED / SIGNATURE_INVALID / UNAVAILABLE / INVALID_INPUT`. A missing
  binary or timeout is `UNAVAILABLE` with an install hint — never a faked verdict.
  `attest cosign-sign | cosign-verify | rekor-get` expose it (exit 0/1/4/5).
- **Rekor client (live, read-only, no creds): `RekorClient`** (`DefaultRekor` =
  `https://rekor.sigstore.dev`): `PublicKey` liveness probe, `Entry(uuid)` fetch with
  `NOT_FOUND / UNAVAILABLE / MALFORMED_EVIDENCE` typing. Deterministic tests use a stub
  CLI binary plus `httptest`; live paths are env-gated (`QUORUM_LIVE_COSIGN=1`,
  `make test-external`) and assert reachability/round-trip, never pinned log data.
- Verification binds: source commit + artifact digest + builder id + build procedure +
  timestamp. Preserve raw envelope; never mutate signed bytes. Replay onto a different
  artifact/commit/release must fail with explicit reason.
