# Quorum — Judge-ready demonstration runbook

This is a practical presentation script for a technical panel. It is designed
to prove what Quorum actually does, make every claim reproducible, and clearly
separate deterministic local proof from live dependencies and production work.

> Core claim: Quorum does **not** declare software safe. It produces a bounded,
> auditable decision that a pinned source identity, artifact digest, signed
> evidence, and quorum policy did (or did not) agree.

## 1. What to say in the first minute

“Quorum is a software supply-chain verification system. Builders reproduce a
pinned source revision and report an artifact digest with provenance. A consumer
applies an explicit quorum policy, rather than trusting one builder or a single
signature. Conflicting evidence is surfaced, and decisions are audit chained.
The blockchain contains only decision/evidence hashes, never binaries or
secrets.”

Do **not** say that `VERIFIED` means safe, that two Docker containers on one
machine are independent infrastructure, or that an unavailable blockchain was
used. Those claims are deliberately not made by the software.

## 2. Feature-to-proof map

| Panel requirement | Demonstration proof | Expected result |
|---|---|---|
| Independent verification | Policy evidence has distinct `independenceGroup` values; one vote per group | Two groups can meet policy; repeated same-group votes cannot |
| Artifact integrity | SHA-256 binding plus signed subject digest | A one-byte change rejects |
| Source integrity | Exact source commit is bound in provenance | Replay under another commit rejects |
| Signed provenance | DSSE/in-toto/SLSA statement verification | Valid envelope passes; tampered signature fails |
| Quorum trust assessment | Policy evaluator output and CLI exit contract | `VERIFIED`, `REJECTED`, `INSUFFICIENT_EVIDENCE`, or `VERIFIED_WITH_CONFLICT` |
| Disagreement handling | Deliberately inject a minority result | Conflict appears in output; it is never hidden |
| Auditability | Hash-chain verification | Original chain passes; changed record identifies the break |
| Blockchain anchoring | Local Anvil contract deployment/anchor/readback | Matching event and stored hash, or an explicit `SKIPPED` label |
| Real open-source evidence | Pinned `golang/example` worker rebuild and/or live OSS Rebuild | Reproducible digest / external evidence, subject to documented prerequisites |

## 3. Preflight: do this before the judges arrive

Use PowerShell from the repository root.

```powershell
node --version                 # Node 20 or later
go version                     # Go 1.27.1 expected
docker info                    # needed only for Postgres/S3/Anvil/isolated worker demos
node scripts/doctor.js
npm test
go test ./...
go build -o quorum.exe ./apps/cli
go build -o quorum-api.exe ./apps/api
go build -o quorum-worker.exe ./apps/worker
```

Record the commit being demonstrated and start from a clean worktree:

```powershell
git rev-parse --short HEAD
git status --short
```

If a judge requests a clean-room proof, use a fresh clone and repeat the four
test/build commands above. Do not rely on a prebuilt executable.

### Live-stack preflight (optional but recommended)

```powershell
docker compose -f infra/compose/docker-compose.yml up -d postgres minio anvil
docker compose -f infra/compose/docker-compose.yml ps
$env:QUORUM_DATABASE_URL = 'postgres://quorum:quorum@localhost:5433/quorum?sslmode=disable'
$env:QUORUM_S3_ENDPOINT = 'http://localhost:4566'
$env:QUORUM_S3_ACCESS_KEY = 'test'
$env:QUORUM_S3_SECRET_KEY = 'test'
$env:QUORUM_S3_PATH_STYLE = 'true'
$env:QUORUM_API_KEYS = 'judge-demo-admin-key'
Start-Process -FilePath .\quorum-api.exe -WindowStyle Hidden
Start-Sleep -Seconds 2
Invoke-RestMethod http://localhost:8080/api/v1/health
Invoke-RestMethod http://localhost:8080/api/v1/ready
```

