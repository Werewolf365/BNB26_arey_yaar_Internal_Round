"use client";
import { use, useEffect, useState } from "react";
import Link from "next/link";
import { ApiError, Badge, Empty, Footer, PageHeader } from "../../../components/ui";
import { evidenceBlobUrl, getAnchor, getVerification, listJobs, type Anchor, type BuildJob, type Conflict, type Evidence, type Excluded, type Verification } from "../../../lib/api";

function EvTable({ rows }: { rows: Evidence[] | undefined }) {
  if (!rows || rows.length === 0) return <p className="text-sm italic text-sky-900/70">No evidence recorded.</p>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-xs">
        <thead><tr className="uppercase text-sky-800/70">
          <th className="px-2 py-1">Builder</th><th className="px-2 py-1">Group</th><th className="px-2 py-1">Digest</th><th className="px-2 py-1">Sig</th><th className="px-2 py-1">Source</th>
        </tr></thead>
        <tbody>
          {rows.map((e: Evidence, i: number) => (
            <tr key={i} className="border-t border-sky-100">
              <td className="px-2 py-2 font-bold">{e.builderId}</td>
              <td className="px-2 py-2">{e.independenceGroup}</td>
              <td className="px-2 py-2 mono break-all">{e.artifactDigest}</td>
              <td className="px-2 py-2">{e.signatureValid ? "valid" : "INVALID"}</td>
              <td className="px-2 py-2">{e.verificationSource}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function VerificationDetail({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [v, setV] = useState<Verification | null>(null);
  const [jobs, setJobs] = useState<BuildJob[]>([]);
  const [anchor, setAnchor] = useState<Anchor | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const vid = decodeURIComponent(id);
    getVerification(vid).then(setV).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
    listJobs(vid).then(setJobs).catch(() => setJobs([]));
    getAnchor(vid).then(setAnchor).catch(() => setAnchor(null));
  }, [id]);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[1100px] space-y-6">
        <PageHeader kicker="Verification" title={v ? v.decision : "Verification"} sub={v ? v.id : "Loading…"} />
        {error ? <ApiError message={error} /> : null}
        {!v && !error ? <p className="section-card">Loading…</p> : v ? (
          <>
            <section className="section-card space-y-2 text-sm" aria-label="decision">
              <div className="flex items-center gap-3 flex-wrap"><Badge status={v.decision} /><span>required {v.required}, satisfied {v.satisfied}</span></div>
              <p className="mono text-xs break-all">release {v.releaseId} · policy {v.policyId} · source {v.expectedSource}</p>
              {(v.result?.reasons ?? []).length > 0 && (
                <ul className="list-disc ml-5">{v.result.reasons.map((r: string, i: number) => <li key={i}>{r}</li>)}</ul>
              )}
            </section>
            {(v.result?.conflicts ?? []).length > 0 && (
              <section className="section-card" aria-label="conflicts">
                <h2 className="font-black mb-2">Conflicts ({v.result.conflicts.length})</h2>
                {v.result.conflicts.map((c: Conflict, i: number) => (
                  <div key={i} className="glass-soft p-3 text-sm mb-2">
                    <div className="mono text-xs break-all">{c.digest}</div>
                    <div>minority builders: {c.builders.join(", ")}</div>
                  </div>
                ))}
              </section>
            )}
            <section className="section-card" aria-label="counted evidence">
              <h2 className="font-black mb-2">Counted evidence ({v.result?.countedEvidence?.length ?? 0})</h2>
              <EvTable rows={v.result?.countedEvidence ?? []} />
            </section>
            {(v.result?.excludedEvidence ?? []).length > 0 && (
              <section className="section-card" aria-label="excluded evidence">
                <h2 className="font-black mb-2">Excluded evidence ({v.result.excludedEvidence.length})</h2>
                {v.result.excludedEvidence.map((x: Excluded, i: number) => (
                  <div key={i} className="glass-soft p-3 text-sm mb-2">
                    <span className="font-bold">{x.evidence.builderId}</span> — {x.reason}
                    <div className="mono text-xs break-all">{x.evidence.artifactDigest}</div>
                  </div>
                ))}
              </section>
            )}
            {jobs.length > 0 && (
              <section className="section-card" aria-label="builder jobs">
                <h2 className="font-black mb-2">Builder jobs ({jobs.length})</h2>
                <ul className="space-y-2 text-sm">
                  {jobs.map((j) => (
                    <li key={j.id} className="glass-soft p-3 flex flex-wrap gap-2 justify-between">
                      <span><b>{j.builderId}</b> <span className="mono text-xs">{j.id}</span></span>
                      <Badge status={j.status} />
                    </li>
                  ))}
                </ul>
              </section>
            )}
            <section className="section-card" aria-label="blockchain anchor">
              <div className="flex items-center justify-between mb-2">
                <h2 className="font-black">Blockchain anchor</h2>
                <Link href="/blockchain" className="text-xs font-bold text-sky-900">open explorer</Link>
              </div>
              {!anchor ? <Empty>Not anchored (or anchor lookup failed).</Empty> : (
                <dl className="text-sm grid grid-cols-1 sm:grid-cols-2 gap-2">
                  <div className="glass-soft p-3"><dt className="text-xs uppercase">tx</dt><dd className="mono text-xs break-all">{anchor.txHash}</dd></div>
                  <div className="glass-soft p-3"><dt className="text-xs uppercase">block</dt><dd className="mono text-xs">{anchor.blockNumber}</dd></div>
                  <div className="glass-soft p-3"><dt className="text-xs uppercase">chain</dt><dd className="mono text-xs">{anchor.chainId}</dd></div>
                  <div className="glass-soft p-3"><dt className="text-xs uppercase">contract</dt><dd className="mono text-xs break-all">{anchor.contractAddress}</dd></div>
                </dl>
              )}
            </section>
            <p className="text-xs text-sky-900/60">Raw evidence blobs: POST /api/v1/evidence returns an id; download at {evidenceBlobUrl(":id")}.</p>
          </>
        ) : <Empty>Not found.</Empty>}
        <Footer />
      </div>
    </main>
  );
}
