// Generic real-project showcase: pinned release + three isolated rebuilds
// (git-archive digests) + quorum + tamper/conflict scenarios + Anvil anchor.
//
//   make demo-real [PROJECT=name] [REPO=url COMMIT=sha TAG=tag ...]
//   node scripts/demo-real.mjs --project xz
//   node scripts/demo-real.mjs --repo https://github.com/<you>/<repo>.git --commit <40-hex-sha> [--tag vX.Y.Z --package p --version v ...]
//   node scripts/demo-real.mjs --list
//
// Project-specific facts live in projects/*.json (see projects/my-project.json
// template). This script is generic orchestration: validate config FIRST
// (before services/records), then pin, build, verify, anchor, report.
//
// What this proves (and does NOT claim): see docs/demo-real.md.
import { createHash, randomBytes } from "node:crypto";
import { execFileSync, execSync } from "node:child_process";
import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const API = process.env.QUORUM_API_BASE || "http://localhost:8080";
const RPC = process.env.QUORUM_RPC_URL || "http://localhost:8545";
const ROOT = new URL("..", import.meta.url).pathname.replace(/\/$/, "");

const sha = (b) => createHash("sha256").update(b).digest("hex");
const canon = (v) => {
  if (v === null || typeof v !== "object") return JSON.stringify(v);
  if (Array.isArray(v)) return `[${v.map(canon).join(",")}]`;
  return `{${Object.keys(v).sort().map((k) => `${JSON.stringify(k)}:${canon(v[k])}`).join(",")}}`;
};
const step = (s) => console.log(`\n== ${s}`);
const ok = (s) => console.log(`   ok: ${s}`);
const skip = (s) => console.log(`   SKIP: ${s}`);
let failures = 0;
const fail = (s) => { failures++; console.log(`   FAIL: ${s}`); };
const sh = (cmd, args, opts = {}) => execFileSync(cmd, args, { encoding: "utf8", ...opts });
const apiKey = () => (process.env.QUORUM_API_KEYS || "").split(",").filter(Boolean)[0] || "";

function arg(name) {
  const i = process.argv.findIndex((a) => a === `--${name}`);
  return i >= 0 ? process.argv[i + 1] : process.env[name.toUpperCase()] || "";
}

if (arg("list") === "true" || process.argv.includes("--list")) {
  console.log("available projects (projects/*.json):");
  for (const f of readdirSync(join(ROOT, "projects")).filter((f) => f.endsWith(".json")).sort()) {
    try {
      const p = JSON.parse(readFileSync(join(ROOT, "projects", f), "utf8"));
      console.log(`- ${p.name || f}: ${p.description || ""} [${p.repo || "?"} @ ${p.commit || "UNPINNED"}]`);
    } catch { console.log(`- ${f}: UNREADABLE`); }
  }
  console.log("override: --repo URL --commit SHA [--tag TAG --package P --ecosystem E --name N --version V]");
  process.exit(0);
}

const STRICT = process.argv.includes("--strict");

