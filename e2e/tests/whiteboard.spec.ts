import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

function authorWhiteboard(saga: SagaFixture, nested = false, opening = false, spacious = false) {
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
    const diagram = { version: 1, width: 720, height: 440, reveal: "fade", elements: [
      { id: entity, kind: "node", shape: "entity", icon: "lucide:database", label: entity === "local-edit" ? "Local edit" : "Cloud record", x: 40, y: 35, width: 380, height: 145, style: "normal", source: pin("data-entity", entity), fields: [{ name: "id", type: "uuid", key: "pk" }, { name: "body", type: "text" }] },
      { id: "local-note", kind: "annotation", visibility: "detail", shape: "bubble", label: "The saved definition explains these fields.", note: "The source model keeps this note beside its entities.", x: 180, y: 230, width: 470, height: 145, style: "normal", color: "blue", about: entity, source: pin("data-entity", entity) }
    ] };
    run("erd", "add", "--id", id, "--from", file(id + ".json", { name, explanation: "Independently readable source diagram.", directory: [pin("data-entity", entity)], bindings: [{ id: entity, element: entity, entity: pin("data-entity", entity) }], diagram }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  if (nested) {
    run("erd", "add", "--id", "cluster", "--from", file("cluster.json", {
      name: "Cloud cluster", explanation: "A smaller diagram inside its cloud context.", directory: [pin("data-entity", "cloud-record")],
      diagram: { version: 1, width: 720, height: 440, elements: [
        { id: "records", kind: "frame", label: "Shared records", summary: "Records inside the cloud", x: 300, y: 0, width: 360, height: 220, style: "normal", source: pin("erd", "postgres"), detail_scale: .85 }
      ] }
    }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  const svg = file("legacy.svg", '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 700 200"><g id="legacy-record"><rect x="20" y="20" width="660" height="160" fill="#f0f5ff" stroke="#537099"/><text x="45" y="110" font-size="32">Existing SVG stays independently readable</text></g><path id="legacy-line" d="M20 190L680 190" stroke="#537099"/></svg>');
  run("erd", "add", "--id", "legacy", "--from", file("legacy.json", { name: "Cross-store source", explanation: "An existing drawing without structured entities.", directory: [pin("data-entity", "local-edit"), pin("data-entity", "cloud-record")], bindings: [{ id: "saved-binding", element: "legacy-record", entity: pin("data-entity", "local-edit") }, { id: "line-binding", element: "legacy-line", entity: pin("data-entity", "local-edit") }] }), "--visual", svg, "--repo", saga.sourceRepo, saga.sagaRoot);
  const board = { version: 1, width: 1800, height: 1120, ...(opening ? { viewport: {x: 20, y: 20, width: 900, height: 700} } : {}), styles: {
    normal: { fill: "@diagram-normal-fill", stroke: "@diagram-normal-stroke", ink: "@diagram-normal-ink", stroke_width: 2, font_size: 34 },
    title: { fill: "none", stroke: "none", ink: "@diagram-title-ink", stroke_width: 1, font_size: 46 },
    primary: { fill: "@diagram-primary-fill", stroke: "@diagram-primary-stroke", ink: "@diagram-primary-ink", stroke_width: 2, font_size: 28 }
  }, elements: [
    { id: "title", kind: "text", label: "Offline edits become shared records", x: 60, y: 30, width: 1680, height: 65, style: "title" },
    { id: "overview", kind: "text", label: "Two stores, one synchronization flow. Focus a diagram to read its fields.", x: 60, y: 105, width: 1680, height: 65, style: "normal" },
    { id: "local", kind: "frame", label: "Local SQLite", summary: "Keeps edits available offline", x: 60, y: 210, width: 720, height: 440, style: "normal", source: pin("erd", "sqlite"), detail_scale: .85 },
    { id: "cloud", kind: "group", label: "Shared cloud", x: 1020, y: 210, width: 720, height: 440, style: "boundary" },
    { id: "postgres", kind: "frame", parent: "cloud", label: "Cloud Postgres", summary: "Stores synchronized shared records", x: 0, y: 0, width: 720, height: 440, style: "normal", source: pin("erd", nested ? "cluster" : "postgres"), detail_scale: .85 },
    { id: "sync", kind: "connection", note: "Delivery follows a saved generation.", from: "local/local-edit", to: nested ? "cloud/postgres/records/cloud-record" : "cloud/postgres/cloud-record", bends: nested ? [{ x: 900, y: 420 }] : [], label: "synchronizes into", style: "primary", head: "arrow", label_box: { x: 775, y: 330, width: 245, height: 65 } },
    { id: "flow-note", kind: "annotation", visibility: "overview", shape: "bubble", label: "Synchronization produces a cloud record. This flow is not a foreign key.", x: 60, y: 780, width: 580, height: 190, style: "normal", color: "blue", target: { x: 840, y: -290 } },
    { id: "legacy", kind: "frame", label: "Cross-store source", summary: "Existing SVG, shown as a whole diagram", x: 760, y: 760, width: 980, height: 290, style: "normal", source: pin("erd", "legacy"), detail_scale: .85 }
  ] };
  if (spacious) {
    board.width = 7000; board.height = 4500;
    Object.assign(board.elements.find(element => element.id === "cloud")!, { x: 5400, y: 2600 });
    Object.assign(board.elements.find(element => element.id === "legacy")!, { x: 2000, y: 3200 });
  }
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
  await expect(local.locator(":scope > rect")).toHaveCSS("fill", "rgba(0, 0, 0, 0)");
  await expect(board.locator('[data-frame-overview="local"]')).toBeVisible();
  await expect(board.locator('[data-frame-detail="local"]')).toBeVisible();
  await expect(board.locator('[data-instance-path="local/local-edit"]')).toBeVisible();
  await expect(board.locator('[data-instance-path="local/local-edit"]')).toHaveAttribute("tabindex", "-1");
  await expect(board.locator('[data-instance-path="flow-note"]')).toBeVisible();
  const connection = board.locator('[data-instance-path="sync"]');
  await expect(connection).toHaveAttribute("data-from", "local/local-edit");
  await expect(connection).toHaveAttribute("data-whiteboard-connection-mode", "overview");
  await connection.focus(); await page.keyboard.press("Enter");
  await page.locator(".diagram-affordance .element-note-trigger").click();
  await expect(page.locator("#element-note-popover")).toContainText("Delivery follows a saved generation.");
  await page.keyboard.press("Escape");
  await page.screenshot({ path: testInfo.outputPath("whiteboard-overview.png") });
  await expectNoSeriousAccessibilityViolations(page, "[data-whiteboard-view]");
  // The embedded ERD itself is the overview focus target, not a separate page.
  const previewEntity = await board.locator('[data-instance-path="local/local-edit"]').boundingBox();
  await page.mouse.click(previewEntity!.x + previewEntity!.width / 2, previewEntity!.y + previewEntity!.height / 2);
  await expect(local).toHaveAttribute("data-whiteboard-detail", "true");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);
  await board.getByRole("button", { name: "Fit board" }).click();
  await connection.focus(); await page.keyboard.press("Enter");
  await expect(page.locator(".diagram-affordance .element-note-trigger")).toBeVisible();
  await board.getByLabel("Focus section").focus();
  await board.getByLabel("Focus section").selectOption("local");
  await expect(local).toHaveAttribute("data-whiteboard-detail", "true");
  await expect(board.locator('[data-frame-detail="local"]')).toBeVisible();
  await expect(board.locator('[data-instance-path="local/local-note"]')).toBeVisible();
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
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
  await board.getByLabel("Focus section").selectOption("legacy");
  await expect(board.locator('[data-frame-detail="legacy"]')).toBeVisible();
  const opaqueBinding = board.locator('[data-instance-path="legacy/legacy-record"]');
  await expect(opaqueBinding).toHaveAttribute("data-documentation-target", /data-entity:local-edit$/);
  await expect(opaqueBinding).toHaveAttribute("tabindex", "0");
  await expect(opaqueBinding).toHaveAttribute("role", "button");
  await expect(opaqueBinding.locator('[data-diagram-summary="entity"]')).toBeVisible();
  await expect(opaqueBinding.locator('[data-diagram-detail="entity"]')).toBeHidden();
  await board.locator("[data-whiteboard-fields]").click();
  await expect(opaqueBinding.locator('[data-diagram-detail="entity"]')).toBeVisible();
  await expect(opaqueBinding.locator('[data-diagram-summary="entity"]')).toBeHidden();
  const lineBinding = board.locator('[data-instance-path="legacy/legacy-line"]');
  await expect(lineBinding).toHaveAttribute("tabindex", "0");
  await lineBinding.focus(); await page.keyboard.press("Enter");
  await expect(page.locator(".diff-drawer")).toContainText("The saved content.");
  await page.keyboard.press("Escape");
  await board.getByText("Source provenance", { exact: true }).click();
  await expect(board.locator('[data-whiteboard-frame="legacy"]')).toContainText("bound entities");
  await board.locator('[data-whiteboard-focus-frame="local"]').click();
  await expect(local).toHaveAttribute("data-whiteboard-detail", "true");
  await expect(board).toBeVisible();
  await expect(page.locator(".topbar")).toBeHidden();
  await board.getByText("Source provenance", { exact: true }).click();
  await board.getByRole("button", { name: "Fit board" }).click();
  for (let i = 0; i < 5; i++) await board.getByRole("button", { name: "Zoom in whiteboard" }).click();
  const entityControl = board.locator('[data-instance-path="local/local-edit"]');
  await expect(local).toHaveAttribute("data-whiteboard-detail", "true");
  await entityControl.focus();
  // Resize while fitted remains false: a large shrink crosses back into preview.
  await page.setViewportSize({ width: 400, height: 500 });
  await expect(local).toHaveAttribute("data-whiteboard-detail", "false");
  await expect(viewport).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);
  await page.setViewportSize({ width: 600, height: 800 });
  await board.getByRole("button", { name: "Fit board" }).click();
  await expect.poll(async () => (await viewport.boundingBox())?.height).toBe(800);
  const narrowCanvas = await viewport.boundingBox();
  expect(narrowCanvas!.x + narrowCanvas!.width).toBe(600);
  expect(narrowCanvas!.y).toBe(0);
  await expect(board.getByRole("button", { name: "Fit board" })).toBeInViewport();
  await expect(board.getByLabel("Focus section")).toBeInViewport();
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
  await board.getByLabel("Focus section").selectOption("local");
  await expect(edge).toHaveAttribute("data-whiteboard-from", "local/local-edit");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres/records");
  await expect(line).not.toHaveAttribute("d", overview!);
  // Authored bend and marker remain while just the endpoints adapt.
  await expect(line).toHaveAttribute("d", / L900 420 L/);
  await expect(line).toHaveAttribute("marker-end", /url\(#whiteboard-/);
  await expect(edge).toHaveAttribute("data-to", "cloud/postgres/records/cloud-record");
  await board.getByLabel("Focus section").selectOption("cloud/postgres");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  // A child at source y=0 has its caption above that source SVG viewport.
  // It must stay visible and clickable, rather than being clipped away.
  const nestedCaption = board.locator('[data-frame-overview="cloud/postgres/records"] text').first();
  await nestedCaption.click();
  await expect(board.getByLabel("Focus section")).toHaveValue("cloud/postgres/records");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres/records/cloud-record");
  await expect(board.locator('[data-instance-path="cloud/postgres/records/cloud-record"]')).toBeVisible();
  await expectNoSeriousAccessibilityViolations(page, "[data-whiteboard-view]");
  await board.getByRole("button", { name: "Fit board" }).click();
  await expect(edge).toHaveAttribute("data-whiteboard-from", "local");
  await expect(edge).toHaveAttribute("data-whiteboard-to", "cloud/postgres");
});


test("authored opening camera, fields and renderer reveal stay separate from full-board fit", async ({ page, saga }) => {
  authorWhiteboard(saga, false, true);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  const svg = board.locator("[data-whiteboard-viewport] > svg");
  const initial = await svg.getAttribute("viewBox");
  const fields = board.locator('[data-instance-path="local/local-edit"] [data-field]');
  await expect(fields.first()).toBeHidden();
  await board.locator('[data-whiteboard-fit]').click();
  await expect(svg).not.toHaveAttribute("viewBox", initial!);
  const whole = await svg.getAttribute("viewBox");
  await expect(page).toHaveURL(/view=all/);
  await page.reload();
  await waitForSettledSaga(page);
  await expect(svg).toHaveAttribute("viewBox", whole!);
  await board.locator('[data-whiteboard-home]').click();
  await expect(svg).toHaveAttribute("viewBox", initial!);
  await page.goBack();
  await expect(svg).toHaveAttribute("viewBox", whole!);
  await board.getByLabel("Focus section").selectOption("local");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  await expect(fields.first()).toBeHidden();
  await board.locator('[data-whiteboard-toolbar] [data-whiteboard-evidence]').click();
  await expect(page.locator('[data-erd-evidence]')).toContainText('members have no code references');
  await page.locator('.diff-drawer summary').filter({hasText:'Definition history'}).click();
  const historyRequest = page.waitForResponse(response => response.url().includes('/api/documentation?'));
  await page.locator('.diff-drawer details button[data-documentation-revision]').last().click();
  await historyRequest;
  await expect(page.locator('[data-erd-evidence]')).toBeVisible();
  await page.keyboard.press('Escape');
  await board.locator('[data-whiteboard-fields]').click();
  await expect(fields.first()).toBeVisible();
  await board.locator('[data-whiteboard-replay]').click();
  expect(await board.locator('[data-reveal-step]').evaluateAll(nodes => nodes.some(node => node.getAnimations().length > 0))).toBe(true);
  await board.locator('[data-whiteboard-fields]').click();
  await expect(fields.first()).toBeHidden();
  await board.locator('[data-instance-path="local/local-note"]').hover();
  await page.locator(".diagram-affordance .element-note-trigger").click();
  await expect(page.locator("#element-note-popover")).toContainText("source model keeps this note");
});


test("offscreen diagrams restore on focus while camera updates batch without screen transform reads", async ({ page, saga }) => {
  authorWhiteboard(saga, false, true, true);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator('[data-whiteboard-view]');
  const viewport = board.locator('[data-whiteboard-viewport]');
  const far = board.locator('[data-frame-path="cloud/postgres"]');
  const legacy = board.locator('[data-frame-path="legacy"]');
  await expect(far).toHaveAttribute('data-whiteboard-culled','true');
  await expect(far).toBeHidden();
  await expect(legacy.locator('[data-diagram-summary]')).toHaveCount(0);
  await board.getByLabel('Focus section').selectOption('local');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  const entity = board.locator('[data-instance-path="local/local-edit"]');
  await expect(entity.locator(':scope > svg')).toHaveCount(1); // shared Lucide icon
  await entity.hover();
  await expect(entity).toHaveClass(/diagram-interaction-active/);
  const note = board.locator('[data-instance-path="local/local-note"]');
  await note.hover();
  await page.locator('.diagram-affordance .element-note-trigger').click();
  await expect(page.locator('#element-note-popover')).toContainText('source model keeps this note');
  await page.keyboard.press('Escape');
  await expect(page.locator('#element-note-popover')).toBeHidden();
  await entity.hover();
  await viewport.focus();
  await note.hover();
  await page.locator('.diagram-affordance .element-note-trigger').click();
  await expect(page.locator('#element-note-popover')).toBeVisible();
  await viewport.dispatchEvent('keydown',{key:'ArrowRight',bubbles:true});
  await expect(page.locator('#element-note-popover')).toBeHidden();
  await expect(note).not.toHaveClass(/diagram-interaction-active/);
  await entity.focus();
  await board.locator('[data-whiteboard-focus]').evaluate((select: HTMLSelectElement) => { select.value = 'cloud/postgres'; select.dispatchEvent(new Event('change')); });
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await expect(far).toHaveAttribute('data-whiteboard-culled','false');
  await expect(far).toBeVisible();
  await expect(viewport).toBeFocused();
  await expect(entity).toHaveAttribute('tabindex','-1');
  await page.reload();
  await waitForSettledSaga(page);
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await expect(far).toBeVisible();
  await board.locator('[data-instance-path="cloud/postgres/cloud-record"]').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('.diff-drawer')).toContainText('The saved content.');
  await page.keyboard.press('Escape');
  await board.getByLabel('Focus section').selectOption('legacy');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await expect(legacy.locator('[data-diagram-summary]')).toHaveCount(1);
  await expect(legacy.locator('[data-diagram-summary]')).toBeVisible();

  // A burst of pointer input must coalesce into one camera render; panning
  // does not require screen transforms or rebuilding keyboard affordances.
  await viewport.evaluate(node => node.addEventListener('pointerdown', event => { (window as any).__pointer = (event as PointerEvent).pointerId; },{once:true}));
  const box = (await viewport.boundingBox())!;
  await page.mouse.move(box.x + 20, box.y + box.height/2); await page.mouse.down();
  const stats = await viewport.evaluate(async node => {
    let transforms = 0, cameraWrites = 0;
    const original = SVGGraphicsElement.prototype.getScreenCTM;
    SVGGraphicsElement.prototype.getScreenCTM = function() { transforms++; return original.call(this); };
    const svg = node.querySelector('svg')!;
    const observer = new MutationObserver(records => { cameraWrites += records.length; });
    observer.observe(svg,{attributes:true,attributeFilter:['viewBox']});
    const box = node.getBoundingClientRect();
    for (let i=1;i<=120;i++) node.dispatchEvent(new PointerEvent('pointermove',{bubbles:true,pointerId:(window as any).__pointer,clientX:box.x+20+i,clientY:box.y+box.height/2}));
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    observer.disconnect(); SVGGraphicsElement.prototype.getScreenCTM = original;
    return {transforms,cameraWrites};
  });
  await page.mouse.up();
  expect(stats.transforms).toBe(0);
  expect(stats.cameraWrites).toBe(1);
  await expect(board).toHaveAttribute('data-whiteboard-interacting','false');
  await board.locator('[data-whiteboard-home]').click();
  await expect(legacy).toBeHidden();
  await expect(legacy.locator('[data-diagram-summary]')).toHaveCount(1);
  await board.getByLabel('Focus section').selectOption('legacy');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await expect(legacy.locator('[data-diagram-summary]')).toHaveCount(1);
  await expect(legacy.locator('[data-diagram-summary]')).toBeVisible();
});

test("wheel and trackpad scroll pan the board while modified scroll zooms", async ({ page, saga }) => {
  authorWhiteboard(saga, false, true);
  await page.goto(saga.baseURL + '/technical/erd');
  await waitForSettledSaga(page);
  const board = page.locator('[data-whiteboard-view]');
  const viewport = board.locator('[data-whiteboard-viewport]');
  const svg = viewport.locator(':scope > svg');
  const camera = () => svg.evaluate((node: SVGSVGElement) => {
    const {x,y,width,height} = node.viewBox.baseVal;
    return {x,y,width,height};
  });
  const start = await camera(), box = (await viewport.boundingBox())!;
  const scale = Math.min(box.width/start.width,box.height/start.height);
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2);
  await page.mouse.wheel(120,90);
  await expect.poll(async () => (await camera()).y).not.toBe(start.y);
  const scrolled = await camera();
  expect(scrolled.x).toBeCloseTo(start.x+120/scale,3);
  expect(scrolled.y).toBeCloseTo(start.y+90/scale,3);
  expect(scrolled.width).toBe(start.width);
  expect(scrolled.height).toBe(start.height);
  expect(await page.evaluate(() => window.scrollY)).toBe(0);

  // Hardware may emit line/page deltas rather than pixels. Shift provides
  // horizontal movement for a mouse with only a vertical wheel.
  for (const [deltaMode,shiftKey,expectedX,expectedY] of [[1,true,32,0],[2,false,0,box.height]] as const) {
    const before = await camera();
    const canceled = await viewport.evaluate((node,args) => {
      const event = new WheelEvent('wheel',{bubbles:true,cancelable:true,deltaY:args.deltaMode === 1 ? 2 : 1,...args});
      return !node.dispatchEvent(event);
    },{deltaMode,shiftKey});
    expect(canceled).toBe(true);
    await expect.poll(async () => (await camera())[shiftKey ? 'x' : 'y']).not.toBe(before[shiftKey ? 'x' : 'y']);
    const after = await camera();
    expect(after.x).toBeCloseTo(before.x+expectedX/scale,3);
    expect(after.y).toBeCloseTo(before.y+expectedY/scale,3);
    expect(after.width).toBe(before.width);
  }
  await page.keyboard.down('Control');
  const beforeZoom = await camera();
  await page.mouse.wheel(0,-100);
  await page.keyboard.up('Control');
  await expect.poll(async () => (await camera()).width).toBeLessThan(beforeZoom.width);
  await expect(board).toHaveAttribute('data-whiteboard-interacting','false');
  await board.locator('[data-whiteboard-home]').click();
  await expect.poll(camera).toEqual(start);
});
