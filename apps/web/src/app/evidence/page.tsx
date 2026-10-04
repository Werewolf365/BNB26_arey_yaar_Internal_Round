"use client";
import { useState } from "react";
import { Empty, Footer, PageHeader } from "../../components/ui";
import { evidenceBlobUrl, getAttestationObject, getEvidenceObject, type EvidenceObject } from "../../lib/api";

export default function EvidencePage() {
  const [id, setId] = useState("");
  const [obj, setObj] = useState<EvidenceObject | null>(null);
  const [err, setErr] = useState("");
  async function lookup(attestation: boolean) {
    setErr(""); setObj(null);
    if (!id.trim()) { setErr("enter an evidence or attestation id"); return; }
    try { setObj(attestation ? await getAttestationObject(id.trim()) : await getEvidenceObject(id.trim())); }
    catch (e: unknown) { setErr(e instanceof Error ? e.message : String(e)); }
  }
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Evidence" title="Evidence explorer" sub="Inspect raw stored evidence objects and download hash-verified blobs." />
        <section className="section-card space-y-3">
          <div className="flex gap-2 flex-wrap">
            <input value={id} onChange={(e) => setId(e.target.value)} placeholder="evidence / attestation id"
              className="glass-soft px-3 py-2 text-sm mono flex-1 min-w-[240px]" aria-label="evidence id" />
            <button onClick={() => lookup(false)} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">evidence</button>
            <button onClick={() => lookup(true)} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">attestation</button>
          </div>
          {err ? <p className="text-rose-700 text-sm" role="alert">{err}</p> : null}
          {!obj && !err ? <Empty>Blobs are content-addressed (sha256/…) and re-verified on every read.</Empty> : null}
          {obj ? (
            <dl className="text-sm grid grid-cols-1 sm:grid-cols-2 gap-2">
              {[["ID", obj.id], ["Verification", obj.verificationId], ["Kind", obj.kind], ["Key", obj.storageKey], ["SHA-256", obj.sha256]].map(([k, v]) => (
                <div key={k} className="glass-soft p-3"><dt className="text-xs uppercase text-sky-800/70">{k}</dt><dd className="mono text-xs break-all mt-1">{v}</dd></div>
              ))}
              <div className="glass-soft p-3 sm:col-span-2">
                <a className="font-bold underline decoration-sky-300 underline-offset-2" href={evidenceBlobUrl(obj.id)} download>download blob (hash-verified)</a>
              </div>
            </dl>
          ) : null}
        </section>
        <Footer />
      </div>
    </main>
  );
}
