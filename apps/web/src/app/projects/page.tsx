"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { ApiError, Empty, Footer, PageHeader, SkeletonRows } from "../../components/ui";
import { asArray, listProjects } from "../../lib/api";

export default function ProjectsPage() {
  const [rows, setRows] = useState<Awaited<ReturnType<typeof listProjects>>>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    listProjects().then((r) => setRows(asArray(r))).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))).finally(() => setLoading(false));
  }, []);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Projects" title="Projects" sub="Onboarded repositories pinned to immutable commits." />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          <div className="mb-4">
            <Link href="/onboard" className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">+ onboard a repository</Link>
          </div>
          {loading ? <SkeletonRows /> : rows.length === 0 ? <Empty>No projects yet — onboard your first repository.</Empty> : (
            <ul className="space-y-2">
              {rows.map((p) => (
                <li key={p.id} className="glass-soft p-4 flex flex-wrap items-center gap-3 justify-between">
                  <div>
                    <Link href={`/projects/${encodeURIComponent(p.id)}`} className="font-bold underline decoration-sky-300 underline-offset-2">{p.name}</Link>
                    <div className="mono text-xs text-sky-800/70">{p.repo} @ {(p.commit || "").slice(0, 12)} · {p.ecosystem || "?"}</div>
                  </div>
                  <span className="mono text-[11px] text-sky-800/70">{p.tag || "pinned"}</span>
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
