"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { ApiError, Badge, Empty, Footer, PageHeader, SkeletonRows } from "../../components/ui";
import { asArray, listReleases } from "../../lib/api";

export default function ReleasesPage() {
  const [releases, setReleases] = useState<Awaited<ReturnType<typeof listReleases>>>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    listReleases(100).then((r) => setReleases(asArray(r))).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e))).finally(() => setLoading(false));
  }, []);
  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[1100px] space-y-6">
        <PageHeader kicker="Releases" title="Releases" sub="Every pinned source identity under verification." />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          {loading ? <SkeletonRows /> : releases.length === 0 ? <Empty>No releases yet.</Empty> : (
            <ul className="space-y-2">
              {releases.map((r) => (
                <li key={r.id} className="glass-soft p-4 flex flex-wrap items-center gap-3 justify-between">
                  <div>
                    <Link href={`/releases/${encodeURIComponent(r.id)}`} className="font-bold underline decoration-sky-300 underline-offset-2">
                      {r.name || r.package || r.repo} v{r.version || "?"}
                    </Link>
                    <div className="mono text-xs text-sky-800/70">{r.repo} @ {r.commit?.slice(0, 12)} · {r.ecosystem || "?"}</div>
                  </div>
                  <Badge status={r.status} />
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
