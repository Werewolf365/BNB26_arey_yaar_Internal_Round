# Threat model

## Attacker capabilities (prompt section 24 — each mapped to a control or test)

| Capability | Control / detection |
|---|---|
| Compromised maintainer / release pipeline | Independent rebuilds + quorum; single builder can never verify alone |
| Malicious artifact swapped post-build | `ARTIFACT_HASH_MISMATCH` tripwire (expected vs rebuilt digest) |
| Malicious builder (valid sig, wrong bytes) | Evaluated as disagreement, not trusted by signature |
| Malicious evidence provider | Allowlisted sources; unknown sources excluded with reason |
| Stolen key / forged attestation | Signer-identity + subject/source binding checks; Sigstore phase adds transparency |
| Replayed attestation | Source-commit + subject-digest binding; replay tests per artifact/commit/release |
| Tampered DB record / audit log | Hash chain (`H(prev\|\|canonical)`); `audit verify` pinpoints the record; chain re-verified on read |
| Manipulated blockchain submission | Backend re-reads chain + receipt + event; never trusts client metadata |
| Malicious repo URL (SSRF) | https + host allowlist; no creds/loopback/link-local |
| Malicious archive / traversal / symlink / bomb | Path guards now; extraction sandboxing lands with builder workers |
| Malicious build script / dep confusion | Builders run isolated (Docker now); never on the API host |
| Network MITM | TLS expected in deployment; local dev is loopback |
| Malicious insider / API client | Admin writes authenticated (roadmap); audit logs every mutation |

## Security boundaries

CLI (untrusted input) → API (validation, typed errors) → workers/builders
(isolated execution) → external evidence (untrusted, allowlisted) → Postgres
(constraints, FK) → object store (content-addressed) → chain (hash anchors only)
→ browser (renders decisions + conflicts, never "safe" labels).

## Trust assumptions (explicit)

- Builder images/config are what we think (pin digests in deployment).
- Docker-on-one-host builders are NOT fully independent (see limitations).
- Quorum proves *build integrity evidence*, never source benignity.
- Blockchain adds tamper-evidence, not decentralization.
- Local test keys prove format/verification only; real trust needs Sigstore/Cosign.
