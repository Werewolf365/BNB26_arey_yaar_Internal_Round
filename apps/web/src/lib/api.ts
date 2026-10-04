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
  const j = (await r.json().catch(() => null)) as (Data<T> & { error?: { code: string; message: string } }) | null;
  if (!r.ok) throw new Error(j?.error ? `${j.error.code}: ${j.error.message}` : `${r.status} ${path}`);
  if (!j) throw new Error(`empty response ${path}`);
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

async function post<T>(path: string, body: unknown): Promise<T> {
  const r = await fetch(`${API}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  const j = (await r.json()) as Data<T> & { error?: { code: string; message: string } };
  if (!r.ok) throw new Error(j.error ? `${j.error.code}: ${j.error.message}` : `${r.status} ${path}`);
  return j.data;
}

export type DiscoveredRepo = { owner: string; repo: string; tags: string[] };
export type ResolvedRef = {
  owner: string; repo: string; tag: string; commit: string;
  ecosystem: string; buildKind: string; displayName: string;
  defaultPolicy: Record<string, unknown>;
};
export type Project = {
  id: string; name: string; description?: string; repo: string; tag: string;
  commit: string; package?: string; ecosystem?: string; version?: string;
  buildKind: string; createdAt: string;
};

export const discoverRepo = (repo: string) =>
  get<DiscoveredRepo>(`/api/v1/onboarding/discover?repo=${encodeURIComponent(repo)}`);
export const resolveRef = (repo: string, ref: string, ecosystem?: string) =>
  post<ResolvedRef>(`/api/v1/onboarding/resolve`, { repo, ref, ecosystem: ecosystem || "" });
export const createProject = (input: {
  repo: string; ref: string; displayName?: string; description?: string;
  ecosystem?: string; package?: string; version?: string;
}) => post<Project>(`/api/v1/projects`, input);
export const listProjects = () => get<Project[]>(`/api/v1/projects`);
export const getProject = (id: string) => get<Project>(`/api/v1/projects/${encodeURIComponent(id)}`);
export const createRelease = (input: {
  package?: string; ecosystem?: string; name?: string; version?: string;
  repo: string; commit: string; expectedDigest?: string;
}) => post<Release>(`/api/v1/releases`, input);

export function asArray<T>(v: T[] | T | undefined): T[] {
  if (Array.isArray(v)) return v;
  if (v) return [v];
  return [];
}
