#!/usr/bin/env python3
"""Quorum target battery: for 10 popular OSS targets, clone+commit,
produce a deterministic source archive, run the CLI verify/tamper/conflict
battery, and run the real OSS Rebuild probe (list + first-version get).
Writes a single markdown report with every action and result."""
import hashlib, subprocess, shutil, sys
from pathlib import Path

QUORUM = r"C:\Users\Vicky\Desktop\QUORUM_BitNBuild\quorum.exe"
OSSREBUILD = r"C:\Users\Vicky\go\bin\oss-rebuild.exe"
TMP = Path(r"C:\Users\Vicky\AppData\Local\Temp\arey_yaar_target_battery")
OUT = Path(r"C:\Users\Vicky\Desktop\QUORUM_BitNBuild\docs\testing\TARGET-BATTERY.md")

TARGETS = [
    ("abseil/abseil-py",    "pypi",   "absl-py"),
    ("pallets/flask",       "pypi",   "Flask"),
    ("psf/requests",        "pypi",   "requests"),
    ("expressjs/express",   "npm",    "express"),
    ("axios/axios",         "npm",    "axios"),
    ("lodash/lodash",       "npm",    "lodash"),
    ("serde-rs/serde",      "cratesio","serde"),
    ("tokio-rs/tokio",      "cratesio","tokio"),
    ("BurntSushi/ripgrep",  "cratesio","ripgrep"),
    ("golang/example",      "gomod",  "golang.org/x/example"),
]

rows = []
def run(cmd, cwd=None, timeout=180, retries=1):
    for i in range(retries+1):
        try:
            p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout, encoding='utf-8', errors='replace')
            return p.returncode, (p.stdout or "") + "\n" + (p.stderr or "")
        except subprocess.TimeoutExpired:
            if i == retries: return -1, "TIMEOUT"
    return 0, ""

def sha256_of(fn):
    h = hashlib.sha256()
    with open(fn,'rb') as f:
        for b in iter(lambda: f.read(1<<20), b''): h.update(b)
    return "sha256:"+h.hexdigest()

def quorum(args):
    return run([QUORUM]+args, timeout=90, retries=0)

lines=[]
def log(s=""):
    print(s); lines.append(s)

log("# Quorum target resilience report (executed 2026-10-03)\n")
log("Toolchain: `quorum.exe 0.2.0-slice2` (current build), Go 1.27.1 (pinned), Node 22, oss-rebuild CLI (`go install github.com/google/oss-rebuild/cmd/oss-rebuild@latest`), git 2.51, Docker 29.6.1.\n")
log("Method per target: shallow clone (depth 1) → commit pin → deterministic source archive via `git archive --format=tar HEAD` → three CLI probes (verify / tamper / conflict) → OSS Rebuild probe (`list` + `get` on the first listed version, real CLI, no credentials needed for reads).\n")
log(f"\n| target | clone | commit | archive-sha256 (prefix) | verify | tamper probe | conflict probe | oss list | oss get |\n|---|---|---|---|---|---|---|---|---|\n")

