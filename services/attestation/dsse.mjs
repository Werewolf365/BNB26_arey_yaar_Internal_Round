// Standard DSSE / in-toto attestation helpers (Slice 1).
// Envelope: DSSE. Statement: https://in-toto.io/Statement/v1.
// Predicate: https://slsa.dev/provenance/v1 (SLSA v1.2 current; v1.0 retired).
// Keys here are LOCAL TEST keys only. Production path is Sigstore/Cosign
// (see docs/attestations.md). No custom crypto, no custom envelope.
import { createHash, generateKeyPairSync, sign, verify } from "node:crypto";
import { canonical } from "../policy/quorum.mjs";

export const STATEMENT_TYPE = "https://in-toto.io/Statement/v1";
export const PROVENANCE_PREDICATE = "https://slsa.dev/provenance/v1";

export function sha256Hex(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

export function buildProvenanceStatement({ artifactName, artifactDigestHex, sourceCommit, builderId, buildType, externalParameters = {} }) {
  return {
    _type: STATEMENT_TYPE,
    subject: [{ name: artifactName, digest: { sha256: artifactDigestHex } }],
    predicateType: PROVENANCE_PREDICATE,
    predicate: {
      buildDefinition: {
        buildType,
        externalParameters: { ...externalParameters, sourceCommit },
        resolvedDependencies: [{ uri: `git+https://example.invalid/repo@${sourceCommit}` }],
      },
      runDetails: {
        builder: { id: builderId },
        metadata: { invocationId: `test-${sha256Hex(Buffer.from(builderId + sourceCommit)).slice(0, 12)}` },
      },
    },
  };
}

// DSSE Pre-Auth Encoding: "DSSEv1 <len(type)> <type> <len(body)> <body> <len(ext)> <ext>"
export function dssePreAuth(payloadType, body) {
  const ext = "";
  return Buffer.concat([
    Buffer.from(`DSSEv1 ${payloadType.length} `, "utf8"), Buffer.from(payloadType, "utf8"),
    Buffer.from(` ${body.length} `, "utf8"), body,
    Buffer.from(` ${ext.length} `, "utf8"), Buffer.from(ext, "utf8"),
  ]);
}

export function createTestKey() {
  // P-256 to match SLSA envelope recommendation (ECDSA P-256 + SHA-256).
  const { privateKey, publicKey } = generateKeyPairSync("ec", { namedCurve: "P-256" });
  return { privateKey, publicKey };
}

export function dsseSign(statement, privateKey) {
  const body = Buffer.from(canonical(statement), "utf8");
  const payloadType = "application/vnd.in-toto+json";
  const pae = dssePreAuth(payloadType, body);
  const sig = sign("sha256", pae, privateKey);
  return { payloadType, payload: body.toString("base64"), signatures: [{ sig: sig.toString("base64") }] };
}

export function dsseVerify(envelope, publicKey, { expectedSubjectDigest, expectedSourceCommit } = {}) {
  const body = Buffer.from(envelope.payload, "base64");
  const pae = dssePreAuth(envelope.payloadType, body);
  const sig = Buffer.from(envelope.signatures[0].sig, "base64");
  const sigValid = verify("sha256", pae, publicKey, sig);
  if (!sigValid) return { signatureValid: false, reason: "SIGNATURE_INVALID" };
  let statement;
  try {
    statement = JSON.parse(body.toString("utf8"));
  } catch {
    return { signatureValid: false, reason: "MALFORMED_STATEMENT" };
  }
  if (statement._type !== STATEMENT_TYPE) return { signatureValid: true, statementValid: false, reason: "UNKNOWN_STATEMENT_TYPE" };
  if (statement.predicateType !== PROVENANCE_PREDICATE) return { signatureValid: true, statementValid: false, reason: "UNKNOWN_PREDICATE" };
  const subjectDigest = statement.subject?.[0]?.digest?.sha256;
  // Binding check: what was signed must match what we verify (never trust sig alone).
  if (expectedSubjectDigest && subjectDigest !== expectedSubjectDigest)
    return { signatureValid: true, statementValid: false, reason: "SUBJECT_DIGEST_MISMATCH" };
  const committed = statement.predicate?.buildDefinition?.externalParameters?.sourceCommit;
  if (expectedSourceCommit && committed !== expectedSourceCommit)
    return { signatureValid: true, statementValid: false, reason: "SOURCE_COMMIT_MISMATCH" };
  return { signatureValid: true, statementValid: true, statement };
}
