// Unit: standard DSSE/in-toto attestation — sign, verify, tamper, replay.
// No custom envelopes. SLSA v1.2 predicateType https://slsa.dev/provenance/v1.
import test from "node:test";
import assert from "node:assert/strict";
import { buildProvenanceStatement, createTestKey, dsseSign, dsseVerify, sha256Hex } from "../services/attestation/dsse.mjs";

test("sign -> verify valid (standard DSSE + in-toto + SLSA v1.2)", () => {
  const digest = sha256Hex(Buffer.from("quorum-tiny-artifact-v1"));
  const st = buildProvenanceStatement({
    artifactName: "tiny-1.0.0.tar.gz", artifactDigestHex: digest,
    sourceCommit: "abc123", builderId: "https://quorum.example/builders/a",
    buildType: "https://quorum.example/buildtypes/tiny/v1",
  });
  assert.equal(st._type, "https://in-toto.io/Statement/v1");
  assert.equal(st.predicateType, "https://slsa.dev/provenance/v1");
  const { privateKey, publicKey } = createTestKey();
  const env = dsseSign(st, privateKey);
  assert.equal(env.payloadType, "application/vnd.in-toto+json");
  const v = dsseVerify(env, publicKey, { expectedSubjectDigest: digest, expectedSourceCommit: "abc123" });
  assert.equal(v.signatureValid, true);
  assert.equal(v.statementValid, true);
});

test("1-byte artifact tamper -> SUBJECT_DIGEST_MISMATCH (cannot silently pass)", () => {
  const good = sha256Hex(Buffer.from("quorum-tiny-artifact-v1"));
  const evil = sha256Hex(Buffer.from("quorum-tiny-artifact-v1X"));
  assert.notEqual(good, evil);
  const st = buildProvenanceStatement({
    artifactName: "tiny.tar.gz", artifactDigestHex: good, sourceCommit: "abc123",
    builderId: "https://quorum.example/builders/a", buildType: "https://quorum.example/buildtypes/tiny/v1",
  });
  const { privateKey, publicKey } = createTestKey();
  const env = dsseSign(st, privateKey);
  const v = dsseVerify(env, publicKey, { expectedSubjectDigest: evil, expectedSourceCommit: "abc123" });
  assert.equal(v.reason, "SUBJECT_DIGEST_MISMATCH");
});

test("signature over bytes tampered -> SIGNATURE_INVALID", () => {
  const digest = sha256Hex(Buffer.from("x"));
  const st = buildProvenanceStatement({
    artifactName: "x", artifactDigestHex: digest, sourceCommit: "abc123",
    builderId: "https://quorum.example/builders/a", buildType: "https://quorum.example/buildtypes/tiny/v1",
  });
  const { privateKey, publicKey } = createTestKey();
  const env = dsseSign(st, privateKey);
  const raw = Buffer.from(env.payload, "base64");
  raw[raw.length - 1] ^= 0x01; // flip one byte of signed payload
  env.payload = raw.toString("base64");
  const v = dsseVerify(env, publicKey, {});
  assert.equal(v.signatureValid, false);
});

test("replay onto another commit -> SOURCE_COMMIT_MISMATCH (rejected)", () => {
  const digest = sha256Hex(Buffer.from("x"));
  const st = buildProvenanceStatement({
    artifactName: "x", artifactDigestHex: digest, sourceCommit: "commit-OLD",
    builderId: "https://quorum.example/builders/a", buildType: "https://quorum.example/buildtypes/tiny/v1",
  });
  const { privateKey, publicKey } = createTestKey();
  const env = dsseSign(st, privateKey);
  const v = dsseVerify(env, publicKey, { expectedSubjectDigest: digest, expectedSourceCommit: "commit-NEW" });
  assert.equal(v.reason, "SOURCE_COMMIT_MISMATCH");
});
