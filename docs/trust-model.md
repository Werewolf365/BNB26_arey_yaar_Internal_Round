# Trust model (precise language — prompt section 1)

Quorum separates ten distinct concepts; the UI, API, and CLI must never
conflate them:

source identity · artifact identity · build provenance · attestation validity ·
signer identity · builder independence · artifact reproducibility · policy
satisfaction · quorum agreement · overall trust evidence.

Forbidden equations (enforced in copy + tests):

- "signature valid" = "software safe" — a valid signature only authenticates *who* made a claim.
- "rebuild matches" = "source is benign" — reproduction says nothing about malice in the source.
- "two builders agree" = "absolute security" — agreement is bounded evidence under a policy.

Allowed statements: `VERIFIED` (evidence + policy satisfied), `VERIFIED_WITH_CONFLICT`
(satisfied with visible minority), `INSUFFICIENT_EVIDENCE`, `REJECTED`,
`INVESTIGATE`, `ERROR` — each with predicates, counted/excluded evidence, and reasons.
