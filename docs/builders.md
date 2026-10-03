# Builders

## Abstraction

identity → environment → source acquisition → build command → artifact
collection → hashing → provenance → attestation → signing.

Every builder carries `id` + `independence_group`. The quorum engine counts one
vote per `(digest, independence_group)`: `AWS-1 + AWS-2 + AWS-3` in group `aws`
is **one** independent source, not three. Demo profiles: `builder-a/cloud-a`,
`builder-b/cloud-b`, `builder-c/cloud-c`.

## Current state

- Local registry: `quorum builders register --id … --group …` (CLI) and
  `POST /api/v1/builders` (API); disable via `PATCH /builders/:id`.
- Isolation: Docker containers (`infra/docker/builder.Dockerfile`,
  network-restricted demo proven: `--network none` reproduced the tiny digest).
- Builds run pinned-source → clean env → hash → compare; no secrets in build
  envs; network access only when declared.

## Limitation (explicit)

Separate containers on one host ≠ independent infrastructure. Real independence
needs separate clouds/hosts/operators — tracked as future work with remote
builder attestation.
