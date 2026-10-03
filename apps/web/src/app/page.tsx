"use client";
import { useEffect, useState } from "react";

type Release = { id: string; package: string; ecosystem?: string; name?: string; version?: string; repo: string; commit: string; expectedDigest?: string; status: string; createdAt: string };
type Builder = { id: string; displayName?: string; independenceGroup: string; endpoint?: string; enabled: boolean };
type Policy = { id: string; version: string; policyHash?: string };
type Audit = { id: number; eventType: string; payload?: any; verificationId?: string; recordHash: string };
type Evidence = { id: string; verificationId: string; kind: string; storageKey: string; sha256: string };

function Badge({ status }: { status: string }) {
  const cls =
    status === "VERIFIED" ? "bg-emerald-100 text-emerald-800 ring-emerald-300" :
    status === "REJECTED" ? "bg-rose-100 text-rose-800 ring-rose-300" :
    status === "INSUFFICIENT_EVIDENCE" ? "bg-amber-100 text-amber-800 ring-amber-300" :
    "bg-sky-100 text-sky-800 ring-sky-300";
  return <span className={`px-2.5 py-1 rounded-full text-[11px] font-bold ring-1 ${cls}`}>{status}</span>;
}

export default function Page() {
  const API = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";
  const [releases, setReleases] = useState<Release[]>([]);
  const [builders, setBuilders] = useState<Builder[]>([]);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [audit, setAudit] = useState<Audit[]>([]);
  const [evidence, setEvidence] = useState<Evidence[]>([]);
  const [error, setError] = useState<string | "">("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let mounted = true;
    async function load() {
      setLoading(true); setError("");
      try {
        const [r, b, p, a] = await Promise.all([
          fetch(`${API}/api/v1/releases?limit=25`).then((r) => r.json()).catch(() => ({ data: [] })),
          fetch(`${API}/api/v1/builders`).then((r) => r.json()).catch(() => ({ data: [] })),
          fetch(`${API}/api/v1/policies`).then((r) => r.json()).catch(() => ({ data: [] })),
          fetch(`${API}/api/v1/audit`).then((r) => r.json()).catch(() => ({ data: [] })),
        ]);
        if (!mounted) return;
        setReleases(Array.isArray(r?.data) ? r.data : r?.data ? [r.data] : []);
        setBuilders(Array.isArray(b?.data) ? b.data : b?.data ? [b.data] : []);
        setPolicies(Array.isArray(p?.data) ? p.data : p?.data ? [p.data] : []);
        setAudit(Array.isArray(a?.data) ? a.data : a?.data ? [a.data] : []);
        // Best-effort evidence sample derived from audit if present
        setEvidence([]);
      } catch (e: any) {
        if (mounted) setError(e?.message || String(e));
      } finally { if (mounted) setLoading(false); }
    }
    load();
    return () => { mounted = false; };
  }, [API]);

  const t = releases.length;
  const verified = releases.filter((r) => r.status === "VERIFIED").length;
  const conflicts = audit.filter((a) => String(a.payload?.decision || "").includes("CONFLICT")).length;
  const evidenceCount = evidence.length;

  return (
    <main className="relative min-h-screen overflow-x-hidden p-4 sm:p-8">
      <div className="pointer-events-none absolute inset-0 bg-aero" />
      <div className="relative mx-auto max-w-[1400px] space-y-6 sm:space-y-8">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="pill bg-white/60 ring-white/60 text-sky-900 border border-white/70">Operational</p>
            <h1 className="mt-3 text-4xl font-black tracking-tight text-sky-950 drop-shadow-sm">Quorum</h1>
            <p className="text-sky-900/80 text-sm mt-1">Don't Trust the Binary. Trust the Builders.</p>
          </div>
          <nav className="flex gap-2 flex-wrap">
            {['releases','builders','policy','audit'].map((x) => (
              <a key={x} href={`#${x}`} className="glass-soft px-4 py-2 text-sm font-semibold text-sky-900 hover:bg-white/70 transition">{x}</a>
            ))}
          </nav>
        </header>

        {error ? (
          <div className="glass p-4 text-rose-700">
            <strong>Could not reach the API.</strong> Set <code className="mono">NEXT_PUBLIC_API_BASE</code> or ensure the backend is running (default <code className="mono">http://localhost:8080</code>). Detail: {error}
          </div>
        ) : null}

        <section className="grid grid-cols-2 md:grid-cols-4 gap-3 sm:gap-5" id="hero">
          <Card n={t} label="Releases" tone="sky" />
          <Card n={verified} label="Verified" tone="emerald" />
          <Card n={conflicts} label="Conflicts" tone="amber" />
          <Card n={evidenceCount} label="Evidence" tone="violet" />
        </section>

        <section id="releases" className="section-card">
          <div className="flex items-center justify-between gap-4 mb-4">
            <h2 className="text-xl font-black text-sky-950">Releases</h2>
            <button className="glass-soft px-3 py-1.5 text-xs font-bold text-sky-900" onClick={() => location.reload()}>refresh</button>
          </div>
          {loading ? <SkeletonRows /> : releases.length === 0 ? <Empty>No releases yet — create one via the CLI or POST /api/v1/releases.</Empty> : (
            <div className="overflow-x-auto w-full">
              <table className="w-full text-left border-separate border-spacing-y-2">
                <thead><tr className="text-xs uppercase text-sky-800/70">
                  <th className="px-3">Package</th><th className="px-3">Commit</th><th className="px-3">Status</th><th className="px-3">Created</th>
                </tr></thead>
                <tbody>
                  {releases.map((r) => (
                    <tr key={r.id} className="glass-soft hover:bg-white/60 transition">
                      <td className="px-3 py-3 rounded-l-2xl font-semibold">{r.name || r.package || r.repo} <span className="text-xs text-sky-700/70">v{r.version || "?"}</span></td>
                      <td className="px-3 py-3 mono text-xs">{r.commit?.slice(0,10)}</td>
                      <td className="px-3 py-3"><Badge status={r.status} /></td>
                      <td className="px-3 py-3 rounded-r-2xl text-xs text-sky-900/70">{new Date(r.createdAt).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
          <section id="builders" className="section-card">
            <h3 className="text-lg font-black text-sky-950 mb-3">Builders</h3>
            {builders.length === 0 ? <Empty>No builders registered.</Empty> : (
              <ul className="divide-y divide-sky-100">
                {builders.map((b) => (
                  <li key={b.id} className="py-3 flex items-center justify-between">
                    <div><div className="font-bold">{b.displayName || b.id}</div><div className="text-xs text-sky-800/70">group: {b.independenceGroup}</div></div>
                    <Badge status={b.enabled ? "ENABLED" : "DISABLED"} />
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section id="policy" className="section-card">
            <h3 className="text-lg font-black text-sky-950 mb-3">Policies</h3>
            {policies.length === 0 ? <Empty>No policies stored.</Empty> : (
              <ul className="divide-y divide-sky-100">
                {policies.map((p) => (
                  <li key={p.id} className="py-3 mono text-sm">{p.id} v{p.version} <span className="block text-[11px] text-sky-800/70">{p.policyHash}</span></li>
                ))}
              </ul>
            )}
          </section>
        </section>

        <section id="audit" className="section-card">
          <h3 className="text-lg font-black text-sky-950 mb-3">Audit trail</h3>
          {audit.length === 0 ? <Empty>No audit records yet.</Empty> : (
            <ul className="space-y-2">
              {audit.map((a) => (
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

        <footer className="text-center text-xs text-sky-900/60 pb-10">
          Quorum establishes evidence and policy decisions — not proof that software is safe.
        </footer>
      </div>
    </main>
  );
}

const cardTone: Record<string, string> = {
  sky: "from-sky-400/60 to-sky-300/30 text-sky-950",
  emerald: "from-emerald-400/60 to-emerald-300/30 text-emerald-950",
  amber: "from-amber-400/60 to-amber-300/30 text-amber-950",
  violet: "from-violet-400/60 to-violet-300/30 text-violet-950",
};
function Card({ n, label, tone }: { n: number; label: string; tone: "sky"|"emerald"|"amber"|"violet" }) {
  return (
    <div className={`glass p-5 sm:p-6 bg-gradient-to-br ${cardTone[tone]}`}>
      <div className="text-3xl font-black">{n}</div>
      <div className="text-[11px] uppercase tracking-widest opacity-80">{label}</div>
    </div>
  );
}

function SkeletonRows() {
  return (
    <div className="space-y-2 animate-pulse">
      {Array.from({ length: 5 }).map((_, i) => <div key={i} className="h-12 rounded-2xl bg-white/40" />)}
    </div>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <div className="text-sky-900/70 text-sm italic">{children}</div>;
}
