// SBOM generator: prefers `syft` (real CycloneDX) when installed, else falls
// back to a deterministic Go-module + npm inventory so `make sbom` never
// silently produces nothing. Output: sbom.json (and sbom.syft.json when syft
// ran). Never fails the build on missing tools — it reports SKIP lines;
// CI installs syft for the authoritative artifact.
import { execSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";

function have(bin) {
  try { execSync(`${bin} --version`, { stdio: "pipe", timeout: 15000 }); return true; }
  catch { return false; }
}

const out = { tool: null, packages: [] };
if (have("syft")) {
  try {
    execSync("syft . -o cyclonedx-json=sbom.syft.json", { stdio: "pipe", timeout: 180000 });
    const doc = JSON.parse(readFileSync("sbom.syft.json", "utf8"));
    for (const c of doc.components || []) out.packages.push({ name: c.name, version: c.version, type: c.type });
    out.tool = "syft";
    console.log(`sbom: syft recorded ${out.packages.length} components -> sbom.syft.json`);
  } catch (e) {
    console.log(`sbom: syft failed (${String(e).split("\n")[0]}), falling back to module inventory`);
  }
}
if (!out.tool) {
  try {
    const lines = execSync("go list -m all", { encoding: "utf8", timeout: 60000 }).trim().split("\n");
    for (const line of lines.slice(1)) { // skip the main module line
      const [name, version] = line.trim().split(/\s+/);
      if (name && version) out.packages.push({ name, version, type: "go-module" });
    }
  } catch (e) {
    console.log(`sbom: go list failed (${String(e).split("\n")[0]})`);
  }
  for (const lock of ["package-lock.json", "apps/web/package-lock.json"]) {
    if (!existsSync(lock)) continue;
    try {
      const doc = JSON.parse(readFileSync(lock, "utf8"));
      for (const [name, meta] of Object.entries(doc.packages || {})) {
        if (!name || !meta.version) continue;
        out.packages.push({ name: `npm:${name}`, version: meta.version, type: "npm" });
      }
    } catch { /* ignore malformed lock */ }
  }
  out.tool = "module-inventory";
  console.log(`sbom: module inventory recorded ${out.packages.length} packages (install syft for CycloneDX)`);
}
out.packages.sort((a, b) => (a.name < b.name ? -1 : 1));
writeFileSync("sbom.json", JSON.stringify({ generatedBy: `quorum-sbom:${out.tool}`, packages: out.packages }, null, 2));
console.log(`sbom: wrote sbom.json (${out.packages.length} entries via ${out.tool})`);
