import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { git, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test, waitForSettledSaga } from "../support/test.js";

// An author can annotate any diagram element with a short Markdown note. A
// reader sees it, rendered, in a popover beside the element on hover, keyboard
// focus, or tap; an Item's popover also carries its label and description.
// The popover lives on the app page, never inside the sandboxed slide frame.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

const notedDiagram = {
  version: 1, width: 1280, height: 720, background: "#fafaf8",
  elements: [
    { id: "heading", kind: "text", label: "Greeting flow", x: 60, y: 28, width: 900, height: 48, style: "title", note: "Why this slide exists: callers **now pass a name**." },
    { id: "caller", kind: "node", shape: "ellipse", label: "Caller", x: 120, y: 280, width: 240, height: 110, style: "normal", note: "Any HTTP client; `name` is *required*." },
    { id: "greeting", kind: "node", shape: "service", label: "Greeting", icon: "lucide:server", x: 760, y: 280, width: 260, height: 110, style: "primary" },
    { id: "name", kind: "edge", from: "caller", to: "greeting", points: [{ x: 360, y: 335 }, { x: 760, y: 335 }], head: "arrow", label: "name", label_box: { x: 510, y: 297, width: 100, height: 26 }, style: "secondary",
      note: "Sent as a query parameter.\n\n- empty names are refused\n- see [the handler](https://example.com/handler)" },
  ],
};

