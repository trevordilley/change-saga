import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

function authorWhiteboard(saga: SagaFixture, nested = false) {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, args.join(" ") + "\n" + result.stdout + result.stderr).toBe(0);
  };
  const file = (name: string, data: unknown) => {
    const path = join(saga.root, name);
    writeFileSync(path, typeof data === "string" ? data : JSON.stringify(data));
    return path;
  };
  const pin = (kind: string, id: string) => ({ target: `urn:change-saga:wave-one:${kind}:${id}`, revision: `urn:change-saga:wave-one:${kind}:${id}:revision:r1` });
  run("inventory", "adopt-format", "--format", "2", saga.sagaRoot);
  for (const [id, name] of [["local-edit", "Local edit"], ["cloud-record", "Cloud record"]]) {
    run("data-entity", "add", "--id", id, "--from", file(id + ".json", { name, explanation: "Records used by synchronization.", intent: "proposed", baseline: "none", fields: [{ id: "id", name: "id", type: "uuid", keys: ["primary"] }, { id: "body", name: "body", type: "text", note: "The saved content." }] }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  for (const [id, entity, name] of [["sqlite", "local-edit", "Local SQLite"], ["postgres", "cloud-record", "Cloud Postgres"]]) {
    const diagram = { version: 1, width: 720, height: 440, elements: [
      { id: entity, kind: "node", shape: "entity", label: entity === "local-edit" ? "Local edit" : "Cloud record", x: 40, y: 35, width: 380, height: 145, style: "normal", source: pin("data-entity", entity), fields: [{ name: "id", type: "uuid", key: "pk" }, { name: "body", type: "text" }] },
      { id: "local-note", kind: "annotation", visibility: "detail", shape: "bubble", label: "The saved definition explains these fields.", x: 180, y: 230, width: 470, height: 145, style: "normal", color: "blue", about: entity, source: pin("data-entity", entity) }
    ] };
    run("erd", "add", "--id", id, "--from", file(id + ".json", { name, explanation: "Independently readable source diagram.", directory: [pin("data-entity", entity)], bindings: [{ id: entity, element: entity, entity: pin("data-entity", entity) }], diagram }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  if (nested) {
    run("erd", "add", "--id", "cluster", "--from", file("cluster.json", {
      name: "Cloud cluster", explanation: "A smaller diagram inside its cloud context.", directory: [pin("data-entity", "cloud-record")],
      diagram: { version: 1, width: 720, height: 440, elements: [
        { id: "records", kind: "frame", label: "Shared records", summary: "Records inside the cloud", x: 100, y: 120, width: 360, height: 220, style: "normal", source: pin("erd", "postgres"), detail_scale: .85 }
      ] }
    }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  const svg = file("legacy.svg", '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 700 200"><rect x="20" y="20" width="660" height="160" fill="#f0f5ff" stroke="#537099"/><text x="45" y="110" font-size="32">Existing SVG stays independently readable</text></svg>');
  run("erd", "add", "--id", "legacy", "--from", file("legacy.json", { name: "Cross-store source", explanation: "An existing drawing without structured entities.", directory: [pin("data-entity", "local-edit"), pin("data-entity", "cloud-record")] }), "--visual", svg, "--repo", saga.sourceRepo, saga.sagaRoot);
  const board = { version: 1, width: 1800, height: 1120, styles: {
    normal: { fill: "@diagram-normal-fill", stroke: "@diagram-normal-stroke", ink: "@diagram-normal-ink", stroke_width: 2, font_size: 34 },
    title: { fill: "none", stroke: "none", ink: "@diagram-title-ink", stroke_width: 1, font_size: 46 },
    primary: { fill: "@diagram-primary-fill", stroke: "@diagram-primary-stroke", ink: "@diagram-primary-ink", stroke_width: 2, font_size: 28 }
  }, elements: [
    { id: "title", kind: "text", label: "Offline edits become shared records", x: 60, y: 30, width: 1680, height: 65, style: "title" },
    { id: "overview", kind: "text", label: "Two stores, one synchronization flow. Focus a diagram to read its fields.", x: 60, y: 105, width: 1680, height: 65, style: "normal" },
    { id: "local", kind: "frame", label: "Local SQLite", summary: "Keeps edits available offline", x: 60, y: 210, width: 720, height: 440, style: "normal", source: pin("erd", "sqlite"), detail_scale: .85 },
    { id: "cloud", kind: "group", label: "Shared cloud", x: 1020, y: 210, width: 720, height: 440, style: "boundary" },
    { id: "postgres", kind: "frame", parent: "cloud", label: "Cloud Postgres", summary: "Stores synchronized shared records", x: 0, y: 0, width: 720, height: 440, style: "normal", source: pin("erd", nested ? "cluster" : "postgres"), detail_scale: .85 },
    { id: "sync", kind: "connection", from: "local/local-edit", to: nested ? "cloud/postgres/records/cloud-record" : "cloud/postgres/cloud-record", bends: nested ? [{ x: 900, y: 420 }] : [], label: "synchronizes into", style: "primary", head: "arrow", label_box: { x: 775, y: 330, width: 245, height: 65 } },
    { id: "flow-note", kind: "annotation", visibility: "overview", shape: "bubble", label: "Synchronization produces a cloud record. This flow is not a foreign key.", x: 60, y: 780, width: 580, height: 190, style: "normal", color: "blue", target: { x: 840, y: -290 } },
    { id: "legacy", kind: "frame", label: "Cross-store source", summary: "Existing SVG, shown as a whole diagram", x: 760, y: 760, width: 980, height: 290, style: "normal", source: pin("erd", "legacy"), detail_scale: .85 }
  ] };
  run("diagram", "publish", "--whiteboard", "erd", "--from", file("board.json", board), "--name", "Offline and shared data", "--revision", "r1", saga.sagaRoot);
}

test("whiteboard explains the overview and focuses pinned entity detail @critical", async ({ page, saga }, testInfo) => {
  authorWhiteboard(saga);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  const viewport = board.locator("[data-whiteboard-viewport]");
  const svg = viewport.locator(":scope > svg");
  const local = board.locator('[data-frame-path="local"]');
  await expect(board.locator("[data-whiteboard-note]")).toHaveCount(0);
  await expect(page.locator(".topbar")).toBeHidden();
  await expect(board.getByRole("heading", { level: 1 })).toHaveText("Offline and shared data");
  const canvas = await viewport.boundingBox();
  const sidebar = await page.locator(".sidebar").boundingBox();
  const windowSize = page.viewportSize()!;
  expect(canvas).toEqual({ x: sidebar!.width, y: 0, width: windowSize.width - sidebar!.width, height: windowSize.height });
  const fittedBox = await svg.getAttribute("viewBox");
  await board.locator(".whiteboard-description > summary").click();
  await expect(board.locator(".whiteboard-description-body")).toBeVisible();
  expect(await viewport.boundingBox()).toEqual(canvas);
  await board.locator(".whiteboard-description > summary").click();
  await expect(board.locator(".whiteboard-description-body")).toBeHidden();

  await expect(local).toHaveAttribute("data-whiteboard-detail", "false");
  await expect(board.locator('[data-frame-overview="local"]')).toBeVisible();
  await expect(board.locator('[data-frame-detail="local"]')).toBeHidden();
  await expect(board.locator('[data-instance-path="flow-note"]')).toBeVisible();
  const connection = board.locator('[data-instance-path="sync"]');
  await expect(connection).toHaveAttribute("data-from", "local/local-edit");
  await expect(connection).toHaveAttribute("data-whiteboard-connection-mode", "overview");
  await page.screenshot({ path: testInfo.outputPath("whiteboard-overview.png") });
  await expectNoSeriousAccessibilityViolations(page, "[data-whiteboard-view]");
  await connection.focus(); await page.keyboard.press("Enter");
  await expect(board.locator("[data-whiteboard-inspection]")).toContainText("local/local-edit → cloud/postgres/cloud-record");
  await board.getByLabel("Focus diagram").focus();
  await board.getByLabel("Focus diagram").selectOption("local");
  await expect(local).toHaveAttribute("data-whiteboard-detail", "true");
  await expect(board.locator('[data-frame-detail="local"]')).toBeVisible();
  await expect(board.locator('[data-instance-path="local/local-note"]')).toBeVisible();
  await board.screenshot({ path: testInfo.outputPath("whiteboard-detail.png") });
  await board.locator('[data-instance-path="local/local-edit"]').focus(); await page.keyboard.press("Enter");
  await expect(page.locator(".diff-drawer")).toContainText("The saved content.");
  await expect(page.locator(".diff-drawer")).toContainText("uuid");
  await page.keyboard.press("Escape");
  const before = await svg.getAttribute("viewBox");
  await viewport.focus(); await page.keyboard.press("+");
  await expect(svg).not.toHaveAttribute("viewBox", before!);
  const zoomed = await svg.getAttribute("viewBox");
  const box = await viewport.boundingBox();
  await page.mouse.move(box!.x + 40, box!.y + box!.height / 2); await page.mouse.down();
  await page.mouse.move(box!.x + 100, box!.y + box!.height / 2 + 40, { steps: 5 }); await page.mouse.up();
  await expect(svg).not.toHaveAttribute("viewBox", zoomed!);
  await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);
  await viewport.focus(); await page.keyboard.press("0");
  await expect(svg).toHaveAttribute("viewBox", fittedBox!);
  await expect(local).toHaveAttribute("data-whiteboard-detail", "false");
  await board.getByLabel("Focus diagram").selectOption("legacy");
  await expect(board.locator('[data-frame-detail="legacy"]')).toBeVisible();
  await board.getByText("Diagram sources and saved revisions", { exact: true }).click();
  await expect(board.locator('[data-whiteboard-frame="legacy"]')).toContainText("open the source ERD for entity inspection");
  await board.locator('[data-whiteboard-frame="local"] a').click();
  await expect(page).toHaveURL(/\/technical\/erd\/sqlite\?revision=r1$/);
  await expect(page.locator('[data-erd-view]')).toHaveAttribute("data-erd-revision", "r1");
  await expect(page.locator(".topbar")).toBeVisible();
  await page.goBack(); await waitForSettledSaga(page);
  await expect(page.locator('[data-whiteboard-toolbar]')).toBeVisible();
  await expect(page.locator(".topbar")).toBeHidden();
  await page.setViewportSize({ width: 600, height: 800 });
  await expect.poll(async () => (await viewport.boundingBox())?.height).toBe(800);
  const narrowCanvas = await viewport.boundingBox();
  expect(narrowCanvas!.x + narrowCanvas!.width).toBe(600);
  expect(narrowCanvas!.y).toBe(0);
  await expect(board.getByRole("button", { name: "Fit board" })).toBeInViewport();
  await expect(board.getByLabel("Focus diagram")).toBeInViewport();
  await board.locator(".whiteboard-description > summary").click();
  await page.screenshot({ path: testInfo.outputPath("whiteboard-narrow.png") });
  await expectNoSeriousAccessibilityViolations(page, "[data-whiteboard-view]");

});

test("mixed nested detail keeps each connection on its visible semantic endpoint", async ({ page, saga }) => {
  authorWhiteboard(saga, true);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  const edge = board.locator('[data-instance-path="sync"]');
  const line = edge.locator('[data-connection-detail] > path');
  await expect(edge).toHaveAttribute("data-whiteboard-from", "local");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres");
  const overview = await line.getAttribute("d");
  // Without choosing a focus, one nested frame remains collapsed while the
  // other diagram is readable. Its local detail note follows that diagram.
  for (let i = 0; i < 4; i++) await board.getByRole("button", { name: "Zoom in whiteboard" }).click();
  await expect(board.locator('[data-frame-path="local"]')).toHaveAttribute("data-whiteboard-detail", "true");
  await expect(board.locator('[data-frame-path="cloud/postgres/records"]')).toHaveAttribute("data-whiteboard-detail", "false");
  await expect(board.locator('[data-instance-path="local/local-note"]')).toBeVisible();
  await expect(board.locator('[data-instance-path="flow-note"]')).toBeVisible();
  await board.getByLabel("Focus diagram").selectOption("local");
  await expect(edge).toHaveAttribute("data-whiteboard-from", "local/local-edit");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres/records");
  await expect(line).not.toHaveAttribute("d", overview!);
  // Authored bend and marker remain while just the endpoints adapt.
  await expect(line).toHaveAttribute("d", / L900 420 L/);
  await expect(line).toHaveAttribute("marker-end", /url\(#whiteboard-/);
  await expect(edge).toHaveAttribute("data-to", "cloud/postgres/records/cloud-record");
  await board.getByLabel("Focus diagram").selectOption("cloud/postgres/records");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres/records/cloud-record");
  await expect(board.locator('[data-instance-path="cloud/postgres/records/cloud-record"]')).toBeVisible();
  await expectNoSeriousAccessibilityViolations(page, "[data-whiteboard-view]");
  await board.getByRole("button", { name: "Fit board" }).click();
  await expect(edge).toHaveAttribute("data-whiteboard-from", "local");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres");
});
