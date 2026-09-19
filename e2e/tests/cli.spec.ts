import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import {
  codeDigest,
  codeLocation,
  declaredRepository,
  git,
  readJSON,
  reviewFiles,
  runCLI,
  treeSnapshot
} from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

test("@critical refuses malformed, non-canonical, and unresolvable code locations without writing", async ({ sagaRepositories }) => {
  const { identity, sagaRoot, sourceRepo } = sagaRepositories;
  const { head } = identity;
  const canonical = codeLocation(head, "src/app.go", 3, 4);
  expect(canonical).toBe(`${head}:src/app.go#L3-L4`);

  const malformed: Array<[string, string]> = [
    ["not a location", "no-location-here"],
    ["missing path", `${head}:`],
    ["missing commit", ":src/app.go#L3-L4"],
    ["abbreviated commit", `${head.slice(0, 12)}:src/app.go#L3-L4`],
    ["over-long commit", `${head}ab:src/app.go#L3-L4`],
    ["uppercase commit", `${head.toUpperCase()}:src/app.go#L3-L4`],
    ["symbolic revision", "HEAD:src/app.go#L3-L4"],
    ["legacy diff URI", `saga-diff://v1/line?head=${head}&path=src%2Fapp.go&side=new&start=3&end=4`],
    ["non-canonical single-line range", `${head}:src/app.go#L3-L3`],
    ["inverted range", `${head}:src/app.go#L9-L2`],
    ["path traversal", `${head}:../../etc/passwd#L1`],
    ["embedded traversal", `${head}:src/../src/app.go#L3-L4`],
    ["absolute path", `${head}:/etc/passwd#L1`]
  ];
  // Well-formed locations the source repository cannot resolve. A suffix that
  // is not a canonical line range is part of the path, so it names no file.
  const unresolvable: Array<[string, string]> = [
    ["zero line", `${head}:src/app.go#L0-L4`],
    ["GitHub-style range", `${head}:src/app.go#L3-4`],
    ["unknown commit", codeLocation("0".repeat(40), "src/app.go", 3, 4)],
    ["path absent at the commit", codeLocation(head, "src/missing.go", 1, 1)],
    ["range past the end of the file", codeLocation(head, "src/app.go", 3, 400)]
  ];

  const before = treeSnapshot(sagaRoot);
  for (const [label, ref] of malformed) {
    const result = runCLI(sagaRepositories, ["cover", "--repo", sourceRepo, "--target", "___overview/overview.fragment", "--name", "must-not-exist", "--ref", ref, sagaRoot]);
    expect(result.status, `cover with ${label} location`).not.toBe(0);
    expect(`${result.stdout}${result.stderr}`, `cover with ${label} location`).toContain("invalid --ref");
  }
  for (const [label, ref] of unresolvable) {
    const result = runCLI(sagaRepositories, ["cover", "--repo", sourceRepo, "--target", "overview.fragment", "--name", "must-not-exist", "--ref", ref, sagaRoot]);
    expect(result.status, `cover with ${label}`).not.toBe(0);
    expect(`${result.stdout}${result.stderr}`, `cover with ${label}`).not.toContain(sagaRepositories.root);
  }
  expect(treeSnapshot(sagaRoot), "saga tree after rejected code locations").toBe(before);
  expect(reviewFiles(sagaRepositories, /must-not-exist/)).toEqual([]);

  // Positive control: the same location, canonically spelled at a commit the
  // repository holds, is accepted and written with the digest of exactly the
  // referenced bytes, so the rejections above are the location check.
  const accepted = runCLI(sagaRepositories, ["cover", "--repo", sourceRepo, "--target", "___overview/overview.fragment", "--name", "accepted-evidence", "--ref", canonical, sagaRoot]);
  expect(accepted.status, accepted.stderr).toBe(0);
  const records = reviewFiles(sagaRepositories, /___code\/accepted-evidence\.json$/);
  expect(records).toHaveLength(1);
  expect(readJSON(records[0])).toEqual({
    version: 2,
    references: [{ commit: head, path: "src/app.go", start: 3, end: 4, digest: codeDigest(sourceRepo, head, "src/app.go", 3, 4) }]
  });
});

