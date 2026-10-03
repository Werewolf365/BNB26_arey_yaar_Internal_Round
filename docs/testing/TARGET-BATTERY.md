# Quorum target resilience report (executed 2026-10-03)

Toolchain: `quorum.exe 0.2.0-slice2` (current build), Go 1.27.1 (pinned), Node 22, oss-rebuild CLI (`go install github.com/google/oss-rebuild/cmd/oss-rebuild@latest`), git 2.51, Docker 29.6.1.

Method per target: shallow clone (depth 1) → commit pin → deterministic source archive via `git archive --format=tar HEAD` → three CLI probes (verify / tamper / conflict) → OSS Rebuild probe (`list` + `get` on the first listed version, real CLI, no credentials needed for reads).


| target | clone | commit | archive-sha256 (prefix) | verify | tamper probe | conflict probe | oss list | oss get |
|---|---|---|---|---|---|---|---|---|

| `abseil/abseil-py` | PASS (reused) | `87523671c432` | `fbea19d23b00` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 8 indexed | 1.3.0: verified |
| `pallets/flask` | PASS (reused) | `d73fa1cdcbd8` | `3e1b12afb9fb` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 0 indexed | no indexed versions |
| `psf/requests` | PASS (reused) | `611c6162cbc4` | `6734f94a3b82` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 26 indexed | exit 1 |
| `expressjs/express` | PASS (reused) | `7ef98448f8b3` | `e4233aa5f09b` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 11 indexed | 4.16.4: verified |
| `axios/axios` | PASS (reused) | `c4303eeb601d` | `2699933fcd05` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 1 indexed | 0.30.2: verified |
| `lodash/lodash` | PASS (reused) | `2b5e6f7399a7` | `382e37ccb126` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 4 indexed | 4.17.20: verified |
| `serde-rs/serde` | PASS (reused) | `6693a89cca77` | `443ca4c98a22` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 0 indexed | no indexed versions |
| `tokio-rs/tokio` | PASS (reused) | `5d5cd8b5b896` | `16c9a2b79b18` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 0 indexed | no indexed versions |
| `BurntSushi/ripgrep` | PASS (reused) | `3fce3b5bb023` | `fd4ee28d85c3` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | 0 indexed | no indexed versions |
| `golang/example` | PASS (reused) | `7f05d217867b` | `3595105d03f3` | `VERIFIED` (0) | `REJECTED` (1) | exit 3, minority surfaced=YES | SKIP (out of scope) | — |



## Detailed action log per target


### `abseil/abseil-py`

- clone: `git clone --depth 1 https://github.com/abseil/abseil-py.git` → commit `87523671c43233c1c5feaa11d446685ab2c67c91`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:fbea19d23b0041c7fa1267d0968ffc66e527d8d7c5731d2e747e014afd8d49fa`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list pypi absl-py` + `oss-rebuild get pypi absl-py <first-listed>` (see matrix)


### `pallets/flask`

- clone: `git clone --depth 1 https://github.com/pallets/flask.git` → commit `d73fa1cdcbd8b1465c151db8924ba58b1dd14e35`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:3e1b12afb9fbe5096426acfcefa812c87ea9c31af71dadf1acf5e4f203763437`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list pypi Flask` + `oss-rebuild get pypi Flask <first-listed>` (see matrix)


### `psf/requests`

- clone: `git clone --depth 1 https://github.com/psf/requests.git` → commit `611c6162cbc4ac2020a2f91c7cfa4f3abf9bbb60`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:6734f94a3b82b9964fdd6171048a5f400bd86467946ad440f187155dcdf28b42`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list pypi requests` + `oss-rebuild get pypi requests <first-listed>` (see matrix)


### `expressjs/express`

- clone: `git clone --depth 1 https://github.com/expressjs/express.git` → commit `7ef98448f8b38099ab1ded55e458538ad47a51e7`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:e4233aa5f09b5bba7ab65cd5a6c113fbf95d7a852ab61f51202d9115fb7d5395`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list npm express` + `oss-rebuild get npm express <first-listed>` (see matrix)


### `axios/axios`

- clone: `git clone --depth 1 https://github.com/axios/axios.git` → commit `c4303eeb601d3d5914a0f425cc7539616fdc7ea7`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:2699933fcd05ef75c3b41c272a0f6e5ff39c64d5bf565b7cb5f0c35c0ce127d6`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list npm axios` + `oss-rebuild get npm axios <first-listed>` (see matrix)


### `lodash/lodash`

- clone: `git clone --depth 1 https://github.com/lodash/lodash.git` → commit `2b5e6f7399a7b48005140b5d5c6bc6c0e62919a8`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:382e37ccb12698cc9404ca06bd0bacced64d949b7d1b5186bdc7ddb5b443511e`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list npm lodash` + `oss-rebuild get npm lodash <first-listed>` (see matrix)


### `serde-rs/serde`

- clone: `git clone --depth 1 https://github.com/serde-rs/serde.git` → commit `6693a89cca77e0151437da1c7f890090b9ebf04c`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:443ca4c98a22c2deebe70b6aa468e41cebed968e7de4dbdc96eb345dc1226c77`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list cratesio serde` + `oss-rebuild get cratesio serde <first-listed>` (see matrix)


### `tokio-rs/tokio`

- clone: `git clone --depth 1 https://github.com/tokio-rs/tokio.git` → commit `5d5cd8b5b896796445920b3b78c1ad5f9b853fc6`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:16c9a2b79b18da9e9afe25c3ede513c20a38122d5d4baef08d77b865243dbfc8`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list cratesio tokio` + `oss-rebuild get cratesio tokio <first-listed>` (see matrix)


### `BurntSushi/ripgrep`

- clone: `git clone --depth 1 https://github.com/BurntSushi/ripgrep.git` → commit `3fce3b5bb0236da2df6d99672afb8a719642eca7`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:fd4ee28d85c32eb7e5c88e522f7d651900638444a4506b88b43af0870a28a510`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: `oss-rebuild list cratesio ripgrep` + `oss-rebuild get cratesio ripgrep <first-listed>` (see matrix)


### `golang/example`

- clone: `git clone --depth 1 https://github.com/golang/example.git` → commit `7f05d217867b2af52b0a28c6d1c91df97e1b5b39`

- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `sha256:3595105d03f3f157cc3712e39b436c1e75695532684ea5da6ed19719cafdf9c5`

- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced

- OSS Rebuild: skipped (ecosystems supported: npm/pypi/cratesio)
