# Attestations

- Envelope: **DSSE** (`application/vnd.in-toto+json`, ECDSA P-256 + SHA-256).
- Statement: **in-toto Statement v1** (`https://in-toto.io/Statement/v1`).
- Predicate: **SLSA provenance v1** (`https://slsa.dev/provenance/v1`).
  SLSA **v1.2 is current; v1.0 pages are retired** — do not hard-code v1.0 schemas;
  re-check https://slsa.dev at implementation time (done Oct 2026).
- Slice 1 uses **local test keys** with real signing/verification (no custom crypto,
  no custom envelope). Production integrates **Sigstore/Cosign** (blob sign/verify,
  in-toto attestation support) with signer-identity + transparency checks.
- **Cosign adapter (live): `services/cosign/`** shells the real `cosign` CLI.
  Blob sign/verify pins explicit keys: `SignBlob` (`sign-blob --key
  --output-signature [--output-bundle] --yes`), `VerifyBlob` (`verify-blob
  --key --signature [--bundle]`) with states `VERIFIED / SIGNATURE_INVALID /
  UNAVAILABLE / INVALID_INPUT`. A missing binary or timeout is `UNAVAILABLE`
  with an install hint — never a faked verdict. `attest cosign-sign |
  cosign-verify | rekor-get` expose it (exit 0/1/4/5).
- **Cosign DSSE attest-blob path (live): `services/cosign/attest.go`** —
  `AttestBlob` (`attest-blob --predicate --type [--key] --bundle --yes`) and
  `VerifyBlobAttestation` (`verify-blob-attestation --bundle [--key |
  --certificate-identity + --certificate-oidc-issuer]`, exit 0 + `Verified OK`
  marker ⇒ `VERIFIED`). Flag surface verified against the cosign main-branch
  CLI reference (`doc/cosign_attest-blob.md`,
  `doc/cosign_verify-blob-attestation.md`, fetched Oct 2026): both commands
  are bundle-based upstream — there is no `--output-attestation` or
  `--signature` flag on the real CLI, so Quorum keeps those names as its own
  CLI flags (`attest cosign-attest-blob --output-attestation`,
  `attest cosign-verify-attestation --signature`) and maps them to `--bundle`
  when shelling out. Upstream `verify-blob-attestation` also has no
  `--rekor-url` (trust moves to `--trusted-root` / signing-config), so
  `AttestOpts.RekorURL` is an opt-in passthrough for older CLIs, omitted when
  empty.
- **Keyless/Fulcio flow:** `AttestBlob` with an empty keyRef omits `--key` and
  passes through to the CLI — this needs interactive OIDC (browser or ambient
  token) and will block/fail in headless CI, so it is never used in tests.
  Keyless *verify* (`AttestOpts{CertIdentity, CertIssuer}`, no key) is fully
  non-interactive and supported: `attest cosign-verify-attestation --artifact
  --signature BUNDLE --certificate-identity ID --certificate-oidc-issuer
  ISSUER`. Deterministic tests use the stub CLI (`STUBATTEST:` envelopes) plus
  `httptest`; live paths are env-gated (`QUORUM_LIVE_COSIGN=1` +
  `COSIGN_PASSWORD` for the ephemeral key, `make test-external`) and assert a
  real attest/verify round-trip, never pinned log data.
- **Rekor client (live, read-only, no creds): `RekorClient`** (`DefaultRekor` =
  `https://rekor.sigstore.dev`): `PublicKey` liveness probe, `Entry(uuid)` fetch with
  `NOT_FOUND / UNAVAILABLE / MALFORMED_EVIDENCE` typing. Deterministic tests use a stub
  CLI binary plus `httptest`; live paths are env-gated (`QUORUM_LIVE_COSIGN=1`,
  `make test-external`) and assert reachability/round-trip, never pinned log data.
- Verification binds: source commit + artifact digest + builder id + build procedure +
  timestamp. Preserve raw envelope; never mutate signed bytes. Replay onto a different
  artifact/commit/release must fail with explicit reason.
