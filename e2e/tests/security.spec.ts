import { tmpdir } from "node:os";
import { codeLocation, git, runCLI, serverRequest, treeSnapshot, type HTTPResponse } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

const overviewTarget = "urn:change-saga:wave-one:fragment:wave-one-overview";

// The Saga is documentation: the reviewer has no way to comment on, annotate,
// approve, or mark it reviewed. The endpoints that used to write those records
// are gone, so nothing a page or a forged request sends can change the Saga.
test("@critical has no documentation write endpoints and writes nothing when they are called", async ({ saga }) => {
  const before = treeSnapshot(saga.sagaRoot);
  const form = { "Content-Type": "application/x-www-form-urlencoded" };
  for (const path of ["/api/thread", "/api/reply", "/api/thread-state", "/api/thread-anchor", "/api/review", "/api/diff-review"]) {
    const response = await serverRequest(saga.baseURL, path, {
      method: "POST",
      headers: form,
      body: new URLSearchParams({ target: overviewTarget, state: "approved", body: "Forged." }).toString()
    });
    expect([404, 405], `POST ${path} answered ${response.status}`).toContain(response.status);
  }
  const activity = await serverRequest(saga.baseURL, "/api/activity");
  expect(activity.status, "the review activity feed").toBe(404);
  expect(treeSnapshot(saga.sagaRoot), "saga tree after calls to removed endpoints").toBe(before);
});

test("@critical rejects cross-origin and foreign-Host requests before any handler runs", async ({ saga }) => {
  const port = new URL(saga.baseURL).port;

  for (const [label, headers] of [
    ["a foreign Origin", { Origin: "http://evil.test" }],
    ["a look-alike Origin", { Origin: `http://127.0.0.1.evil.test:${port}` }],
    ["cross-site fetch metadata", { "Sec-Fetch-Site": "cross-site" }],
    ["cross-origin fetch metadata", { "Sec-Fetch-Site": "cross-origin" }]
  ] as const) {
    const response = await serverRequest(saga.baseURL, "/api/runtime-stop", { method: "POST", headers });
    expect(response.status, `POST with ${label}`).toBe(403);
    expect(response.body, `POST with ${label}`).toContain("Cross-origin request rejected.");
  }

  for (const host of ["attacker.test", `attacker.test:${port}`, `127.0.0.1.evil.test:${port}`, `evil.test:${port}`]) {
    const response = await serverRequest(saga.baseURL, "/", { headers: { Host: host } });
    expect(response.status, `page request with Host ${host}`).toBe(403);
    expect(response.body).toContain("Invalid request host.");
  }

  // The two hostnames that genuinely address this loopback listener still work.
  for (const host of [`127.0.0.1:${port}`, `localhost:${port}`]) {
    const response = await serverRequest(saga.baseURL, "/", { headers: { Host: host } });
    expect(response.status, `page request with Host ${host}`).toBe(200);
  }
  // Positive control: a same-origin POST passes the origin gate and reaches the
  // handler, which then refuses it for its own reason.
  const sameOrigin = await serverRequest(saga.baseURL, "/api/runtime-stop", {
    method: "POST",
    headers: { Origin: `http://127.0.0.1:${port}`, "Sec-Fetch-Site": "same-origin" }
  });
  expect(sameOrigin.status).toBe(403);
  expect(sameOrigin.body).toContain("shutdown token");
});

