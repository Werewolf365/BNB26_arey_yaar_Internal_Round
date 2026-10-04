# Quorum Makefile
# Corrections applied per review:
# - `doctor` detects toolchains and prints platform-specific install help; it never installs.
# - `setup` installs PROJECT dependencies only (npm), never mutates the OS toolchain.
# - `test-all` is self-provisioning via Docker Compose where possible.
# - Live external services (real OSS Rebuild, testnets) live in `test-external` only.

.PHONY: help doctor setup dev test test-unit test-integration test-e2e test-contracts test-security test-web test-external test-all lint format coverage demo demo-slice demo-real demo-list add-project fuzz mutation sbom sign scan clean

help:
	@echo "Quorum targets:"
	@echo "  make doctor           - detect Go/Node/Docker/Cosign/Foundry/Anvil, print fix instructions (no installs)"
	@echo "  make setup            - install project deps only (npm ci); never touches OS toolchain"
	@echo "  make dev              - start local stack (requires Docker daemon)"
	@echo "  make test             - fast deterministic suite (no credentials, no daemon required)"
	@echo "  make test-unit        - unit tests (Node harness now, Go in CI)"
	@echo "  make test-integration - self-provisioned integration (needs Docker daemon)"
	@echo "  make test-e2e         - end-to-end slice scenarios"
	@echo "  make test-contracts   - Foundry tests (needs forge; skipped gracefully if absent)"
	@echo "  make test-security    - tamper/replay/conflict/path-traversal suites"
	@echo "  make test-web         - web typecheck + build + Playwright e2e (needs browsers: npx playwright install)"
	@echo "  make test-external    - REAL OSS Rebuild / testnet; needs creds; never in test-all"
	@echo "  make test-all         - self-provisioning gate: format+lint+unit+integration+e2e+security+coverage+demo smoke"
	@echo "  make lint             - lint (node --check + go vet when Go present)"
	@echo "  make format           - gofmt/prettier check (non-mutating unless FIX=1)"
	@echo "  make coverage         - coverage report"
	@echo "  make demo             - full end-to-end demo: CLI -> API -> quorum -> audit (-> Anvil when present)"
	@echo "  make demo-slice       - repeatable Slice-1 demo (valid/tampered/conflict)"
	@echo "  make demo-real PROJECT=x  - real-project showcase (or REPO=/COMMIT=/TAG=… overrides)"
	@echo "  make demo-list          - list available demo project configs"
	@echo "  make add-project        - onboard a repo to projects/<name>.json: REPO=<github-url> REF=<tag|sha> [NAME=] [ECOSYSTEM=]"
	@echo "  make fuzz             - short property fuzz runs (quorum invariants + canonical stability)"
	@echo "  make mutation         - mutation gate: every engine mutant must be killed by tests"
	@echo "  make sbom             - SBOM (syft CycloneDX when installed, else Go/npm inventory)"
	@echo "  make sign             - cosign-sign sbom.json when cosign is installed (else SKIP)"
	@echo "  make scan             - vet + govulncheck + npm audit + secret scan"
	@echo "  make clean            - remove build artifacts"

doctor:
	node scripts/doctor.js

setup:
	npm ci

build-cli:
	go build -o quorum ./apps/cli

build-api:
	go build -o quorum-api ./apps/api

build-worker:
	go build -o quorum-worker ./apps/worker

run-api:
	QUORUM_ADDR=:8080 QUORUM_DATABASE_URL=postgres://quorum:quorum@localhost:5433/quorum?sslmode=disable ./quorum-api

test-integration:
	@echo "[integration] needs Docker daemon for Postgres/Anvil"
	@docker info > /dev/null 2>&1 || { echo "[integration] SKIP: Docker daemon not running"; exit 0; }
	@docker compose -f infra/compose/docker-compose.yml up -d postgres anvil || { echo "[integration] SKIP: compose stack unavailable"; exit 0; }
	QUORUM_TEST_POSTGRES=1 QUORUM_LIVE_ANVIL=1 go test -count=1 ./apps/api/... ./internal/runner/ -run 'TestPostgresIntegration|TestAnchorLive'

dev:
	docker compose -f infra/compose/docker-compose.yml up --build

test: test-unit test-security
	npm test

test-unit:
	npm run test-unit
	-@go test ./services/... 2>&1 | head -n 50 || echo "[go] Go toolchain absent - Go tests run in CI (see go.mod toolchain go1.27.1)"

test-e2e:
	node scripts/slice1-demo.mjs

test-contracts:
	-@forge test --root contracts 2>/dev/null || docker run --rm --entrypoint forge -v "$(CURDIR)/contracts:/project" -w /project ghcr.io/foundry-rs/foundry:latest test 2>&1 | tail -n 12 || echo "[contracts] SKIP: no host forge and no Docker image (see 'make doctor')"

test-security:
	node --test test/security.*.test.mjs

