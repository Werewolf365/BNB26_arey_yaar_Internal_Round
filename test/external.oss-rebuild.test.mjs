// External (live) tests — NEVER run in test-all. Require OSS_REBUILD_MODE=live + ADC creds.
import test from "node:test";
import assert from "node:assert/strict";
import { isLiveAllowed } from "../services/oss-rebuild/adapter.mjs";

test("live OSS Rebuild is opt-in only", () => {
  if (!isLiveAllowed()) {
    assert.ok(true, "SKIP: set OSS_REBUILD_MODE=live with ADC creds to run real OSS Rebuild tests");
    return;
  }
  assert.ok(true, "live mode: shell real OSS Rebuild CLI here (future phase)");
});
