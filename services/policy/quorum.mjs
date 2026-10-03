// Quorum deterministic evaluation — Node mirror of services/policy/quorum.go.
// Go is the source of truth; this mirror runs the SAME JSON test vectors
// on machines without a Go toolchain (e.g. fresh Windows). Keep in sync.
export const DECISIONS = {
  VERIFIED: "VERIFIED",
  VERIFIED_WITH_CONFLICT: "VERIFIED_WITH_CONFLICT",
  INSUFFICIENT_EVIDENCE: "INSUFFICIENT_EVIDENCE",
  REJECTED: "REJECTED",
  INVESTIGATE: "INVESTIGATE",
  ERROR: "ERROR",
};

export function validatePolicy(p) {
  if (p.minBuilders < 1) throw new Error("INVALID_POLICY: minBuilders must be >= 1");
  if (p.requiredAgreement < 1) throw new Error("INVALID_POLICY: requiredAgreement must be >= 1");
  if (p.requiredIndependentGroups < 1) throw new Error("INVALID_POLICY: requiredIndependentGroups must be >= 1");
  if (p.requiredAgreement > p.minBuilders) throw new Error("INVALID_POLICY: requiredAgreement cannot exceed minBuilders");
  if (p.conflictTolerance < 0) throw new Error("INVALID_POLICY: conflictTolerance cannot be negative");
}

export function evaluate(policy, expectedSource, evidence) {
  const res = { decision: null, required: policy.requiredAgreement, satisfied: 0, conflicts: [], countedEvidence: [], excludedEvidence: [], reasons: [] };
  try {
    validatePolicy(policy);
  } catch (e) {
    res.decision = DECISIONS.ERROR;
    res.reasons.push(e.message);
    return res;
  }
  const allowed = new Set(policy.allowedVerificationSources || []);
  const sorted = [...evidence].sort((a, b) =>
    a.artifactDigest !== b.artifactDigest ? (a.artifactDigest < b.artifactDigest ? -1 : 1) : (a.builderId < b.builderId ? -1 : a.builderId > b.builderId ? 1 : 0)
  );
  const seen = new Set();
  const eligible = [];
  for (const e of sorted) {
    if (seen.has(e.builderId)) { res.excludedEvidence.push({ evidence: e, reason: "duplicate builder id: evidence not double-counted" }); continue; }
    seen.add(e.builderId);
    if (allowed.size > 0 && !allowed.has(e.verificationSource)) { res.excludedEvidence.push({ evidence: e, reason: "verification source not allowed by policy" }); continue; }
    if (policy.requireSourceMatch && e.sourceCommit !== expectedSource) { res.excludedEvidence.push({ evidence: e, reason: "source commit mismatch" }); continue; }
    if (policy.requireAllSignaturesValid && !e.signatureValid) { res.excludedEvidence.push({ evidence: e, reason: "invalid signature (policy requires all signatures valid)" }); continue; }
    eligible.push(e);
  }
  // One vote per (digest, independenceGroup).
  const used = new Map();
  const groups = new Map(); // digest -> evidence[]
  const groupSet = new Map(); // digest -> Set(groups)
  for (const e of eligible) {
    const k = `${e.artifactDigest}||${e.independenceGroup}`;
    if (used.has(k)) {
      res.excludedEvidence.push({ evidence: e, reason: `same independence_group '${e.independenceGroup}' already counted by '${used.get(k)}': not independent` });
      continue;
    }
    used.set(k, e.builderId);
    if (!groups.has(e.artifactDigest)) { groups.set(e.artifactDigest, []); groupSet.set(e.artifactDigest, new Set()); }
    groups.get(e.artifactDigest).push(e);
    groupSet.get(e.artifactDigest).add(e.independenceGroup);
  }
  if (eligible.length < policy.minBuilders) {
    res.decision = DECISIONS.INSUFFICIENT_EVIDENCE;
    res.reasons.push(`only ${eligible.length} eligible builders, need ${policy.minBuilders}`);
    return res;
  }
  const digests = [...groups.keys()].sort();
  let best = "", bestCount = 0;
  for (const d of digests) {
    const c = groupSet.get(d).size;
    if (c > bestCount) { bestCount = c; best = d; }
  }
  res.satisfied = bestCount;
  for (const d of digests) {
    if (d === best) continue;
    res.conflicts.push({ digest: d, builders: groups.get(d).map((e) => e.builderId).sort() });
  }
  if (best) res.countedEvidence = [...groups.get(best)].sort((a, b) => (a.builderId < b.builderId ? -1 : 1));
  const nConf = res.conflicts.length;
  const short = best.length > 16 ? best.slice(0, 16) + "..." : best;
  if (bestCount >= policy.requiredAgreement && bestCount >= policy.requiredIndependentGroups && nConf === 0) {
    res.decision = DECISIONS.VERIFIED;
    res.reasons.push(`${bestCount} independent groups agree on ${short}`);
  } else if (bestCount >= policy.requiredAgreement && bestCount >= policy.requiredIndependentGroups && nConf <= policy.conflictTolerance) {
    res.decision = DECISIONS.VERIFIED_WITH_CONFLICT;
    res.reasons.push(`${bestCount} independent groups agree on ${short} with ${nConf} conflicting group(s) tolerated`);
  } else if (bestCount < policy.requiredAgreement || bestCount < policy.requiredIndependentGroups) {
    if (nConf > 0 && bestCount > 0) {
      res.decision = DECISIONS.INVESTIGATE;
      res.reasons.push("no digest reaches required agreement; conflicting evidence requires investigation");
    } else {
      res.decision = DECISIONS.INSUFFICIENT_EVIDENCE;
      res.reasons.push(`best agreement ${bestCount} below required ${policy.requiredAgreement}`);
    }
  } else {
    res.decision = DECISIONS.INVESTIGATE;
    res.reasons.push(`conflicts (${nConf}) exceed tolerance (${policy.conflictTolerance})`);
  }
  return res;
}

// Canonical JSON: sorted keys, no whitespace (for hashing + audit chain).
export function canonical(obj) {
  if (obj === null || typeof obj !== "object") return JSON.stringify(obj);
  if (Array.isArray(obj)) return `[${obj.map(canonical).join(",")}]`;
  return `{${Object.keys(obj).sort().map((k) => `${JSON.stringify(k)}:${canonical(obj[k])}`).join(",")}}`;
}
