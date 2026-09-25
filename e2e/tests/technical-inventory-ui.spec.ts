import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

// Technical design: a landing page over three areas (the ERD, Systems and
// Components), each a page of its own and a section of the sidebar; the
// canonical page of one exact revision; and the round trip from a slide Item
// to that definition, its usages, and back to the same slide.

const prefix = "urn:change-saga:wave-one:";
const systemTarget = prefix + "system:FeatureFlag";

function author(saga: SagaFixture) {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const code = [{ commit: "HEAD", path: "src/app.go", start: 3, end: 3, note: "Exact implementation line used by this disposable definition." }];
  const definition = (id: string, extra = {}) => {
    const path = join(saga.root, id + ".json");
    writeFileSync(path, JSON.stringify({ name: id, explanation: "A shared implementation unit with exact code and a stable identity.", code, ...extra }));
    return path;
  };
  for (const id of ["FlagClient", "FlagStore"]) run("component", "add", "--id", id, "--from", definition(id), "--repo", saga.sourceRepo, saga.sagaRoot);
  const pins = ["FlagClient", "FlagStore"].map(id => ({ target: prefix + "component:" + id, revision: prefix + "component:" + id + ":revision:r1" }));
  const system = definition("FeatureFlag", { components: pins, interactions: [{ id: "read", from: pins[0].target, to: pins[1].target, description: "Read a flag value from the shared store.", code }] });
  run("system", "add", "--id", "FeatureFlag", "--from", system, "--repo", saga.sourceRepo, saga.sagaRoot);
  run("add-deck", "--feature", "wave-one", "--id", "flag-contexts", "--objective", "A slide uses the shared flag System", saga.sagaRoot, "flags");
  run("add-slide", "--deck", "flag-contexts", "--id", "checkout-flag", "--title", "checkout-flag", "--intent", "explain", "--layout", "diagram", saga.sagaRoot, "checkout-flag");
  run("add-item", "--slide", "checkout-flag", "--id", "shared", "--kind", "node", "--element-id", "slide-title", "--description", "Checkout reads the shared flag decision.", "--documentation", systemTarget, "--documentation-revision", systemTarget + ":revision:r1", saga.sagaRoot);
  return { run, system };
}

