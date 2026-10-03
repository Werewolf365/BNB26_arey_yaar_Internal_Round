# MASTER BUILD PROMPT --- QUORUM

## "Don't Trust the Binary. Trust the Builders."

You are the principal AI software engineer, security engineer, DevSecOps
engineer, smart-contract engineer, QA lead, and technical writer
responsible for building the **entire Quorum platform end-to-end**.

Build the product described below as a real, runnable, testable
application --- not a mockup, not a collection of TODOs, and not a
superficial demo.

The goal is to produce a complete monorepo containing:

-   a production-quality CLI,
-   a verification/orchestration backend,
-   independent builder workers,
-   Google OSS Rebuild integration,
-   signed attestations and provenance,
-   a configurable quorum/policy engine,
-   tamper-evident audit history,
-   an EVM-compatible blockchain anchoring layer,
-   a web dashboard,
-   database and object storage integration,
-   local development infrastructure,
-   CI/CD,
-   security controls,
-   documentation,
-   realistic end-to-end demonstrations,
-   and an extremely rigorous automated testing system.

Do not declare the project complete until every requirement in this
prompt is implemented, tested, documented, and demonstrated.

------------------------------------------------------------------------

# 0. PRODUCT CONTEXT

## Product

**Quorum: Don't Trust the Binary. Trust the Builders**

## Problem

Modern software depends on thousands of open-source packages, yet
consumers generally trust that a published binary/artifact was actually
built from its claimed public source code.

A compromised maintainer account, build server, release pipeline,
dependency, or artifact distribution path can introduce malicious
changes into software that otherwise appears legitimate.

The 2024 XZ Utils incident demonstrates why software supply-chain
provenance and independent verification matter.

Quorum addresses the problem by allowing multiple independent
builders/verifiers to reproduce software releases from a pinned source
commit, produce signed attestations, compare artifact digests, detect
disagreements, evaluate configurable quorum policies, and maintain an
auditable/tamper-evident history.

## Required PS capabilities

Quorum MUST support:

1.  Independent Verification
2.  Artifact Integrity
3.  Disagreement Handling
4.  Trust Assessment
5.  Tamper Resistance
6.  Release Decision
7.  Demonstration using real open-source software
8.  Demonstration using valid releases
9.  Demonstration using intentionally compromised/inconsistent artifacts

------------------------------------------------------------------------

# 1. CORE PRODUCT PRINCIPLE

The system must distinguish between:

-   source identity,
-   artifact identity,
-   build provenance,
-   attestation validity,
-   signer identity,
-   builder independence,
-   artifact reproducibility,
-   policy satisfaction,
-   quorum agreement,
-   and overall trust evidence.

Do NOT equate:

"signature valid" = "software safe"

Do NOT equate:

"rebuild matches" = "source is benign"

Do NOT equate:

"two builders agree" = "absolute security"

The UI and API must clearly communicate that Quorum establishes
**verification evidence and policy-based trust decisions**, not
mathematical proof that software is free from vulnerabilities or
malicious source code.

Use precise security terminology throughout the product.

------------------------------------------------------------------------

# 2. HIGH-LEVEL ARCHITECTURE

Build this architecture:

``` text
                         QUORUM PLATFORM
                              |
                +-------------+-------------+
                |                           |
          Quorum CLI                    Web UI
             (Go)                    (Next.js/TS)
                |                           |
                +-------------+-------------+
                              |
                         API Gateway
                              |
                         Backend API
                              |
       +----------------------+----------------------+
       |                      |                      |
       v                      v                      v
 Verification            Builder Orchestrator   Audit Service
 Engine                      |                      |
       |              +-------+-------+              |
       |              |       |       |              |
       v              v       v       v              v
 OSS Rebuild       Builder A Builder B Builder C  Database
       |
       v
 Attestation / Provenance Verification
       |
       +------------------+
       |                  |
       v                  v
  Sigstore/Cosign      in-toto/SLSA
       |
       v
 Policy / Quorum Engine
       |
       +-------------------+
       |                   |
       v                   v
 Release Decision     Conflict Analysis
       |
       v
 Evidence Bundle
       |
       +-----------------------+
       |                       |
       v                       v
 Object Storage        Blockchain Anchor
                         (EVM/Solidity)
```

The architecture must be modular enough that every external trust source
can be replaced by an adapter.

------------------------------------------------------------------------

# 3. TECHNOLOGY STACK

Use the following stack unless there is a documented technical reason to
change something.

## CLI

-   Go
-   Cobra
-   standard Go crypto libraries
-   structured JSON output
-   human-readable terminal output
-   Rich terminal presentation via a Go-compatible terminal UI library
    where appropriate

## Backend

Prefer Go for consistency with the CLI:

-   Go
-   HTTP REST API
-   OpenAPI specification
-   PostgreSQL
-   Redis only if genuinely required
-   object storage via S3-compatible API
-   background jobs using a simple durable queue or database-backed
    worker system before introducing unnecessary infrastructure

## Frontend

-   Next.js
-   TypeScript
-   Tailwind CSS
-   accessible component system
-   React Query/TanStack Query or equivalent
-   charts only where they materially improve auditability

## Blockchain

-   Solidity
-   Foundry
-   OpenZeppelin contracts where useful
-   EVM-compatible architecture
-   Anvil for local development/testing
-   testnet deployment through environment configuration
-   never require a public testnet for local development

The blockchain MUST NOT store complete artifacts or sensitive data.

Store only compact, deterministic evidence anchors such as:

-   verification record hash,
-   evidence bundle hash,
-   release identifier,
-   artifact digest,
-   source commit identifier or hash,
-   policy version/hash,
-   quorum decision,
-   timestamp,
-   schema/version identifiers.

Use events for auditability.

## Supply-chain/security standards

Integrate established tooling instead of inventing cryptography:

-   Google OSS Rebuild
-   Sigstore / Cosign
-   in-toto attestations
-   SLSA provenance
-   SHA-256 artifact digests
-   Git commit hashes
-   OCI/container tooling where applicable

The current external standards/documentation must be checked before
implementation. Do not assume CLI flags, APIs, attestation schemas, or
repository behavior from memory.

## Infrastructure

-   Docker
-   Docker Compose for local stack
-   Makefile or Taskfile
-   GitHub Actions CI
-   optional Kubernetes manifests only after the local/CI system works

