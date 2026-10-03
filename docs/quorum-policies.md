# Quorum policies

Deterministic evaluation (`services/policy`): same evidence + same policy =
same decision. No trust scores — only transparent predicates:

signed attestation required/optional · OSS Rebuild required/optional ·
independent builder count ≥ N · digest agreement ≥ N · source match required ·
signer allowlist · independence-group uniqueness required ·
freshness ≤ duration · conflict tolerance = k.

## Built-in shapes

- **A** — 2 of 3 independent builders agree (default: `policy-a`).
- **B** — OSS Rebuild verifies AND ≥2 independent builders agree.
- **C** — 3 independent builders, zero conflicts.
- **D** — 2 agree AND all signatures valid AND source matches.

## Policy JSON

See `packages/schemas/policy.json`, `apps/cli/testdata/policy-a.json`
(2-of-3) and `policy-strict.json` (3-of-3, tolerance 0). Hash a policy with
`quorum policy validate --policy-file` (canonical sha256, also used for anchors).

Rules: `requiredAgreement ≤ minBuilders`; one vote per `(digest,
independenceGroup)`; duplicates and same-group re-votes are excluded *with
reasons*; minority digests always surface as `conflicts`; tolerance exceeded →
`INVESTIGATE`, never silent acceptance.
