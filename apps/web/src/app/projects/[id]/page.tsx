"use client";
import Link from "next/link";
import { use, useEffect, useState } from "react";
import { ApiError, Empty, Footer, PageHeader, SkeletonRows } from "../../../components/ui";
import { createRelease, getProject, listBuilders } from "../../../lib/api";

export default function ProjectDetail({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [pr, setPr] = useState<Awaited<ReturnType<typeof getProject>> | null>(null);
  const [builders, setBuilders] = useState<Awaited<ReturnType<typeof listBuilders>>>([]);
  const [error, setError] = useState("");
  const [releaseId, setReleaseId] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const pid = decodeURIComponent(id);
    getProject(pid).then(setPr).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
    listBuilders().then(setBuilders).catch(() => setBuilders([]));
  }, [id]);

  async function verifyRelease() {
    if (!pr) return;
    setError(""); setBusy(true);
    try {
      const rel = await createRelease({
        package: pr.package || pr.repo, ecosystem: pr.ecosystem,
        name: pr.name, version: pr.version, repo: pr.repo, commit: pr.commit,
      });
      setReleaseId(rel.id);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  }

  const groups = new Set(builders.filter((b) => b.enabled).map((b) => b.independenceGroup));
  const ready = groups.size >= 2;

  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Project" title={pr ? pr.name : "Project"} sub={pr ? `${pr.repo} @ ${pr.commit}` : "Loading project…"} />
        {error ? <ApiError message={error} /> : null}
        <section className="section-card">
          {!pr && !error ? <SkeletonRows /> : !pr ? <Empty>Not found.</Empty> : (
            <dl className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
              {[["ID", pr.id], ["Repository", pr.repo], ["Tag", pr.tag || "— (raw SHA)"],
                ["Commit (immutable)", pr.commit], ["Ecosystem", pr.ecosystem || "—"],
                ["Build mode", `${pr.buildKind} (only supported mode)`],
                ["Package", `${pr.package || ""} ${pr.version || ""}`.trim() || "—"],
                ["Created", pr.createdAt ? new Date(pr.createdAt).toLocaleString() : "—"]].map(([k, v]) => (
                <div key={k} className="glass-soft p-3"><dt className="text-xs uppercase text-sky-800/70">{k}</dt><dd className="mono text-xs break-all mt-1">{v}</dd></div>
              ))}
            </dl>
          )}
        </section>
        <section className="section-card space-y-3" aria-label="verification prerequisites">
          <h2 className="font-black text-sky-950">Verify this release</h2>
          <p className="text-sm text-sky-900/70">
            {builders.length === 0
              ? "No builders registered yet — an administrator must register at least two builders in distinct independence groups (builders page) before verification can succeed."
              : ready
                ? `${builders.length} builders across ${groups.size} independence groups — quorum can proceed.`
                : `Only ${groups.size} independence group${groups.size === 1 ? "" : "s"} among ${builders.length} builders — register at least two distinct groups or verification cannot reach quorum.`}
          </p>
          <div className="flex gap-2 flex-wrap">
            {!releaseId ? (
              <button onClick={verifyRelease} disabled={busy || !pr} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
                {busy ? "registering…" : "verify release"}
              </button>
            ) : (
              <Link href={`/releases/${encodeURIComponent(releaseId)}`} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
                open release {releaseId.slice(0, 12)}…
              </Link>
            )}
            <Link href="/builders" className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">builders</Link>
          </div>
          <p className="text-xs text-sky-900/60">Builders then rebuild the pinned commit in isolation and submit signed evidence via workers; the quorum engine decides. These demo builders share one machine — same-machine agreement proves the protocol, not organizational independence.</p>
        </section>
        <Footer />
      </div>
    </main>
  );
}