Keep a second terminal open for API logs. It is useful evidence that request
IDs, status codes, and typed errors are emitted without logging keys.

## 4. Recommended 20-minute presentation order

1. Explain trust boundaries and show the policy model (2 min).
2. Run the deterministic valid/tampered/conflict/audit demonstration (4 min).
3. Show signed provenance and replay protection (3 min).
4. Show API persistence, evidence storage, and signed-worker admission (5 min).
5. Show audit and optional chain anchor (3 min).
6. Show test gate, real-repository evidence, limitations, and take questions (3 min).

If time is short, sections 5 and 7 are optional. Never skip the tamper and
conflict demonstrations: they are the strongest proof that this is not merely
a happy-path dashboard.

## 5. Deterministic core demo — run live in front of the panel

```powershell
node scripts/slice1-demo.mjs
```

Expected claims to point out in the output:

- Scenario 1: `VERIFIED` for agreeing evidence.
- Scenario 2: `REJECTED` with `ARTIFACT_HASH_MISMATCH` after a one-byte change.
- Scenario 3: `VERIFIED_WITH_CONFLICT` with the minority evidence visible.
- Audit chain: `OK`.
- Blockchain: `SKIPPED` is an honest result if Anvil was not configured.

Then let the judges modify the scenario through the CLI:

```powershell
# Valid deterministic fixture.
.\quorum.exe verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny

# Exact digest tripwire: must reject.
.\quorum.exe verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny --expected-digest sha256:0000000000000000000000000000000000000000000000000000000000000000

# Conflict must remain visible and return the investigation/conflict exit code.
.\quorum.exe verify --repo https://github.com/example/tiny --commit abc123 --fixture-tiny --inject-conflict builder-c --verbose

# Same group cannot manufacture independence.
.\quorum.exe policy test --policy-file apps/cli/testdata/policy-a.json --evidence-file fixtures/conflicting-builders/case.json

# Audit negative control.
.\quorum.exe audit verify --demo
.\quorum.exe audit verify --demo --tamper
```

The exit-code contract is part of the consumer interface: `0` verified, `1`
rejected, `2` insufficient evidence, `3` conflict/investigation, `4`
operational failure, and `5` invalid input.

## 6. Signed provenance demonstration

Create a disposable workspace; never demonstrate with a production private key.

```powershell
$repo = (Get-Location).Path
$demo = Join-Path $env:TEMP 'quorum-judge-demo'
New-Item -ItemType Directory -Force $demo | Out-Null
Set-Location $demo
Set-Content -NoNewline artifact.bin 'judge-demo-artifact-v1'

& "$repo\quorum.exe" attest genkey --private builder-a.pem --public builder-a.pub.pem
& "$repo\quorum.exe" attest sign --artifact artifact.bin --commit abc123 --builder builder-a --key builder-a.pem --output builder-a.dsse.json
& "$repo\quorum.exe" attest verify --envelope builder-a.dsse.json --key builder-a.pub.pem --expect-commit abc123 --allow-builder builder-a
```

For a challenge test, change the expected commit (or append a byte to the
artifact and use its expected digest). Verification must fail; it must not
return a warning-only success.

```powershell
& "$repo\quorum.exe" attest verify --envelope builder-a.dsse.json --key builder-a.pub.pem --expect-commit another-commit --allow-builder builder-a
```

Return to the repository root before continuing.

## 7. API, object storage, signed worker admission, and quorum

This section demonstrates the server trust boundary. Administrative writes use
the API key configured in preflight.

