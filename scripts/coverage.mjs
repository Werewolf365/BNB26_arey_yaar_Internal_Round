// quorum coverage gate — per-package profiles, weighted total, thresholds.
//
// Usage:
//   node scripts/coverage.mjs                 # offline profile (no creds, no daemon)
//   node scripts/coverage.mjs --ci            # CI profile (postgres service up)
//
// Offline reality: postgres.go, docker-gated rebuild paths, and live
// network paths are covered by their own suites (QUORUM_TEST_POSTGRES=1
// integration job, TestLive* env-gated) — not by this run. Those files are
// reported separately, never silently counted as covered.
//
// Thresholds encode REQ-016: 90% overall / 95% critical. The offline profile
// enforces locked floors (measured, committed below) so coverage can never
// regress unnoticed; the CI profile enforces the full 90/95 where the
// postgres service closes the store gap.
import { execSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const ALL = [
  "services/canonical",
  "services/cosign",
  "services/ossrebuild",
  "services/policy",
  "services/sigstore",
  "services/storage",
  "internal/runner",
  "apps/api/internal/server",
  "apps/api/internal/store",
  "apps/cli/cmd",
  "apps/worker/internal/rebuild",
];
// Integration-gated: 0% offline by design (own suites in CI/live).
const GATED = new Set([
  "apps/api/internal/store", // postgres.go needs QUORUM_TEST_POSTGRES=1
  "apps/worker/internal/rebuild", // docker paths need a daemon
]);
// Security-critical core (prompt section 32): 95% in CI.
const CRITICAL = new Set([
  "services/policy",
  "services/sigstore",
  "services/cosign",
  "internal/runner",
  "apps/api/internal/server",
]);

const OFFLINE_FLOORS = {
  total: 80,
  "services/canonical": 85,
  "services/cosign": 88,
  "services/ossrebuild": 85,
  "services/policy": 95,
  "services/sigstore": 89,
  "services/storage": 80,
  "internal/runner": 89,
  "apps/api/internal/server": 87,
  "apps/cli/cmd": 85,
};

function run(cmd, opts = {}) {
  return execSync(cmd, { stdio: "pipe", encoding: "utf8", ...opts });
}

function profileOf(pkg, dir) {
  const out = join(dir, pkg.replaceAll("/", "_") + ".out");
  try {
    run(`go test -count=1 -coverprofile="${out}" ./${pkg}/`);
  } catch (e) {
    console.error(`FAIL  tests failed in ${pkg}\n${e.stdout ?? ""}${e.stderr ?? ""}`);
    process.exit(1);
  }
  let stmts = 0;
  let covered = 0;
  for (const line of readFileSync(out, "utf8").split("\n")) {
    const m = line.match(/^\S+:(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)$/);
    if (!m) continue;
    stmts += parseInt(m[3], 10);
    if (parseInt(m[4], 10) > 0) covered += parseInt(m[3], 10);
  }
  return { stmts, covered, pct: stmts === 0 ? 100 : (100 * covered) / stmts };
}

const ci = process.argv.includes("--ci");
const dir = mkdtempSync(join(tmpdir(), "quorum-cov-"));
const results = {};
for (const pkg of ALL) results[pkg] = profileOf(pkg, dir);

let totalS = 0;
let totalC = 0;
let unitS = 0;
let unitC = 0;
console.log("package coverage (offline run, no creds/daemon):");
for (const pkg of ALL) {
  const r = results[pkg];
  totalS += r.stmts;
  totalC += r.covered;
  if (!GATED.has(pkg)) {
    unitS += r.stmts;
    unitC += r.covered;
  }
  const tag = GATED.has(pkg) ? " [integration-gated]" : CRITICAL.has(pkg) ? " [critical]" : "";
  console.log(`  ${r.pct.toFixed(1).padStart(5)}%  ${pkg}${tag}`);
}
const total = (100 * totalC) / totalS;
const unit = (100 * unitC) / unitS;
console.log(`  ${total.toFixed(1).padStart(5)}%  TOTAL (weighted, incl. gated)`);
console.log(`  ${unit.toFixed(1).padStart(5)}%  UNIT SCOPE (gated files excluded; covered by integration/live suites)`);

rmSync(dir, { recursive: true, force: true });

let failures = 0;
const check = (name, got, want) => {
  if (got + 1e-9 < want) {
    console.error(`GATE  ${name}: ${got.toFixed(1)}% < ${want}%`);
    failures++;
  }
};
if (ci) {
  check("total", total, 90);
  check("unit scope", unit, 90);
  for (const pkg of CRITICAL) check(pkg, results[pkg].pct, 95);
  console.log("CI profile: total>=90, unit>=90, critical>=95.");
} else {
  check("total", total, OFFLINE_FLOORS.total);
  check("unit scope", unit, 90);
  for (const [pkg, floor] of Object.entries(OFFLINE_FLOORS)) {
    if (pkg === "total") continue;
    check(pkg, results[pkg].pct, floor);
  }
  console.log("Offline profile: unit>=90 plus locked floors (integration-gated files excluded by design).");
}
process.exit(failures === 0 ? 0 : 1);
