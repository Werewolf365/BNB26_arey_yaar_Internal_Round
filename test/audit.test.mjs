// Unit: audit hash chain — append, verify, tamper detection.
import test from "node:test";
import assert from "node:assert/strict";
import { appendRecord, verifyChain } from "../services/audit/chain.mjs";

test("chain verifies when untouched", () => {
  const chain = [];
  appendRecord(chain, "release.created", { releaseId: "r1" });
  appendRecord(chain, "builder.completed", { builderId: "builder-a" });
  appendRecord(chain, "quorum.evaluated", { decision: "VERIFIED" });
  assert.deepEqual(verifyChain(chain), { ok: true });
});

test("modifying record 2 breaks the chain at record 2", () => {
  const chain = [];
  appendRecord(chain, "release.created", { releaseId: "r1" });
  appendRecord(chain, "builder.completed", { builderId: "builder-a" });
  appendRecord(chain, "quorum.evaluated", { decision: "VERIFIED" });
  chain[1].payload.builderId = "builder-EVIL"; // tamper, like prompt section 48
  const v = verifyChain(chain);
  assert.equal(v.ok, false);
  assert.equal(v.brokenAt, 1);
});
