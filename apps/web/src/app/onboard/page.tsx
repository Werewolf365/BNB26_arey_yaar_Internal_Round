"use client";
import Link from "next/link";
import { useState } from "react";
import { Empty, Footer, PageHeader } from "../../components/ui";
import { createRelease, createProject, discoverRepo, resolveRef, type DiscoveredRepo, type Project, type ResolvedRef } from "../../lib/api";

type Phase = "url" | "ref" | "preview" | "done";

export default function OnboardPage() {
  const [repo, setRepo] = useState("");
  const [disc, setDisc] = useState<DiscoveredRepo | null>(null);
  const [ref, setRef] = useState("");
  const [customRef, setCustomRef] = useState(false);
  const [resolved, setResolved] = useState<ResolvedRef | null>(null);
  const [ecosystem, setEcosystem] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [project, setProject] = useState<Project | null>(null);
  const [releaseId, setReleaseId] = useState("");
  const [phase, setPhase] = useState<Phase>("url");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function run(fn: () => Promise<void>) {
    setErr(""); setLoading(true);
    try { await fn(); } catch (e: unknown) { setErr(e instanceof Error ? e.message : String(e)); }
    finally { setLoading(false); }
  }

  const doDiscover = () => run(async () => {
    if (!repo.trim()) { setErr("paste a repository URL first"); return; }
    const d = await discoverRepo(repo.trim());
    setDisc(d); setRef(d.tags[0] || ""); setResolved(null); setProject(null); setPhase("ref");
  });

  const doResolve = () => run(async () => {
    if (!ref.trim()) { setErr("select a tag or paste a commit SHA"); return; }
    const r = await resolveRef(repo.trim(), ref.trim(), ecosystem || undefined);
    setResolved(r);
    setEcosystem(r.ecosystem || "");
    setDisplayName((prev) => prev || r.displayName || r.repo);
    setPhase("preview");
  });

  const doCreate = () => run(async () => {
    if (!resolved) return;
    if (!ecosystem) { setErr("pick an ecosystem — it could not be detected"); return; }
    const p = await createProject({
      repo: repo.trim(), ref: ref.trim(), displayName: displayName.trim() || undefined,
      ecosystem, package: resolved.repo, version: resolved.tag || undefined,
    });
    setProject(p); setPhase("done");
  });

  const doVerify = () => run(async () => {
    if (!project) return;
    const rel = await createRelease({
      package: project.package || project.repo, ecosystem: project.ecosystem,
      name: project.name, version: project.version,
      repo: project.repo, commit: project.commit,
    });
    setReleaseId(rel.id);
  });

  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Onboard" title="Add a repository" sub="Paste a public GitHub URL, pick a release, review the pinned configuration — no JSON, no commit SHA typing." />
        <section className="section-card space-y-4" aria-label="repository onboarding">
          <div>
            <label className="text-xs uppercase text-sky-800/70" htmlFor="repo-url">Repository URL</label>
            <div className="flex gap-2 flex-wrap mt-1">
              <input id="repo-url" value={repo} onChange={(e) => setRepo(e.target.value)}
                placeholder="https://github.com/owner/repo"
                className="glass-soft px-3 py-2 text-sm mono flex-1 min-w-[240px]" />
              <button onClick={doDiscover} disabled={loading} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
                {loading && phase === "url" ? "checking…" : "inspect"}
              </button>
            </div>
          </div>
          {err ? <p className="text-rose-700 text-sm" role="alert">{err}</p> : null}

          {disc ? (
            <div className="space-y-3" aria-label="version selection">
              <p className="text-sm"><b>{disc.owner}/{disc.repo}</b> — {disc.tags.length} tags found.</p>
              {disc.tags.length === 0 ? <Empty>This repository has no tags — paste a full commit SHA below.</Empty> : null}
              <div className="flex gap-2 flex-wrap items-center">
                {!customRef ? (
                  <select value={ref} onChange={(e) => setRef(e.target.value)} aria-label="release tag"
                    className="glass-soft px-3 py-2 text-sm mono">
                    {disc.tags.map((t) => <option key={t} value={t}>{t}</option>)}
                  </select>
                ) : (
                  <input value={ref} onChange={(e) => setRef(e.target.value)} placeholder="full 40-hex commit SHA"
                    className="glass-soft px-3 py-2 text-sm mono flex-1 min-w-[240px]" aria-label="commit SHA" />
                )}
                <button onClick={() => setCustomRef(!customRef)} className="text-xs font-bold text-sky-900 underline">
                  {customRef ? "choose a tag instead" : "paste a commit SHA instead"}
                </button>
                <button onClick={doResolve} disabled={loading} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
                  {loading && phase === "ref" ? "resolving…" : "resolve"}
                </button>
              </div>
            </div>
          ) : null}

          {resolved ? (
            <div className="space-y-3" aria-label="configuration preview">
              <h3 className="font-black text-sky-950">Review pinned configuration</h3>
              <dl className="text-sm grid grid-cols-1 sm:grid-cols-2 gap-2">
                {[["Repository", `${resolved.owner}/${resolved.repo}`], ["Tag", resolved.tag || "— (raw SHA)"],
                  ["Commit (immutable)", resolved.commit],
                  ["Ecosystem", resolved.ecosystem || "undetected"],
                  ["Build mode", `${resolved.buildKind} (only supported mode)`]].map(([k, v]) => (
                  <div key={k} className="glass-soft p-3"><dt className="text-xs uppercase text-sky-800/70">{k}</dt><dd className="mono text-xs break-all mt-1">{v}</dd></div>
                ))}
              </dl>
              {!resolved.ecosystem ? (
                <label className="text-xs uppercase text-sky-800/70">Ecosystem (required — could not be detected)
                  <select value={ecosystem} onChange={(e) => setEcosystem(e.target.value)} aria-label="ecosystem"
                    className="glass-soft px-3 py-2 text-sm mono w-full mt-1">
                    <option value="">— select —</option>
                    {["go", "npm", "cargo", "pypi", "c"].map((e) => <option key={e} value={e}>{e}</option>)}
                  </select>
                </label>
              ) : null}
              <label className="text-xs uppercase text-sky-800/70">Display name
                <input value={displayName} onChange={(e) => setDisplayName(e.target.value)}
                  className="glass-soft px-3 py-2 text-sm w-full mt-1" aria-label="display name" />
              </label>
              <div className="text-xs text-sky-900/70">
                Quorum policy <b>{String(resolved.defaultPolicy.policyId)}</b>: minBuilders {String(resolved.defaultPolicy.minBuilders)},
                agreement {String(resolved.defaultPolicy.requiredAgreement)}, groups {String(resolved.defaultPolicy.requiredIndependentGroups)},
                signatures required, source pinned, tolerance {String(resolved.defaultPolicy.conflictTolerance)}.
                Builders are <b>not</b> auto-provisioned — register at least two in distinct groups before verifying.
              </div>
              <button onClick={doCreate} disabled={loading} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
                {loading && phase === "preview" ? "creating…" : "create project"}
              </button>
            </div>
          ) : null}

          {project ? (
            <div className="space-y-3" aria-label="project created">
              <p className="text-emerald-800 font-bold">Project created: <span className="mono text-sm">{project.id}</span></p>
              <div className="flex gap-2 flex-wrap">
                <Link href={`/projects/${encodeURIComponent(project.id)}`} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">open project</Link>
                {!releaseId ? (
                  <button onClick={doVerify} disabled={loading} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
                    {loading ? "registering…" : "verify release"}
                  </button>
                ) : (
                  <Link href={`/releases/${encodeURIComponent(releaseId)}`} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">open release {releaseId.slice(0, 12)}…</Link>
                )}
              </div>
              <p className="text-xs text-sky-900/70">“Verify release” registers the pinned release; builders then rebuild it via the worker flow (see runbook). No internal IDs to copy.</p>
            </div>
          ) : null}
        </section>
        <Footer />
      </div>
    </main>
  );
}
