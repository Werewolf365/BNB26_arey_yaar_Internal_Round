# Provenance

Standard formats only — envelope DSSE (`application/vnd.in-toto+json`),
statement `https://in-toto.io/Statement/v1`, predicate
`https://slsa.dev/provenance/v1` (**SLSA v1.2 current**; v1.0 retired — re-check
slsa.dev before release).

Each provenance binds: source commit + artifact digest + builder id + build
procedure + environment + timestamp. Verification checks signature, signer,
subject digest, source commit, builder id, predicate schema, and policy
constraints — then binds the *verified content* to the release (never "valid
signature → accepted"). Raw envelopes are preserved byte-for-byte for forensics.

Slice status: DSSE/in-toto/SLSA-v1 construction + sign/verify proven in
`services/attestation` (Node) with tamper/replay tests; Go verifier and
Sigstore/Cosign integration are scheduled with the worker/backend phases.
