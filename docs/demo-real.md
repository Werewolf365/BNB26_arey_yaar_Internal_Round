# Showcase runbook: real-project end-to-end (copy-paste presenter script)

One command does everything for the default project: `make demo-real`
(XZ Utils v5.8.1). Your own repository: `make demo-real PROJECT=my-project`
or bare overrides (see below). `make demo-list` shows configured projects.
This runbook is the manual version of the same 7 phases, so you can narrate
each step live. Total ~5 minutes on a warm machine.

## Bring your own repository (do this before the presentation)

1. Create a **small, public** GitHub repository with a reproducibly
   archivable program (keep it under ~10 MB; `git archive` must be
   byte-deterministic — test that first, below).
2. Commit your release, tag it (`git tag v0.1.0 && git push --tags`), and
   resolve the tag to the full commit SHA: `git ls-remote <url> v0.1.0`.
3. Copy `projects/my-project.json` to `projects/<yours>.json` and fill in
   `repo` (https `.git` URL), `tag`, `commit` (40-hex SHA — never the tag),
   `package`/`ecosystem`/`version`. Or generate it instead of hand-writing:
   `make add-project REPO=https://github.com/<you>/<repo> REF=<tag|sha>
   [NAME=<name>] [ECOSYSTEM=go|npm|cargo|pypi|c]` (resolves via the API, then
   `make demo-real PROJECT=<name>`). Direct overrides also work:
   `make demo-real REPO=<git-url> COMMIT=<sha> TAG=<tag> ...`. Large trees (>50 MiB) need an explicit
   cap at CLI time: `verify --max-bytes <bytes>` (default 50 MiB guard).
4. Sanity-check determinism yourself (this is what every builder repeats):
   `git archive <sha> | sha256sum` twice → identical bytes.
5. The script validates everything up front (allowlisted host, full SHA,
   tag-must-resolve-to-commit, ≥2 distinct builders/groups) and refuses to
   create any records on drift — a moving tag can never silently change the
   build.

> Honesty labels — say these out loud:
> - Builders reproduce the **pinned SOURCE ARCHIVE digest** (`git archive`
>   bytes), not compiled binaries. Cross-toolchain binary reproducibility is
>   explicitly out of scope.
> - The "conflicting builder" is a **controlled SIMULATED minority**, never
>   presented as a genuine independent build, and never the historical XZ
>   incident (this demo does not reproduce any real compromise).
> - Chain transactions are **local Anvil demo transactions** (chainId 31337),
>   not public-chain transactions.

Pinned facts (baked into `scripts/demo-real.mjs`, overridable via
`XZ_REPO`/`XZ_TAG`/`XZ_COMMIT`):

| item | value |
|---|---|
| repo | `https://github.com/tukaani-project/xz` |
| tag | `v5.8.1` |
| commit | `a522a226545730551f7e7c2685fab27cf567746c` |
| reference artifact | `git archive HEAD` bytes, `sha256:20982c73…` (recomputed live, step 1) |

## 0. Start services (one command)

```bash
docker compose -f infra/compose/docker-compose.yml up --build -d
curl localhost:8080/api/v1/health          # {"data":{"status":"ok"}}
curl localhost:8080/api/v1/ready          # {"data":{"status":"ready"}}
curl -X POST localhost:8545 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}'
go build -o quorum ./apps/cli && go build -o quorum-worker ./apps/worker
```

## 1. Pin the source + compute the reference digest (what builders must match)

```bash
rm -rf /tmp/xz-stage && git init -q /tmp/xz-stage
git -C /tmp/xz-stage remote add origin https://github.com/tukaani-project/xz.git
git -C /tmp/xz-stage fetch -q --depth 1 origin v5.8.1
git -C /tmp/xz-stage checkout -q FETCH_HEAD
git -C /tmp/xz-stage log -1 --format=%H   # must be a522a226…746c
git -C /tmp/xz-stage archive HEAD > /tmp/xz.tar
git -C /tmp/xz-stage archive HEAD | sha256sum   # twice: identical bytes
```

## 2. CLI ritual: hash + policy on the real tarball (exits are the interface)

```bash
REF=$(git -C /tmp/xz-stage archive HEAD | sha256sum | awk '{print "sha256:"$1}')
./quorum verify --repo https://github.com/tukaani-project/xz \
  --commit a522a226545730551f7e7c2685fab27cf567746c \
  --artifact /tmp/xz.tar --expected-digest "$REF"      # exit 0 VERIFIED
printf 'x' >> /tmp/xz.tar
./quorum verify --repo https://github.com/tukaani-project/xz \
  --commit a522a226545730551f7e7c2685fab27cf567746c \
  --artifact /tmp/xz.tar --expected-digest "$REF"; echo "exit=$? (want 1, ARTIFACT_HASH_MISMATCH)"
```