------------------------------------------------------------------------

# 4. MONOREPO STRUCTURE

Create a clean monorepo similar to:

``` text
quorum/
├── apps/
│   ├── cli/
│   ├── api/
│   ├── web/
│   └── worker/
│
├── packages/
│   ├── schemas/
│   ├── client/
│   └── fixtures/
│
├── services/
│   ├── verification/
│   ├── builders/
│   ├── policy/
│   ├── audit/
│   ├── oss-rebuild/
│   └── blockchain/
│
├── contracts/
│   ├── src/
│   ├── test/
│   ├── script/
│   └── foundry.toml
│
├── deployments/
│
├── infra/
│   ├── docker/
│   ├── compose/
│   └── ci/
│
├── docs/
│
├── fixtures/
│   ├── valid/
│   ├── mismatch/
│   ├── conflicting-builders/
│   ├── invalid-signatures/
│   ├── malicious-metadata/
│   └── reproducibility/
│
├── scripts/
│
├── Makefile
├── README.md
├── SECURITY.md
├── CONTRIBUTING.md
├── LICENSE
└── .github/
    └── workflows/
```

Adapt the structure if needed, but preserve clear separation of
concerns.

------------------------------------------------------------------------

# 5. DOMAIN MODEL

Implement explicit domain models for at least:

## Release

Fields:

-   release ID
-   package/project identifier
-   ecosystem
-   package name
-   version
-   repository
-   pinned commit
-   expected artifact digest(s)
-   artifact metadata
-   creation time
-   status

## Artifact

Fields:

-   artifact ID
-   filename
-   media type
-   size
-   SHA-256
-   optional SHA-512
-   storage location
-   source release
-   builder outputs

## Builder

Fields:

-   builder ID
-   display name
-   builder type
-   trust domain
-   infrastructure identity
-   independence group
-   version
-   endpoint
-   capabilities
-   enabled/disabled
-   registration metadata

## Verification

Fields:

-   verification ID
-   release ID
-   builder ID
-   source commit
-   artifact digest
-   verification type
-   status
-   started/finished timestamps
-   logs/evidence references
-   attestation references
-   error classification

## Attestation

Fields:

-   attestation ID
-   statement type
-   subject
-   digest
-   predicate type
-   builder identity
-   signer identity
-   certificate/key metadata
-   signature/bundle reference
-   verification result

## Quorum Policy

Fields:

-   policy ID
-   version
-   minimum builders
-   required agreement count
-   required independent groups
-   allowed verification sources
-   required signature validity
-   allowed conflict states
-   artifact matching requirement
-   source matching requirement
-   expiry rules
-   policy hash

## Release Decision

Possible states:

-   VERIFIED
-   VERIFIED_WITH_CONFLICT
-   INSUFFICIENT_EVIDENCE
-   REJECTED
-   INVESTIGATE
-   ERROR

Never silently convert an error into a rejection.

## Audit Record

Must contain:

-   event ID
-   event type
-   actor
-   timestamp
-   previous record hash
-   current record hash
-   payload hash
-   verification ID
-   optional blockchain transaction
-   chain ID
-   contract address
-   anchor status

------------------------------------------------------------------------

# 6. GOOGLE OSS REBUILD INTEGRATION

Google OSS Rebuild must be the **initial external verification/evidence
layer**.

Do not make it merely a README mention.

Create a dedicated adapter/service:

``` text
OSSRebuildProvider
    |
    +-- resolve package
    +-- obtain rebuild evidence
    +-- obtain attestation/evidence
    +-- verify evidence
    +-- extract artifact digest
    +-- extract source/build metadata
    +-- normalize result into Quorum schema
```

The adapter must be isolated behind an interface.

Do not tightly couple the entire application to a specific OSS Rebuild
CLI version.

Support:

-   local CLI execution where appropriate,
-   configurable executable path,
-   configurable remote/API provider if supported,
-   fixture-based operation in tests,
-   timeout handling,
-   retries where safe,
-   explicit unsupported-package handling,
-   version detection,
-   structured parsing,
-   raw evidence preservation.

IMPORTANT:

Before coding the integration, inspect the current official Google OSS
Rebuild documentation/repository and determine the actual current
interfaces and supported ecosystems. Do not fabricate an API.

The adapter must support an explicit state:

``` text
SUPPORTED_AND_VERIFIED
SUPPORTED_BUT_FAILED
UNSUPPORTED
NOT_FOUND
UNAVAILABLE
MALFORMED_EVIDENCE
```

OSS Rebuild evidence must be treated as **one evidence source**, not
automatically as a quorum member.

Do not count duplicate or derived evidence twice.

------------------------------------------------------------------------

# 7. INDEPENDENT BUILDER SYSTEM

Implement a builder abstraction:

``` text
Builder
  ├── identity
  ├── environment
  ├── source acquisition
  ├── build command
  ├── artifact collection
  ├── hashing
  ├── provenance
  ├── attestation
  └── signing
```

Create at least three logical builder profiles for the demonstration.

They must have distinct:

-   builder IDs,
-   identity metadata,
-   execution environments,
-   evidence records.

Where feasible, use separate containers.

Builder configuration must include an `independence_group`.

The quorum engine MUST be capable of preventing multiple builders from
the same independence group from being counted as independent evidence.

Example:

``` yaml
builders:
  - id: builder-a
    independence_group: cloud-a

  - id: builder-b
    independence_group: cloud-b

  - id: builder-c
    independence_group: cloud-c
```

For the MVP/demo, Docker-based isolated builders are acceptable.

Document the limitation that separate containers on the same physical
host are not equivalent to fully independent infrastructure.

------------------------------------------------------------------------

# 8. REPRODUCIBLE BUILD ENGINE

Implement:

1.  pinned source checkout
2.  clean build environment
3.  dependency resolution
4.  build execution
5.  artifact discovery
6.  canonical artifact selection
7.  hashing
8.  comparison
9.  provenance collection
10. attestation generation

Capture:

-   repository
-   commit
-   builder ID
-   container/image digest
-   build command
-   environment metadata
-   dependency lock information
-   timestamps
-   artifact digest
-   toolchain version
-   OS/architecture

Do not inject secrets into build environments.

Build environments must default to network-restricted operation where
possible.