```powershell
Set-Location $repo
$headers = @{ 'X-Api-Key' = 'judge-demo-admin-key'; 'Content-Type' = 'application/json' }

# Create builder key material. Use separate keys and separate operator hosts
# in a real deployment. For a one-machine demo, explain that this proves the
# protocol, not infrastructure independence.
.\quorum.exe attest genkey --private builder-a.pem --public builder-a.pub.pem
.\quorum.exe attest genkey --private builder-b.pem --public builder-b.pub.pem

$builderA = @{ id='builder-a'; displayName='Judge demo A'; independenceGroup='operator-a'; signingPublicKey=(Get-Content -Raw builder-a.pub.pem) } | ConvertTo-Json
$builderB = @{ id='builder-b'; displayName='Judge demo B'; independenceGroup='operator-b'; signingPublicKey=(Get-Content -Raw builder-b.pub.pem) } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/builders -Headers $headers -Body $builderA
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/builders -Headers $headers -Body $builderB

$release = @{ package='golang-example'; ecosystem='go'; name='example'; version='pinned-demo'; repo='https://github.com/golang/example'; commit='7f05d21' } | ConvertTo-Json
$releaseReply = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/releases -Headers $headers -Body $release
$releaseId = $releaseReply.data.id

# Create an initially insufficient verification; workers subsequently add evidence.
$verification = @{ releaseId=$releaseId; evidence=@() } | ConvertTo-Json -Depth 5
$verificationReply = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/verifications -Headers $headers -Body $verification
$verificationId = $verificationReply.data.id

$jobs = @{ builderIds=@('builder-a','builder-b') } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/verifications/$verificationId/jobs" -Headers $headers -Body $jobs
```

Run workers only on hosts controlled by their corresponding operators. The
following commands demonstrate the protocol locally; use `--once` when
presenting and inspect the job/verification after each completion.

```powershell
.\quorum-worker.exe --api http://localhost:8080 --owner operator-a --signing-key builder-a.pem --once
.\quorum-worker.exe --api http://localhost:8080 --owner operator-b --signing-key builder-b.pem --once

Invoke-RestMethod "http://localhost:8080/api/v1/verifications/$verificationId/jobs"
Invoke-RestMethod "http://localhost:8080/api/v1/verifications/$verificationId"
Invoke-RestMethod 'http://localhost:8080/api/v1/audit?limit=20'
```

What to explain:

- The API stores the original envelope with the job and calculates
  `signatureValid`; a worker cannot set that field itself.
- It validates the trusted builder key, source commit, digest, and builder ID
  before counting the result.
- An unsigned completion is retained as evidence but cannot satisfy the
  signature-required default policy.
- The source-archive worker requires Docker by default and uses `--network none`
  with a read-only source mount. Host fallback is explicitly dev-only.

### Evidence-object proof

Show content addressing and on-read integrity checking independently of a
worker result:

```powershell
$bytes = [Text.Encoding]::UTF8.GetBytes('evidence payload for judges')
$sha = ([System.Security.Cryptography.SHA256]::HashData($bytes) | ForEach-Object { $_.ToString('x2') }) -join ''
$body = @{ verificationId=$verificationId; kind='demo-note'; sha256=$sha; data=[Convert]::ToBase64String($bytes) } | ConvertTo-Json
$evidence = Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/evidence -Headers $headers -Body $body
$evidence
Invoke-WebRequest "http://localhost:8080/api/v1/evidence/$($evidence.data.id)/blob" -OutFile downloaded-evidence.bin
Get-FileHash downloaded-evidence.bin -Algorithm SHA256
```

## 8. Policy and disagreement challenge tests

Show the human-readable policy, then invite edits to the JSON policy file.

```powershell
Get-Content apps/cli/testdata/policy-a.json
.\quorum.exe policy validate --policy-file apps/cli/testdata/policy-a.json
.\quorum.exe policy test --policy-file apps/cli/testdata/policy-a.json --evidence-file apps/cli/testdata/evidence-agree.json
.\quorum.exe policy test --policy-file apps/cli/testdata/policy-a.json --evidence-file apps/cli/testdata/evidence-conflict.json
```

Challenge responses:

- “Three builders in AWS?” — same `independenceGroup` is one vote.
- “What if two agree and a third differs?” — minority digest appears in
  `conflicts`; tolerance controls whether the decision remains verified or is
  investigation.