test("shows a diagram element's rendered note on hover, focus, and tap, and dismisses it with Escape", async ({ page, browser, sagaRepositories }) => {
  const { sagaRoot, sourceRepo, root } = sagaRepositories;
  cli(sagaRepositories, "review", "create", "--id", "pr-3", "--base", "main", "--head", "feature/wave-one", "--pr", "3", "--title", "Noted review", sagaRoot);
  const request = {
    version: 1, operation: "create", request_id: "noted-flow", review: "pr-3", expected_snapshot: "absent",
    slide: { id: "noted-flow", title: "Greeting flow", rank: 10, intent: "explain", layout: "diagram", takeaway: "The caller's name flows into the greeting.", reading_order: ["caller", "greeting"] },
    diagram: notedDiagram,
    items: [
      { id: "caller", rank: 10, kind: "node", label: "Caller", description: "Callers now pass a name.", selector: { type: "element", element_id: "caller" } },
      { id: "greeting", rank: 20, kind: "node", label: "Greeting", description: "Greeting returns the supplied name.", selector: { type: "element", element_id: "greeting" } },
    ],
  };
  const requestPath = join(root, "noted-flow.json");
  writeFileSync(requestPath, JSON.stringify(request));
  const published = JSON.parse(cli(sagaRepositories, "apply-slide", "--from", requestPath, "--json", sagaRoot));
  cli(sagaRepositories, "cover", "--repo", sourceRepo, "--target", `${published.target}:item:greeting`, "--path", "src/app.go", "--changed-lines", sagaRoot);
  expect(cli(sagaRepositories, "diagram", "describe", "--slide", published.target, sagaRoot)).toContain("  caller \"Caller\" shape=ellipse\n    note: Any HTTP client; `name` is *required*.\n");
  git(sagaRepositories.sagaRepo, "add", ".");
  git(sagaRepositories.sagaRepo, "commit", "-m", "Noted review slide");

  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${running.baseURL}/reviews/pr-3`);
    const slide = page.locator('[data-deck-slide][data-slide-target$=":slide:noted-flow"]');
    await expect(slide).toBeVisible();
    const popover = page.locator("#element-note-popover");
    const edge = slide.locator('.element-note-hotspot[data-element-note-visual="name"]');
    const heading = slide.locator('.element-note-hotspot[data-element-note-visual="heading"]');
    await expect(edge).toHaveAttribute("aria-label", "Note: name");
    await expect(heading).toHaveCount(1);

    // Hover: an element without an Item shows just its rendered note.
    await edge.hover();
    await expect(popover).toBeVisible();
    await expect(popover.getByText("Sent as a query parameter.")).toBeVisible();
    await expect(popover.locator("li")).toHaveText(["empty names are refused", "see the handler"]);
    await expect(popover.getByRole("link", { name: "the handler" })).toHaveAttribute("href", "https://example.com/handler");
    await expect(popover.locator(".element-note-label")).toHaveCount(0);
    const edgeBox = (await edge.boundingBox())!;
    const popoverBox = (await popover.boundingBox())!;
    const overlaps = popoverBox.x < edgeBox.x + edgeBox.width && edgeBox.x < popoverBox.x + popoverBox.width && popoverBox.y < edgeBox.y + edgeBox.height && edgeBox.y < popoverBox.y + popoverBox.height;
    expect(overlaps, "the popover sits beside the element, never over it").toBe(false);
    await page.screenshot({ path: test.info().outputPath("diagram-note-hover.png") });

    // An Item's element shows its label and description with the note.
    const caller = slide.locator('.landmark-hotspot[data-element-id="caller"]');
    await caller.hover();
    await expect(popover.locator(".element-note-label")).toHaveText("Caller");
    await expect(popover.locator(".element-note-description")).toHaveText("Callers now pass a name.");
    await expect(popover.locator(".element-note-markdown code")).toHaveText("name");
    await expect(popover.locator(".element-note-markdown em")).toHaveText("required");

    // Moving away dismisses it.
    await page.mouse.move(5, 5);
    await expect(popover).toBeHidden();

    // Keyboard: Tab reaches the note hotspot, which opens the popover, and
    // Escape closes it while focus stays put.
    await slide.locator(".fragment").focus();
    for (let step = 0; step < 30 && !(await heading.evaluate(node => node === document.activeElement)); step++) await page.keyboard.press("Tab");
    await expect(heading).toBeFocused();
    await expect(popover).toBeVisible();
    await expect(popover.locator("strong")).toHaveText("now pass a name");
    await expect(heading).toHaveAttribute("aria-expanded", "true");
    await expect(heading).toHaveAttribute("aria-describedby", "element-note-popover");
    await page.keyboard.press("Escape");
    await expect(popover).toBeHidden();
    await expect(heading).toBeFocused();
    await expect(heading).toHaveAttribute("aria-expanded", "false");
    await page.keyboard.press("Enter");
    await expect(popover).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    // The popover never blocks the click that opens an Item's drawer.
    const greeting = slide.locator('.landmark-hotspot[data-element-id="greeting"]');
    await greeting.hover();
    await expect(popover.locator(".element-note-label")).toHaveText("Greeting");
    await greeting.click({ position: { x: 20, y: 60 } });
    await expect(page.locator("#review-drawer")).toBeVisible();
    await expect(popover).toBeHidden();
  } finally {
    await stopSagaServer(running);
  }

  // Tap: a touch reader taps a note open and taps again to close it.
  const touch = await browser.newContext({ hasTouch: true, viewport: { width: 1280, height: 900 } });
  const reopened = await startSagaServer(sagaRepositories);
  try {
    const tablet = await touch.newPage();
    await tablet.goto(`${reopened.baseURL}/reviews/pr-3`);
    const edge = tablet.locator('[data-deck-slide][data-slide-target$=":slide:noted-flow"] .element-note-hotspot[data-element-note-visual="name"]');
    const popover = tablet.locator("#element-note-popover");
    await edge.tap();
    await expect(popover.getByText("Sent as a query parameter.")).toBeVisible();
    await edge.tap();
    await expect(popover).toBeHidden();
  } finally {
    await touch.close();
    await stopSagaServer(reopened);
  }
});

test("an implementation deck shows its diagram notes the same way", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };
  run("add-deck", "--feature", "wave-one", "--id", "noted-flow", "--title", "Noted flow", "--objective", "Explain the greeting flow.", saga.sagaRoot, "Noted flow");
  const request = {
    version: 1, operation: "create", request_id: "noted-implementation", deck: "noted-flow", expected_snapshot: "absent",
    slide: { id: "noted-implementation", title: "Greeting flow", rank: 10, intent: "explain", layout: "diagram", takeaway: "The caller's name flows into the greeting.", reading_order: [] },
    diagram: notedDiagram,
    items: [],
  };
  const requestPath = join(saga.root, "noted-implementation.json");
  writeFileSync(requestPath, JSON.stringify(request));
  run("apply-slide", "--from", requestPath, saga.sagaRoot);

  await page.reload();
  await waitForSettledSaga(page);
  await page.getByRole("button", { name: "Show slide: Greeting flow" }).click();
  const slide = page.locator('#view-slides [data-deck-slide][data-slide-title="Greeting flow"]');
  await expect(slide).toBeVisible();
  const caller = slide.locator('.element-note-hotspot[data-element-note-visual="caller"]');
  await caller.hover();
  const popover = page.locator("#element-note-popover");
  await expect(popover.locator("code")).toHaveText("name");
  await caller.focus();
  await page.keyboard.press("Escape");
  await expect(popover).toBeHidden();
});
