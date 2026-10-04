// make add-project REPO=<github-url> REF=<tag|sha> [NAME=x] [ECOSYSTEM=e]
// Resolves the repository through the running API (no manual SHA lookup, no
// hand-written JSON) and writes projects/<name>.json in the demo schema.
// Then: make demo-real PROJECT=<name>.
//
// Needs the API up (QUORUM_API_BASE, default http://localhost:8080).
import { writeFileSync } from "node:fs";
import { join } from "node:path";

const API = process.env.QUORUM_API_BASE || "http://localhost:8080";
const ROOT = new URL("..", import.meta.url).pathname.replace(/\/$/, "");

function arg(name) {
  const i = process.argv.findIndex((a) => a === `--${name}`);
  return i >= 0 ? process.argv[i + 1] : process.env[name.toUpperCase()] || "";
}
async function call(method, path, body) {
  const res = await fetch(API + path, {
    method, headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* non-JSON */ }
  return { status: res.status, json, text };
}

const repo = arg("repo"), ref = arg("ref");
if (!repo || !ref) {
  console.log("usage: make add-project REPO=https://github.com/<owner>/<repo> REF=<tag|commit-sha> [NAME=<name>] [ECOSYSTEM=go|npm|cargo|pypi|c]");
  process.exit(2);
}
let apiOk = false;
try { apiOk = (await fetch(API + "/api/v1/health")).ok; } catch { /* down */ }
if (!apiOk) {
  console.log(`FAIL: API down at ${API} (start it: docker compose -f infra/compose/docker-compose.yml up --build -d)`);
  process.exit(1);
}
// 1. validate URL + repo exists (also shows available tags on failure modes).
const disc = await call("GET", `/api/v1/onboarding/discover?repo=${encodeURIComponent(repo)}`);
if (disc.status !== 200) {
  const e = disc.json?.error;
  console.log(`FAIL: cannot use repository: ${e ? `${e.code}: ${e.message}` : disc.status}`);
  process.exit(1);
}
const { owner, repo: reponame, tags } = disc.json.data;
console.log(`repo: ${owner}/${reponame} (${tags.length} tags)`);
// 2. resolve ref -> immutable commit + detected ecosystem (no writes).
const ecosystem = arg("ecosystem");
const res = await call("POST", "/api/v1/onboarding/resolve", { repo, ref, ecosystem });
if (res.status !== 200) {
  const e = res.json?.error;
  console.log(`FAIL: cannot resolve: ${e ? `${e.code}: ${e.message}` : res.status}`);
  if (e?.code === "TAG_NOT_FOUND" && tags.length) console.log(`available tags: ${tags.slice(0, 15).join(", ")}${tags.length > 15 ? " ..." : ""}`);
  if (/ecosystem/i.test(e?.message || "")) console.log("hint: re-run with ECOSYSTEM=go|npm|cargo|pypi|c");
  process.exit(1);
}
const r = res.json.data;
const eco = r.ecosystem || ecosystem;
if (!eco) {
  console.log("FAIL: ecosystem could not be detected; re-run with ECOSYSTEM=go|npm|cargo|pypi|c");
  process.exit(1);
}
const name = arg("name") || r.repo;
const cfg = {
  name,
  description: `${r.repo} ${r.tag || r.commit} — onboarded via make add-project`,
  repo: `https://github.com/${r.owner}/${r.repo}.git`,
  webRepo: `https://github.com/${r.owner}/${r.repo}`,
  tag: r.tag || "",
  commit: r.commit,
  package: r.repo,
  ecosystem: eco,
  version: r.tag || "",
  build: {
    kind: "git-archive",
    notes: "Builders reproduce git archive bytes for the pinned commit. This verifies source-archive integrity, not compiled-binary reproducibility.",
  },
  builders: [
    { suffix: "a", group: "operator-a", owner: "operator-a" },
    { suffix: "b", group: "operator-b", owner: "operator-b" },
    { suffix: "c", group: "operator-c", owner: "operator-c" },
  ],
};
const path = join(ROOT, "projects", `${name}.json`);
writeFileSync(path, JSON.stringify(cfg, null, 2) + "\n");
console.log(`wrote ${path}`);
console.log(`  ${r.owner}/${r.repo} @ ${r.commit} (${r.tag || "raw SHA"}, ${eco})`);
console.log(`next: make demo-real PROJECT=${name}`);