test("Technical design lists shared definitions and traces a slide Item to its pinned page and back @critical", async ({ page, saga }) => {
  const { run, system } = author(saga);
  await page.goto(saga.baseURL + "/");
  await waitForSettledSaga(page);
  const part = page.locator('[data-overview-part="Technical design"]');
  await expect(part).toContainText("1 System · 2 Components");
  await part.getByRole("link", { name: "Technical design" }).click();
  await expect(page).toHaveURL(/\/technical$/);
  // The landing page names the areas; it is not the directories.
  await expect(page.locator("[data-technical-area]")).toHaveCount(3);
  await expect(page.locator('[data-technical-area="erd"]')).toContainText("No data model yet");
  await expect(page.locator("[data-directory]")).toHaveCount(0);
  await page.locator('[data-technical-area="systems"]').getByRole("link", { name: "Systems" }).click();
  await expect(page).toHaveURL(/\/technical\/systems$/);
  const systems = page.locator('[data-technical-kind="system"]');
  const row = systems.locator('[data-directory-row="FeatureFlag"]');
  await expect(row).toContainText("unspecified");
  await expect(row).toContainText("active");
  // Columns: explanation, intent, lifecycle, revision, code, used by, since.
  await expect(row.locator("td").nth(5)).toHaveText("1");
  // This fixture compares against main with the Saga in a companion
  // repository, whose base inventory cannot be read: newness is unknown,
  // never "new".
  await expect(row.locator("td").nth(6)).toHaveText("unknown");
  await expect(page.locator("[data-technical-newness]")).toHaveAttribute("data-technical-newness", "false");
  await expect(page.locator('[data-technical-kind="component"]')).toHaveCount(0);

  // Keyboard: the definition link is reachable and opens its canonical page.
  const link = row.getByRole("link", { name: "FeatureFlag" });
  await link.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/technical\/system\/FeatureFlag$/);
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r1");
  await expect(page.locator(".documentation-explanation")).toContainText("Read a flag value");
  const usage = page.locator("[data-technical-usages] [data-usage-item]");
  await expect(usage).toHaveText("shared");

  // The usage opens the exact slide; its Item opens the same pinned definition.
  await usage.click();
  await waitForSettledSaga(page);
  const slide = page.locator("[data-deck-slide]:visible");
  await expect(slide).toHaveAttribute("data-slide-title", "checkout-flag");
  const slideURL = page.url();
  await slide.locator(".landmark-menu summary").click();
  const open = slide.locator(".landmark-menu [data-documentation-target]");
  await open.click();
  await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin", systemTarget + ":revision:r1");
  const usedBy = page.locator(".drawer-body details.documentation-usages");
  await usedBy.locator("summary").click();
  await expect(usedBy.locator("[data-usage-item]")).toHaveText("shared");
  await expect(page.locator(".drawer-body [data-documentation-page]")).toHaveAttribute("href", "/technical/system/FeatureFlag?revision=r1");
  await page.keyboard.press("Escape");
  await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);
  await expect(open).toBeFocused();
  expect(page.url()).toBe(slideURL);

  // A revised definition leaves the saved pin readable and says so.
  run("system", "revise", "--id", "FeatureFlag", "--revision", "r2", "--parent", systemTarget + ":revision:r1", "--from", system, "--repo", saga.sourceRepo, saga.sagaRoot);
  await page.goto(saga.baseURL + "/technical/system/FeatureFlag?revision=r1");
  await expect(page.locator("[data-technical-pin-status]")).toHaveAttribute("data-technical-pin-status", "stale");
  await expect(page.locator("[data-technical-usage]")).toHaveAttribute("data-usage-status", "stale");
  await page.getByRole("link", { name: "Read the current definition" }).click();
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r2");
  await page.reload();
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r2");
  await page.goto(saga.baseURL + "/technical/systems");
  await expect(page.locator('[data-directory-row="FeatureFlag"]')).toContainText("1 not current");

  // Members open their own pinned pages.
  await page.goto(saga.baseURL + "/technical/system/FeatureFlag?revision=r1");
  await page.locator(".documentation-members").getByRole("link", { name: "FlagStore" }).click();
  await expect(page).toHaveURL(/\/technical\/component\/FlagStore\?revision=r1$/);
});

test("Technical design stays readable on a 390px touch screen", async ({ browser, saga }) => {
  author(saga);
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const narrow = await context.newPage();
  await narrow.goto(saga.baseURL + "/technical/systems");
  await waitForSettledSaga(narrow);
  // A mobile browser widens its layout viewport to fit wide content, so
  // compare against the device width rather than innerWidth.
  expect(await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth])).toEqual([390, 390]);
  // The directory's header is sticky; bring the row to the middle of the
  // screen the way a reader scrolling to it would, rather than under it.
  const row = narrow.locator('[data-directory-row="FeatureFlag"]').getByRole("link", { name: "FeatureFlag" });
  // Late-arriving shell content reflows the page, so settle it first.
  await narrow.waitForLoadState("networkidle");
  await row.evaluate(element => element.scrollIntoView({ block: "center" }));
  await row.tap();
  await expect(narrow.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r1");
  expect(await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth])).toEqual([390, 390]);
  await narrow.locator("[data-technical-usages] [data-usage-item]").tap();
  await waitForSettledSaga(narrow);
  await expect(narrow.locator("[data-deck-slide]:visible")).toHaveAttribute("data-slide-title", "checkout-flag");

  // The sidebar's areas open by touch, and a definition row opens its page.
  await narrow.goto(saga.baseURL + "/technical");
  await waitForSettledSaga(narrow);
  const toggle = narrow.getByRole("button", { name: "Toggle Components", exact: true });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await toggle.tap();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await narrow.locator("#nav-technical-components").getByRole("link", { name: "FlagStore" }).tap();
  await expect(narrow).toHaveURL(/\/technical\/component\/FlagStore$/);
  expect(await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth])).toEqual([390, 390]);
  await context.close();
});