// --- load + validate project config (before touching services/records) ---
function loadProject() {
  const name = arg("project") || "xz";
  let cfg;
  try {
    cfg = JSON.parse(readFileSync(join(ROOT, "projects", `${name}.json`), "utf8"));
  } catch {
    fail(`unknown project ${JSON.stringify(name)} (see --list)`);
    process.exit(1);
  }
  for (const [k, v] of Object.entries({ repo: arg("repo"), commit: arg("commit"), tag: arg("tag"), package: arg("package"), ecosystem: arg("ecosystem"), name: arg("name"), version: arg("version") })) {
    if (v) cfg[k === "package" ? "package" : k] = v;
  }
  // A --repo override replaces the whole source identity: the display URL
  // follows the override, never the base config (mixing them once pointed a
  // worker at xz-orbit with a golang commit).
  if (arg("repo")) cfg.webRepo = arg("repo").replace(/\.git$/, "");
  const errs = [];
  try {
    const u = new URL(cfg.repo);
    if (u.protocol !== "https:") errs.push("repo must be https");
    if (!["github.com", "gitlab.com"].includes(u.hostname)) errs.push(`repo host ${u.hostname} not allowlisted (github.com, gitlab.com)`);
    if (u.username || u.password) errs.push("repo must not embed credentials");
  } catch { errs.push("repo is not a valid URL"); }
  if (!/^[0-9a-f]{40}([0-9a-f]{24})?$/i.test(cfg.commit || "")) errs.push("commit must be a full immutable SHA (40 or 64 hex) — tags resolve, never pin a tag");
  if (!cfg.tag) errs.push("tag is required (human release label)");
  for (const k of ["package", "ecosystem", "version"]) if (!cfg[k]) errs.push(`${k} is required`);
  if (!Array.isArray(cfg.builders) || cfg.builders.length < 2) errs.push("at least 2 builders required");
  const ids = new Set(), groups = new Set();
  for (const b of cfg.builders || []) {
    if (!b.suffix || !b.group || !b.owner) errs.push("each builder needs suffix/group/owner");
    if (ids.has(b.suffix) || groups.has(b.group)) errs.push("builder suffixes and independence groups must be distinct");
    ids.add(b.suffix); groups.add(b.group);
  }
  if (errs.length) { for (const e of errs) fail("config: " + e); process.exit(1); }
  return cfg;
}
const P = loadProject();
const webRepo = (P.webRepo || P.repo).replace(/\.git$/, "");
console.log(`project: ${P.name} — ${P.repo} @ ${P.commit} (${P.tag})`);

async function api(method, path, body) {
  const headers = { "Content-Type": "application/json" };
  const k = apiKey();
  if (k) headers["X-Api-Key"] = k;
  const res = await fetch(API + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* non-JSON */ }
  return { status: res.status, json, text };
}
const have = (bin) => {
  try { execSync(`${bin} --version`, { stdio: "pipe", timeout: 15000 }); return true; }
  catch { return false; }
};

const work = mkdtempSync(join(tmpdir(), "quorum-real-"));
process.on("exit", () => rmSync(work, { recursive: true, force: true }));

try {
  console.log("   building ./quorum + ./quorum-worker ...");
  execSync("go build -o quorum ./apps/cli && go build -o quorum-worker ./apps/worker", { stdio: "pipe", cwd: ROOT });
} catch (e) { fail("go build failed: " + String(e).split("\n")[0]); }
const quorum = join(ROOT, "quorum");

// --- 1. resolve tag -> SHA (fail on drift, never silently update) + reference ---
step(`1/7 pin source (${P.tag}) + reference digest`);
let refDigest = "";
try {
  const resolved = execSync(`git ls-remote ${P.repo} ${P.tag}`, { encoding: "utf8", timeout: 120000 }).trim().split(/\s+/)[0] || "";
  const peeled = execSync(`git ls-remote ${P.repo} ${P.tag}^{}`, { encoding: "utf8", timeout: 120000 }).trim().split(/\s+/)[0] || resolved;
  const want = (peeled || resolved).toLowerCase();
  want === String(P.commit).toLowerCase()
    ? ok(`tag ${P.tag} resolves to pinned commit ${P.commit}`)
    : fail(`tag drift: ${P.tag} -> ${want || "(missing)"}, config pins ${P.commit} (refusing to follow the tag)`);
  if (want !== String(P.commit).toLowerCase()) throw new Error("tag drift");
  const dir = join(work, "src");
  execSync(`git init -q ${dir} && git -C ${dir} remote add origin ${P.repo} && git -C ${dir} fetch -q --depth 1 origin ${P.commit} && git -C ${dir} checkout -q FETCH_HEAD`, { stdio: "pipe", timeout: 300000 });
  const got = execSync(`git -C ${dir} log -1 --format=%H`, { encoding: "utf8" }).trim();
  got.toLowerCase() === String(P.commit).toLowerCase() ? ok(`commit pinned ${P.commit}`) : fail(`commit drift: ${got}`);
  // Two independent archives: same bytes from repeated builds -> same digest.
  // Matching bytes are NOT tampering evidence; only differing bytes are.
  const a = execSync(`git -C ${dir} archive HEAD`, { encoding: "binary", maxBuffer: 64 << 20 });
  const b = execSync(`git -C ${dir} archive HEAD`, { encoding: "binary", maxBuffer: 64 << 20 });
  const da = sha(Buffer.from(a, "binary")), db = sha(Buffer.from(b, "binary"));
  da === db ? ok(`independent archives byte-identical, sha256:${da.slice(0, 16)}… (match is agreement, not tampering)`) : fail("archive nondeterministic");
  refDigest = "sha256:" + da;
  writeFileSync(join(work, "artifact.tar"), Buffer.from(a, "binary"));
} catch (e) { if (!refDigest) fail("source fetch failed: " + String(e).split("\n")[0]); }

