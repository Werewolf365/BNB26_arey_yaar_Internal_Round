// quorum doctor — detection only, never installs.
// Prints platform-specific remediation (Windows PowerShell aware).
import { execSync } from "node:child_process";

function check(cmd, versionArgs = "--version") {
  try {
    const out = execSync(`${cmd} ${versionArgs}`, { stdio: ["ignore", "pipe", "pipe"], timeout: 8000 }).toString().trim().split("\n")[0];
    return { ok: true, detail: out.slice(0, 120) };
  } catch {
    return { ok: false, detail: "not found on PATH" };
  }
}

function dockerDaemon() {
  try {
    execSync("docker info", { stdio: ["ignore", "pipe", "pipe"], timeout: 8000 });
    return true;
  } catch {
    return false;
  }
}

const isWin = process.platform === "win32";
console.log("Quorum doctor (detection only — no installs performed)");
console.log(`Platform: ${process.platform} ${process.arch}, node ${process.version}`);
console.log("");

const tools = [
  ["go", "version", "Go (pinned: go1.27.x per go.mod toolchain; CI enforces). Install: https://go.dev/dl/" + (isWin ? " — Windows: winget install GoLang.Go.Gzip or MSI from go.dev" : "")],
  ["node", "--version", "Node 20+ required for slice-1 harness + web."],
  ["docker", "--version", "Docker for builders/postgres/minio/anvil." + (isWin ? " Windows: start Docker Desktop; daemon must be running (npipe)." : "")],
  ["cosign", "version", "Cosign for Sigstore verification (later phase). Install: https://docs.sigstore.dev/cosign/installation/"],
  ["forge", "--version", "Foundry forge for contracts. No host install needed: Docker image ghcr.io/foundry-rs/foundry:latest works (see scripts/forge-docker.ps1 + docs/blockchain.md). Windows PowerShell: curl|bash foundryup does NOT work in stock PowerShell 5.1."],
  ["anvil", "--version", "Anvil local chain (comes with Foundry; or compose `anvil` service, chainId 31337)."],
  ["git", "--version", "Git for pinned source checkout."],
];

let missing = 0;
for (const [cmd, args, help] of tools) {
  const r = check(cmd, args);
  const icon = r.ok ? "PASS" : "MISS";
  if (!r.ok) missing++;
  console.log(`${icon}  ${cmd.padEnd(8)} ${r.ok ? r.detail : "MISSING"} `);
  if (!r.ok) console.log(`       -> ${help}`);
}
const daemon = dockerDaemon();
console.log(`${daemon ? "PASS" : "MISS"}  docker-daemon ${daemon ? "running" : "NOT running"}`);
if (!daemon) console.log("       -> Start Docker Desktop (Windows) before make dev / test-integration.");
console.log("");
console.log(`Go pin: see go.mod (toolchain go1.27.1, verified via https://go.dev/doc/devel/release Oct 2026).`);
console.log(`SLSA: use predicateType https://slsa.dev/provenance/v1 (SLSA v1.2 current; v1.0 retired) — resolved at implementation time.`);
console.log(`OSS Rebuild: deterministic fixtures in test-all (no creds). Real CLI lives in test-external (needs ADC creds unless sig-verify disabled).`);
if (missing > 0 || !daemon) {
  console.log(`doctor: ${missing} tool(s) missing${daemon ? "" : ", daemon down"} — follow the -> hints above. This is expected on a fresh Windows box.`);
  process.exitCode = 1;
} else {
  console.log("doctor: all toolchains present.");
}