test("@critical refuses malformed, non-canonical, and foreign code locations", async ({ page, saga }) => {
  const { base, head } = saga.identity;
  await page.goto(`${saga.baseURL}/?view=code&file=${encodeURIComponent("src/app.go")}`);
  const row = page.locator('article.file-diff[data-file-path="src/app.go"] [data-diff-row][data-side="new"]').first();
  const line = Number(await row.getAttribute("data-line"));
  // Positive control for the whole table below: the location this suite builds
  // is byte-for-byte the canonical spelling the product itself renders.
  expect(await row.getAttribute("data-diff-ref")).toBe(codeLocation(head, "src/app.go", line));
  const canonical = codeLocation(head, "src/app.go");
  expect(canonical).toBe(`${head}:src/app.go`);

  // A commit that is neither side of the comparison: its file exists, but
  // showing it would present code the change never touched.
  const outside = git(saga.sourceRepo, "commit-tree", `${head}^{tree}`, "-p", head, "-m", "outside the comparison");
  const rejected: Array<[string, string]> = [
    ["not a location", "not-a-code-location"],
    ["missing path", `${head}:`],
    ["missing commit", ":src/app.go"],
    ["abbreviated commit", `${head.slice(0, 12)}:src/app.go`],
    ["over-long commit", `${head}0:src/app.go`],
    ["uppercase commit", `${head.toUpperCase()}:src/app.go`],
    ["symbolic revision", "HEAD:src/app.go"],
    ["legacy diff URI", `saga-diff://v1/file?base=${base}&head=${head}&path=src%2Fapp.go`],
    ["path traversal", `${head}:../../../etc/passwd`],
    ["embedded traversal", `${head}:src/../src/app.go`],
    ["absolute path", `${head}:/etc/passwd`],
    ["non-canonical single-line range", `${head}:src/app.go#L3-L3`],
    ["zero line", `${head}:src/app.go#L0`],
    ["inverted range", `${head}:src/app.go#L4-L3`],
    ["trailing fragment", `${canonical}#top`],
    ["commit outside the comparison", codeLocation(outside, "src/app.go")],
    ["unknown commit", codeLocation("0".repeat(40), "src/app.go")]
  ];
  const select = (ref: string) => serverRequest(saga.baseURL, `/api/code?file=${encodeURIComponent("src/app.go")}&ref=${encodeURIComponent(ref)}`);
  for (const [label, ref] of rejected) {
    const response = await select(ref);
    expect([400, 404], `code view with ${label} location answered ${response.status}`).toContain(response.status);
    expect(response.body, `code view with ${label} location`).not.toContain(saga.root);
  }
  expect((await select(`${head}:src/app.go#L3-L3`)).status, "code view with a non-canonical location").toBe(400);
  expect((await select(codeLocation(outside, "src/app.go"))).status, "code view at a commit outside the comparison").toBe(404);
  expect((await select(codeLocation(head, "src/app.go", line))).status, "code view with a changed line").toBe(200);
});

test("@critical never exposes filesystem paths in browser-facing responses", async ({ saga }) => {
  const responses: HTTPResponse[] = [
    await serverRequest(saga.baseURL, "/"),
    await serverRequest(saga.baseURL, "/does-not-exist"),
    await serverRequest(saga.baseURL, "/chapters/missing-chapter"),
    await serverRequest(saga.baseURL, "/f/diagram/..%2F..%2F..%2Fetc%2Fpasswd"),
    await serverRequest(saga.baseURL, "/f/no-such-fragment/content.md"),
    await serverRequest(saga.baseURL, "/", { headers: { Host: "attacker.test" } }),
    await serverRequest(saga.baseURL, `/api/code?file=${encodeURIComponent("../../escape")}&ref=not-a-code-location`),
    await serverRequest(saga.baseURL, `/api/history?target=${encodeURIComponent("../../escape")}`),
    await serverRequest(saga.baseURL, `/api/fragment?target=${encodeURIComponent("../../escape")}`)
  ];

  const secrets = [saga.root, saga.sagaRoot, saga.sourceRepo, saga.tempDir, tmpdir()];
  const phrases = ["no such file", "permission denied", "goroutine", "/private/var/folders"];
  for (const response of responses) {
    for (const secret of secrets) {
      expect(response.body, `response leaked ${secret}`).not.toContain(secret);
    }
    for (const phrase of phrases) {
      expect(response.body.toLowerCase(), `response leaked ${phrase}`).not.toContain(phrase);
    }
  }
});

test("@critical refuses to serve on a non-loopback address", async ({ sagaRepositories }) => {
  for (const address of ["0.0.0.0:0", "[::]:0", "192.0.2.10:7342", "example.test:7342"]) {
    const result = runCLI(sagaRepositories, ["serve", "--addr", address, sagaRepositories.sagaRoot]);
    expect(result.status, `serve --addr ${address}`).not.toBe(0);
    expect(`${result.stdout}${result.stderr}`, `serve --addr ${address}`).toContain("non-loopback");
    expect(result.stdout, `serve --addr ${address}`).not.toContain("Change Saga is available at");
  }
});
