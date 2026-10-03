// Conformance: the SAME vectors as the Go engine (packages/fixtures/conformance.json).
// Any drift between the Node mirror and Go fails one of the suites.
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { canonical, evaluate } from "../services/policy/quorum.mjs";

const doc = JSON.parse(readFileSync(new URL("../packages/fixtures/conformance.json", import.meta.url)));

test("canonical vectors match", () => {
  assert.ok(doc.canonicalCases.length > 0);
  for (const tc of doc.canonicalCases) {
    assert.equal(canonical(tc.input), tc.want);
  }
});

test("quorum vectors match", () => {
  assert.ok(doc.quorumCases.length > 0);
  for (const tc of doc.quorumCases) {
    const got = evaluate(tc.policy, tc.expectedSource, tc.evidence);
    assert.equal(got.decision, tc.wantDecision, tc.name);
    assert.equal(got.satisfied, tc.wantSatisfied, tc.name);
    assert.equal(got.conflicts.length, tc.wantConflicts, tc.name);
    // Order-independence.
    const again = evaluate(tc.policy, tc.expectedSource, [...tc.evidence].reverse());
    assert.deepEqual(again, got, `${tc.name}: not order-independent`);
  }
});
