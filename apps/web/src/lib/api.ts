const API = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export const apiBase = API;

export type Release = {
  id: string; package: string; ecosystem?: string; name?: string; version?: string;
  repo: string; commit: string; expectedDigest?: string; status: string; createdAt: string;
};
export type Builder = {
  id: string; displayName?: string; independenceGroup: string; endpoint?: string; enabled: boolean;
};
export type PolicyRecord = { id: string; version: string; policyHash?: string; body?: unknown };
export type Audit = {
  id: number; eventType: string; payload?: Record<string, unknown>;
  verificationId?: string; recordHash: string; previousHash?: string; createdAt?: string;
};
export type Evidence = {
  builderId: string; independenceGroup: string; sourceCommit: string;
  artifactDigest: string; signatureValid: boolean; verificationSource: string;
};
export type Conflict = { digest: string; builders: string[] };
export type Excluded = { evidence: Evidence; reason: string };
export type VerificationResult = {
  decision: string; required: number; satisfied: number;
  conflicts: Conflict[]; countedEvidence: Evidence[]; excludedEvidence: Excluded[]; reasons: string[];
};
export type Verification = {
  id: string; releaseId: string; policyId: string; decision: string;
  required: number; satisfied: number; result: VerificationResult;
  evidence: Evidence[]; expectedSource: string; createdAt: string;
};
export type BuildJob = {
  id: string; verificationId: string; builderId: string; status: string;
  attempts: number; digest?: string; commit?: string; errorCode?: string; errorDetail?: string;
};
export type EvidenceObject = {
  id: string; verificationId: string; kind: string; storageKey: string; sha256: string;
};
export type Anchor = {
  verificationId: string; txHash: string; blockNumber: string;
  chainId: string; contractAddress: string; eventFound: boolean;
};

type Data<T> = { data: T; requestId?: string };

async function get<T>(path: string): Promise<T> {
  const r = await fetch(`${API}${path}`, { cache: "no-store" });
  if (!r.ok) throw new Error(`${r.status} ${path}`);
  const j = (await r.json()) as Data<T>;
  return j.data;
}

export const listReleases = (limit = 25) => get<Release[]>(`/api/v1/releases?limit=${limit}`);
export const getRelease = (id: string) => get<Release>(`/api/v1/releases/${encodeURIComponent(id)}`);
export const getVerification = (id: string) => get<Verification>(`/api/v1/verifications/${encodeURIComponent(id)}`);
export const listBuilders = () => get<Builder[]>(`/api/v1/builders`);
export const listPolicies = () => get<PolicyRecord[]>(`/api/v1/policies`);
export const listAudit = (limit = 50) => get<Audit[]>(`/api/v1/audit?limit=${limit}`);
export const getAudit = (id: string) => get<Audit>(`/api/v1/audit/${encodeURIComponent(id)}`);
export const listJobs = (verificationId: string) =>
  get<BuildJob[]>(`/api/v1/verifications/${encodeURIComponent(verificationId)}/jobs`);
export const getEvidenceObject = (id: string) => get<EvidenceObject>(`/api/v1/evidence/${encodeURIComponent(id)}`);
export const getAttestationObject = (id: string) =>
  get<EvidenceObject>(`/api/v1/attestations/${encodeURIComponent(id)}`);
export const getAnchor = (verificationId: string) =>
  get<Anchor>(`/api/v1/blockchain/${encodeURIComponent(verificationId)}`);
export type AnchorRead = {
  anchored: boolean; chainId?: string; contract?: string;
  txHash?: string; blockNumber?: string; eventFound?: boolean;
};
export const lookupAnchor = (verificationId: string, rpc: string, contract: string) =>
  get<AnchorRead>(`/api/v1/blockchain/lookup/${encodeURIComponent(verificationId)}?rpc=${encodeURIComponent(rpc)}&contract=${encodeURIComponent(contract)}`);
export const evidenceBlobUrl = (id: string) => `${API}/api/v1/evidence/${encodeURIComponent(id)}/blob`;

export function asArray<T>(v: T[] | T | undefined): T[] {
  if (Array.isArray(v)) return v;
  if (v) return [v];
  return [];
}
