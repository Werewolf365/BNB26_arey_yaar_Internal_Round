# Test matrix (Slice 1 running; full plan tracked here)

- Unit: quorum/policy, canonical JSON, DSSE sign/verify, digest compare, audit hash, guards, exit codes, config parse. Gate: >=90% overall, >=95% quorum/policy/security (Go coverage in CI).
- Integration (self-provisioned, needs Docker daemon): Postgres, MinIO, API, workers, OSS adapter fixture, builders, Anvil RPC, contracts. `make test-integration` starts compose; skips gracefully if daemon down.
- Contracts: forge test (deployment, ACL, events, duplicates, zero/max/malformed, replay, gas). Skipped gracefully without forge.
- E2E: scripts/slice1-demo.mjs (valid/tampered/conflict + audit check). Full CLI->API->DB->builder->quorum->audit->chain->UI later.
- Browser: Playwright later (loading/success/empty/conflict/rejected/insufficient/API-fail/chain states/malformed/unknown/a11y/mobile).
- Property/fuzz: Go fuzz + Foundry fuzz for canonical/hash/quorum/policy/parser/archive/API inputs. Invariants: determinism, monotonic evidence, conflict visibility, hash integrity, independence, signature integrity.
- Mutation: quorum/policy/sig/audit/compare (document limits).
- Chaos: DB/store/chain/builder/OSS down, restarts, timeouts, crashes, duplicate jobs, dropped tx, corrupted evidence/audit — deterministic documented behavior.
- Security: 44 fixtures from prompt section 25, each with test (5 shipped in Slice 1: valid, 1-byte tamper, conflict, replay, audit tamper; rest per phase).
- External: `make test-external` only (real OSS Rebuild needs ADC; testnet needs funds) — never in test-all.
