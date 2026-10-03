// OSS Rebuild adapter boundary.
//
// CI/test-all:  deterministic FIXTURE mode only. No network, no credentials.
// Integration:  REAL mode shells the actual OSS Rebuild CLI (test-external only).
// Demo:         REAL mode when available, else fixture with explicit label.
//
// Facts baked in (verified Oct 2026):
// - Ecosystems with rebuild support: npm, PyPI, Crates.io (subset of popular packages only).
// - CLI can retrieve rebuild summaries/payloads/bundles/Dockerfiles.
// - Signature validation uses a Google Cloud KMS key; ADC credentials are
//   required unless signature verification is explicitly disabled.
// => test-all MUST NOT require ADC creds or live service.
export const OSS_STATES = [
  "SUPPORTED_AND_VERIFIED",
  "SUPPORTED_BUT_FAILED",
  "UNSUPPORTED",
  "NOT_FOUND",
  "UNAVAILABLE",
  "MALFORMED_EVIDENCE",
];

const SUPPORTED_PREFIXES = ["npm:", "pypi:", "cargo:"];

export function fixtureLookup(packageRef) {
  // Deterministic: tiny demo package verifies; everything else is explicit.
  if (packageRef === "pypi:quorum-tiny@1.0.0") {
    return {
      state: "SUPPORTED_AND_VERIFIED",
      artifactDigest: "sha256:__FILLED_BY_BUILD__",
      sourceCommit: "abc123",
      rawEvidenceRef: "fixtures/tiny-package/oss-rebuild-summary.json",
    };
  }
  if (SUPPORTED_PREFIXES.some((p) => packageRef.startsWith(p))) {
    return { state: "UNSUPPORTED", reason: "package not in OSS Rebuild rebuilt subset (fixture)" };
  }
  return { state: "NOT_FOUND", reason: "unknown ecosystem prefix; want npm:|pypi:|cargo:" };
}

export function isLiveAllowed() {
  return process.env.OSS_REBUILD_MODE === "live";
}
