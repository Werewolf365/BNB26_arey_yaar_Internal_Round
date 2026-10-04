"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { ApiError, Badge, Card, Empty, Footer, PageHeader, SkeletonRows } from "../components/ui";
import { asArray, getVerification, listAudit, listBuilders, listPolicies, listReleases, type Verification } from "../lib/api";

export default function Page() {
  const [releases, setReleases] = useState<Awaited<ReturnType<typeof listReleases>>>([]);
  const [builders, setBuilders] = useState<Awaited<ReturnType<typeof listBuilders>>>([]);
  const [policies, setPolicies] = useState<Awaited<ReturnType<typeof listPolicies>>>([]);
  const [audit, setAudit] = useState<Awaited<ReturnType<typeof listAudit>>>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [vid, setVid] = useState("");
  const [vex, setVex] = useState<Verification | null>(null);
  const [verr, setVerr] = useState("");

  useEffect(() => {
    let mounted = true;
    (async () => {
      try {
        const [r, b, p, a] = await Promise.all([listReleases(), listBuilders(), listPolicies(), listAudit(50)]);
        if (!mounted) return;
        setReleases(asArray(r)); setBuilders(asArray(b)); setPolicies(asArray(p)); setAudit(asArray(a));
      } catch (e: unknown) {
        if (mounted) setError(e instanceof Error ? e.message : String(e));
      } finally { if (mounted) setLoading(false); }
    })();
    return () => { mounted = false; };
  }, []);

  async function inspect() {
    setVerr(""); setVex(null);
    if (!vid.trim()) { setVerr("enter a verification id"); return; }
    try { setVex(await getVerification(vid.trim())); }
    catch (e: unknown) { setVerr(e instanceof Error ? e.message : String(e)); }
  }

  const verified = releases.filter((r) => r.status === "VERIFIED").length;
  const conflicts = audit.filter((a) => String((a.payload as Record<string, unknown> | undefined)?.decision ?? "").includes("CONFLICT")).length;

  return (
    <main className="relative min-h-screen overflow-x-hidden p-4 sm:p-8">
      <div className="pointer-events-none absolute inset-0 bg-aero" />
      <div className="relative mx-auto max-w-[1400px] space-y-6 sm:space-y-8">
        <PageHeader kicker="Operational" title="Quorum" sub="Don't Trust the Binary. Trust the Builders." />
        {error ? <ApiError message={error} /> : null}

        <section className="grid grid-cols-2 md:grid-cols-4 gap-3 sm:gap-5" aria-label="summary">
          <Card n={releases.length} label="Releases" tone="sky" />
          <Card n={verified} label="Verified" tone="emerald" />
          <Card n={conflicts} label="Conflicts" tone="amber" />
          <Card n={builders.length} label="Builders" tone="violet" />
        </section>

        <section id="releases" className="section-card">
          <div className="flex items-center justify-between gap-4 mb-4">
            <h2 className="text-xl font-black text-sky-950">Releases</h2>
            <Link href="/releases" className="glass-soft px-3 py-1.5 text-xs font-bold text-sky-900">view all</Link>
          </div>
          {loading ? <SkeletonRows /> : releases.length === 0 ? <Empty>No releases yet — create one via the CLI or POST /api/v1/releases.</Empty> : (
            <div className="overflow-x-auto w-full">
              <table className="w-full text-left border-separate border-spacing-y-2">
                <thead><tr className="text-xs uppercase text-sky-800/70">
                  <th className="px-3">Package</th><th className="px-3">Commit</th><th className="px-3">Status</th><th className="px-3">Created</th>
                </tr></thead>
                <tbody>
                  {releases.slice(0, 10).map((r) => (
                    <tr key={r.id} className="glass-soft hover:bg-white/60 transition">
                      <td className="px-3 py-3 rounded-l-2xl font-semibold">
                        <Link href={`/releases/${encodeURIComponent(r.id)}`} className="underline decoration-sky-300 underline-offset-2">
                          {r.name || r.package || r.repo}
                        </Link> <span className="text-xs text-sky-700/70">v{r.version || "?"}</span>
                      </td>
                      <td className="px-3 py-3 mono text-xs">{r.commit?.slice(0, 10)}</td>
                      <td className="px-3 py-3"><Badge status={r.status} /></td>
                      <td className="px-3 py-3 rounded-r-2xl text-xs text-sky-900/70">{r.createdAt ? new Date(r.createdAt).toLocaleString() : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="section-card" aria-label="verification lookup">
          <h2 className="text-xl font-black text-sky-950 mb-3">Verification evidence</h2>
          <p className="text-sm text-sky-900/70 mb-3">Paste a verification id to inspect its decision, counted/excluded evidence and conflicts.</p>
          <div className="flex gap-2 flex-wrap">
            <input value={vid} onChange={(e) => setVid(e.target.value)} placeholder="verification id"
              className="glass-soft px-3 py-2 text-sm mono flex-1 min-w-[240px]" aria-label="verification id" />
            <button onClick={inspect} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">inspect</button>
            {vex ? <Link href={`/verifications/${encodeURIComponent(vex.id)}`} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">open detail</Link> : null}
          </div>
          {verr ? <p className="text-rose-700 text-sm mt-2" role="alert">{verr}</p> : null}
          {vex ? (
            <div className="mt-4 space-y-2 text-sm">
              <div className="flex items-center gap-3"><Badge status={vex.decision} /><span className="mono text-xs">{vex.id}</span></div>
              <p>Required {vex.required}, satisfied {vex.satisfied}; counted {vex.result?.countedEvidence?.length ?? 0},
                excluded {vex.result?.excludedEvidence?.length ?? 0}, conflicts {vex.result?.conflicts?.length ?? 0}.</p>
            </div>
          ) : null}
        </section>

        <section className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
          <section id="builders" className="section-card">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-lg font-black text-sky-950">Builders</h3>
              <Link href="/builders" className="text-xs font-bold text-sky-900">view all</Link>
            </div>
            {builders.length === 0 ? <Empty>No builders registered.</Empty> : (
              <ul className="divide-y divide-sky-100">
                {builders.slice(0, 6).map((b) => (
                  <li key={b.id} className="py-3 flex items-center justify-between">
                    <div><div className="font-bold">{b.displayName || b.id}</div><div className="text-xs text-sky-800/70">group: {b.independenceGroup}</div></div>
                    <Badge status={b.enabled ? "ENABLED" : "DISABLED"} />
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section id="policy" className="section-card">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-lg font-black text-sky-950">Policies</h3>
              <Link href="/policies" className="text-xs font-bold text-sky-900">view all</Link>
            </div>
            {policies.length === 0 ? <Empty>No policies stored.</Empty> : (
              <ul className="divide-y divide-sky-100">
                {policies.slice(0, 6).map((p) => (
                  <li key={p.id} className="py-3 mono text-sm">{p.id} v{p.version} <span className="block text-[11px] text-sky-800/70">{p.policyHash}</span></li>
                ))}
              </ul>
            )}
          </section>
        </section>

        <section id="audit" className="section-card">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-lg font-black text-sky-950">Audit trail</h3>
            <Link href="/audit" className="text-xs font-bold text-sky-900">view all</Link>
          </div>
          {audit.length === 0 ? <Empty>No audit records yet.</Empty> : (
            <ul className="space-y-2">
              {audit.slice(0, 10).map((a) => (
                <li key={a.id} className="glass-soft p-3 rounded-xl">
                  <div className="flex items-center justify-between text-sm">
                    <span className="font-bold">{a.eventType}</span>
                    <span className="mono text-[11px] text-sky-800/70">#{a.id}</span>
                  </div>
                  <div className="mono text-[11px] text-sky-800/70 break-all">{a.recordHash}</div>
                </li>
              ))}
            </ul>
          )}
        </section>

        <Footer />
      </div>
    </main>
  );
}