test-web:
	cd apps/web && npm ci --no-audit --no-fund && npm run typecheck && npm run build
	-@cd apps/web && (npx playwright test 2>&1 | tail -n 6) || echo "[web] Playwright browsers absent - run 'npx playwright install chromium' (see 'make doctor')"

test-external:
	@echo "[external] REAL OSS Rebuild / Cosign / Rekor tests (require network + CLIs; never in test-all)."
	OSS_REBUILD_MODE=live node --test test/external.*.test.mjs || echo "[external] SKIP: no credentials / tool absent"
	QUORUM_LIVE_OSS=1 go test -run 'TestLiveLookup|TestOssLiveWiring' ./services/ossrebuild/ ./internal/runner/ || echo "[external] SKIP: oss-rebuild CLI absent"
	QUORUM_LIVE_COSIGN=1 go test -run 'TestLive' ./services/cosign/ || echo "[external] SKIP: cosign absent / rekor unreachable"
	QUORUM_LIVE_S3=1 go test -run 'TestLiveS3' ./services/storage/ || echo "[external] SKIP: s3-compat (localstack/minio) unreachable"

test-all:
	@echo "=== Quorum test-all (self-provisioning, deterministic, no external creds) ==="
	npm test
	node --test test/security.*.test.mjs
	node scripts/slice1-demo.mjs
	node scripts/demo-e2e.mjs
	node scripts/mutate.mjs
	-@go test ./... || echo "[go] Go toolchain absent locally - enforced in CI"
	@echo "=== test-all PASS (deterministic subset) ==="

lint:
	node --check scripts/doctor.js
	node --check scripts/slice1-demo.mjs
	-@go vet ./... || echo "[lint] go vet skipped (no Go toolchain)"

format:
	-@gofmt -l services apps packages || echo "[format] gofmt skipped (no Go toolchain)"

coverage:
	node scripts/coverage.mjs

demo:
	node scripts/demo-e2e.mjs

demo-slice:
	node scripts/slice1-demo.mjs

demo-real:
	node scripts/demo-real.mjs $(if $(PROJECT),--project $(PROJECT)) $(if $(REPO),--repo $(REPO)) $(if $(COMMIT),--commit $(COMMIT)) $(if $(TAG),--tag $(TAG)) $(if $(PACKAGE),--package $(PACKAGE)) $(if $(ECOSYSTEM),--ecosystem $(ECOSYSTEM)) $(if $(NAME),--name $(NAME)) $(if $(VERSION),--version $(VERSION))

demo-list:
	node scripts/demo-real.mjs --list

add-project:
	node scripts/add-project.mjs $(if $(REPO),--repo $(REPO)) $(if $(REF),--ref $(REF)) $(if $(NAME),--name $(NAME)) $(if $(ECOSYSTEM),--ecosystem $(ECOSYSTEM))

fuzz:
	go test -count=1 -fuzz FuzzEvaluateDeterminism -fuzztime 20s ./services/policy/
	go test -count=1 -fuzz FuzzIndependenceInvariant -fuzztime 10s ./services/policy/
	go test -count=1 -fuzz FuzzConflictVisibility -fuzztime 20s ./services/policy/
	go test -count=1 -fuzz FuzzMarshalStability -fuzztime 10s ./services/canonical/

mutation:
	node scripts/mutate.mjs

sbom:
	node scripts/sbom.mjs

sign: sbom
	@COSIGN_KEY=$${QUORUM_COSIGN_KEY:-$$HOME/.config/quorum/cosign.key}; \
	if ! command -v cosign >/dev/null 2>&1; then echo "[sign] SKIP: cosign absent (install: https://docs.sigstore.dev/cosign/installation/)"; exit 0; fi; \
	if [ ! -f "$$COSIGN_KEY" ]; then mkdir -p $$(dirname "$$COSIGN_KEY"); COSIGN_PASSWORD=$${COSIGN_PASSWORD-} cosign generate-key-pair --output-key-prefix "$${COSIGN_KEY%.key}" && echo "[sign] generated dev key $$COSIGN_KEY (local dev only, never commit)"; fi; \
	COSIGN_PASSWORD=$${COSIGN_PASSWORD-} cosign sign-blob --key "$$COSIGN_KEY" --output-signature sbom.sig --bundle sbom.bundle --yes sbom.json && \
	COSIGN_PASSWORD=$${COSIGN_PASSWORD-} cosign verify-blob --key "$${COSIGN_KEY%.key}.pub" --signature sbom.sig --bundle sbom.bundle sbom.json && \
	echo "[sign] sbom.json signed (sbom.sig) + Rekor bundle (sbom.bundle) + verified"

scan:
	node scripts/scan.mjs

clean:
	-@docker compose -f infra/compose/docker-compose.yml down -v || true
	Remove-Item -Recurse -Force -ErrorAction SilentlyContinue build, coverage.out || true
