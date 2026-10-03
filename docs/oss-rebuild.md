# OSS Rebuild integration (live path implemented)

Google OSS Rebuild is the initial external evidence layer (one source, not an
automatic quorum member; duplicates never double-counted).

## Real interface (verified against the live CLI, Oct 2026)

- Install: `go install github.com/google/oss-rebuild/cmd/oss-rebuild@latest`.
- Ecosystems on the wire: **`npm`, `pypi`, `cratesio`** (Quorum maps `cargo:` → `cratesio`).
- `oss-rebuild list pypi absl-py` — versions with attestations.
- `oss-rebuild get pypi absl-py 2.0.0` — summary (`Rebuild found!`, upstream
  digest, pinned source commit, Dockerfile); `--output=payload|bundle|
  dockerfile|build|steps`.
- Reads need **no credentials**; signature verification is on by default.
- CLI quirks the adapter handles (all covered by tests): human markers and the
  artifact-inference NOTE go to **stderr** (machine parsing uses stdout);
  `--output=payload` appends extra top-level JSON documents after the
  statement (first-document validation, whole blob preserved).

## Go adapter (`services/ossrebuild`)

`Provider{Bin, Timeout, MaxAttempts}` with `Lookup(pkgRef)` normalizing to
`SUPPORTED_AND_VERIFIED / SUPPORTED_BUT_FAILED / UNSUPPORTED / NOT_FOUND /
UNAVAILABLE / MALFORMED_EVIDENCE`: upstream digest + source commit extracted
from the summary, full signed payload preserved (in-toto/SLSA-validated lead
document), timeouts + one safe retry. `quorum verify --oss-mode live` attaches
verified rebuilds as `oss-rebuild` evidence under its own independence group —
agreement counts, disagreement surfaces as conflict, commit drift is excluded
with a reason.

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