test("the sidebar nests Technical design's areas and marks where the reader is", async ({ page, saga }) => {
  author(saga);
  await page.goto(saga.baseURL + "/technical/component/FlagStore");
  await waitForSettledSaga(page);
  const sidebar = page.locator(".sidebar");
  // The definition is the current row, inside its open area, inside
  // Technical design; the other area stays shut. With no data model yet the
  // ERD area is not listed.
  await expect(sidebar.locator('a[aria-current="page"]')).toHaveAttribute("href", "/technical/component/FlagStore");
  await expect(sidebar.getByRole("button", { name: "Toggle Technical design", exact: true })).toHaveAttribute("aria-expanded", "true");
  await expect(sidebar.getByRole("button", { name: "Toggle Components", exact: true })).toHaveAttribute("aria-expanded", "true");
  await expect(sidebar.getByRole("button", { name: "Toggle Systems", exact: true })).toHaveAttribute("aria-expanded", "false");
  await expect(sidebar.locator('a[href="/technical/erd"]')).toHaveCount(0);
  await expect(page.locator(".requirements-breadcrumbs")).toContainText("Overview/Technical design/Components/FlagStore");

  // Keyboard: open Systems with its disclosure, then follow a row.
  const systems = sidebar.getByRole("button", { name: "Toggle Systems", exact: true });
  await systems.focus();
  await page.keyboard.press("Enter");
  await expect(systems).toHaveAttribute("aria-expanded", "true");
  await page.keyboard.press("Tab");
  await expect(sidebar.getByRole("link", { name: "Systems", exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  const row = sidebar.locator("#nav-technical-systems").getByRole("link", { name: "FeatureFlag" });
  await expect(row).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/technical\/system\/FeatureFlag$/);
  await expect(sidebar.locator('a[aria-current="page"]')).toHaveAttribute("href", "/technical/system/FeatureFlag");

  // The area row opens the area's page and becomes the current row.
  await sidebar.getByRole("link", { name: "Components", exact: true }).click();
  await expect(page).toHaveURL(/\/technical\/components$/);
  await expect(sidebar.locator('a[aria-current="page"]')).toHaveAttribute("href", "/technical/components");
  await expect(page.locator('[data-technical-kind="component"] [data-directory-row]')).toHaveCount(2);
});

test("old Technical design addresses keep working", async ({ page, saga }) => {
  author(saga);
  // Definition pages and saved pins kept their addresses.
  await page.goto(saga.baseURL + "/technical/system/FeatureFlag?revision=r1");
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r1");
  await page.locator(".requirements-breadcrumbs").getByRole("link", { name: "Systems" }).click();
  await expect(page).toHaveURL(/\/technical\/systems$/);
  // A fragment of the old single page lands on the matching area.
  await page.goto(saga.baseURL + "/technical#technical-components-section");
  await expect(page.locator("#technical-components-section")).toHaveAttribute("data-technical-area", "components");
  await page.locator("#technical-components-section").getByRole("link", { name: "Components" }).click();
  await expect(page).toHaveURL(/\/technical\/components$/);
});

test("a missing definition or revision is a 404, never the latest definition", async ({ page, saga }) => {
  author(saga);
  for (const path of ["/technical/system/FeatureFlag?revision=r9", "/technical/system/Absent", "/technical/nonsense/FeatureFlag", "/technical/nonsense"]) {
    const response = await page.goto(saga.baseURL + path);
    expect(response?.status(), path).toBe(404);
  }
});

// Inventory format 2 through the public CLI: an authored ERD whose drawn
// elements and directory open the same pinned entity, and a slide Item whose
// drawer shows exactly the code it selects.
test("an authored ERD and an Item's exact selection open the same pinned definitions @critical", async ({ page, saga }) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, args.join(" ") + "\n" + result.stdout + result.stderr).toBe(0);
  };
  const json = (name: string, value: unknown) => {
    const path = join(saga.root, name);
    writeFileSync(path, typeof value === "string" ? value : JSON.stringify(value));
    return path;
  };
  const pin = (kind: string, id: string, revision = "r1") => ({ target: prefix + kind + ":" + id, revision: prefix + kind + ":" + id + ":revision:" + revision });
  const evidence = (id: string, start: number, end: number, note: string) => ({ id, commit: "HEAD", path: "src/app.go", start, end, note });
  const add = (kind: string, id: string, definition: unknown, ...extra: string[]) =>
    run(kind, "add", "--id", id, "--from", json(kind + "-" + id + ".json", definition), "--repo", saga.sourceRepo, ...extra, saga.sagaRoot);
  run("inventory", "adopt-format", "--format", "2", saga.sagaRoot);
  add("component", "Store", { name: "Store", explanation: "Persists greetings.", intent: "implemented", code: [evidence("greet", 3, 5, "The greeting the store keeps.")] }, "--delivery", "HEAD");
  add("component", "Worker", { name: "Worker", explanation: "Reports readiness.", intent: "implemented", code: [evidence("ready", 7, 9, "Readiness.")] }, "--delivery", "HEAD");
  add("system", "Pipeline", { name: "Pipeline", explanation: "The worker writes into the store.", intent: "implemented", code: [evidence("wiring", 3, 3, "Entry point.")],
    components: [pin("component", "Store"), pin("component", "Worker")],
    interactions: [{ id: "write", from: prefix + "component:Worker", to: prefix + "component:Store", description: "Will write greetings.", intent: "proposed" }] }, "--delivery", "HEAD");
  add("data-entity", "job", { name: "PDF job", explanation: "Queued payload.", intent: "proposed", baseline: "none", fields: [{ id: "template", name: "template" }] });
  add("data-entity", "report", { name: "PDF report", explanation: "Persisted report.", intent: "implemented", code: [evidence("row", 7, 8, "Report persistence.")],
    fields: [{ id: "id", name: "id", keys: ["primary"] }], holders: [{ component: pin("component", "Store"), role: "persists report rows" }],
    relationships: [{ id: "produced-from-job", meaning: "production", destination: pin("data-entity", "job"), label: "rendered from", flow: "destination_to_owner", explanation: "A worker renders a queued job.", intent: "proposed" }] }, "--delivery", "HEAD");
  const svg = json("erd.svg", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 120"><g id="report"><rect x="10" y="10" width="140" height="80" fill="white" stroke="black"/><text x="20" y="50">PDF report</text></g><path id="produced" d="M300 50H150" stroke="black" stroke-dasharray="6 4"/></svg>`);
  run("erd", "add", "--id", "application", "--from", json("erd.json", { name: "Application data", explanation: "Authored overview.",
    directory: [pin("data-entity", "report"), pin("data-entity", "job")],
    bindings: [{ id: "report", element: "report", entity: pin("data-entity", "report") }, { id: "produced", element: "produced", relationship: { owner: pin("data-entity", "report"), id: "produced-from-job" } }] }),
    "--visual", svg, "--repo", saga.sourceRepo, saga.sagaRoot);
  run("add-deck", "--feature", "wave-one", "--id", "pipeline-deck", "--objective", "A slide selects part of the pipeline", saga.sagaRoot, "pipeline");
  run("add-slide", "--deck", "pipeline-deck", "--id", "greeting-store", "--title", "greeting-store", "--intent", "explain", "--layout", "diagram", saga.sagaRoot, "greeting-store");
  const system = pin("system", "Pipeline");
  const selections = json("selections.json", [{ id: "greet-only", path: [system, pin("component", "Store")], evidence: "greet", code: { path: "src/app.go", start: 4, end: 4, note: "Only the returned greeting matters here." } }]);
  run("add-item", "--slide", "greeting-store", "--id", "store", "--kind", "node", "--element-id", "slide-title", "--description", "The slide explains only the stored greeting.",
    "--documentation", system.target, "--documentation-revision", system.revision, "--selections", selections, "--repo", saga.sourceRepo, saga.sagaRoot);

  // The ERD: the sidebar lists its data entities, and the ERD page draws it.
  await page.goto(saga.baseURL + "/technical");
  await waitForSettledSaga(page);
  await page.locator('[data-technical-area="erd"]').getByRole("link", { name: "ERD" }).click();
  await expect(page).toHaveURL(/\/technical\/erd$/);
  const entities = page.locator(".sidebar #nav-technical-erd");
  await expect(entities).toBeVisible();
  await expect(entities.getByRole("link")).toHaveText(["PDF job", "PDF report"]);
  // A drawn element and its directory row open the same pin.
  const model = page.locator("[data-technical-data-model]");
  await expect(model.locator("figcaption")).toContainText("1 of 2 directory entities are drawn; 1 is in the directory but not drawn.");
  const drawn = model.locator(`[data-erd-visual] svg [id$="-report"]`);
  await expect(drawn).toHaveAttribute("role", "button");
  await drawn.focus();
  await page.keyboard.press("Enter");
  const view = page.locator("[data-documentation-view]");
  await expect(view).toHaveAttribute("data-documentation-pin", pin("data-entity", "report").revision);
  await expect(page.locator(".drawer-body")).toContainText("persists report rows");
  await expect(page.locator(".drawer-body [data-relationship-meaning=production]")).toContainText("production, not a foreign key");
  await page.keyboard.press("Escape");
  await expect(drawn).toBeFocused();
  await model.locator('[data-erd-directory-row$=":report"] button[data-documentation-target]').click();
  await expect(view).toHaveAttribute("data-documentation-pin", pin("data-entity", "report").revision);
  await page.keyboard.press("Escape");
  await expect(model.locator('[data-erd-directory-row$=":job"]')).toContainText("not drawn");
  await expect(model.locator('[data-erd-directory-row$=":job"]')).toContainText("proposed");

  // The slide Item: its drawer shows the selected line before the whole definition.
  await page.goto(saga.featureURL + "?view=slides");
  await waitForSettledSaga(page);
  await page.getByRole("button", { name: "Show slide: greeting-store", exact: true }).click();
  const slide = page.locator("[data-deck-slide]:visible");
  const slideURL = page.url();
  await slide.locator(".landmark-menu summary").click();
  const control = slide.locator(".landmark-menu [data-documentation-target]");
  await expect(control).toHaveAttribute("data-documentation-selections", "1");
  await control.click();
  const selection = page.locator('.drawer-body [data-selection="greet-only"]');
  await expect(selection.locator(".selection-path li")).toHaveText([/Pipeline/, /Store/]);
  await expect(selection.locator(".term-code-lines tr.referenced")).toHaveCount(1);
  await expect(selection.locator(".term-code-lines tr.referenced")).toContainText('return "hello, " + name');
  await expect(selection.locator("[data-selection-integrity]")).toHaveAttribute("data-selection-integrity", "matches");
  await expect(page.locator(".drawer-body")).toContainText("All of this definition's code");
  // A member reached from the drawer shows its definition, not the Item's selection.
  await page.locator(".drawer-body .documentation-members").getByRole("button", { name: "Store", exact: true }).click();
  await expect(view).toHaveAttribute("data-documentation-view", prefix + "component:Store");
  await expect(page.locator(".drawer-body [data-item-selections]")).toHaveCount(0);
  await page.getByRole("button", { name: "Back to previous explanation" }).click();
  await expect(page.locator('.drawer-body [data-selection="greet-only"]')).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(control).toBeFocused();
  expect(page.url()).toBe(slideURL);

  // The reviewer's coverage, like the CLI's, credits the Item with exactly
  // the selected line, labelled as a selection rather than authored evidence.
  let coverage = "";
  for (let attempt = 0; attempt < 200 && !coverage; attempt++) {
    const response = await fetch(saga.baseURL + "/api/coverage-file?file=src%2Fapp.go");
    if (response.status === 200) coverage = await response.text();
    else await new Promise(resolve => setTimeout(resolve, 100));
  }
  expect(coverage).toContain("data-owner-inherited");
  expect(coverage).toContain("selected via Store (greet-only)");
});