## 3. Register builders (independence groups are the trust unit) + release

Each builder gets its own P-256 key. The **independence group** (not the
builder id) is what the engine counts — two builders in one group are one
vote. Register with the public key so completions are signature-checked:

```bash
./quorum attest genkey --private b-a.pem --public b-a.pub.pem
./quorum attest genkey --private b-b.pem --public b-b.pub.pem
./quorum attest genkey --private b-c.pem --public b-c.pub.pem
PUB_A=$(cat b-a.pub.pem); PUB_B=$(cat b-b.pub.pem); PUB_C=$(cat b-c.pub.pem)
curl -X POST localhost:8080/api/v1/builders -H 'Content-Type: application/json' \
  -d "{\"id\":\"builder-a\",\"displayName\":\"Stage A\",\"independenceGroup\":\"operator-a\",\"signingPublicKey\":$(echo "$PUB_A" | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))')}"
# repeat for builder-b/operator-b, builder-c/operator-c
curl -X POST localhost:8080/api/v1/releases -H 'Content-Type: application/json' \
  -d '{"package":"xz","ecosystem":"c","name":"xz","version":"5.8.1","repo":"https://github.com/tukaani-project/xz","commit":"a522a226545730551f7e7c2685fab27cf567746c","expectedDigest":"'"$REF"'"}'
# -> {"data":{"id":"rel_…"}} — save as $REL
```

What each builder installs/runs/submits (the worker loop, one per operator
host in production — same-machine here proves the protocol only):

1. `git init/fetch --depth 1/checkout` the pinned commit (allowlisted https only).
2. `git archive <commit>` **inside** a `--network none` container, source
   mounted read-only; SHA-256 the bytes (`&&`-chained so git failure can
   never silently hash empty stdin).
3. Sign a DSSE/in-toto+SLSA statement (commit+digest+builder) with its key.
4. `POST /jobs/claim` → `POST /jobs/:id/complete {ok,digest,commit,attestation}`.
   The API verifies the attestation against the **registered** key, commit,
   digest, and builder id before counting it; unsigned/forged completions
   can never satisfy the signature-required default policy.

```bash
curl -X POST localhost:8080/api/v1/verifications -H 'Content-Type: application/json' \
  -d '{"releaseId":"'$REL'","evidence":[]}'     # -> ver_… (INSUFFICIENT until jobs land)
curl -X POST localhost:8080/api/v1/verifications/$VER/jobs -H 'Content-Type: application/json' \
  -d '{"builderIds":["builder-a"]}'
./quorum-worker --api http://localhost:8080 --owner operator-a --signing-key b-a.pem --verification $VER --once
# repeat enqueue+worker for b, then c (one builder queued at a time so each
# key provably signs its own builder's attestation; --verification scopes
# each claim to this verification, so stale QUEUED jobs from older runs are
# never touched — reruns are safe without wiping the database)
curl localhost:8080/api/v1/verifications/$VER/jobs
```

## 4. Read the decision (auto-evaluation mints a NEW verification row)

When the last sibling job goes terminal, the API evaluates the default
policy and appends a **new** verification (the pending row stays as history;
partial sibling sets also mint honest intermediate `INSUFFICIENT_EVIDENCE`
rows along the way — point at them as the system reasoning incrementally).
Find the final row via the audit trail (latest `quorum.evaluated` with
`fromJobs:true`), then read it:

```bash
curl 'localhost:8080/api/v1/audit?limit=30' | python3 -c \
  "import json,sys; rows=json.load(sys.stdin)['data']; print([r['payload']['verificationId'] for r in rows if r['eventType']=='quorum.evaluated' and r['payload'].get('fromJobs')][-1])"
curl localhost:8080/api/v1/verifications/$SETTLED | python3 -m json.tool
# decision VERIFIED, 3 counted evidence, all artifactDigest == $REF
```

Or open it in the dashboard: `http://localhost:3000/verifications/$SETTLED`
(decision, counted/excluded evidence, conflicts, jobs, anchor).

## 5. Controlled conflict (SIMULATED minority — say so)

```bash
curl -X POST localhost:8080/api/v1/verifications -H 'Content-Type: application/json' -d '{
  "releaseId":"'$REL'",
  "evidence":[
    {"builderId":"builder-a","independenceGroup":"operator-a","sourceCommit":"a522a226545730551f7e7c2685fab27cf567746c","artifactDigest":"'"$REF"'","signatureValid":true,"verificationSource":"builder"},
    {"builderId":"builder-b","independenceGroup":"operator-b","sourceCommit":"a522a226545730551f7e7c2685fab27cf567746c","artifactDigest":"'"$REF"'","signatureValid":true,"verificationSource":"builder"},
    {"builderId":"simulated-minority","independenceGroup":"operator-c","sourceCommit":"a522a226545730551f7e7c2685fab27cf567746c","artifactDigest":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","signatureValid":true,"verificationSource":"builder"}
  ]}'
# -> VERIFIED_WITH_CONFLICT with conflicts[0] carrying the minority digest
```

