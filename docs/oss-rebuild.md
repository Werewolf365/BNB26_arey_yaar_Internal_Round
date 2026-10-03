# OSS Rebuild integration boundary

Google OSS Rebuild is the initial external evidence layer (one source, not an
automatic quorum member; duplicates never double-counted).

- Supported ecosystems (per current docs): **npm, PyPI, Crates.io — only a subset
  of popular packages actually rebuilt**. CLI retrieves rebuild summaries, payloads,
  bundles, Dockerfiles. Always re-check the official repo before coding; never
  fabricate flags/APIs.
- Auth gotcha: signature validation uses a **Google Cloud KMS key; ADC credentials
  required unless verification explicitly disabled**.
- Hence the hard split:
  - `make test` / `make test-all`: **fixture mode only** (`services/oss-rebuild/adapter.mjs`
    `fixtureLookup`), no network, no creds, deterministic.
  - `make test-external` (`OSS_REBUILD_MODE=live`): shells the real CLI, needs ADC
    + network, never gates CI.
  - Demo: real supported package (`pypi:quorum-tiny@1.0.0` placeholder mapped to the
    tiny fixture) with the evidence source **labeled** fixture-vs-live.
- Adapter states: SUPPORTED_AND_VERIFIED / SUPPORTED_BUT_FAILED / UNSUPPORTED /
  NOT_FOUND / UNAVAILABLE / MALFORMED_EVIDENCE. Raw evidence preserved.