Allow explicit network access only when required by the package build
and record that fact in provenance.

------------------------------------------------------------------------

# 9. SIGNED ATTESTATIONS

Use established attestation formats and signing mechanisms.

Do not implement custom cryptographic algorithms.

The attestation must bind:

``` text
source commit
+
artifact digest
+
builder identity
+
build environment
+
build procedure
+
timestamp
```

Support verification of:

-   signature
-   signer identity
-   certificate chain where applicable
-   transparency evidence where applicable
-   subject digest
-   source commit
-   builder identity
-   predicate schema
-   policy constraints

Preserve the original signed evidence.

Never mutate a signed attestation and then claim it remains the original
signed record.

------------------------------------------------------------------------

# 10. QUORUM ENGINE

This is the core Quorum innovation.

Implement deterministic quorum evaluation.

Inputs:

-   release
-   artifact
-   evidence sources
-   builder results
-   attestation results
-   independence groups
-   policy

Outputs:

``` json
{
  "decision": "VERIFIED",
  "required": 2,
  "satisfied": 3,
  "conflicts": [],
  "counted_evidence": [],
  "excluded_evidence": [],
  "reasons": []
}
```

The evaluator must be deterministic.

Same input evidence + same policy = same decision.

Support policies such as:

### Policy A

``` text
2 of 3 independent builders agree
```

### Policy B

``` text
OSS Rebuild must verify
AND
at least 2 independent builders agree
```

### Policy C

``` text
3 independent builders required
AND
zero conflicts
```

### Policy D

``` text
2 independent builders agree
AND
all signatures valid
AND
source commit matches
```

Do not hard-code one policy.

------------------------------------------------------------------------

# 11. EVIDENCE WEIGHTING

Do not implement arbitrary "trust scores" that pretend to be objective.

Instead implement transparent policy predicates.

Examples:

-   signed attestation = required/optional
-   OSS Rebuild = required/optional
-   independent builder count \>= N
-   artifact digest agreement \>= N
-   source commit match = required
-   signer identity allowed = true/false
-   independence group uniqueness = required
-   evidence freshness \<= duration
-   conflict tolerance = 0/1/etc.

Every decision must expose the exact predicates that passed or failed.

------------------------------------------------------------------------

# 12. DISAGREEMENT HANDLING

This is a first-class feature.

Detect:

-   different artifact hashes
-   different source commits
-   invalid signatures
-   mismatched artifact metadata
-   builder failures
-   malformed attestations
-   stale evidence
-   duplicate builders
-   same independence group
-   inconsistent provenance
-   OSS Rebuild disagreement
-   policy conflicts

Example:

``` text
Builder A -> sha256:abc
Builder B -> sha256:abc
Builder C -> sha256:def

Result:
VERIFIED_WITH_CONFLICT

Reason:
2 independent builders agree on abc,
1 independent builder produced def.
```

Do not hide minority evidence.

Do not silently discard disagreement.

The dashboard and CLI must surface it.

------------------------------------------------------------------------

# 13. TRUST ASSESSMENT UX

Do not use vague labels like "100% safe".

Instead expose:

``` text
BUILD INTEGRITY
✓ Source pinned
✓ Artifact digest verified
✓ 3 valid attestations
✓ 2 independent builder groups agree

SUPPLY-CHAIN EVIDENCE
✓ OSS Rebuild evidence available
✓ Provenance validated
⚠ One builder disagreement

POLICY
Required: 2 independent builders
Satisfied: 2
Decision: VERIFIED_WITH_CONFLICT
```

Make it obvious that:

-   verification evidence exists,
-   the policy was satisfied,
-   unresolved conflicts exist,
-   security conclusions are bounded.

------------------------------------------------------------------------

# 14. TAMper-EVIDENT AUDIT SYSTEM

Implement an append-only audit log.

Every significant action creates an audit event.

At minimum:

-   release created
-   source resolved
-   artifact registered
-   OSS Rebuild checked
-   builder started
-   builder completed
-   artifact hashed
-   attestation created
-   attestation verified
-   quorum evaluated
-   decision made
-   policy changed
-   builder registered/disabled
-   evidence anchored
-   blockchain transaction confirmed

Each record must include:

``` text
previous_hash
payload_hash
record_hash
timestamp
```

Define:

``` text
record_hash = H(previous_hash || canonical_payload || metadata)
```

Use canonical serialization.

Write tests proving that modifying an old record causes chain
verification failure.

------------------------------------------------------------------------

# 15. BLOCKCHAIN LAYER

Build an actual Solidity smart contract.

The contract should provide a tamper-evident public anchor for
verification records.

Example conceptual API:

``` solidity
recordVerification(
    bytes32 verificationId,
    bytes32 evidenceHash,
    bytes32 artifactDigest,
    bytes32 sourceCommit,
    bytes32 policyHash,
    uint8 decision
)
```

Emit an event:

``` solidity
VerificationAnchored(
    verificationId,
    evidenceHash,
    artifactDigest,
    sourceCommit,
    policyHash,
    decision,
    timestamp
)
```

Contract requirements:

-   access control
-   event emission
-   duplicate-record protection
-   deterministic identifiers
-   chain ID recorded
-   contract address recorded
-   no sensitive data stored
-   no large artifacts stored
-   no arbitrary user-controlled executable code
-   safe integer handling
-   reentrancy analysis even if not strictly required
-   pause/emergency mechanism only if justified
-   upgradeability avoided unless genuinely needed

Default development chain:

``` text
Anvil
```

Optional deployment:

``` text
EVM-compatible testnet
```

Do not hard-code a real private key.

Never commit secrets.

------------------------------------------------------------------------

# 16. BLOCKCHAIN INDEXING

Backend must be able to:

1.  submit anchor
2.  wait for transaction
3.  confirm transaction
4.  retrieve receipt
5.  store tx hash
6.  store block number
7.  store chain ID
8.  store contract address
9.  verify emitted event
10. re-read the chain and independently verify the anchor

The UI must link an audit record to its on-chain transaction.

If blockchain is unavailable, the verification system should fail
according to explicit policy:

``` text
BLOCKCHAIN_REQUIRED=true
```

or:

``` text
BLOCKCHAIN_REQUIRED=false
```

Never silently claim a record was anchored when it was not.

------------------------------------------------------------------------

# 17. BACKEND API

