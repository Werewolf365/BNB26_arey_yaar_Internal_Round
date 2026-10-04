// Supply-chain scans for Quorum itself: static analysis, vuln audit,
// dependency audit, secret scan. Every check prints PASS/SKIP/FAIL;
// missing optional tools SKIP (never fake a pass). Exit 1 on any FAIL.
import { execSync } from "node:child_process";
import { existsSync as _exists } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

// Resolve a tool binary deterministically: bare PATH first, then the usual
// install locations (go tools land in ~/go/bin, which login shells often
// lack). Returns "" when genuinely absent — callers SKIP, never fake a PASS.
function resolveBin(bin) {
  for (const dir of (process.env.PATH || "").split(":").filter(Boolean)) {
    if (_exists(join(dir, bin))) return join(dir, bin);
  }
  for (const dir of [join(homedir(), "go", "bin"), "/usr/local/go/bin", join(homedir(), ".local", "bin")]) {
    if (_exists(join(dir, bin))) return join(dir, bin);
  }
  return "";
}

let failures = 0;
const run = (label, cmd, opts = {}) => {
  try {
    execSync(cmd, { stdio: "pipe", encoding: "utf8", timeout: 300000, ...opts });
    console.log(`PASS  ${label}`);
  } catch (e) {
    const out = String(e?.stdout || e?.message || e);
    if (/not found|ENOENT|command not found|no such file/i.test(out.slice(0, 300))) {
      console.log(`SKIP  ${label} (tool absent)`);
    } else {
      failures++;
      console.log(`FAIL  ${label}\n${out.slice(0, 2000)}`);
    }
  }
};
const have = (bin) => {
  try { execSync(`${bin} --version`, { stdio: "pipe", timeout: 15000 }); return true; }
  catch { return false; }
};

// 1. static analysis (always available with the Go toolchain)
run("go vet ./...", "go vet ./...");
// 2. Go vulnerability audit (install on demand; pin GOTOOLCHAIN to the
// repo's go.mod toolchain so the scanner parses go1.27 sources).
// govulncheck matters here: it audits our Go module graph against the Go
// vulnerability database — the one scanner that understands Go stdlib and
// module semantics. We HAVE it once installed; resolveBin finds it even
// when ~/go/bin is not on PATH.
let govulncheck = resolveBin("govulncheck");
if (!govulncheck) {
  try {
    console.log("scan: installing govulncheck ...");
    execSync("GOTOOLCHAIN=go1.27.1 go install golang.org/x/vuln/cmd/govulncheck@latest", { stdio: "pipe", timeout: 300000 });
  } catch { /* fall through to SKIP */ }
  govulncheck = resolveBin("govulncheck");
}
if (govulncheck) run("govulncheck ./...", `"${govulncheck}" ./...`);
else console.log("SKIP  govulncheck ./... (install failed — offline? run with network once)");
// 3. npm audits (root harness + web dashboard). The root package has zero
// dependencies by design (pure-Node test harness), so the absence of a root
// package-lock.json is INTENTIONAL — there is nothing to audit. SKIP,
// never a faked PASS.
if (_exists("package-lock.json")) run("npm audit (root)", "npm audit --omit=dev --audit-level=high");
else console.log("SKIP  npm audit (root) (no lockfile — harness has no dependencies, intentional)");
run("npm audit (web)", "npm audit --omit=dev --audit-level=high", { cwd: "apps/web" });
// 4. secret scan: gitleaks when present, else a deterministic grep for
// committed private keys / tokens (testdata keys are in-memory only by
// policy, so any hit is a real finding).
if (have("gitleaks")) {
  run("gitleaks", "gitleaks detect --no-git -v .");
} else {
  try {
    const hits = execSync(`git grep -n -E -- '-----BEGIN (EC |RSA |OPENSSH |DSA )?PRIVATE KEY-----|xox[bpas]-|gh[pousr]_|AKIA[0-9A-Z]{16}' -- ':!*.lock' ':!go.sum' . || true`, { encoding: "utf8", timeout: 60000 });
    if (hits.trim()) { failures++; console.log(`FAIL  secret-grep\n${hits.slice(0, 2000)}`); }
    else console.log("PASS  secret-grep (no committed keys/tokens)");
  } catch (e) {
    failures++;
    console.log(`FAIL  secret-grep: ${String(e).slice(0, 500)}`);
  }
}
console.log(failures === 0 ? "scan: ALL CHECKS PASS-or-SKIP" : `scan: ${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);
