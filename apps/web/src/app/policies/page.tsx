"use client";
import { useEffect, useState } from "react";
import { ApiError, Empty, Footer, PageHeader, SkeletonRows } from "../../components/ui";
import { asArray, listPolicies } from "../../lib/api";

export default function PoliciesPage() {
  const [rows, setRows] = useState<Awaited<ReturnType<typeof listPolicies>>>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    listPolicies().then((r) => setRows(asArray(r))).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))).finally(() => setLoading(false));
  }, []);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Policy" title="Policies" sub="Quorum rules: thresholds, independence, signature and source requirements." />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          {loading ? <SkeletonRows /> : rows.length === 0 ? <Empty>No policies stored.</Empty> : (
            <ul className="space-y-3">
              {rows.map((p) => (
                <li key={p.id} className="glass-soft p-4">
                  <div className="font-bold mono text-sm">{p.id} <span className="text-sky-800/70">v{p.version}</span></div>
                  <div className="mono text-[11px] text-sky-800/70 break-all mt-1">hash {p.policyHash || "—"}</div>
                  {p.body !== undefined && <pre className="mono text-[11px] mt-2 overflow-x-auto">{JSON.stringify(p.body, null, 2)}</pre>}
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
