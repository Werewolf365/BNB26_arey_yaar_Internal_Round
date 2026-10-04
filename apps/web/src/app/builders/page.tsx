"use client";
import { useEffect, useState } from "react";
import { ApiError, Badge, Empty, Footer, PageHeader, SkeletonRows } from "../../components/ui";
import { asArray, listBuilders } from "../../lib/api";

export default function BuildersPage() {
  const [rows, setRows] = useState<Awaited<ReturnType<typeof listBuilders>>>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    listBuilders().then((r) => setRows(asArray(r))).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))).finally(() => setLoading(false));
  }, []);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Builders" title="Builders" sub="Registered rebuild workers and their independence groups." />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          {loading ? <SkeletonRows /> : rows.length === 0 ? <Empty>No builders registered.</Empty> : (
            <ul className="divide-y divide-sky-100">
              {rows.map((b) => (
                <li key={b.id} className="py-4">
                  <div className="flex items-center justify-between gap-2">
                    <div className="font-bold">{b.displayName || b.id}</div>
                    <Badge status={b.enabled ? "ENABLED" : "DISABLED"} />
                  </div>
                  <div className="text-xs text-sky-800/70 mono mt-1">id {b.id} · group {b.independenceGroup}{b.endpoint ? ` · ${b.endpoint}` : ""}</div>
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
