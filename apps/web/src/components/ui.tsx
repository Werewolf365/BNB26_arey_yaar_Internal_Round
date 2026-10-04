import Link from "next/link";
import type { ReactNode } from "react";

export function Badge({ status }: { status: string }) {
  const cls =
    status === "VERIFIED" ? "bg-emerald-100 text-emerald-800 ring-emerald-300" :
    status === "REJECTED" ? "bg-rose-100 text-rose-800 ring-rose-300" :
    status === "INSUFFICIENT_EVIDENCE" ? "bg-amber-100 text-amber-800 ring-amber-300" :
    status === "ENABLED" ? "bg-emerald-100 text-emerald-800 ring-emerald-300" :
    status === "DISABLED" ? "bg-slate-200 text-slate-600 ring-slate-300" :
    "bg-sky-100 text-sky-800 ring-sky-300";
  return <span className={`px-2.5 py-1 rounded-full text-[11px] font-bold ring-1 ${cls}`}>{status}</span>;
}

export function PageHeader({ kicker, title, sub }: { kicker: string; title: string; sub: string }) {
  return (
    <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <p className="pill bg-white/60 ring-white/60 text-sky-900 border border-white/70">{kicker}</p>
        <h1 className="mt-3 text-4xl font-black tracking-tight text-sky-950 drop-shadow-sm">{title}</h1>
        <p className="text-sky-900/80 text-sm mt-1">{sub}</p>
      </div>
      <nav className="flex gap-2 flex-wrap" aria-label="Sections">
        {[["/", "overview"], ["/releases", "releases"], ["/builders", "builders"], ["/policies", "policies"], ["/audit", "audit"], ["/evidence", "evidence"], ["/blockchain", "blockchain"]].map(([href, x]) => (
          <Link key={x} href={href} className="glass-soft px-4 py-2 text-sm font-semibold text-sky-900 hover:bg-white/70 transition">{x}</Link>
        ))}
      </nav>
    </header>
  );
}

export function Card({ n, label, tone }: { n: number; label: string; tone: "sky" | "emerald" | "amber" | "violet" }) {
  const tones: Record<string, string> = {
    sky: "from-sky-400/60 to-sky-300/30 text-sky-950",
    emerald: "from-emerald-400/60 to-emerald-300/30 text-emerald-950",
    amber: "from-amber-400/60 to-amber-300/30 text-amber-950",
    violet: "from-violet-400/60 to-violet-300/30 text-violet-950",
  };
  return (
    <div className={`glass p-5 sm:p-6 bg-gradient-to-br ${tones[tone]}`}>
      <div className="text-3xl font-black">{n}</div>
      <div className="text-[11px] uppercase tracking-widest opacity-80">{label}</div>
    </div>
  );
}

export function SkeletonRows() {
  return (
    <div className="space-y-2 animate-pulse" aria-label="loading">
      {Array.from({ length: 5 }).map((_, i) => <div key={i} className="h-12 rounded-2xl bg-white/40" />)}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="text-sky-900/70 text-sm italic">{children}</div>;
}

export function ApiError({ message }: { message: string }) {
  return (
    <div className="glass p-4 text-rose-700" role="alert">
      <strong>Could not reach the API.</strong> Ensure the backend is running
      (default <code className="mono">http://localhost:8080</code>). Detail: {message}
    </div>
  );
}

export function Footer() {
  return (
    <footer className="text-center text-xs text-sky-900/60 pb-10">
      Quorum establishes evidence and policy decisions — not proof that software is safe.
    </footer>
  );
}
