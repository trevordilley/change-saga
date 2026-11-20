import { writeFileSync, readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { test, expect, waitForSettledSaga } from "../support/test.js";
import { git, runCLI } from "../support/fixture-builder.js";

test("current whiteboard edits preserve identity and refuse evidence from an older page", async ({ page, saga }) => {
  const source = join(saga.root, "current-board.json");
  const document = {version:1, width:800, height:500, elements:[{id:"owner", kind:"node", shape:"rect", label:"First explanation", x:80, y:80, width:300, height:150, style:"normal", evidence:[{commit:saga.identity.head, path:"src/app.go", start:1, end:1, note:"Exact source declaration"}]}]};
  const publish = () => {
    writeFileSync(source, JSON.stringify(document));
    const result = runCLI(saga, ["whiteboard", "publish", "--id", "current-map", "--name", "Current map", "--repo", saga.sourceRepo, "--from", source, saga.sagaRoot]);
    expect(result.status, result.stderr + result.stdout).toBe(0);
  };
  publish();
  git(saga.sagaRepo,"add",".");
  git(saga.sagaRepo,"commit","-m","Document original current map");
  await page.goto(saga.baseURL + "/technical/whiteboards/current-map");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  const first = await board.getAttribute("data-whiteboard-content-digest");
  expect(first).toMatch(/^sha256:[0-9a-f]{64}$/);
  const query = new URLSearchParams({board:"current-map", element:"owner", content_digest:first!});
  expect((await page.request.get(saga.baseURL + "/api/whiteboard-element?" + query)).status()).toBe(200);
  await expect(page.locator("[data-whiteboard-history]")).toHaveCount(0);
  const owner = board.locator('[data-whiteboard-element="owner"]');
  await owner.hover();
  const actions = page.locator('[data-diagram-affordance]');
  await expect(actions.locator('[data-target-code-href]')).toBeVisible();
  await actions.locator('[data-target-code-href]').click();
  await expect(page.locator('.drawer-body [data-file-path="src/app.go"]')).toBeVisible();
  await expect(page.locator('.drawer-body')).toContainText('Exact source declaration');
  await page.keyboard.press('Escape');

  document.elements[0].label = "Updated explanation";
  document.elements[0].evidence[0].note = "Updated source annotation";
  publish();
  expect((await page.request.get(saga.baseURL + "/api/whiteboard-element?" + query)).status()).toBe(409);
  await page.reload();
  await waitForSettledSaga(page);
  await expect(board).toHaveAttribute("data-whiteboard-id", "current-map");
  expect(await board.getAttribute("data-whiteboard-content-digest")).not.toBe(first);
  await expect(board.locator('[data-whiteboard-element="owner"]')).toContainText("Updated explanation");
  expect((await page.request.get(saga.baseURL + "/technical/whiteboards/current-map?revision=r1")).status()).toBe(410);
  const folder = join(saga.sagaRoot, "___whiteboards", "current-map");
  const stored = JSON.parse(readFileSync(join(folder,"current.json"),"utf8"));
  expect(stored.content.document.elements[0].id).toBe("owner");
  expect(stored).not.toHaveProperty("revisions");
  expect(stored.content).not.toHaveProperty("parents");
  expect(existsSync(join(folder,"revisions"))).toBe(false);

  // Explicit historical viewing reads the committed bytes, while an old
  // current-page action above remains a conflict. It must never look current.
  await page.goto(saga.baseURL + "/technical/whiteboards/current-map?" + new URLSearchParams({content_digest:first!}));
  await waitForSettledSaga(page);
  await expect(board).toHaveAttribute("data-whiteboard-historical","true");
  await expect(board).toContainText("Saved whiteboard content from Git");
  await expect(owner).toContainText("First explanation");
  await owner.hover();
  await expect(actions.locator('[data-target-code-href]')).toBeVisible();
  await actions.locator('[data-target-code-href]').click();
  await expect(page.locator('.drawer-body')).toContainText("Exact source declaration");
  await expect(page.locator('.drawer-body')).not.toContainText("Updated source annotation");
});
