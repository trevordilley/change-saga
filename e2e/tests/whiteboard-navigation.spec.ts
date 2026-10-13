import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

function navigationBoard(saga: SagaFixture) {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const file = (name: string, value: unknown) => {
    const path = join(saga.root, name + ".json");
    writeFileSync(path, JSON.stringify(value));
    return path;
  };
  const pin = (kind: string, id: string) => ({ target: `urn:change-saga:wave-one:${kind}:${id}`, revision: `urn:change-saga:wave-one:${kind}:${id}:revision:r1` });
  run("inventory", "adopt-format", "--format", "2", saga.sagaRoot);
  run("data-entity", "add", "--id", "record", "--from", file("record", { name: "Saved record", explanation: "A saved record.", intent: "proposed", baseline: "none", fields: [{ id: "id", name: "id", type: "uuid" }] }), "--repo", saga.sourceRepo, saga.sagaRoot);
  for (const id of ["local", "remote"]) {
    run("erd", "add", "--id", id, "--from", file(id, { name: id + " diagram", explanation: "An embedded source.", directory: [pin("data-entity", "record")], diagram: { version: 1, width: 600, height: 300, elements: [{ id: "record", kind: "node", shape: "entity", label: "Saved record", x: 30, y: 30, width: 400, height: 180, style: "normal", source: pin("data-entity", "record"), fields: [{ name: "id", type: "uuid" }] }] } }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  // Deliberately author z before a: link resolution must use instance identity,
  // not incidental DOM order, while each repeat retains its own deep link.
  run("diagram", "publish", "--whiteboard", "erd", "--name", "Navigation board", "--revision", "r1", "--from", file("board", { version: 1, width: 2000, height: 1200, elements: [
    { id: "z-copy", kind: "frame", label: "Second local", x: 1100, y: 50, width: 600, height: 300, style: "normal", source: pin("erd", "local") },
    { id: "a-first", kind: "frame", label: "First local", x: 50, y: 50, width: 600, height: 300, style: "normal", source: pin("erd", "local") },
    { id: "remote", kind: "frame", label: "Remote", x: 50, y: 700, width: 600, height: 300, style: "normal", source: pin("erd", "remote") }
  ] }), saga.sagaRoot);
}

test("ERD links focus exact instances without replacing the whiteboard and restore history", async ({ page, saga }) => {
  navigationBoard(saga);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  await board.evaluate(node => node.setAttribute("data-navigation-sentinel", "kept"));
  const requests: string[] = [];
  page.on("request", request => { if (new URL(request.url()).pathname.startsWith("/technical/")) requests.push(request.url()); });
  // The existing sidebar remains the way to choose a diagram.
  const sidebarLink = page.locator('.sidebar a[data-whiteboard-source-href="/technical/erd/local"]');
  await sidebarLink.evaluate((node: HTMLAnchorElement) => node.click());
  await expect(page).toHaveURL(/focus=a-first$/);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await board.locator("[data-whiteboard-focus]").selectOption("z-copy");
  await expect(page).toHaveURL(/focus=z-copy$/);
  await expect(board).toHaveAttribute("data-navigation-sentinel", "kept");
  await page.goBack();
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await page.goBack();
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("");
  await page.goForward();
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  expect(requests).toEqual([]);
  await page.reload();
  await waitForSettledSaga(page);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await page.goto(saga.baseURL + "/technical/erd?focus=z-copy");
  await waitForSettledSaga(page);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("z-copy");
  await page.goto(saga.baseURL + "/technical/erd?focus=removed-frame");
  await waitForSettledSaga(page);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("");
});

test("latest ERD routes enter the board; exact historical routes and entity inspection survive", async ({ page, saga }) => {
  navigationBoard(saga);
  await page.goto(saga.baseURL + "/technical/erd/local");
  await waitForSettledSaga(page);
  await expect(page).toHaveURL(/\/technical\/erd\?focus=a-first$/);
  await expect(page.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await page.locator('[data-instance-path="a-first/record"]').click();
  await expect(page.locator(".diff-drawer.open")).toBeVisible();
  await expect(page).toHaveURL(/focus=a-first$/);
  await page.goto(saga.baseURL + "/technical/erd/local?revision=r1");
  await waitForSettledSaga(page);
  await expect(page.locator('[data-erd-revision="r1"]')).toBeVisible();
  await expect(page.locator("[data-whiteboard-view]")).toHaveCount(0);
  await page.goto(saga.baseURL + "/technical");
  await waitForSettledSaga(page);
  await page.locator('.sidebar a[href="/technical/erd/remote"]').evaluate((node: HTMLAnchorElement) => node.click());
  await expect(page).toHaveURL(/\/technical\/erd\?focus=remote$/);
  await expect(page.locator("[data-whiteboard-focus]")).toHaveValue("remote");
});
