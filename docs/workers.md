# Builder workers

`apps/worker` (`quorum-worker`) executes rebuild jobs from the API queue.
It reports bytes; the quorum engine decides.

## Loop

```
POST /jobs/claim {owner}  →  GET verification → GET release (repo, commit)
→ fetch pinned commit (host git, allowlisted remote only)
→ isolated rebuild: docker run --rm --network none -v src:/src:ro <image>
  sh -c "git -C /src archive <commit> | sha256sum"
→ POST /jobs/:id/complete {ok, digest, commit, ...}
```

API auto-evaluates when all sibling jobs are terminal: succeeded digests become
evidence (groups from the builder registry), failures become visible reasons,
and a new verification row is recorded with an audit event. A successful digest
counts under the default policy only when the worker also supplies a DSSE
attestation verified against that builder's registered public key.

## Guarantees and limits

- Queue: `UNIQUE(verification_id, builder_id)` (convergent enqueue), atomic
  `SKIP LOCKED` claims, expiring leases, bounded retries, terminal protection.
- Sandbox: no network, read-only source mount, pinned image (default
  `golang:1.27.1-bookworm`). Host fallback exists but is **opt-in**
  (`--allow-host-fallback`, dev only, logged).
- Rebuild target is the **source archive** (`git archive` digest): exact across
  homogeneous toolchains. Two isolated workers rebuilding
  `golang/example@7f05d21` produced identical `sha256:39193228…`. A queue
  result becomes `VERIFIED` only after each worker's registered signing key
  verifies its DSSE provenance; raw digest reports stay visible but untrusted.
- Cross-toolchain archive bytes differ (git 2.51 vs 2.39.5 proven) — builders
  are only comparable within one image; see `docs/builders.md`.
- Compiled-binary reproducibility (PE vs ELF etc.) stays platform-scoped and
  is demonstrated, not hidden.

## Run

```powershell
go build -o quorum-worker ./apps/worker
.\quorum-worker.exe --api http://localhost:8080 --owner builder-a --signing-key builder-a.pem
.\quorum-worker.exe --api http://localhost:8080 --owner builder-a --signing-key builder-a.pem --once
```