// A real application ERD is far larger than the content column. Its page
// takes the full width, and the drawing zooms and pans inside its own region
// by toolbar, keyboard, mouse drag and touch, without widening the page.
test("a large ERD drawing zooms and pans by toolbar, keyboard, mouse and touch", async ({ browser, page, saga }) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, args.join(" ") + "\n" + result.stdout + result.stderr).toBe(0);
  };
  const json = (name: string, value: unknown) => {
    const path = join(saga.root, name);
    writeFileSync(path, typeof value === "string" ? value : JSON.stringify(value));
    return path;
  };
  const job = { target: prefix + "data-entity:job", revision: prefix + "data-entity:job:revision:r1" };
  run("inventory", "adopt-format", "--format", "2", saga.sagaRoot);
  run("data-entity", "add", "--id", "job", "--from", json("job.json", { name: "PDF job", explanation: "Queued payload.", intent: "proposed", baseline: "none" }), "--repo", saga.sourceRepo, saga.sagaRoot);
  const svg = json("large.svg", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 1600" width="1600" height="1600"><rect width="1600" height="1600" fill="white"/><g id="job"><rect x="1300" y="1300" width="260" height="200" fill="white" stroke="black"/><text x="1320" y="1400" font-size="18">PDF job</text></g></svg>`);
  run("erd", "add", "--id", "application", "--from", json("erd.json", { name: "Application data", explanation: "Authored overview.", directory: [job], bindings: [{ id: "job", element: "job", entity: job }] }),
    "--visual", svg, "--repo", saga.sourceRepo, saga.sagaRoot);

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  await expect(page.locator(".technical-erd-page")).toHaveCount(1);
  const toolbar = page.getByRole("toolbar", { name: "Application data zoom" });
  const viewport = page.getByRole("region", { name: /^Application data diagram/ });
  const level = toolbar.locator("[data-erd-zoom-level]");
  const fitButton = toolbar.getByRole("button", { name: "Fit width" });
  await expect(toolbar).toBeVisible();
  await expect(fitButton).toHaveAttribute("aria-pressed", "true");
  const widths = () => viewport.evaluate(element => ({ region: element.clientWidth, content: element.scrollWidth, page: document.documentElement.scrollWidth, inner: innerWidth }));
  // Fitted, the whole drawing's width is in view.
  const fitted = await widths();
  expect(fitted.content).toBeLessThanOrEqual(fitted.region + 1);
  const fittedLevel = await level.textContent();

  // Toolbar: zooming in makes the drawing wider than its region, never the page.
  await toolbar.getByRole("button", { name: "Zoom in" }).click();
  await toolbar.getByRole("button", { name: "Zoom in" }).click();
  await expect(level).not.toHaveText(fittedLevel!);
  await expect(fitButton).toHaveAttribute("aria-pressed", "false");
  const zoomed = await widths();
  expect(zoomed.content).toBeGreaterThan(zoomed.region);
  expect(zoomed.page).toBeLessThanOrEqual(zoomed.inner);

  // Mouse: dragging pans the region and does not open what it started on.
  await viewport.evaluate(element => { element.scrollLeft = 0; element.scrollTop = 0; });
  const box = (await viewport.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 - 200, box.y + box.height / 2 - 150, { steps: 8 });
  await page.mouse.up();
  expect(await viewport.evaluate(element => element.scrollLeft)).toBeGreaterThan(100);
  await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);

  // Keyboard: the region is focusable; 0 fits the width, + zooms in again, and
  // the bound entity is still reached with Tab and opens with Enter.
  await viewport.focus();
  await page.keyboard.press("0");
  await expect(fitButton).toHaveAttribute("aria-pressed", "true");
  await expect(level).toHaveText(fittedLevel!);
  await page.keyboard.press("+");
  await expect(fitButton).toHaveAttribute("aria-pressed", "false");
  await page.keyboard.press("Tab");
  const drawn = viewport.locator(`svg [id$="-job"]`);
  await expect(drawn).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin", job.revision);
  await page.keyboard.press("Escape");
  await expect(drawn).toBeFocused();

  // A bound entity stays clickable at every zoom level, fitted and zoomed in.
  for (const zoomSteps of [0, 4]) {
    await fitButton.click();
    for (let i = 0; i < zoomSteps; i++) await toolbar.getByRole("button", { name: "Zoom in" }).click();
    await drawn.scrollIntoViewIfNeeded();
    await drawn.click();
    await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin", job.revision);
    await page.keyboard.press("Escape");
    await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);
  }

  // Touch at 390px: large targets, the region pans, the page keeps its width.
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const narrow = await context.newPage();
  await narrow.goto(saga.baseURL + "/technical/erd/application");
  await waitForSettledSaga(narrow);
  const zoomIn = narrow.getByRole("button", { name: "Zoom in" });
  expect((await zoomIn.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await zoomIn.tap();
  await zoomIn.tap();
  const region = narrow.getByRole("region", { name: /^Application data diagram/ });
  await region.evaluate(element => { element.scrollLeft = element.scrollWidth; });
  expect(await region.evaluate(element => element.scrollLeft)).toBeGreaterThan(0);
  expect(await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth])).toEqual([390, 390]);
  await context.close();
});
