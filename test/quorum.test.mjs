// Unit: quorum engine (Node mirror) — determinism, independence, conflicts.
import test from "node:test";
import assert from "node:assert/strict";
import { evaluate } from "../services/policy/quorum.mjs";

const SRC = "abc123";
const D = (s) => `sha256:${s}`;
const base = () => ({
  policyId: "policy-a", version: "v1", minBuilders: 2, requiredAgreement: 2,
  requiredIndependentGroups: 2, requireSourceMatch: true, requireAllSignaturesValid: true,
  conflictTolerance: 1, allowedVerificationSources: ["builder", "oss-rebuild"],
});
const ev = (builderId, independenceGroup, digest, extra = {}) => ({
  builderId, independenceGroup, sourceCommit: SRC, artifactDigest: digest,
  signatureValid: true, verificationSource: "builder", ...extra,
});

test("VERIFIED: 2 independent builders agree", () => {
  const r = evaluate(base(), SRC, [ev("builder-a", "cloud-a", D("a".repeat(64))), ev("builder-b", "cloud-b", D("a".repeat(64)))]);
  assert.equal(r.decision, "VERIFIED");
  assert.equal(r.satisfied, 2);
});

test("determinism: order-independent", () => {
  const d = D("a".repeat(64));
  const a = evaluate(base(), SRC, [ev("builder-b", "cloud-b", d), ev("builder-a", "cloud-a", d)]);
  const b = evaluate(base(), SRC, [ev("builder-a", "cloud-a", d), ev("builder-b", "cloud-b", d)]);
  assert.deepEqual(a, b);
});

test("same independence_group is NOT independent (aws x2 must not verify)", () => {
  const d = D("a".repeat(64));
  const r = evaluate(base(), SRC, [ev("builder-1", "aws", d), ev("builder-2", "aws", d)]);
  assert.notEqual(r.decision, "VERIFIED");
  assert.notEqual(r.decision, "VERIFIED_WITH_CONFLICT");
  assert.ok(r.excludedEvidence.some((x) => x.reason.includes("independence_group")));
});

test("VERIFIED_WITH_CONFLICT: A=B=abc, C=def", () => {
  const r = evaluate(base(), SRC, [
    ev("builder-a", "cloud-a", D("a".repeat(64))),
    ev("builder-b", "cloud-b", D("a".repeat(64))),
    ev("builder-c", "cloud-c", D("b".repeat(64))),
  ]);
  assert.equal(r.decision, "VERIFIED_WITH_CONFLICT");
  assert.equal(r.conflicts.length, 1);
  assert.deepEqual(r.conflicts[0].builders, ["builder-c"]); // minority never hidden
});

test("INSUFFICIENT_EVIDENCE: 1 of 2 required", () => {
  const r = evaluate(base(), SRC, [ev("builder-a", "cloud-a", D("a".repeat(64)))]);
  assert.equal(r.decision, "INSUFFICIENT_EVIDENCE");
});

test("duplicate builder id counted once", () => {
  const d = D("a".repeat(64));
  const r = evaluate(base(), SRC, [ev("builder-a", "cloud-a", d), ev("builder-a", "cloud-a", d)]);
  assert.ok(r.excludedEvidence.some((x) => x.reason.includes("duplicate")));
});

test("invalid signature rejected when policy requires valid sigs", () => {
  const d = D("a".repeat(64));
  const r = evaluate(base(), SRC, [ev("builder-a", "cloud-a", d), ev("builder-b", "cloud-b", d, { signatureValid: false })]);
  assert.ok(["INSUFFICIENT_EVIDENCE", "INVESTIGATE", "ERROR"].includes(r.decision) || r.excludedEvidence.length > 0);
});

test("source mismatch excluded", () => {
  const d = D("a".repeat(64));
  const r = evaluate(base(), SRC, [ev("builder-a", "cloud-a", d), ev("builder-b", "cloud-b", d, { sourceCommit: "evil" })]);
  assert.ok(r.excludedEvidence.some((x) => x.reason.includes("source commit")));
});

test("invalid policy -> ERROR (never silent reject)", () => {
  const bad = { ...base(), requiredAgreement: 5, minBuilders: 2 };
  const r = evaluate(bad, SRC, []);
  assert.equal(r.decision, "ERROR");
});

test("malformed evidence excluded, never verifies (fuzz-found)", () => {
  const r = evaluate(base(), SRC, [
    ev("", "cloud-a", D("a".repeat(64))),
    ev("builder-b", "", D("a".repeat(64))),
    ev("builder-c", "cloud-c", ""),
  ]);
  assert.notEqual(r.decision, "VERIFIED");
  assert.notEqual(r.decision, "VERIFIED_WITH_CONFLICT");
  assert.equal(r.countedEvidence.length + r.excludedEvidence.length, 3);
});

test("tiebreak: equal counts resolve to lowest digest", () => {
  const p = base(); p.minBuilders = 4;
  const d1 = D("a"), d2 = D("z");
  const r = evaluate(p, SRC, [
    ev("builder-a", "cloud-a", d1), ev("builder-b", "cloud-b", d1),
    ev("builder-c", "cloud-c", d2), ev("builder-d", "cloud-d", d2),
  ]);
  assert.equal(r.countedEvidence.length, 2);
  assert.equal(r.countedEvidence[0].artifactDigest, d1);
  assert.equal(r.conflicts.length, 1);
  assert.equal(r.conflicts[0].digest, d2);
});

test("zero conflicts with tolerance 0 is VERIFIED", () => {
  const p = base(); p.conflictTolerance = 0;
  const d = D("a".repeat(64));
  const r = evaluate(p, SRC, [ev("builder-a", "cloud-a", d), ev("builder-b", "cloud-b", d)]);
  assert.equal(r.decision, "VERIFIED");
});