Implement REST APIs with OpenAPI.

At minimum:

``` text
POST   /api/v1/releases
GET    /api/v1/releases
GET    /api/v1/releases/:id

POST   /api/v1/verifications
GET    /api/v1/verifications/:id
POST   /api/v1/verifications/:id/reverify

GET    /api/v1/evidence/:id
GET    /api/v1/attestations/:id

GET    /api/v1/builders
POST   /api/v1/builders
PATCH  /api/v1/builders/:id

GET    /api/v1/policies
POST   /api/v1/policies
POST   /api/v1/policies/validate

GET    /api/v1/audit
GET    /api/v1/audit/:id
POST   /api/v1/audit/:id/verify-chain

POST   /api/v1/blockchain/anchor
GET    /api/v1/blockchain/:verificationId

GET    /api/v1/health
GET    /api/v1/ready
```

All endpoints must have:

-   request validation
-   response schemas
-   error schemas
-   authentication/authorization where appropriate
-   rate limiting where exposed externally
-   structured logs
-   trace/request IDs
-   tests

------------------------------------------------------------------------

# 18. CLI

The CLI is the primary developer-facing product.

Implement at least:

``` bash
quorum init
quorum verify
quorum inspect
quorum builders list
quorum builders register
quorum policy validate
quorum policy test
quorum evidence export
quorum audit verify
quorum blockchain anchor
quorum doctor
quorum version
```

Core command:

``` bash
quorum verify \
  --repo <repo> \
  --commit <commit> \
  --artifact <artifact>
```

Also support package-oriented verification where feasible:

``` bash
quorum verify pypi:<package>@<version>
quorum verify npm:<package>@<version>
quorum verify cargo:<package>@<version>
```

Unsupported ecosystems must return a clear error.

Output modes:

``` bash
quorum verify ...             # human-readable
quorum verify ... --json      # machine-readable
quorum verify ... --quiet     # exit-code focused
quorum verify ... --verbose   # diagnostics
```

Exit codes must be documented and tested.

For example:

``` text
0 = verified
1 = rejected
2 = insufficient evidence
3 = conflict/investigation
4 = operational error
5 = invalid input
```

Choose final semantics deliberately and document them.

------------------------------------------------------------------------

# 19. CLI DOCTOR COMMAND

Implement:

``` bash
quorum doctor
```

It checks:

-   Docker
-   Git
-   OSS Rebuild availability/version
-   Cosign availability/version
-   Foundry/Anvil availability
-   API connectivity
-   database connectivity
-   object storage connectivity
-   blockchain RPC connectivity
-   configuration
-   required environment variables
-   filesystem permissions

Output actionable remediation instructions.

------------------------------------------------------------------------

# 20. WEB DASHBOARD

Build a professional security/audit dashboard.

Pages:

## Dashboard

Show:

-   total releases
-   verified releases
-   conflicts
-   rejected releases
-   investigations
-   builder status
-   recent verification events

## Release page

Show:

-   package
-   version
-   source repository
-   pinned commit
-   expected artifact
-   artifact digest
-   verification decision
-   policy
-   builder results
-   OSS Rebuild evidence
-   attestation status
-   provenance
-   conflicts
-   audit trail
-   blockchain anchor

## Builder page

Show:

-   builder identity
-   independence group
-   recent builds
-   agreement history
-   failures
-   attestation history

## Policy page

Show:

-   policy configuration
-   version
-   hash
-   examples
-   test cases

## Audit page

Show:

-   append-only records
-   hash chain
-   verification status
-   blockchain anchor
-   transaction hash
-   block number

## Evidence explorer

Allow users to inspect raw normalized evidence and original signed
evidence.

Never expose secrets.

------------------------------------------------------------------------

# 21. DATABASE

Use PostgreSQL.

Implement migrations.

Use explicit constraints.

Add indexes for:

-   release identifier
-   artifact digest
-   source commit
-   verification status
-   builder ID
-   timestamp
-   blockchain tx hash
-   evidence hash

Use transactions around multi-record state changes.

Test:

-   migrations
-   rollback strategy
-   uniqueness constraints
-   foreign keys
-   concurrent updates
-   idempotency

------------------------------------------------------------------------

# 22. OBJECT STORAGE

Store large evidence objects outside PostgreSQL.

Store:

-   raw attestations
-   evidence bundles
-   build logs
-   artifact metadata
-   exported audit packages

Use content-addressed naming where possible.

Example:

``` text
sha256/<digest>
```

Never trust user-provided paths.

Prevent path traversal.

Test malicious filenames.

------------------------------------------------------------------------

# 23. SECURITY REQUIREMENTS

Treat the entire system as security-sensitive.

Implement:

-   strict input validation
-   SSRF protection
-   URL allowlisting where network fetching is required
-   command injection protection
-   shell argument escaping
-   path traversal protection
-   archive extraction protections
-   symlink handling
-   resource limits
-   build timeout
-   disk quotas
-   memory limits
-   CPU limits
-   network isolation
-   secret redaction
-   secure temp directories
-   secure file permissions
-   dependency pinning
-   container image pinning
-   least privilege
-   non-root containers where feasible
-   no privileged Docker by default
-   audit logging
-   authentication for administrative APIs
-   authorization checks
-   rate limiting
-   CORS configuration
-   CSRF protection where applicable
-   secure headers
-   TLS support
-   safe error messages

Never execute arbitrary package code directly on the API server.

Builders must run in isolated worker environments.

------------------------------------------------------------------------

# 24. THREAT MODEL

Create a documented threat model covering:

### Attacker capabilities

-   compromised package maintainer
-   compromised build server
-   malicious artifact
-   malicious builder
-   malicious evidence provider
-   stolen signing key
-   forged attestation
-   replayed attestation
-   tampered database record
-   modified audit log
-   manipulated blockchain submission
-   compromised API client
-   malicious repository URL
-   malicious archive
-   malicious build script
-   dependency confusion
-   network MITM
-   malicious insider

### Security boundaries

Document:

-   CLI
-   API
-   workers
-   builders
-   external evidence providers
-   database
-   object store
-   blockchain
-   user browser

### Trust assumptions

Explicitly document every assumption.

Never claim decentralization merely because blockchain is present.

------------------------------------------------------------------------

# 25. ATTACK/NEGATIVE TEST FIXTURES

