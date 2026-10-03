# Architecture (Slice 1 + full target)

Slice 1 (built now): tiny fixture -> Builder A (Dockerfile ready, local bytes for determinism)
-> SHA-256 -> DSSE envelope + in-toto Statement v1 + SLSA provenance v1 (v1.2 current)
-> quorum/policy engine (Go source of truth, Node mirror for toolchain-less runs)
-> audit hash chain -> CLI-equivalent demo script. Blockchain optional
(`BLOCKCHAIN_REQUIRED=false`): attempt Anvil anchor when daemon present, else record
`anchor: SKIPPED` explicitly — never claim anchored when not.

Full target: CLI (Go+Cobra) + Web (Next.js) -> API (Go, OpenAPI, Postgres, S3-compat,
DB-backed queue) -> verification engine -> OSSRebuildProvider (interface) ->
Sigstore/Cosign + in-toto/SLSA verify -> orchestrator -> builders A/B/C (distinct
IDs + independence_group; Docker now, separate infra later) -> quorum -> evidence bundle
-> object storage (content-addressed) + Solidity anchor (hash-only + event) -> audit service.

Trust rule: signature valid != safe; rebuild match != benign source; 2-agree != absolute.
UI/API expose predicates, conflicts, and bounded conclusions.