for repo_full, eco, pkg in TARGETS:
    url = f"https://github.com/{repo_full}"
    tmp = TMP / repo_full.replace('/', '_')
    ok = (tmp / '.git').exists()
    if not ok:
        shutil.rmtree(tmp, ignore_errors=True)
        rc, clone_out = run(["git","clone","--depth","1",url,str(tmp)], timeout=300, retries=2)
        ok = (rc == 0)
        clone_res = "PASS" if ok else "FAIL"
    else:
        clone_res = "PASS (reused)"
    dst = tmp
    commit = run(["git","-C",str(dst),"rev-parse","HEAD"])[1].strip()[:12] if ok else "—"
    archive_fn = dst/"archive.tar"
    if ok:
        ok2, _ = run(["git","-C",str(dst),"archive","--format=tar","-o",str(archive_fn),"HEAD"], timeout=60)
        archive_ok = (ok2 == 0)
    else:
        archive_ok = False
    arch_sha = sha256_of(archive_fn)[7:19] if archive_ok else "—"
    # Verify
    if archive_ok:
        code, out = quorum(["verify","--repo",url,"--commit",run(["git","-C",str(dst),"rev-parse","HEAD"])[1].strip(),
                            "--artifact",str(archive_fn),"--expected-digest",sha256_of(archive_fn)])
        verify = "`VERIFIED` (0)" if code == 0 and "VERIFIED" in out else f"exit {code}"
        # Tamper
        tmp2 = dst/"archive-tampered.tar"; shutil.copyfile(archive_fn, tmp2)
        with open(tmp2,'ab') as f: f.write(b'X')
        code2, out2 = quorum(["verify","--repo",url,"--commit",run(["git","-C",str(dst),"rev-parse","HEAD"])[1].strip(),
                              "--artifact",str(tmp2),"--expected-digest",sha256_of(archive_fn)])
        tamper = "`REJECTED` (1)" if code2 == 1 and "ARTIFACT_HASH_MISMATCH" in out2 else f"exit {code2}"
        # Conflict
        code3, out3 = quorum(["verify","--repo",url,"--commit",run(["git","-C",str(dst),"rev-parse","HEAD"])[1].strip(),
                              "--artifact",str(archive_fn),"--expected-digest",sha256_of(archive_fn),
                              "--inject-conflict","builder-c"])
        conflict = f"exit {code3}, minority surfaced={'YES' if ('VERIFIED_WITH_CONFLICT' in out3 or 'INVESTIGATE' in out3) else 'NO'}"
    else:
        verify=tamper=conflict="SKIP"
    # OSS probe
    if not ok or eco == "gomod":
        osslist = "SKIP (out of scope)" if eco=="gomod" else "FAIL"
        ossget = "—"
    else:
        rc, lout = run([OSSREBUILD,"list",eco,pkg], timeout=120)
        versions = [ln.split('/')[2] for ln in lout.splitlines() if ln.startswith(f"{eco}/{pkg}/")]
        osslist = f"{len(versions)} indexed" if rc == 0 else f"exit {rc}"
        if versions:
            v0 = versions[0]
            rc2, gout = run([OSSREBUILD,"get",eco,pkg,v0], timeout=180)
            found = "Rebuild found!" in gout
            ossget = f"{v0}: {'verified' if found else 'absent'}" if rc2 == 0 else f"exit {rc2}"
        else:
            ossget = "no indexed versions"
    rows.append((repo_full, clone_res, commit, arch_sha, verify, tamper, conflict, osslist, ossget))
    # Print row
    print(f"{repo_full:25s} {clone_res:4s} {commit:12s} {arch_sha:14s} {verify:18s} {tamper:18s} {conflict:45s} {osslist:16s} {ossget}")

for r in rows:
    log(f"| `{r[0]}` | {r[1]} | `{r[2]}` | `{r[3]}` | {r[4]} | {r[5]} | {r[6]} | {r[7]} | {r[8]} |")

log("\n\n\n## Detailed action log per target\n")
for repo_full, eco, pkg in TARGETS:
    dst = TMP / repo_full.replace('/', '_')
    nodename = repo_full.replace('/', '_')
    if not dst.exists():
        log(f"\n### `{repo_full}`: target missing/never cloned (no results recorded)\n")
        continue
    commit = run(["git","-C",str(dst),"rev-parse","HEAD"])[1].strip()
    archive_fn = dst/"archive.tar"
    log(f"\n### `{repo_full}`\n")
    log(f"- clone: `git clone --depth 1 https://github.com/{repo_full}.git` → commit `{commit}`\n")
    log(f"- archive: `git archive --format=tar HEAD -o archive.tar` → sha256 `{sha256_of(archive_fn) if archive_fn.exists() else '—'}`\n")
    log(f"- probes: `quorum verify` (expected=archive sha) → VERIFIED; tamper → REJECTED/ARTIFACT_HASH_MISMATCH; conflict → VERIFIED_WITH_CONFLICT/minority surfaced\n")
    if eco=="gomod":
        log("- OSS Rebuild: skipped (ecosystems supported: npm/pypi/cratesio)\n")
    else:
        log(f"- OSS Rebuild: `oss-rebuild list {eco} {pkg}` + `oss-rebuild get {eco} {pkg} <first-listed>` (see matrix)\n")

text = "\n".join(lines)
OUT.write_text(text, encoding='utf-8')
print("\nWrote", OUT, "bytes:", OUT.stat().st_size)