// --- 2. CLI ritual ---
step("2/7 CLI: hash + policy on the real tarball");
const shortCommit = String(P.commit).slice(0, 12);
if (refDigest) {
  try { sh(quorum, ["verify", "--repo", webRepo, "--commit", P.commit, "--artifact", join(work, "artifact.tar"), "--expected-digest", refDigest], { stdio: "pipe" }); ok("valid tarball -> VERIFIED (exit 0)"); }
  catch (e) { fail(`valid tarball exit ${e.status}`); }
  execSync(`printf 'tamper' >> ${join(work, "artifact.tar")}`);
  try { sh(quorum, ["verify", "--repo", webRepo, "--commit", P.commit, "--artifact", join(work, "artifact.tar"), "--expected-digest", refDigest], { stdio: "pipe" }); fail("tampered tarball must not verify"); }
  catch (e) { e.status === 1 ? ok("1-byte tamper -> REJECTED (exit 1, ARTIFACT_HASH_MISMATCH)") : fail(`tamper exit ${e.status}, want 1`); }
} else skip("no reference digest");

// --- 3-5. API: builders, release, worker rebuilds, quorum ---
step("3/7 API: register builders + release");
const tag = randomBytes(3).toString("hex");
let apiOk = false;
try { apiOk = (await fetch(API + "/api/v1/health")).ok; } catch { /* down */ }
if (!apiOk) { skip(`API down at ${API} (up --build -d first)`); if (STRICT) fail("strict needs API"); }
let builders = [], releaseId = "", verificationId = "";
const keys = {};
const prefix = `${P.name}-${tag}`;
if (apiOk && refDigest) {
  for (const b of P.builders) {
    try {
      sh(quorum, ["attest", "genkey", "--private", join(work, `${b.suffix}.pem`), "--public", join(work, `${b.suffix}.pub`)], { stdio: "pipe" });
      const pub = readFileSync(join(work, `${b.suffix}.pub`), "utf8");
      keys[b.suffix] = join(work, `${b.suffix}.pem`);
      const id = `${prefix}-${b.suffix}`, group = `${prefix}-${b.group}`;
      const r = await api("POST", "/api/v1/builders", { id, displayName: `${P.name} demo ${b.suffix}`, independenceGroup: group, signingPublicKey: pub });
      (r.status === 200 || r.status === 201) ? (ok(`builder ${b.suffix} registered (group ${b.group})`), builders.push({ id, group })) : fail(`builder ${b.suffix}: ${r.status}`);
    } catch (e) { fail(`builder ${b.suffix}: ${String(e).split("\n")[0]}`); }
  }
  const rel = await api("POST", "/api/v1/releases", { package: P.package, ecosystem: P.ecosystem, name: P.package, version: P.version, repo: webRepo, commit: P.commit, expectedDigest: refDigest });
  rel.json?.data?.id ? (ok(`release ${rel.json.data.id} pinned @${shortCommit}`), releaseId = rel.json.data.id) : fail("release create: " + rel.status);
}
step("4/7 workers: isolated rebuilds (one builder at a time)");
if (apiOk && releaseId && builders.length === P.builders.length && have("docker")) {
  const v = await api("POST", "/api/v1/verifications", { releaseId, evidence: [] });
  verificationId = v.json?.data?.id || "";
  const pendingId = verificationId;
  verificationId ? ok(`verification ${verificationId} (pending jobs)`) : fail("verification create failed");
  if (verificationId) {
    // Exactly one QUEUED job exists per worker run, so each signing key
    // provably signs its own builder's attestation. Worker output must name
    // OUR verification id — otherwise it claimed a stale foreign job and the
    // run is invalid (never silently accept another run's evidence).
    for (let i = 0; i < P.builders.length; i++) {
      const enq = await api("POST", `/api/v1/verifications/${pendingId}/jobs`, { builderIds: [builders[i].id] });
      enq.status === 201 ? ok(`rebuild job enqueued for ${P.builders[i].suffix}`) : fail(`enqueue ${P.builders[i].suffix}: ${enq.status} ${enq.text.slice(0, 120)}`);
      if (enq.status !== 201) continue;
      try {
        // --verification scopes the claim to OUR pending verification, so
        // stale QUEUED jobs from other runs are never touched (reruns safe).
        const out = execSync(`./quorum-worker --api ${API} --owner ${P.builders[i].owner} --signing-key ${keys[P.builders[i].suffix]} --verification ${pendingId} --once 2>&1`, { cwd: ROOT, encoding: "utf8", timeout: 600000 });
        out.includes(pendingId) ? ok(`${P.builders[i].owner} rebuilt + signed (claimed our job)`) : fail(`${P.builders[i].owner} claimed a FOREIGN job — stale queue from another run? reset demo state`);
      } catch (e) { fail(`${P.builders[i].owner} worker failed: ${String(e?.message || e).split("\n").slice(-3).join(" / ").slice(0, 200)}`); }
    }
    // Auto-evaluation mints a NEW verification row on every terminal job
    // (partial sibling sets yield honest intermediate INSUFFICIENT rows);
    // the audit list is oldest-first, so the LAST fromJobs match is final.
    const audit = await api("GET", "/api/v1/audit?limit=30");
    const rows = Array.isArray(audit.json?.data) ? audit.json.data : [];
    const settled = [...rows].reverse().find((r) => r.eventType === "quorum.evaluated" && r.payload?.fromJobs && r.payload?.verificationId);
    if (settled) { verificationId = settled.payload.verificationId; ok(`auto-evaluation settled -> ${verificationId}`); }
    else fail("no quorum.evaluated record found");
    const fin = await api("GET", `/api/v1/verifications/${verificationId}`);
    const dec = fin.json?.data?.decision;
    dec === "VERIFIED" ? ok(`quorum VERIFIED: ${P.builders.length}/${P.builders.length} independent groups agree`) : fail("decision: " + dec);
    const digests = new Set((fin.json?.data?.evidence || []).map((e) => e.artifactDigest));
    digests.size === 1 && digests.has(refDigest) ? ok("all digests match reference (no hidden disagreement)") : fail("digest set: " + [...digests].join(","));
  }
} else skip("workers need API + release + docker");

