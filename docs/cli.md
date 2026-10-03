# CLI (`quorum`)

Primary developer interface. Global flags: `--json` (machine output),
`--quiet` (silent stdout; exit code only), `--verbose` (diagnostics).
Rules: `--json` wins over `--quiet` for stdout; errors always go to stderr
(even under `--quiet`) unless `--json` is set.

## Commands

| Command | Purpose |
|---|---|
| `quorum init [--dir DIR] [--force]` | scaffold `quorum.yaml` (refuses overwrite without `--force`) |
| `quorum verify --repo URL --commit SHA --artifact FILE` | verify a file artifact (exit 0–5) |
| `quorum verify pypi:name@ver` (also `npm:`, `cargo:`) | package-ref form; OSS Rebuild fixture evidence labeled in output |
| `quorum verify ... --expected-digest sha256:..` | tamper tripwire: mismatch → `REJECTED` (1) |
| `quorum verify ... --policy-file P --min-builders N --required-agreement N --required-groups N --tolerance N` | policy control (`--tolerance -1` = default) |
| `quorum builders list [--registry F]` | list registry (default `builders.json`) |
| `quorum builders register --id ID --group G [--name N] [--endpoint E]` | register; duplicates / missing group rejected |
| `quorum policy validate --policy-file F` | 0 valid / 5 invalid; prints policy hash |
| `quorum policy test --policy-file P --evidence-file E` | evaluate `{expectedSource, evidence[]}`; exit follows decision |
| `quorum evidence export ... --output BUNDLE.json` | run verification, write evidence bundle (mode 0600) |
| `quorum inspect RESULT.json` | human summary of a result file; exit follows decision |
| `quorum audit verify [--chain F \| --demo] [--tamper]` | 0 valid / 1 tampered / 5 bad input |
| `quorum blockchain anchor --contract A --verification-id H --evidence-hash H --artifact-digest H --source-commit H --policy-hash H --decision D [--rpc URL] [--from A] [--required]` | submit + receipt + event verification |
| `quorum doctor [--api URL]` | detect-only environment check (never installs) |
| `quorum version` | print version |

## Exit codes (contract, tested in `cli_test.go` + `runner_test.go`)
```
0 = verified        1 = rejected (incl. ARTIFACT_HASH_MISMATCH, oversize)
2 = insufficient    3 = conflict / investigation
4 = operational     5 = invalid input
```

`audit verify` uses 0 valid / 1 tampered / 5 bad input. `doctor` uses 0 healthy /
4 problems found. `blockchain anchor` with `--required=false` (default) reports
an unreachable chain as `SKIPPED` with exit 0 — never a fake success; with
`--required` it exits 4.

## Security notes

- Repo URLs: https + allowlisted hosts only (`github.com`, `gitlab.com`,
  `npmjs.com`, `pypi.org`, `crates.io`); credentials/loopback/link-local rejected.
- Artifact paths: NUL/controls/shell metacharacters rejected (opened directly, no join).
- Registry/bundle files written `0600`. No secrets in logs.
- `--inject-conflict` / `--inject-tamper` are test-only and always labeled
  `[simulated]` in output.
- Live-chain tests are env-gated (`QUORUM_LIVE_ANVIL=1`) so `go test` stays
  deterministic without a daemon; `make test-external` owns live services.
