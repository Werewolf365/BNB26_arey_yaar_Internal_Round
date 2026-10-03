// Security: input guards — path traversal, SSRF, oversized artifact.
// These are pure guards that the API/builder/object-store share.
import test from "node:test";
import assert from "node:assert/strict";
import { isSafeArtifactPath, isAllowedRepoUrl, checkArtifactSize } from "../services/verification/guards.mjs";

test("artifact paths: metacharacters rejected, plain/absolute usable", () => {
  assert.equal(isSafeArtifactPath("a;b.tar.gz"), false);
  assert.equal(isSafeArtifactPath("a|b.tar.gz"), false);
  assert.equal(isSafeArtifactPath("nul\0x"), false);
  assert.equal(isSafeArtifactPath("tiny-1.0.0.tar.gz"), true);
  // Opened directly (no base-dir join): absolute and .. paths cannot escape.
  assert.equal(isSafeArtifactPath("/abs/path.tar.gz"), true);
  assert.equal(isSafeArtifactPath("a/../../b.tar.gz"), true);
});

test("SSRF: only https example-allowlisted hosts; no localhost/metadata", () => {
  assert.equal(isAllowedRepoUrl("https://github.com/example/tiny"), true);
  assert.equal(isAllowedRepoUrl("http://github.com/example/tiny"), false);
  assert.equal(isAllowedRepoUrl("https://169.254.169.254/latest"), false);
  assert.equal(isAllowedRepoUrl("https://localhost:8080/x"), false);
  assert.equal(isAllowedRepoUrl("https://evil.example/x"), false);
});

test("oversized artifact rejected", () => {
  assert.equal(checkArtifactSize(10 * 1024 * 1024, 50 * 1024 * 1024).ok, true);
  assert.equal(checkArtifactSize(60 * 1024 * 1024, 50 * 1024 * 1024).ok, false);
});
