// Append-only audit hash chain: record_hash = H(prev_hash || canonical(payload)).
import { createHash } from "node:crypto";
import { canonical } from "../policy/quorum.mjs";

export function hashRecord(prevHash, payload) {
  const h = createHash("sha256");
  h.update(prevHash, "utf8");
  h.update(canonical(payload), "utf8");
  return h.digest("hex");
}

export function appendRecord(chain, eventType, payload) {
  const prev = chain.length ? chain[chain.length - 1].recordHash : "GENESIS";
  const recordHash = hashRecord(prev, { eventType, ...payload });
  const rec = { index: chain.length, eventType, payload, previousHash: prev, recordHash };
  chain.push(rec);
  return rec;
}

export function verifyChain(chain) {
  let prev = "GENESIS";
  for (let i = 0; i < chain.length; i++) {
    const r = chain[i];
    if (r.previousHash !== prev) return { ok: false, brokenAt: i, reason: `previous hash mismatch at record ${i}` };
    const recomputed = hashRecord(r.previousHash, { eventType: r.eventType, ...r.payload });
    if (recomputed !== r.recordHash) return { ok: false, brokenAt: i, reason: `payload modified at record ${i}` };
    prev = r.recordHash;
  }
  return { ok: true };
}
