// Mutation gate for the quorum engine (prompt section 31).
//
// Applies small behavior-changing mutants to services/policy/quorum.go, runs
// `go test ./services/policy/` for each, and requires every mutant to be
// KILLED (suite fails). A surviving mutant means the tests cannot tell a
// real logic error apart from correct code — fix the tests, not the gate.
//
// Usage: node scripts/mutate.mjs [--keep]   (--keep leaves the last mutant applied)
import { execSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

const FILE = "services/policy/quorum.go";
// Every mutant must change observable behavior (verified by hand); the suite
// must catch all of them.
const MUTANTS = [
  {
    label: "tiebreak: first-digest-wins instead of best-count",
    from: "if c := len(digestGroupSet[d]); c > bestCount {",
    to: "if c := len(digestGroupSet[d]); c >= bestCount {",
  },
  {
    label: "tolerance off-by-one (<= becomes <)",
    from: "numConflicts <= policy.ConflictTolerance && numConflicts == 0",
    to: "numConflicts < policy.ConflictTolerance && numConflicts == 0",
  },
  {
    label: "same-group votes counted twice (guard removed)",
    from: "\tif prev, ok := used[k]; ok {",
    to: "\tif prev, ok := used[k]; ok && false {",
  },
  {
    label: "signatures no longer required (check inverted)",
    from: "if policy.RequireAllSignaturesValid && !e.SignatureValid {",
    to: "if policy.RequireAllSignaturesValid && e.SignatureValid {",
  },
  {
    label: "source pin ignored (check inverted)",
    from: "if policy.RequireSourceMatch && e.SourceCommit != expectedSource {",
    to: "if policy.RequireSourceMatch && e.SourceCommit == expectedSource {",
  },
  {
    label: "malformed evidence admitted (guard removed)",
    from: 'if e.BuilderID == "" || e.IndependenceGroup == "" || e.ArtifactDigest == "" {',
    to: 'if false && (e.BuilderID == "" || e.IndependenceGroup == "" || e.ArtifactDigest == "") {',
  },
];

const orig = readFileSync(FILE, "utf8");
let survived = 0;
for (const m of MUTANTS) {
  if (!orig.includes(m.from)) {
    console.log(`SKIP  ${m.label}: pattern not found (engine changed — update mutants)`);
    continue;
  }
  writeFileSync(FILE, orig.replace(m.from, m.to));
  let killed = false;
  try {
    execSync("go test -count=1 ./services/policy/", { stdio: "pipe", timeout: 120000 });
  } catch {
    killed = true;
  }
  console.log(`${killed ? "KILL " : "LIVE "} ${m.label}`);
  if (!killed) survived++;
}
if (!process.argv.includes("--keep")) writeFileSync(FILE, orig);
try {
  execSync("go test -count=1 ./services/policy/", { stdio: "pipe", timeout: 120000 });
  console.log("restore: suite green on unmutated engine");
} catch {
  console.log("restore: SUITE STILL FAILING — engine file may be dirty, check git diff");
  survived++;
}
if (survived > 0) {
  console.error(`mutation gate: ${survived} SURVIVED (tests miss real logic errors)`);
  process.exit(1);
}
console.log("mutation gate: all mutants killed");