Create deterministic fixtures for:

1.  valid artifact
2.  artifact modified after build
3.  artifact modified before verification
4.  wrong SHA-256
5.  wrong source commit
6.  wrong repository
7.  invalid signature
8.  valid signature over wrong artifact
9.  expired certificate
10. wrong signer identity
11. malformed attestation
12. missing attestation
13. duplicate attestation
14. replayed attestation
15. builder disagreement
16. two builders from same independence group
17. insufficient quorum
18. quorum satisfied
19. OSS Rebuild match
20. OSS Rebuild mismatch
21. OSS Rebuild unavailable
22. unsupported package
23. builder timeout
24. builder crash
25. build output missing
26. nondeterministic build
27. malicious archive filename
28. path traversal archive
29. symlink attack
30. oversized artifact
31. malformed JSON
32. invalid policy
33. policy version mismatch
34. tampered audit record
35. blockchain transaction mismatch
36. wrong chain ID
37. wrong contract address
38. missing blockchain anchor
39. duplicate blockchain anchor
40. database failure
41. object store failure
42. API timeout
43. network failure
44. dependency unavailable

Each fixture must have a test.

------------------------------------------------------------------------

# 26. REQUIRED PS DEMONSTRATION

Create a fully scripted demo.

Use a real open-source package.

Prefer a package with strong support from Google OSS Rebuild.

The demo must include:

## Demo 1 --- valid release

``` text
source commit
     ↓
OSS Rebuild
     ↓
Builder A
     ↓
Builder B
     ↓
matching artifact
     ↓
quorum
     ↓
VERIFIED
```

## Demo 2 --- intentionally compromised artifact

Start from a valid artifact.

Make a controlled modification to the artifact in an isolated test
fixture.

Expected:

``` text
artifact digest mismatch
→ verification fails/rejects
```

Do not distribute malware or real malicious payloads.

The compromise fixture can simply alter bytes or append deterministic
harmless test data.

## Demo 3 --- inconsistent builder

Builder A:

``` text
SHA abc
```

Builder B:

``` text
SHA abc
```

Builder C:

``` text
SHA def
```

Expected:

``` text
VERIFIED_WITH_CONFLICT
```

or another policy-determined result.

## Demo 4 --- insufficient quorum

Example:

``` text
1 valid builder
2 required
```

Expected:

``` text
INSUFFICIENT_EVIDENCE
```

## Demo 5 --- tamper-evident audit

1.  Create verification.
2.  Anchor evidence.
3.  Modify local audit data.
4.  Run audit verification.
5.  Show failure.
6.  Compare with blockchain anchor.
7.  Show mismatch.

------------------------------------------------------------------------

# 27. END-TO-END DEMO SCRIPT

Create:

``` bash
make demo
```

It should:

1.  start infrastructure
2.  seed test data
3.  verify environment
4.  create valid release
5.  query/use OSS Rebuild where available
6.  run independent builders
7.  generate/verify attestations
8.  calculate quorum
9.  persist evidence
10. anchor verification
11. show dashboard URL
12. run compromised-artifact scenario
13. run builder-conflict scenario
14. show audit trail
15. verify blockchain anchors

The demo must be repeatable.

It must clean up after itself.

It must work from a fresh checkout following documented prerequisites.

------------------------------------------------------------------------

# 28. TESTING PHILOSOPHY

This is NON-NEGOTIABLE:

**Do not test only happy paths.**

Every module, function, API endpoint, CLI command, contract method,
parser, adapter, policy rule, persistence operation, and security
boundary must have tests.

The agent must maintain a requirement-to-test matrix.

Create:

``` text
docs/testing/TEST-MATRIX.md
```

Every requirement gets:

-   requirement ID
-   implementation location
-   unit tests
-   integration tests
-   E2E tests
-   negative tests
-   security tests where relevant
-   status

No requirement can be marked complete without evidence.

------------------------------------------------------------------------

# 29. TESTING PYRAMID

Implement:

## Unit tests

For:

-   domain models
-   hash functions
-   canonical serialization
-   quorum evaluation
-   policy evaluation
-   attestation parsing
-   signature verification
-   digest comparison
-   conflict detection
-   audit hashing
-   decision logic
-   exit codes
-   configuration parsing

## Integration tests

For:

-   PostgreSQL
-   object storage
-   API
-   workers
-   OSS Rebuild adapter
-   Sigstore/Cosign
-   builders
-   blockchain RPC
-   smart contracts

Use disposable test infrastructure.

## Contract tests

For:

-   deployment
-   permissions
-   event emission
-   duplicate protection
-   anchor storage
-   event reconstruction
-   unauthorized calls
-   malformed inputs
-   replay attempts

## E2E tests

Run the complete flow:

``` text
CLI
→ API
→ DB
→ OSS Rebuild
→ Builder
→ Attestation
→ Quorum
→ Audit
→ Blockchain
→ UI
```

## Browser tests

Use Playwright.

Test:

-   dashboard loading
-   release page
-   verification results
-   conflicts
-   audit history
-   blockchain link
-   filters
-   error states
-   accessibility basics
-   mobile/responsive behavior where relevant

------------------------------------------------------------------------

# 30. PROPERTY-BASED TESTING

Use property/fuzz testing for:

-   canonical JSON serialization
-   hash-chain integrity
-   quorum calculations
-   malformed attestations
-   policy parser
-   artifact metadata parsing
-   archive extraction
-   API input parsing

Important invariants:

### Determinism

Same evidence + same policy = same result.

### Monotonic evidence

Adding valid agreeing evidence must not make a previously satisfied
quorum fail unless policy explicitly says additional evidence can
invalidate the result.

### Conflict visibility

Conflicting evidence must never disappear silently.

### Hash integrity

Changing any canonical payload must change its hash except for
cryptographically negligible collisions.

### Independence

Two builders in the same independence group cannot satisfy a policy
requiring two independent groups.

### Signature integrity

Changing signed content must invalidate verification.

------------------------------------------------------------------------

# 31. MUTATION TESTING

Where practical, run mutation testing against:

-   quorum engine
-   policy engine
-   signature verification
-   audit chain
-   artifact comparison

The objective is not merely high line coverage.

The tests must actually detect deliberately introduced logic errors.

Document mutation testing limitations.

------------------------------------------------------------------------

