"use client";
import { useEffect, useState } from "react";
import { ApiError, Empty, Footer, PageHeader, SkeletonRows } from "../../components/ui";
import { asArray, listAudit } from "../../lib/api";

export default function AuditPage() {
  const [rows, setRows] = useState<Awaited<ReturnType<typeof listAudit>>>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    listAudit(100).then((r) => setRows(asArray(r))).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))).finally(() => setLoading(false));
  }, []);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Audit" title="Audit trail" sub="Append-only hash-chained records. Tampering breaks the chain." />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          {loading ? <SkeletonRows /> : rows.length === 0 ? <Empty>No audit records yet.</Empty> : (
            <ul className="space-y-2">
              {rows.map((a) => (
                <li key={a.id} className="glass-soft p-3">
                  <div className="flex items-center justify-between text-sm gap-2">
                    <span className="font-bold">{a.eventType}</span>
                    <span className="mono text-[11px] text-sky-800/70">#{a.id}</span>
                  </div>
                  <div className="mono text-[11px] text-sky-800/70 break-all mt-1">hash {a.recordHash}</div>
                  {a.payload !== undefined && <pre className="mono text-[11px] mt-1 overflow-x-auto">{JSON.stringify(a.payload)}</pre>}
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
