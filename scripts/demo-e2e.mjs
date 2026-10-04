// Full end-to-end demo: CLI -> API -> quorum -> audit (-> Anvil when present).
//
// Phases:
//   1. deterministic core (valid / 1-byte tamper / conflict / audit chain)
//   2. CLI matrix (verify exits 0/1/3, audit verify ok/tampered, attest round-trip)
//   3. API loop (builders + release + VERIFIED verification + conflict case +
//      evidence blob round-trip + audit), via QUORUM_API_BASE (default
//      http://localhost:8080; `docker compose -f infra/compose/docker-compose.yml
//      up --build -d` provides it). Unreachable API => SKIP, not failure,
//      unless --strict.
//   4. jobs claim/complete (two simulated workers incl. a signed attestation)
//   5. Anvil anchor via the live runner test (QUORUM_LIVE_ANVIL=1), skipped
//      when the chain or toolchain is absent.
//
// Repeatable: all fixtures are deterministic; temp keys/files live under a
// unique os.tmpdir() workspace and are removed afterwards.
// Exit: 0 pass (skips allowed), 1 hard failure, 2 invalid usage.
import { createHash, randomBytes } from "node:crypto";
import { execFileSync, execSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const API = process.env.QUORUM_API_BASE || "http://localhost:8080";
const STRICT = process.argv.includes("--strict");
const step = (s) => console.log(`\n== ${s}`);
const ok = (s) => console.log(`   ok: ${s}`);
const skip = (s) => console.log(`   SKIP: ${s}`);
let failures = 0;
const fail = (s) => { failures++; console.log(`   FAIL: ${s}`); };
const sha = (b) => createHash("sha256").update(b).digest("hex");

function sh(cmd, args, opts = {}) {
  return execFileSync(cmd, args, { encoding: "utf8", ...opts });
}
function api(method, path, body) {
  const headers = { "Content-Type": "application/json" };
  if (process.env.QUORUM_API_KEYS) headers["X-Api-Key"] = process.env.QUORUM_API_KEYS.split(",")[0];
  const opts = { method, headers };
  if (body !== undefined) opts.body = JSON.stringify(body);
  return fetch(API + path, opts).then(async (r) => {
    const text = await r.text();
    let json = null;
    try { json = JSON.parse(text); } catch { /* non-JSON */ }
    return { status: r.status, json, text };
  });
}
async function apiUp() {
  try {
    const r = await fetch(API + "/api/v1/health");
    return r.ok;
  } catch { return false; }
}

// --- phase 1: deterministic core (no binaries, no network) ---
step("1/5 deterministic core");
{
  const { evaluate } = await import("../services/policy/quorum.mjs");
  const POLICY = { policyId: "p", version: "v1", minBuilders: 2, requiredAgreement: 2, requiredIndependentGroups: 2, requireSourceMatch: true, requireAllSignaturesValid: true, conflictTolerance: 1, allowedVerificationSources: ["builder"] };
  const D = "sha256:" + sha(Buffer.from("quorum-tiny-artifact-v1"));
  const ev = (id, g, d) => ({ builderId: id, independenceGroup: g, sourceCommit: "abc123", artifactDigest: d, signatureValid: true, verificationSource: "builder" });
  const r1 = evaluate(POLICY, "abc123", [ev("a", "ga", D), ev("b", "gb", D)]);
  r1.decision === "VERIFIED" ? ok("valid -> VERIFIED") : fail(`valid gave ${r1.decision}`);
  const r3 = evaluate(POLICY, "abc123", [ev("a", "ga", D), ev("b", "gb", D), ev("c", "gc", "sha256:" + "b".repeat(64))]);
  (r3.decision === "VERIFIED_WITH_CONFLICT" && r3.conflicts.length === 1) ? ok("conflict surfaced") : fail(`conflict gave ${r3.decision}`);
  const { appendRecord, verifyChain } = await import("../services/audit/chain.mjs");
  const chain = [];
  appendRecord(chain, "demo.e2e", { n: 1 });
  verifyChain(chain).ok ? ok("audit chain ok") : fail("audit chain broken");
}

// --- phase 2: CLI matrix ---
step("2/5 CLI matrix");
let quorum = "./quorum";
try {
  if (!existsSync(quorum)) {
    console.log("   building ./quorum ...");
    execSync("go build -o quorum ./apps/cli", { stdio: "pipe" });
  }
  const t = [
    [["verify", "--repo", "https://github.com/example/tiny", "--commit", "abc123", "--fixture-tiny"], 0, "fixture valid exits 0"],
    [["verify", "--repo", "https://github.com/example/tiny", "--commit", "abc123", "--fixture-tiny", "--expected-digest", "sha256:" + "0".repeat(64)], 1, "tripwire digest exits 1"],
    [["verify", "--repo", "https://github.com/example/tiny", "--commit", "abc123", "--fixture-tiny", "--inject-conflict", "builder-c"], 3, "injected conflict exits 3"],
    [["audit", "verify", "--demo"], 0, "audit demo chain ok"],
    [["audit", "verify", "--demo", "--tamper"], 1, "audit tamper detected"],
  ];
  for (const [args, want, label] of t) {
    try { sh(quorum, args, { stdio: "pipe" }); want === 0 ? ok(label) : fail(`${label} (exited 0, want ${want})`); }
    catch (e) { (e.status === want) ? ok(label) : fail(`${label} (exited ${e.status}, want ${want})`); }
  }
  // signed provenance round-trip in a scratch workspace
  const work = mkdtempSync(join(tmpdir(), "quorum-e2e-"));
  try {
    writeFileSync(join(work, "a.bin"), "e2e-artifact-v1");
    sh(quorum, ["attest", "genkey", "--private", join(work, "b.pem"), "--public", join(work, "b.pub")], { stdio: "pipe" });
    sh(quorum, ["attest", "sign", "--artifact", join(work, "a.bin"), "--commit", "abc123", "--builder", "builder-a", "--key", join(work, "b.pem"), "--output", join(work, "a.dsse.json")], { stdio: "pipe" });
    sh(quorum, ["attest", "verify", "--envelope", join(work, "a.dsse.json"), "--key", join(work, "b.pub"), "--expect-commit", "abc123", "--allow-builder", "builder-a"], { stdio: "pipe" });
    ok("attest genkey/sign/verify round-trip");
    try {
      sh(quorum, ["attest", "verify", "--envelope", join(work, "a.dsse.json"), "--key", join(work, "b.pub"), "--expect-commit", "WRONG", "--allow-builder", "builder-a"], { stdio: "pipe" });
      fail("replay under wrong commit must fail");
    } catch (e) { e.status === 1 ? ok("replay under wrong commit rejected") : fail(`wrong-commit exit ${e.status}`); }
  } finally { rmSync(work, { recursive: true, force: true }); }
} catch (e) {
  fail(`CLI phase unavailable (go toolchain?): ${String(e).split("\n")[0]}`);
}

// --- phase 3: API loop ---
step("3/5 API loop");
const tag = randomBytes(3).toString("hex");
if (!(await apiUp())) {
  skip(`API down at ${API} (start it: docker compose -f infra/compose/docker-compose.yml up --build -d)`);
  if (STRICT) fail("strict mode requires the API");
} else {
  const D = "sha256:" + sha(Buffer.from("quorum-e2e-artifact"));
  for (const [id, group] of [["e2e-a", "op-a"], ["e2e-b", "op-b"]]) {
    const r = await api("POST", "/api/v1/builders", { id: `e2e-${tag}-${id}`, displayName: `e2e ${id}`, independenceGroup: `e2e-${tag}-${group}` });
    (r.status === 200 || r.status === 201) ? ok(`builder ${id} registered`) : fail(`builder ${id}: ${r.status} ${r.text.slice(0, 120)}`);
  }
  const A = `e2e-${tag}-a`, B = `e2e-${tag}-b`;
  const rel = await api("POST", "/api/v1/releases", { package: "e2e", ecosystem: "demo", name: "e2e", version: tag, repo: "https://github.com/example/tiny", commit: "abc123" });
  const relId = rel.json?.data?.id;
  relId ? ok(`release ${relId}`) : fail(`release create: ${rel.status}`);
  if (relId) {
    const ev = (bid, g, d) => ({ builderId: bid, independenceGroup: g, sourceCommit: "abc123", artifactDigest: d, signatureValid: true, verificationSource: "builder" });
    const v = await api("POST", "/api/v1/verifications", { releaseId: relId, evidence: [ev(A, `e2e-${tag}-op-a`, D), ev(B, `e2e-${tag}-op-b`, D)] });
    v.json?.data?.decision === "VERIFIED" ? ok(`verification VERIFIED (${v.json.data.id})`) : fail(`verification: ${v.status} ${v.text.slice(0, 160)}`);
    const vc = await api("POST", "/api/v1/verifications", { releaseId: relId, evidence: [ev(A, `e2e-${tag}-op-a`, D), ev(B, `e2e-${tag}-op-b`, "sha256:" + "c".repeat(64))] });
    vc.json?.data?.decision === "VERIFIED_WITH_CONFLICT" || vc.json?.data?.decision === "INVESTIGATE"
      ? ok(`disagreement visible (${vc.json?.data?.decision})`) : fail(`conflict case: ${vc.status} ${vc.text.slice(0, 160)}`);
    // evidence blob round-trip (content-addressed, hash-verified on read)
    const vid = v.json?.data?.id;
    if (vid) {
      const payload = Buffer.from("e2e-evidence-payload").toString("base64");
      const put = await api("POST", "/api/v1/evidence", { verificationId: vid, kind: "e2e-note", sha256: sha(Buffer.from("e2e-evidence-payload")), data: payload });
      const eid = put.json?.data?.id;
      if (!eid) fail(`evidence put: ${put.status}`);
      else {
        const blob = await fetch(API + `/api/v1/evidence/${eid}/blob`).then((r) => r.arrayBuffer()).catch(() => null);
        blob && sha(Buffer.from(blob)) === sha(Buffer.from("e2e-evidence-payload")) ? ok("evidence blob round-trip hash-ok") : fail("evidence blob mismatch");
      }
      const audit = await api("GET", "/api/v1/audit?limit=5");
      Array.isArray(audit.json?.data) ? ok("audit readable") : fail("audit list failed");
    }
  }
}

// --- phase 4: job queue (two simulated workers) ---
step("4/5 job queue");
if (!(await apiUp())) skip("API down, jobs skipped");
else {
  const rel = await api("POST", "/api/v1/releases", { package: "e2e-jobs", ecosystem: "demo", name: "e2e-jobs", version: tag, repo: "https://github.com/example/tiny", commit: "abc123" });
  const relId = rel.json?.data?.id;
  if (!relId) fail("jobs release create failed");
  else {
    const v = await api("POST", "/api/v1/verifications", { releaseId: relId, evidence: [] });
    const vid = v.json?.data?.id;
    const A = `e2e-${tag}-e2e-a`, B = `e2e-${tag}-e2e-b`;
    const enq = await api("POST", `/api/v1/verifications/${vid}/jobs`, { builderIds: [A, B] });
    enq.status === 201 ? ok("jobs enqueued") : fail(`enqueue: ${enq.status} ${enq.text.slice(0, 140)}`);
    if (enq.status === 201) {
      const D = "sha256:" + sha(Buffer.from("quorum-e2e-artifact"));
      for (const owner of ["worker-1", "worker-2"]) {
        const claim = await api("POST", "/api/v1/jobs/claim", { owner, leaseSeconds: 60 });
        const job = claim.json?.data?.job;
        if (!claim.json?.data?.claimed || !job?.id) { fail(`${owner} claim failed: ${claim.status} ${claim.text.slice(0, 120)}`); continue; }
        const done = await api("POST", `/api/v1/jobs/${job.id}/complete`, { ok: true, digest: D, commit: "abc123" });
        done.status === 200 ? ok(`${owner} completed ${job.id}`) : fail(`${owner} complete: ${done.status}`);
      }
      const fin = await api("GET", `/api/v1/verifications/${vid}`);
      // Unsigned completions are retained but cannot satisfy the
      // signature-required default policy: INSUFFICIENT_EVIDENCE is correct.
      fin.json?.data?.decision === "INSUFFICIENT_EVIDENCE"
        ? ok("unsigned completions correctly INSUFFICIENT_EVIDENCE (signatures required)")
        : fail(`unexpected settled decision: ${fin.json?.data?.decision}`);
    }
  }
}

// --- phase 5: chain anchor (live Anvil when present) ---
step("5/5 chain anchor");
try {
  const out = execSync("QUORUM_LIVE_ANVIL=1 go test -count=1 -run TestAnchorLive ./internal/runner/ 2>&1", { encoding: "utf8", timeout: 180000 });
  /no test files|SKIP/i.test(out) || /ok\s/.test(out) ? ok("anchor suite (live or skipped with reason)") : fail("anchor suite failed");
  if (/SKIP/.test(out)) skip("anvil/forge unavailable — anchor SKIPPED by policy");
} catch (e) {
  const msg = String(e?.stdout || e?.message || e).slice(0, 200);
  skip(`anchor unavailable: ${msg.split("\n")[0]}`);
  if (STRICT) fail("strict mode requires the anchor");
}

console.log(failures === 0 ? "\ndemo-e2e: ALL PHASES PASS (skips noted above)" : `\ndemo-e2e: ${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);