# 32. COVERAGE REQUIREMENTS

Set strong coverage gates.

Target:

-   = 90% line coverage overall

-   = 95% for quorum/policy/security-critical core logic

-   meaningful branch coverage

-   100% coverage of critical decision branches where feasible

But do NOT treat coverage alone as proof of correctness.

A module is complete only when:

``` text
coverage
+
unit tests
+
negative tests
+
integration tests
+
security tests
+
E2E tests where applicable
```

are all satisfied.

------------------------------------------------------------------------

# 33. CHAOS/FAILURE TESTING

Test controlled failures:

-   database unavailable
-   database restart
-   object store unavailable
-   blockchain unavailable
-   builder unavailable
-   builder timeout
-   OSS Rebuild unavailable
-   invalid external response
-   network timeout
-   worker crash
-   duplicate job
-   API restart
-   transaction dropped
-   transaction reverted
-   corrupted evidence
-   corrupted local audit log

Expected behavior must be deterministic and documented.

No silent success.

------------------------------------------------------------------------

# 34. IDEMPOTENCY

All operations that can be retried must be idempotent where possible.

Examples:

``` text
verify release
register builder
submit evidence
anchor verification
create audit event
```

Test repeated requests.

Test concurrent requests.

Test retry after partial failure.

------------------------------------------------------------------------

# 35. OBSERVABILITY

Implement structured logging.

Every verification request must have:

``` text
request_id
verification_id
release_id
builder_id
```

Logs must never contain:

-   private keys
-   credentials
-   tokens
-   secrets

Add metrics for:

-   verification count
-   verification latency
-   builder failures
-   quorum conflicts
-   OSS Rebuild failures
-   blockchain anchor latency
-   API errors
-   queue depth
-   artifact sizes

Health endpoints:

``` text
/health
/ready
```

------------------------------------------------------------------------

# 36. ERROR MODEL

Define typed errors.

At minimum:

``` text
INVALID_INPUT
SOURCE_NOT_FOUND
COMMIT_NOT_FOUND
ARTIFACT_NOT_FOUND
ARTIFACT_HASH_MISMATCH
ATTESTATION_INVALID
SIGNATURE_INVALID
SIGNER_NOT_TRUSTED
BUILDER_FAILED
BUILDER_TIMEOUT
BUILDER_CONFLICT
OSS_REBUILD_UNAVAILABLE
OSS_REBUILD_UNSUPPORTED
INSUFFICIENT_EVIDENCE
POLICY_VIOLATION
AUDIT_TAMPERED
BLOCKCHAIN_UNAVAILABLE
BLOCKCHAIN_REVERTED
INTERNAL_ERROR
```

Errors must be machine-readable and human-readable.

------------------------------------------------------------------------

# 37. API SECURITY TESTING

Test:

-   authentication bypass
-   authorization bypass
-   IDOR
-   malformed JSON
-   oversized payloads
-   injection
-   SSRF
-   path traversal
-   command injection
-   replay
-   race conditions
-   rate limits
-   CORS
-   security headers

Do not use destructive real-world targets.

All security testing must remain inside the controlled development
environment.

------------------------------------------------------------------------

# 38. SUPPLY-CHAIN SECURITY OF QUORUM ITSELF

Quorum is itself software.

Therefore secure its own supply chain.

Implement:

-   pinned dependencies
-   dependency lockfiles
-   dependency vulnerability scanning
-   SBOM generation
-   signed releases
-   provenance generation
-   container image signing where applicable
-   reproducible build documentation where feasible
-   CI provenance
-   secret scanning
-   static analysis
-   container scanning

Add CI checks for:

``` text
go test
go vet
static analysis
dependency audit
frontend lint
frontend typecheck
contract tests
security scans
E2E
```

------------------------------------------------------------------------

# 39. CI/CD

Create CI workflows:

``` text
ci.yml
security.yml
contracts.yml
e2e.yml
release.yml
```

PR CI must run:

1.  formatting checks
2.  lint
3.  type checks
4.  unit tests
5.  integration tests
6.  contract tests
7.  frontend tests
8.  E2E tests
9.  security checks
10. coverage checks

Release CI must:

1.  build CLI
2.  build containers
3.  generate checksums
4.  generate SBOM
5.  generate provenance
6.  sign artifacts
7.  publish artifacts
8.  publish release metadata

------------------------------------------------------------------------

# 40. CLI EXIT-CODE TEST MATRIX

Every command must have explicit exit-code tests.

For example:

``` text
valid verification
invalid input
insufficient evidence
conflict
rejection
operational error
```

Do not let exit codes depend accidentally on internal errors.

------------------------------------------------------------------------

# 41. FRONTEND TEST MATRIX

Test every major UI state:

-   loading
-   success
-   empty
-   conflict
-   rejected
-   insufficient evidence
-   API failure
-   blockchain pending
-   blockchain confirmed
-   blockchain failed
-   malformed data
-   unknown verification
-   inaccessible page
-   mobile viewport
-   keyboard navigation
-   screen reader labels for important controls

------------------------------------------------------------------------

# 42. DATABASE TEST MATRIX

Test:

-   migrations from empty DB
-   migrations from seeded DB
-   rollback where supported
-   unique constraints
-   foreign keys
-   indexes
-   concurrent writes
-   transaction rollback
-   partial failures
-   duplicate insertion
-   deletion restrictions
-   retention behavior

------------------------------------------------------------------------

# 43. SMART CONTRACT TEST MATRIX

Test every public/external method.

Include:

-   happy path
-   unauthorized caller
-   duplicate ID
-   zero values
-   max values
-   malformed values
-   event correctness
-   state correctness
-   replay
-   gas behavior
-   chain configuration
-   deployment verification

Run fuzz tests on contract inputs.

Run static analysis if available.

------------------------------------------------------------------------

# 44. REPRODUCIBILITY TESTS

Build the same fixture multiple times.

Compare:

-   artifact digest
-   normalized metadata
-   provenance
-   environment
-   build logs

Explicitly classify differences:

``` text
REPRODUCIBLE
NON_REPRODUCIBLE
EXPECTED_NONDETERMINISM
BUILD_FAILURE
UNKNOWN
```

Do not label all byte differences as malicious.

------------------------------------------------------------------------

# 45. SECURITY TEST: COMPROMISED ARTIFACT

