# Repository onboarding

Add a public GitHub repository without hand-writing JSON or looking up SHAs:

- Web: `/onboard` (paste URL → pick tag → review pinned config → create →
  verify release) and `/projects` + `/projects/:id`.
- CLI/scripted: `make demo-list`, then either
  `make add-project REPO=<github-url> REF=<tag|sha> [NAME=] [ECOSYSTEM=]`
  (writes `projects/<name>.json`, then `make demo-real PROJECT=<name>`),
  or run `make demo-real` with `REPO=`/`COMMIT=`/`TAG=` overrides directly.

## API

Base `/api/v1`, typed errors, `requestId` everywhere (see `api.md`).

```
GET  /onboarding/discover?repo=URL   # validate URL + list tags (read-only)
POST /onboarding/resolve             # {repo, ref, ecosystem?} -> pinned preview, no writes
POST /projects                       # resolve + persist (dedupe on repo+commit: 200 existing, 201 new)
GET  /projects
GET  /projects/:id
```

After creating a project, start verification with the existing release flow:
`POST /releases {package, ecosystem, name, version, repo, commit}` (the
`/projects/:id` and `/onboard` pages do this via a **verify release**
button), then builders + workers as in `docs/demo-real.md`.

## Rules

- Only `https://github.com/<owner>/<repo>(.git)` is accepted. Tags resolve
  to full SHAs server-side (annotated tags peeled); raw SHAs are verified to
  exist. Nothing is ever pinned to a mutable tag.
- Ecosystem is detected from root files (`go.mod`→go, `package.json`→npm,
  `Cargo.toml`→cargo, `pyproject.toml`/`setup.py`→pypi, `Makefile`→c).
  Unknown → empty, and the UI/API require an explicit choice from
  `go,npm,cargo,pypi,c`. The build is always the `git-archive` source
  reproduction; ecosystem labels the release.
- Builder/policy setup is NOT automated: the preview shows the default
  policy (`policy-a`: 2 builders, agreement 2, groups 2, sigs + source
  required, tolerance 1) and the project page warns when fewer than two
  independence groups are registered. No auto-generated builder names.
- Projects table has `UNIQUE(repo, commit)`; duplicates return the existing
  row instead of a second project.

## GitHub access

Unauthenticated quota is 60 req/hour per egress IP and shared networks
exhaust it — the API then answers `GITHUB_UNAVAILABLE` (502) with the
upstream reason. Set `GITHUB_TOKEN` (classic or fine-grained PAT, **empty
scope is enough**) for 5000 req/hour:

```bash
# host-run API:
GITHUB_TOKEN=ghp_... ./quorum-api
# compose (env file, never committed):
echo 'GITHUB_TOKEN=ghp_...' >> .env.local   # then reference via env_file
```

The token is a rate-limit bearer for **public** endpoints only: sent as a
`Bearer` header, never logged, stored, or placed in URLs. No OAuth, no
private repositories.

## Security notes

- HTTPS + github.com allowlist, owner/repo charset checks, no credentials in
  URLs, query/fragment rejected; tags allowlisted to a safe charset, SHAs to
  hex; responses capped at 1 MiB with 10 s timeouts; redirects confined to
  `api.github.com`; no shell, no git subprocess, no repository code executed.
- `writeErr` masks 500s as `internal error` but passes 502/504 messages
  through — those describe downstream state (rate limits, unreachable
  chains), not internals.
