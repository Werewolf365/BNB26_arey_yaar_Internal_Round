"use client";
import { use, useEffect, useState } from "react";
import { ApiError, Badge, Empty, Footer, PageHeader, SkeletonRows } from "../../../components/ui";
import { getRelease } from "../../../lib/api";

export default function ReleaseDetail({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [rel, setRel] = useState<Awaited<ReturnType<typeof getRelease>> | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    getRelease(decodeURIComponent(id)).then(setRel).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, [id]);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Release" title={rel ? (rel.name || rel.package || "Release") : "Release"} sub={rel ? `${rel.repo} @ ${rel.commit}` : "Loading release…"} />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          {!rel && !error ? <SkeletonRows /> : !rel ? <Empty>Not found.</Empty> : (
            <dl className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
              {[["ID", rel.id], ["Package", `${rel.package ?? ""} ${rel.version ?? ""}`], ["Ecosystem", rel.ecosystem ?? "—"],
                ["Repository", rel.repo], ["Commit", rel.commit], ["Expected digest", rel.expectedDigest || "—"],
                ["Created", rel.createdAt ? new Date(rel.createdAt).toLocaleString() : "—"]].map(([k, v]) => (
                <div key={k} className="glass-soft p-3"><dt className="text-xs uppercase text-sky-800/70">{k}</dt><dd className="mono text-xs break-all mt-1">{v}</dd></div>
              ))}
              <div className="glass-soft p-3"><dt className="text-xs uppercase text-sky-800/70">Status</dt><dd className="mt-1"><Badge status={rel.status} /></dd></div>
            </dl>
          )}
        </section>
        <Footer />
      </div>
    </main>
  );
}