Create a controlled test:

``` text
valid artifact
    ↓
modify one byte
    ↓
recalculate local digest
    ↓
attempt verification
```

Expected:

``` text
FAIL
ARTIFACT_HASH_MISMATCH
```

The test must prove the compromised artifact cannot silently pass.

------------------------------------------------------------------------

# 46. SECURITY TEST: COMPROMISED BUILDER

Create a fake builder fixture that returns:

``` text
source = expected
artifact = malicious fixture
attestation = correctly signed by test builder
```

Expected:

Quorum must not simply say:

``` text
signature valid → accepted
```

It must evaluate the artifact disagreement against other evidence and
policy.

------------------------------------------------------------------------

# 47. SECURITY TEST: REPLAY

Take an old valid attestation.

Attempt to reuse it for:

-   another artifact
-   another source commit
-   another release
-   another version

Expected:

``` text
REJECTED
```

------------------------------------------------------------------------

# 48. SECURITY TEST: AUDIT TAMPERING

Take:

``` text
record 1
record 2
record 3
```

Modify record 2.

Run:

``` bash
quorum audit verify
```

Expected:

``` text
AUDIT CHAIN INVALID

Broken at record:
record 2

Expected previous hash:
...

Observed:
...
```

Then compare the final evidence hash to the blockchain anchor.

------------------------------------------------------------------------

# 49. SECURITY TEST: BLOCKCHAIN TAMPERING

Test:

-   wrong transaction hash
-   wrong contract
-   wrong chain
-   altered evidence hash
-   missing event
-   fake local transaction status

The backend must verify on-chain state rather than trusting
client-provided metadata.

------------------------------------------------------------------------

# 50. DOCUMENTATION

Create high-quality docs:

``` text
docs/
├── architecture.md
├── threat-model.md
├── security-model.md
├── trust-model.md
├── quorum-policies.md
├── oss-rebuild.md
├── builders.md
├── attestations.md
├── provenance.md
├── blockchain.md
├── audit.md
├── api.md
├── cli.md
├── testing.md
├── demo.md
└── limitations.md
```

README must include:

-   problem
-   architecture
-   quickstart
-   installation
-   CLI examples
-   screenshots
-   demo instructions
-   security model
-   limitations
-   test instructions

------------------------------------------------------------------------

# 51. LIMITATIONS DOCUMENT

Be explicit about:

-   builder independence assumptions
-   source-code maliciousness
-   dependency compromise
-   reproducibility limitations
-   nondeterministic builds
-   external service availability
-   OSS Rebuild ecosystem coverage
-   blockchain trust assumptions
-   key/certificate trust
-   hardware/host compromise
-   network assumptions

Never claim more security than the architecture provides.

------------------------------------------------------------------------

# 52. DEVELOPER EXPERIENCE

A new developer should be able to run:

``` bash
git clone ...
cd quorum
make setup
make test
make demo
```

and get a working system.

Provide:

``` bash
make help
make setup
make dev
make test
make test-unit
make test-integration
make test-e2e
make test-contracts
make test-security
make test-all
make lint
make format
make coverage
make demo
make clean
```

------------------------------------------------------------------------

# 53. TEST-ALL GATE

Create one canonical command:

``` bash
make test-all
```

It must execute:

1.  formatting verification
2.  lint
3.  unit tests
4.  integration tests
5.  property/fuzz tests
6.  mutation tests where configured
7.  contract tests
8.  frontend tests
9.  API tests
10. E2E tests
11. security tests
12. dependency audit
13. container scan
14. coverage checks
15. demo smoke test

The command must exit non-zero on failure.

------------------------------------------------------------------------

# 54. MASTER AGENT DEVELOPMENT LOOP

You are an autonomous implementation agent.

Follow this loop continuously:

``` text
READ
 ↓
PLAN
 ↓
IMPLEMENT
 ↓
FORMAT
 ↓
UNIT TEST
 ↓
INTEGRATION TEST
 ↓
SECURITY TEST
 ↓
E2E TEST
 ↓
INSPECT OUTPUT
 ↓
FIX
 ↓
RE-RUN
 ↓
REVIEW REQUIREMENTS
 ↓
REPEAT
```

Never stop after compilation succeeds.

Never stop after unit tests pass.

Never assume an integration works because a mock works.

Never create a TODO and move on when the feature is required.

If a dependency/API is uncertain:

1.  inspect official documentation,
2.  inspect installed version,
3.  create a minimal experiment,
4.  write a regression test,
5.  integrate it.

------------------------------------------------------------------------

# 55. AUTONOMOUS DEBUGGING LOOP

When a test fails:

1.  reproduce the failure
2.  isolate the smallest failing component
3.  inspect logs
4.  inspect inputs
5.  determine root cause
6.  fix the implementation
7.  add a regression test
8.  rerun the smallest relevant test
9.  rerun the entire affected suite
10. rerun `make test-all`

Do not simply weaken a test to make it pass.

Do not delete a failing test.

Do not change acceptance criteria because implementation is difficult.

If a requirement is genuinely impossible because of an external
dependency, document the exact limitation and provide a deterministic
adapter/mock while preserving the real integration path.

------------------------------------------------------------------------

# 56. REQUIREMENT TRACEABILITY

Create:

``` text
docs/testing/REQUIREMENT-MATRIX.md
```

Map:

``` text
REQ-001 → architecture → implementation → tests → demo
REQ-002 → ...
```

No requirement may be marked complete without tests.

No test may be considered sufficient merely because it executes code.

Include:

-   expected behavior
-   failure behavior
-   security implication
-   evidence of testing

------------------------------------------------------------------------

# 57. DEFINITION OF DONE

The project is NOT DONE unless all of the following are true:

### Product

-   [ ] CLI works
-   [ ] backend works
-   [ ] frontend works
-   [ ] builders work
-   [ ] OSS Rebuild integration works
-   [ ] attestations work
-   [ ] provenance works
-   [ ] quorum works
-   [ ] disagreement handling works
-   [ ] audit history works
-   [ ] blockchain anchoring works
-   [ ] real OSS package demo works

### Security

-   [ ] threat model documented
-   [ ] security controls implemented
-   [ ] malicious fixture tests pass
-   [ ] replay tests pass
-   [ ] signature tests pass
-   [ ] tamper tests pass
-   [ ] archive/path tests pass
-   [ ] SSRF/command injection protections tested