## 6. Anchor (hashes only) + independent re-read

Bindings (all deterministic, all content-derived):
`vid=H("quorum-demo:"+verificationId)`, `evidence=H(canonical bundle)`,
`artifact=<reference digest>`, `source=H(commit)`, `policy=H("policy-a/v1")`.
Anvil is ephemeral — deploy a fresh contract per demo (never assume an address):

```bash
BC=$(python3 -c "import json;print(json.load(open('contracts/out/QuorumAnchor.sol/QuorumAnchor.json'))['bytecode']['object'])")
docker run --rm --network host --entrypoint cast ghcr.io/foundry-rs/foundry:latest \
  send --mnemonic "test test test test test test test test test test test junk" \
  --mnemonic-index 0 --rpc-url http://localhost:8545 --create "0x$BC"  # -> contract
./quorum blockchain anchor --required --rpc http://localhost:8545 --contract $CONTRACT \
  --verification-id $VID --evidence-hash $EV --artifact-digest $ART \
  --source-commit $SRC --policy-hash $POL --decision VERIFIED   # submits + verifies event
./quorum blockchain verify --rpc http://localhost:8545 --contract $CONTRACT \
  --verification-id $VID    # exit 0: anchored: true chain=0x7a69 tx=… block=… event=true
```

Inspect: web explorer `http://localhost:3000/blockchain` (RPC
`http://localhost:8545` for host-run API, `http://anvil:8545` when the API
runs in compose — the lookup executes server-side, so the RPC must resolve
**from the API container**), or API directly:

```bash
curl "localhost:8080/api/v1/blockchain/lookup/$VID?rpc=http://anvil:8545&contract=$CONTRACT"
# {"data":{"anchored":true,"chainId":"0x7a69","txHash":"…","blockNumber":"…","eventFound":true}}
```

## 7. Audit trail

```bash
curl 'localhost:8080/api/v1/audit?limit=10'   # release/builders/jobs/evaluations
# full-chain verify of the newest record:
AID=$(curl -s 'localhost:8080/api/v1/audit?limit=1' | python3 -c "import json,sys;print(json.load(sys.stdin)['data'][0]['id'])")
curl -X POST localhost:8080/api/v1/audit/$AID/verify-chain   # {"ok":true,"records":N}
```

Cleanup: delete only what you created (`rm -rf /tmp/xz-stage /tmp/xz.tar b-*.pem b-*.pub`).
DB rows stay — the audit trail is append-only by design. For a pristine stage,
reset the LOCAL demo stack only: `docker compose -f infra/compose/docker-compose.yml down -v`
(Deletes local demo Postgres/S3/evidence volumes. Never run against shared data.)

## Policy reference (for Q&A)

- Engine uses **absolute counts, not percentages** (`minBuilders`,
  `requiredAgreement`, `requiredIndependentGroups`, `conflictTolerance`) —
  deliberate: with N=3 builders a percentage is marketing, not security.
- Default `policy-a/v1`: 2 builders, agreement 2, groups 2, source must
  match, all signatures valid, tolerance 1, sources builder+oss-rebuild.
- Decisions: `VERIFIED` (quorum, no conflict) · `VERIFIED_WITH_CONFLICT`
  (quorum + tolerated minority, minority always shown) · `INVESTIGATE`
  (quorum missed but conflict exists, or tolerance exceeded) · `REJECTED`
  (digest/signature/source binding failed) · `INSUFFICIENT_EVIDENCE`
  (not enough countable evidence — unsigned/forged/mismatched evidence is
  *excluded with reasons*, never silently dropped) · `ERROR` (invalid policy).
- Same group = one vote (duplicate builder ids and same-group second votes
  are excluded with reasons). Malformed evidence (empty id/group/digest) is
  excluded, never counted — a fuzzer proved this path.
- Configure: CLI flags (`verify --min-builders/--required-agreement/
  --required-groups/--tolerance`, `policy test/validate`), API
  (`POST /policies`, per-verification `policy` object, `POST
  /policies/validate`), stored policy hash binds the rule into the anchor.
  Recommendation for the showcase: keep the default; mention tolerance 0
  (`policy-strict.json`) flips the conflict demo to INVESTIGATE.