test("@critical exposes mapping scrutiny, claims, and verification as an AI review harness", async ({ sagaRepositories }) => {
  const { identity, sagaRepo, sagaRoot, sourceRepo } = sagaRepositories;
  const evidence = codeLocation(identity.head, "src/app.go", 3);

  const claim = runCLI(sagaRepositories, [
    "add-claim", "--repo", sourceRepo, "--id", "greeting-behavior", "--target", "___overview/overview.fragment#greeting-input", "--kind", "behavior",
    "--statement", "Greeting accepts a name in its function signature.", "--ref", evidence, sagaRoot
  ]);
  expect(claim.status, claim.stderr).toBe(0);
  const verification = runCLI(sagaRepositories, [
    "verify-claim", "--id", "greeting-inspection", "--claim", "greeting-behavior", "--status", "verified",
    "--method", "inspection", "--summary", "The changed function signature and return expression were inspected.", sagaRoot
  ]);
  expect(verification.status, verification.stderr).toBe(0);
  const claimRecords = reviewFiles(sagaRepositories, /___claims\/greeting-behavior\.json$/);
  expect(claimRecords).toHaveLength(1);
  // The claim's evidence is the code reference itself, digested by the CLI.
  expect(readJSON<{ evidence: unknown[] }>(claimRecords[0]).evidence).toEqual([
    { commit: identity.head, path: "src/app.go", start: 3, end: 3, digest: codeDigest(sourceRepo, identity.head, "src/app.go", 3) }
  ]);
  expect(reviewFiles(sagaRepositories, /___verifications\/greeting-inspection\.json$/)).toHaveLength(1);

  git(sagaRepo, "add", ".");
  git(sagaRepo, "commit", "-m", "record author claim and verification");

  const mappings = runCLI(sagaRepositories, ["query", "mappings", "--saga", sagaRoot, "--repo", sourceRepo, "--against", "main", "--sort", "scrutiny"]);
  expect(mappings.status, mappings.stderr).toBe(0);
  const mappingEnvelope = JSON.parse(mappings.stdout) as { data: { mappings: Array<{ scrutiny_score: number; atoms_per_note: number; target_file_count: number; reasons: unknown[] }> } };
  expect(mappingEnvelope.data.mappings.length).toBeGreaterThan(0);
  expect(mappingEnvelope.data.mappings[0]).toEqual(expect.objectContaining({ scrutiny_score: expect.any(Number), atoms_per_note: expect.any(Number), target_file_count: expect.any(Number), reasons: expect.any(Array) }));

  const claims = runCLI(sagaRepositories, ["query", "claims", "--saga", sagaRoot, "--repo", sourceRepo, "--against", "main", "--status", "verified"]);
  expect(claims.status, claims.stderr).toBe(0);
  const claimEnvelope = JSON.parse(claims.stdout) as { data: { claims: Array<{ id: string; verification_status: string; attribution: { status: string }; evidence: Array<{ mapped_to_target: boolean }> }> } };
  expect(claimEnvelope.data.claims).toHaveLength(1);
  expect(claimEnvelope.data.claims[0]).toEqual(expect.objectContaining({ id: "greeting-behavior", verification_status: "verified", attribution: expect.objectContaining({ status: "committed" }) }));
  expect(claimEnvelope.data.claims[0].evidence.every((item) => item.mapped_to_target)).toBe(true);

  const verifications = runCLI(sagaRepositories, ["query", "verifications", "--saga", sagaRoot, "--repo", sourceRepo, "--against", "main", "--claim", "greeting-behavior"]);
  expect(verifications.status, verifications.stderr).toBe(0);
  const verificationEnvelope = JSON.parse(verifications.stdout) as { data: { verifications: Array<{ id: string; status: string; attribution: { status: string } }> } };
  expect(verificationEnvelope.data.verifications).toEqual([
    expect.objectContaining({ id: "greeting-inspection", status: "verified", attribution: expect.objectContaining({ status: "committed" }) })
  ]);

  const owners = runCLI(sagaRepositories, ["query", "diff-owners", "--saga", sagaRoot, "--repo", sourceRepo, "--against", "main", "--ref", evidence]);
  expect(owners.status, owners.stderr).toBe(0);
  const ownerEnvelope = JSON.parse(owners.stdout) as { data: { atoms: Array<{ owners: Array<{ mapping?: { scrutiny_score: number } }> }> } };
  expect(ownerEnvelope.data.atoms.flatMap((atom) => atom.owners).some((owner) => typeof owner.mapping?.scrutiny_score === "number")).toBe(true);

  // Mapping every changed line is necessary but not sufficient: this Saga has
  // no accepted stories, so it is not ready for review, and status says why.
  const status = runCLI(sagaRepositories, ["status", "--repo", sourceRepo, "--against", "main", sagaRoot]);
  expect(status.status, status.stderr).toBe(3);
  expect(status.stdout).toContain("ALL ATOMS MAPPED");
  expect(status.stdout).toContain("does not establish explanation quality or correctness");
  expect(status.stdout).toMatch(/ready_for_review\s+blocked/);
});