### Testing

-   [ ] unit tests
-   [ ] integration tests
-   [ ] E2E tests
-   [ ] contract tests
-   [ ] browser tests
-   [ ] property/fuzz tests
-   [ ] failure tests
-   [ ] security tests
-   [ ] coverage gate
-   [ ] requirement matrix complete
-   [ ] test-all passes

### Demo

-   [ ] valid release
-   [ ] compromised artifact
-   [ ] conflicting builder
-   [ ] insufficient quorum
-   [ ] audit tampering
-   [ ] blockchain anchor
-   [ ] dashboard evidence

### Documentation

-   [ ] README
-   [ ] architecture
-   [ ] threat model
-   [ ] trust model
-   [ ] CLI
-   [ ] API
-   [ ] policy
-   [ ] builders
-   [ ] OSS Rebuild
-   [ ] blockchain
-   [ ] testing
-   [ ] limitations

------------------------------------------------------------------------

# 58. FINAL QUALITY GATE

Before claiming completion, perform a final independent review as if you
were a hostile external evaluator.

Ask:

1.  Can a compromised artifact pass unnoticed?
2.  Can one malicious builder override honest builders?
3.  Can two builders from the same infrastructure fake independence?
4.  Can an old attestation be replayed?
5.  Can an invalid signature pass?
6.  Can the audit log be modified without detection?
7.  Can blockchain metadata be forged locally?
8.  Can a malicious repository URL cause SSRF?
9.  Can a build execute outside its sandbox?
10. Can archive extraction escape its directory?
11. Can secrets appear in logs?
12. Can an API user access another user's evidence?
13. Can a duplicate verification corrupt state?
14. Can concurrent verification produce inconsistent quorum results?
15. Can OSS Rebuild failure be confused with a valid verification?
16. Can the UI accidentally display an unverified release as verified?
17. Does the CLI exit code accurately communicate the decision?
18. Can the system explain exactly why a release was accepted/rejected?
19. Is every PS requirement demonstrated?
20. Can a fresh developer reproduce the demo from scratch?

For every "yes, this is possible," add a mitigation or explicitly
document the limitation.

------------------------------------------------------------------------

# 59. FINAL REPORT

At the end of implementation, produce:

``` text
docs/FINAL-REPORT.md
```

Include:

-   architecture
-   implemented modules
-   technology choices
-   security model
-   threat model
-   test statistics
-   coverage
-   fuzz/property results
-   mutation testing results
-   E2E results
-   contract testing results
-   demo results
-   known limitations
-   future work
-   exact commands used to validate the system

Also generate a concise final status:

``` text
QUORUM IMPLEMENTATION STATUS
=============================

Core platform:          PASS
CLI:                    PASS
Backend:                PASS
Frontend:               PASS
OSS Rebuild:            PASS
Builders:               PASS
Attestations:           PASS
Quorum engine:          PASS
Audit chain:            PASS
Blockchain:             PASS
Security suite:         PASS
E2E suite:              PASS
PS demonstration:       PASS

All required acceptance criteria: PASS
```

Do not print `PASS` for anything that has not actually been tested.

------------------------------------------------------------------------

# 60. CRITICAL ENGINEERING RULES

1.  Never invent external APIs.
2.  Verify current external tool behavior before integrating it.
3.  Never invent cryptography.
4.  Never store private keys in source control.
5.  Never trust client-provided verification results.
6.  Never trust a signature without checking what was signed.
7.  Never count duplicate evidence twice.
8.  Never count builders from the same independence group as
    independent.
9.  Never hide disagreement.
10. Never silently convert operational failure into security success.
11. Never call an artifact "safe" merely because it was reproduced.
12. Never claim blockchain makes the system magically decentralized.
13. Never execute untrusted build code on the API server.
14. Never skip tests because a component is "simple."
15. Never remove a failing test just to achieve green CI.
16. Never weaken security checks to make the demo pass.
17. Prefer deterministic fixtures over flaky external dependencies in
    automated tests.
18. Keep real external integrations tested separately from deterministic
    fixture tests.
19. Preserve raw evidence for forensic inspection.
20. Every security-sensitive decision must be explainable.

------------------------------------------------------------------------

# 61. IMPLEMENTATION ORDER

Follow this order unless dependency constraints require a small
deviation:

### Phase 1

Repository + architecture + schemas + local infrastructure

### Phase 2

Core domain models + hashing + canonicalization

### Phase 3

Quorum/policy engine

### Phase 4

Attestation/provenance verification

### Phase 5

OSS Rebuild adapter

### Phase 6

Independent builders

### Phase 7

Backend/API/database/object storage

### Phase 8

Audit chain

### Phase 9

Smart contract + blockchain integration

### Phase 10

CLI

### Phase 11

Frontend

### Phase 12

End-to-end integration

### Phase 13

Security hardening

### Phase 14

Full test suite

### Phase 15

Real OSS package demonstration

### Phase 16

Documentation + final audit

After EACH phase:

``` text
implement
→ test
→ inspect
→ fix
→ test again
```

Do not accumulate untested work across phases.

------------------------------------------------------------------------

# 62. START NOW

Begin by:

1.  inspecting the environment,
2.  checking available tools,
3.  checking current official documentation for Google OSS Rebuild,
    Sigstore/Cosign, in-toto, SLSA, Foundry, and relevant framework
    versions,
4.  creating the monorepo,
5.  creating architecture and requirement documents,
6.  implementing the foundational schemas,
7.  creating the test harness,
8.  implementing the smallest vertical slice,
9.  running tests,
10. iterating until green,
11. expanding feature-by-feature,
12. continuously maintaining the requirement/test matrix.

At every stage, prioritize:

**correctness → security → testability → explainability → developer
experience → visual polish**

The finished product must be a genuinely runnable Quorum platform, not a
simulated presentation.

The final acceptance criterion is:

> A user can provide a real open-source release, Quorum can collect and
> verify independent build evidence, incorporate Google OSS Rebuild
> evidence, evaluate a configurable quorum policy, detect conflicting or
> compromised artifacts, generate an auditable evidence record, anchor
> that record on an EVM-compatible blockchain, and clearly explain the
> resulting release decision through both the CLI and web interface ---
> with automated tests proving the behavior and failure modes.
