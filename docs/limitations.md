# Limitations (explicit, per prompt section 51)

- Builders in Docker on one host are NOT fully independent infra; independence_group models intent, not proof. Separate clouds/hosts required for real independence.
- VERIFIED means evidence+policy satisfied, NOT "safe": source may still be malicious, dependencies may be compromised, reproducibility has nondeterminism limits.
- OSS Rebuild covers only npm/PyPI/Crates.io subsets; many packages UNSUPPORTED. Live mode needs GCP ADC creds.
- Blockchain anchors hashes+events only (no artifacts/secrets); with BLOCKCHAIN_REQUIRED=false a down chain degrades to labeled SKIPPED, not silent success.
- Local test keys in Slice 1 prove format+verification, not real-world trust; Sigstore/Cosign + transparency required later.
- SLSA schema pinned at implementation time to v1.2 (predicateType https://slsa.dev/provenance/v1); re-verify against slsa.dev before release.
