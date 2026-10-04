"use client";
import { useState } from "react";
import { Empty, Footer, PageHeader } from "../../components/ui";
import { getAnchor, lookupAnchor, type Anchor, type AnchorRead } from "../../lib/api";

export default function BlockchainPage() {
  const [rpc, setRpc] = useState("http://localhost:8545");
  const [contract, setContract] = useState("");
  const [vid, setVid] = useState("");
  const [read, setRead] = useState<AnchorRead | null>(null);
  const [stored, setStored] = useState<Anchor | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function inspect() {
    setErr(""); setRead(null); setStored(null);
    if (!contract.trim() || !vid.trim()) { setErr("contract address and verification id are required"); return; }
    setLoading(true);
    try {
      setRead(await lookupAnchor(vid.trim(), rpc.trim() || "http://localhost:8545", contract.trim()));
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally { setLoading(false); }
    // Best effort: show the API-stored anchor row too (present when the
    // anchor was submitted through the API with a matching verification id).
    try { setStored(await getAnchor(vid.trim())); } catch { /* absent is fine */ }
  }

  return (
    <main className="relative min-h-screen p-4 sm:p-8">
      <div className="relative mx-auto max-w-[900px] space-y-6">
        <PageHeader kicker="Local demo chain" title="Blockchain explorer" sub="Independent re-read of QuorumAnchor state and events. Local Anvil transactions — not public-chain transactions." />
        <section className="section-card space-y-3" aria-label="anchor lookup">
          <div className="grid grid-cols-1 gap-2">
            <label className="text-xs uppercase text-sky-800/70">RPC endpoint (must be reachable FROM the API server — use http://anvil:8545 when the API runs in compose)
              <input value={rpc} onChange={(e) => setRpc(e.target.value)}
                className="glass-soft px-3 py-2 text-sm mono w-full mt-1" aria-label="rpc endpoint" />
            </label>
            <label className="text-xs uppercase text-sky-800/70">Contract address
              <input value={contract} onChange={(e) => setContract(e.target.value)} placeholder="0x…"
                className="glass-soft px-3 py-2 text-sm mono w-full mt-1" aria-label="contract address" />
            </label>
            <label className="text-xs uppercase text-sky-800/70">Verification id (bytes32)
              <input value={vid} onChange={(e) => setVid(e.target.value)} placeholder="0x…64 hex chars"
                className="glass-soft px-3 py-2 text-sm mono w-full mt-1" aria-label="verification id" />
            </label>
          </div>
          <button onClick={inspect} disabled={loading} className="glass-soft px-4 py-2 text-sm font-bold text-sky-900">
            {loading ? "reading…" : "re-read anchor"}
          </button>
          {err ? <p className="text-rose-700 text-sm" role="alert">{err}</p> : null}
          {!read && !err ? <Empty>Read-only: eth_call anchored(id) plus the VerificationAnchored event log. Nothing is submitted.</Empty> : null}
          {read ? (
            <dl className="text-sm grid grid-cols-1 sm:grid-cols-2 gap-2" aria-label="on-chain state">
              <div className="glass-soft p-3 sm:col-span-2">
                <dt className="text-xs uppercase text-sky-800/70">Anchored on-chain</dt>
                <dd className="font-black text-lg">{read.anchored ? "YES" : "NO — this id is not anchored"}</dd>
              </div>
              {[["Chain", read.chainId || "—"], ["Contract", read.contract || "—"],
                ["Tx hash", read.txHash || "— (no event found)"],
                ["Block", read.blockNumber || "—"],
                ["Event found", String(read.eventFound ?? false)]].map(([k, v]) => (
                <div key={k} className="glass-soft p-3"><dt className="text-xs uppercase text-sky-800/70">{k}</dt><dd className="mono text-xs break-all mt-1">{v}</dd></div>
              ))}
            </dl>
          ) : null}
          {stored ? (
            <div className="text-sm" aria-label="stored anchor">
              <h3 className="font-black mb-2">API-stored anchor row</h3>
              <div className="mono text-xs break-all">tx {stored.txHash} · block {stored.blockNumber} · chain {stored.chainId}</div>
            </div>
          ) : null}
        </section>
        <section className="section-card text-sm text-sky-900/70">
          <h3 className="font-black text-sky-950 mb-2">How to check by hand</h3>
          <pre className="mono text-[11px] overflow-x-auto whitespace-pre-wrap">{`quorum blockchain verify --contract ${contract || "0x…"} --verification-id ${vid || "0x…"}\n# exit 0 anchored / 1 not anchored / 4 chain unreachable`}</pre>
        </section>
        <Footer />
      </div>
    </main>
  );
}