test("@critical refuses to mutate or serve a structurally invalid saga with zero side effects", async ({ sagaRepositories }) => {
  const { sagaRoot, sourceRepo } = sagaRepositories;
  const manifestPath = join(sagaRoot, "saga.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as Record<string, unknown>;
  // A loadable manifest that fails schema validation: exactly the state the
  // product promises never to write review records into.
  writeFileSync(manifestPath, `${JSON.stringify({ ...manifest, title: "" }, null, 2)}\n`);

  const validation = runCLI(sagaRepositories, ["validate", sagaRoot]);
  expect(validation.status, "validate must report the corrupted saga").not.toBe(0);
  expect(validation.stdout).toContain("Invalid saga");

  const before = treeSnapshot(sagaRoot);
  const refused: Array<[string, string[]]> = [
    ["comment", ["thread", "--target", "___overview/overview.fragment", "--body", "Should never be stored.", sagaRoot]],
    ["reply", ["reply", "--thread", "20250101T000000000Z", "--body", "Should never be stored.", sagaRoot]],
    ["approval", ["review", "--target", "___overview/overview.fragment", "--state", "approved", "--reviewer-kind", "human", "--body", "Should never be stored.", sagaRoot]],
    ["rejection", ["review", "--target", ".", "--state", "rejected", "--reviewer-kind", "human", sagaRoot]]
  ];
  for (const [label, args] of refused) {
    const result = runCLI(sagaRepositories, args);
    expect(result.status, `${label} on an invalid saga`).not.toBe(0);
    expect(`${result.stdout}${result.stderr}`, `${label} on an invalid saga`).toContain("structurally invalid");
  }

  const serve = runCLI(sagaRepositories, ["serve", "--addr", "127.0.0.1:0", "--repo", sourceRepo, sagaRoot]);
  expect(serve.status, "serve on an invalid saga").not.toBe(0);
  expect(`${serve.stdout}${serve.stderr}`).toContain("structurally invalid");
  expect(serve.stdout).not.toContain("Change Saga is available at");

  // `init` scaffolds the empty record directories, so the contract is that they
  // stay empty and that nothing anywhere under the saga moved.
  expect(reviewFiles(sagaRepositories, /___review\//), "review records after refused mutations").toEqual([]);
  expect(reviewFiles(sagaRepositories, /___approvals\//), "approval records after refused mutations").toEqual([]);
  expect(treeSnapshot(sagaRoot), "saga tree after refused mutations").toBe(before);
});

test("@critical refuses a checkout whose origin does not match the declared repository", async ({ sagaRepositories }) => {
  const { sagaRoot, sourceRepo } = sagaRepositories;
  git(sourceRepo, "remote", "set-url", "origin", "https://example.test/acme/impostor.git");
  const before = treeSnapshot(sagaRoot);

  const status = runCLI(sagaRepositories, ["status", "--repo", sourceRepo, "--against", "main", sagaRoot]);
  expect(status.status, "status against a mismatched checkout").not.toBe(0);
  expect(`${status.stdout}${status.stderr}`).toContain("does not match declared repository");

  const cover = runCLI(sagaRepositories, ["cover", "--repo", sourceRepo, "--against", "main", "--target", "___overview/overview.fragment", "--path", "src/app.go", "--side", "new", "--lines", "3", "--name", "must-not-exist", sagaRoot]);
  expect(cover.status, "cover against a mismatched checkout").not.toBe(0);
  expect(`${cover.stdout}${cover.stderr}`).toContain("does not match declared repository");
  expect(treeSnapshot(sagaRoot), "saga tree after a refused mismatched checkout").toBe(before);

  // The override exists and is explicit; nothing else unblocks the check.
  const overridden = runCLI(sagaRepositories, ["cover", "--repo", sourceRepo, "--against", "main", "--allow-repository-mismatch", "--target", "___overview/overview.fragment", "--path", "src/app.go", "--side", "new", "--lines", "3", "--name", "explicit-override", sagaRoot]);
  expect(overridden.status, overridden.stderr).toBe(0);

  git(sourceRepo, "remote", "set-url", "origin", declaredRepository);
});

test("@critical compares a change: the code it touched lights up the records that reference it", async ({ sagaRepositories }) => {
  const { sagaRepo, sagaRoot, sourceRepo } = sagaRepositories;
  const incomingBase = git(sourceRepo, "rev-parse", "HEAD");
  // The Saga lives in its own repository, so its sync cursor records the code
  // commit it documents; a comparison reads the Saga that documented its base.
  const synced = runCLI(sagaRepositories, ["sync", "--repo", sourceRepo, sagaRoot]);
  expect(synced.status, synced.stderr).toBe(0);
  git(sagaRepo, "add", ".");
  git(sagaRepo, "commit", "-m", "document the incoming base");
  writeFileSync(join(sourceRepo, "src", "app.go"), `package demo\n\nfunc Greeting(name string) string {\n\treturn "welcome, " + name\n}\n\nfunc Ready() bool {\n\treturn true\n}\n\nfunc Audited() bool {\n\treturn true\n}\n`);
  writeFileSync(join(sourceRepo, "new-capability.go"), "package demo\n\nconst NewCapability = true\n");
  git(sourceRepo, "add", ".");
  git(sourceRepo, "commit", "-m", "change greeting and add capability");

  const compared = runCLI(sagaRepositories, ["status", "--json", "--repo", sourceRepo, "--against", incomingBase, sagaRoot]);
  const result = JSON.parse(compared.stdout) as {
    opening: { mode: string; base_oid: string };
    comparison: {
      changed: Array<{ urn: string }>;
      affected: Array<{ urn: string; because: Array<{ kind: string }>; reasons: Array<{ subject: string }> }>;
      code: { groups: Array<{ urn: string }>; unreferenced: Array<{ path: string }> };
    };
  };
  expect(result.opening.mode).toBe("compare");
  expect(result.opening.base_oid).toBe(incomingBase);
  // The Saga did not change, so nothing is Changed; the greeting's owners are
  // Affected by code, with the commit that changed it beside them.
  expect(result.comparison.changed).toEqual([]);
  const greeting = result.comparison.affected.find((record) => record.urn.includes(":fragment:") && record.urn.includes("overview"));
  expect(greeting?.because.some((cause) => cause.kind === "code")).toBe(true);
  expect(greeting?.reasons.map((reason) => reason.subject)).toContain("change greeting and add capability");
  expect(result.comparison.code.unreferenced.some((hunk) => hunk.path === "new-capability.go")).toBe(true);

  const observed = runCLI(sagaRepositories, ["status", "--json", "--repo", sourceRepo, sagaRoot]);
  const observation = JSON.parse(observed.stdout) as { opening: { mode: string }; comparison?: unknown };
  expect(observation.opening.mode).toBe("observe");
  expect(observation.comparison).toBeUndefined();
});
