// Shared input guards (used by API, builders, object storage).
// Never trust user-provided paths; SSRF allowlist; explicit size caps.
import { URL } from "node:url";

const ALLOWED_REPO_HOSTS = new Set(["github.com", "gitlab.com", "npmjs.com", "pypi.org", "crates.io"]);

export function isSafeArtifactPath(p) {
  // Opened directly (no base-dir join), so `..`/absolute paths cannot escape.
  // Reject NUL/controls, shell metacharacters, and absurd lengths.
  if (typeof p !== "string" || p.length === 0 || p.length > 512) return false;
  // eslint-disable-next-line no-control-regex
  if (/[\0-\x1f]/.test(p)) return false;
  if (/["*?<>|;&$`]/.test(p)) return false;
  return true;
}

export function isAllowedRepoUrl(raw) {
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:") return false;
    if (!ALLOWED_REPO_HOSTS.has(u.hostname)) return false;
    if (u.username || u.password) return false;
    return true;
  } catch {
    return false;
  }
}

export function checkArtifactSize(sizeBytes, maxBytes) {
  if (!Number.isInteger(sizeBytes) || sizeBytes < 0) return { ok: false, reason: "INVALID_SIZE" };
  if (sizeBytes > maxBytes) return { ok: false, reason: "ARTIFACT_OVERSIZED" };
  return { ok: true };
}