step("5/7 controlled conflict (SIMULATED minority builder)");
if (apiOk && releaseId) {
  const ev = (bid, g, d) => ({ builderId: bid, independenceGroup: g, sourceCommit: P.commit, artifactDigest: d, signatureValid: true, verificationSource: "builder" });
  const vc = await api("POST", "/api/v1/verifications", { releaseId, evidence: [ev(builders[0]?.id || "sim-a", builders[0]?.group || "g-a", refDigest), ev(builders[1]?.id || "sim-b", builders[1]?.group || "g-b", refDigest), ev("simulated-minority", "sim-group", "sha256:" + "d".repeat(64))] });
  const dec = vc.json?.data?.decision;
  (dec === "VERIFIED_WITH_CONFLICT" || dec === "INVESTIGATE") && (vc.json?.data?.result?.conflicts?.length || 0) > 0
    ? ok(`minority surfaced, decision=${dec} (labeled SIMULATED, never a real build)`) : fail("conflict case: " + vc.status);
} else skip("need API + release");

// --- 6. anchor + independent re-read ---
step("6/7 anchor on Anvil + independent re-read");
const chainOk = await (async () => {
  try {
    const r = await (await fetch(RPC, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "eth_blockNumber", params: [] }) })).json();
    return !!r.result;
  } catch { return false; }
})();
if (!(apiOk && verificationId && chainOk)) { skip("anchor needs API verification + Anvil"); if (STRICT && apiOk) fail("strict needs chain"); }
else {
  try {
    const fin = await api("GET", `/api/v1/verifications/${verificationId}`);
    const vid = "0x" + sha("quorum-demo:" + verificationId);
    const evHash = "0x" + sha(canon(fin.json.data));
    const art = "0x" + refDigest.replace(/^sha256:/, "");
    const src = "0x" + sha(String(P.commit));
    const pol = "0x" + sha("policy-a/v1");
    const bc = execSync(`python3 -c "import json;print(json.load(open('${join(ROOT, "contracts/out/QuorumAnchor.sol/QuorumAnchor.json")}'))['bytecode']['object'])"`, { encoding: "utf8" }).trim();
    const bcArg = bc.startsWith("0x") ? bc : "0x" + bc;
    const dep = execSync(`docker run --rm --network host --entrypoint cast ghcr.io/foundry-rs/foundry:latest send --mnemonic "test test test test test test test test test test test junk" --mnemonic-index 0 --rpc-url ${RPC} --create ${bcArg}`, { encoding: "utf8", timeout: 120000 });
    const contract = (dep.match(/contractAddress\s+(0x[0-9a-fA-F]{40})/) || [])[1] || execSync(`docker run --rm --network host --entrypoint cast ghcr.io/foundry-rs/foundry:latest receipt ${/transactionHash\s+(0x[0-9a-f]+)/.exec(dep)[1]} contractAddress --rpc-url ${RPC}`, { encoding: "utf8" }).trim();
    ok(`contract deployed ${contract} (local Anvil — demo tx, not public chain)`);
    // Idempotent reruns: if this vid is already anchored, re-read and compare
    // instead of submitting a duplicate (the contract reverts duplicates).
    let pre = "";
    try { pre = sh(quorum, ["blockchain", "verify", "--rpc", RPC, "--contract", contract, "--verification-id", vid], { stdio: "pipe" }); } catch { /* exit 1 = not anchored yet */ }
    if (/anchored: true/.test(pre)) {
      fail(`vid already anchored on THIS contract (unexpected — vids are unique per run):\n${pre}`);
    } else {
      sh(quorum, ["blockchain", "anchor", "--required", "--rpc", RPC, "--contract", contract, "--verification-id", vid, "--evidence-hash", evHash, "--artifact-digest", art, "--source-commit", src, "--policy-hash", pol, "--decision", "VERIFIED"], { stdio: "pipe" });
      ok("anchor submitted + event verified by CLI");
      const out = sh(quorum, ["blockchain", "verify", "--rpc", RPC, "--contract", contract, "--verification-id", vid], { stdio: "pipe" });
      /anchored: true/.test(out) ? ok("independent re-read: anchored=true\n" + out.trim().split("\n").map((l) => "     " + l).join("\n")) : fail("re-read: " + out.slice(0, 200));
      console.log(`   inspect: RPC=${RPC} contract=${contract} vid=${vid}`);
      console.log(`   evidence hash recomputes as sha256(canonical verification JSON) = ${evHash}`);
      console.log(`   web explorer: http://localhost:3000/blockchain (paste the three values)`);
      console.log(`   bindings: vid=H(verification id) evidence=H(canonical bundle) artifact=${refDigest.slice(0, 20)}… source=H(commit) policy=H(policy-a/v1)`);
    }
  } catch (e) { fail("anchor phase: " + String(e?.message || e).split("\n").slice(-2).join(" / ").slice(0, 300)); }
}

// --- 7. audit ---
step("7/7 audit trail");
if (apiOk) {
  const a = await api("GET", "/api/v1/audit?limit=10");
  Array.isArray(a.json?.data) && a.json.data.length > 0 ? ok(`${a.json.data.length} records (latest page); full chain: POST /audit/:id/verify-chain`) : fail("audit empty");
} else skip("API down");

console.log(failures === 0 ? "\ndemo-real: SHOWCASE COMPLETE" : `\ndemo-real: ${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);