- “What if only one builder is available?” — `INSUFFICIENT_EVIDENCE`, not a
  soft pass.
- “What if signature is invalid?” — the evidence is excluded where the policy
  requires valid signatures.

## 9. Blockchain demonstration (optional live Anvil)

Run this only after the deterministic demo works. The contract is intentionally
hash-only: no package contents, secrets, or private keys enter the chain.

```powershell
make test-contracts
$env:QUORUM_LIVE_ANVIL = '1'
go test -run TestAnchorLive ./internal/runner/ -v
```

Use [docs/blockchain.md](docs/blockchain.md) for the deployed local contract,
receipt/event expectations, and Docker Foundry commands. State clearly:

- Local Anvil proves contract interaction and event re-verification.
- It is not a public-chain availability or decentralization claim.
- With anchoring optional, a down chain is reported as `SKIPPED`; with required
  anchoring it becomes an operational failure.

## 10. Real open-source package demonstration

Use this only after rehearsing it on the same network and toolchain. The
documented reproducible target is `golang/example@7f05d21`; see
[docs/demo.md](docs/demo.md). The worker rebuilds a pinned source archive,
not a generic claim that every language ecosystem has bit-for-bit compiled
binary reproducibility across operating systems.

For external OSS Rebuild evidence, this is deliberately opt-in because it can
require the real CLI, credentials, and network access:

```powershell
# Requires installed oss-rebuild and the required cloud credentials.
.\quorum.exe verify pypi:absl-py@2.0.0 --repo https://github.com/abseil/abseil-py --commit 37dad4d3 --artifact path\to\absl_py.whl --oss-mode live --verbose
```

If external credentials are unavailable, do not fake the run. Show the
recorded executed run in `docs/demo.md`, then run the deterministic fixture and
worker/API signed-attestation tests in front of the panel instead.

## 11. Full test evidence

Run these before the presentation and retain terminal output or CI artifacts:

```powershell
npm test
go test ./...
go vet ./...
node scripts/slice1-demo.mjs
make test-contracts
make coverage
```

For live services:

```powershell
$env:QUORUM_TEST_POSTGRES = '1'
go test -run TestPostgresIntegration ./apps/api/... -v
$env:QUORUM_LIVE_ANVIL = '1'
go test -run TestAnchorLive ./internal/runner/ -v
$env:QUORUM_LIVE_S3 = '1'
go test -run TestLiveS3 ./services/storage/ -v
```

## 12. Limitations to disclose before a judge finds them

1. Separate containers on one host are **not** independently operated
   infrastructure. Production independence needs separate clouds/hosts,
   operators, credentials, monitoring, and incident processes.
2. A valid signature and a reproducible build do **not** prove that source or
   dependencies are benign.
3. Local P-256 signing supports deterministic protocol proof. The repository
   also provides real Cosign/Fulcio/Rekor paths; production keyless policy and
   trust-root governance must be configured by the operator.
4. S3-compatible storage is implemented; production credentials, bucket policy,
   retention, backups, and access control remain deployment responsibilities.
5. The local chain demo is Anvil. A public-chain deployment requires reviewed
   contract configuration, key custody, RPC resilience, and a fee policy.
6. The web dashboard remains a roadmap item; the CLI/API are the presentation
   interfaces today.

These are not defects to hide. Naming them precisely demonstrates that Quorum
distinguishes evidence from guarantees—the same discipline it applies to
software releases.

## 13. Tear-down

```powershell
Get-Process quorum-api -ErrorAction SilentlyContinue | Stop-Process
docker compose -f infra/compose/docker-compose.yml down
Remove-Item builder-a.pem,builder-a.pub.pem,builder-b.pem,builder-b.pub.pem,downloaded-evidence.bin -ErrorAction SilentlyContinue
```

Do not run `down -v` unless the panel explicitly agrees to remove the local
Postgres and LocalStack demonstration data.
