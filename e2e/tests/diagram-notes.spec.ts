import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { codeDigest, git, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
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
    slide: { id: "noted-flow", title: "Greeting flow", rank: 10, intent: "explain", layout: "diagram", takeaway: "The caller's name flows into the greeting.", reading_order: ["caller", "greeting", "echo"] },
    diagram: notedDiagram,
    items: [
      { id: "caller", rank: 10, kind: "node", label: "Caller", description: "Callers now pass a name.", selector: { type: "element", element_id: "caller" } },
      { id: "greeting", rank: 20, kind: "node", label: "Greeting", description: "Greeting returns the supplied name.", selector: { type: "element", element_id: "greeting" } },
      { id: "echo", rank: 30, kind: "callout", label: "Names are echoed", description: "Why an empty name still greets.", about: "greeting", body: "You might expect an empty name to be refused here; the greeting echoes it.", selector: { type: "element", element_id: "greeting" } },
    ],
  };
  const requestPath = join(root, "noted-flow.json");
  writeFileSync(requestPath, JSON.stringify(request));
  const published = JSON.parse(cli(sagaRepositories, "apply-slide", "--from", requestPath, "--json", sagaRoot));
  // Two stacked elements: a long note on the upper one would cover the lower.
  const stackedPath = join(root, "stacked.json");
  writeFileSync(stackedPath, JSON.stringify({
    version: 1, operation: "create", request_id: "stacked", review: "pr-3", expected_snapshot: "absent",
    slide: { id: "stacked", title: "Stacked", rank: 20, intent: "explain", layout: "diagram", takeaway: "Two elements sit close together.", reading_order: ["lower"] },
    diagram: { version: 1, width: 1280, height: 720, elements: [
      { id: "upper", kind: "node", shape: "rect", label: "Upper", x: 440, y: 200, width: 400, height: 80, style: "normal",
        note: "A long note.\n\n- one\n- two\n- three\n- four\n- five\n- six" },
      { id: "lower", kind: "node", shape: "rect", label: "Lower", x: 440, y: 320, width: 400, height: 80, style: "primary" },
    ] },
    items: [{ id: "lower", rank: 10, kind: "node", label: "Lower", description: "The element beneath the note.", selector: { type: "element", element_id: "lower" } }],
  }));
  cli(sagaRepositories, "apply-slide", "--from", stackedPath, sagaRoot);
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
    // A hover popover lets the pointer through to anything beneath it.
    await expect(popover).toHaveCSS("pointer-events", "none");
    // A click pins it for its links, and moving away still closes it.
    await edge.click();
    await expect(popover).toBeVisible();
    await expect(popover).toHaveCSS("pointer-events", "auto");
    await page.mouse.move(5, 5);
    await expect(popover).toBeHidden();
    await edge.hover();
    await expect(popover).toBeVisible();
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

    // A note's links follow its hotspot in the Tab order, and Tab leaves them.
    // This note precedes the heading in DOM order, so reaching it again makes
    // one circuit through the visible reviewer, face and Viewed controls.
    for (let step = 0; step < 60 && !(await edge.evaluate(node => node === document.activeElement)); step++) {
      await page.keyboard.press("Tab");
      expect(await page.evaluate(() => Boolean(document.activeElement?.closest('[data-deck-overview][hidden]'))), 'hidden Overview content must not receive focus').toBe(false);
    }
    await expect(edge).toBeFocused();
    await expect(popover).toHaveAttribute("role", "note");
    await page.keyboard.press("Tab");
    const handler = popover.getByRole("link", { name: "the handler" });
    await expect(handler).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(edge).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(handler).toBeFocused();
    await page.keyboard.press("Tab");
    // Focus moves on to the next hotspot, whose own note replaces this one.
    await expect(edge).not.toBeFocused();
    await expect(handler).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(popover).toBeHidden();

    // The popover never blocks the click that opens an Item's drawer.
    const greeting = slide.locator('.landmark-hotspot:not(.callout-hotspot)[data-element-id="greeting"]');
    await greeting.hover();
    await expect(popover.locator(".element-note-label")).toHaveText("Greeting");
    await greeting.click({ position: { x: 20, y: 60 } });
    await expect(page.locator("#review-drawer")).toBeVisible();
    await expect(popover).toBeHidden();
    await expect(page.locator("#review-drawer .review-item-panel h2")).toHaveText("Greeting");
    // Closing the drawer returns focus without reopening the popover.
    await page.mouse.move(5, 5);
    await page.keyboard.press("Escape");
    await expect(page.locator("#review-drawer")).toHaveAttribute("aria-hidden", "true");
    await expect(popover).toBeHidden();

    // A surprise about the same element is its own badge at the element's
    // bottom-left, so it never covers the element's own controls.
    const surprise = slide.locator('.callout-hotspot[data-element-id="greeting"]');
    await expect(surprise).toHaveClass(/callout-shared/);
    await expect(surprise).toHaveCSS("pointer-events", "none");
    const badge = surprise.getByRole("button", { name: "Open surprise: Names are echoed" });
    const greetingBox = (await greeting.boundingBox())!;
    const badgeBox = (await badge.boundingBox())!;
    expect(badgeBox.x - greetingBox.x).toBeLessThan(40);
    expect(greetingBox.y + greetingBox.height - (badgeBox.y + badgeBox.height)).toBeLessThan(40);
    await greeting.getByRole("button", { name: /Open linked code .* for Greeting/ }).click();
    await expect(page.locator("#review-drawer .review-item-panel h2")).toHaveText("Greeting");
    await page.keyboard.press("Escape");
    await badge.hover();
    await expect(popover.locator(".element-note-label")).toHaveText("Names are echoed");
    await badge.click();
    await expect(page.locator("#review-drawer .review-item-panel h2")).toHaveText("Names are echoed");
    await page.keyboard.press("Escape");

    // A popover never takes the click meant for a hotspot beneath it.
    await page.getByRole("button", { name: "Show slide: Stacked" }).click();
    const stacked = page.locator('[data-deck-slide][data-slide-target$=":slide:stacked"]');
    const upper = stacked.locator('.element-note-hotspot[data-element-note-visual="upper"]');
    const lower = stacked.locator('.landmark-hotspot[data-element-id="lower"]');
    await upper.hover();
    await expect(popover).toBeVisible();
    const lowerBox = (await lower.boundingBox())!;
    await page.mouse.move(lowerBox.x + lowerBox.width / 2, lowerBox.y + lowerBox.height / 2, { steps: 4 });
    await page.mouse.click(lowerBox.x + lowerBox.width / 2, lowerBox.y + lowerBox.height / 2);
    await expect(page.locator("#review-drawer")).toBeVisible();
    await expect(page.locator("#review-drawer h2", { hasText: "Lower" })).toBeVisible();
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

test("an implementation deck shows its diagram notes the same way, and in an Item's code drawer", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };
  run("add-deck", "--feature", "wave-one", "--id", "noted-flow", "--title", "Noted flow", "--objective", "Explain the greeting flow.", saga.sagaRoot, "Noted flow");
  const story = "urn:change-saga:wave-one:story:noted-greeting";
  run("story", "add", "--feature", "wave-one", "--id", "noted-greeting", "--revision", "r1", "--event", "proposed",
    "--title", "Greet by name", "--statement", "As a caller I am greeted by name.", "--criterion", "named=The greeting names the caller", saga.sagaRoot);
  const head = saga.identity.head;
  const request = {
    version: 1, operation: "create", request_id: "noted-implementation", deck: "noted-flow", expected_snapshot: "absent",
    slide: { id: "noted-implementation", title: "Greeting flow", rank: 10, intent: "explain", layout: "diagram", takeaway: "The caller's name flows into the greeting.", reading_order: ["greeting"] },
    diagram: { ...notedDiagram, elements: notedDiagram.elements.map((element): Record<string, unknown> => element.id === "greeting" ? { ...element, note: "Formats the **name** it is given." } : element) },
    items: [{
      id: "greeting", rank: 10, kind: "node", label: "Greeting", description: "Greeting returns the supplied name.", selector: { type: "element", element_id: "greeting" },
      evidence: [{ version: 2, references: [{ commit: head, path: "src/app.go", start: 3, end: 3, digest: codeDigest(saga.sourceRepo, head, "src/app.go", 3), note: "The greeting names the caller." }] }],
      criterion_links: [{ id: "greeting-named", criterion: `${story}:criterion:named`, story_revision: `${story}:revision:r1`, rationale: "The greeting is the named line." }],
    }],
  };
  const requestPath = join(saga.root, "noted-implementation.json");
  writeFileSync(requestPath, JSON.stringify(request));
  run("apply-slide", "--from", requestPath, "--repo", saga.sourceRepo, saga.sagaRoot);

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

  // A reader who cannot hover, such as on a touch screen, opens the Item and
  // reads its note at the head of its code.
  const greeting = slide.locator('.landmark-hotspot[data-element-id="greeting"]');
  await greeting.hover();
  await expect(popover.locator(".element-note-label")).toHaveText("Greeting");
  await greeting.click({ position: { x: 20, y: 60 } });
  const drawerNote = page.locator(".diff-drawer.open .drawer-element-note");
  await expect(drawerNote.locator(".element-note-label")).toHaveText("Greeting");
  await expect(drawerNote.locator("strong")).toHaveText("name");
  await expect(popover).toBeHidden();
});

test("pinned notes, notes inside a selected group, and surprise badges stay reachable", async ({ page, sagaRepositories }) => {
  const { sagaRoot, root } = sagaRepositories;
  cli(sagaRepositories, "review", "create", "--id", "pr-4", "--base", "main", "--head", "feature/wave-one", "--pr", "4", "--title", "Polish review", sagaRoot);
  const requestPath = join(root, "polish.json");
  writeFileSync(requestPath, JSON.stringify({
    version: 1, operation: "create", request_id: "polish", review: "pr-4", expected_snapshot: "absent",
    slide: { id: "polish", title: "Polish", rank: 10, intent: "explain", layout: "diagram", takeaway: "Every note and surprise stays reachable.",
      reading_order: ["outer", "lower", "corner", "corner-surprise", "small", "small-surprise"] },
    diagram: { version: 1, width: 1280, height: 720, elements: [
      { id: "outer", kind: "group", shape: "rect", label: "Outer", x: 60, y: 120, width: 560, height: 300, style: "normal" },
      { id: "inner", kind: "node", shape: "rect", label: "Inner", parent: "outer", x: 40, y: 80, width: 240, height: 100, style: "primary", note: "The **inner** note." },
      { id: "upper", kind: "node", shape: "rect", label: "Upper", x: 760, y: 120, width: 400, height: 80, style: "normal", note: "Upper note with [a link](https://example.com/a)." },
      { id: "lower", kind: "node", shape: "rect", label: "Lower", x: 760, y: 230, width: 400, height: 80, style: "primary" },
      { id: "corner", kind: "node", shape: "rect", label: "Corner", x: 20, y: 610, width: 320, height: 90, style: "normal" },
      { id: "small", kind: "node", shape: "rect", label: "Small", x: 900, y: 420, width: 200, height: 60, style: "normal" },
    ] },
    items: [
      { id: "outer", rank: 10, kind: "node", label: "Outer", description: "A group an Item selects.", selector: { type: "element", element_id: "outer" } },
      { id: "lower", rank: 20, kind: "node", label: "Lower", description: "Between the upper note and its popover.", selector: { type: "element", element_id: "lower" } },
      { id: "corner", rank: 30, kind: "node", label: "Corner", description: "In the corner the Surprises panel covers.", selector: { type: "element", element_id: "corner" } },
      { id: "corner-surprise", rank: 40, kind: "callout", label: "Corner surprise", description: "A surprise in the corner.", about: "corner", body: "Its badge must stay clear of the panel.", selector: { type: "element", element_id: "corner" } },
      { id: "small", rank: 50, kind: "node", label: "Small", description: "A short element.", selector: { type: "element", element_id: "small" } },
      { id: "small-surprise", rank: 60, kind: "callout", label: "Small surprise", description: "A surprise on a short element.", about: "small", body: "Its badge must not crowd the controls.", selector: { type: "element", element_id: "small" } },
    ],
  }));
  cli(sagaRepositories, "apply-slide", "--from", requestPath, sagaRoot);
  git(sagaRepositories.sagaRepo, "add", ".");
  git(sagaRepositories.sagaRepo, "commit", "-m", "Polish review slide");

  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${running.baseURL}/reviews/pr-4`);
    const slide = page.locator('[data-deck-slide][data-slide-target$=":slide:polish"]');
    await expect(slide).toBeVisible();
    const popover = page.locator("#element-note-popover");
    const center = (box: { x: number; y: number; width: number; height: number }) => ({ x: box.x + box.width / 2, y: box.y + box.height / 2 });
    const apart = (a: { x: number; y: number; width: number; height: number }, b: { x: number; y: number; width: number; height: number }) =>
      a.x + a.width <= b.x || b.x + b.width <= a.x || a.y + a.height <= b.y || b.y + b.height <= a.y;

    // B: a note inside a group an Item selects is reachable with the pointer.
    const inner = slide.locator('.element-note-hotspot[data-element-note-visual="inner"]');
    await expect(inner).toHaveClass(/element-note-inner/);
    await inner.hover();
    await expect(popover.locator("strong")).toHaveText("inner");

    // A: crossing another hotspot on the way to a pinned note keeps it.
    const upper = slide.locator('.element-note-hotspot[data-element-note-visual="upper"]');
    const lower = slide.locator('.landmark-hotspot[data-element-id="lower"]');
    await upper.click();
    await expect(popover).toHaveClass(/pinned/);
    const popoverAt = center((await popover.boundingBox())!);
    const lowerAt = center((await lower.boundingBox())!);
    await page.mouse.move(lowerAt.x, lowerAt.y, { steps: 2 });
    await page.mouse.move(popoverAt.x, popoverAt.y, { steps: 2 });
    await expect(popover.getByRole("link", { name: "a link" })).toBeVisible();
    await page.mouse.move(5, 5);
    await expect(popover).toBeHidden();

    // C: the Surprises panel never covers a badge in the corner beneath it.
    const cornerBadge = slide.locator('.callout-hotspot[data-element-id="corner"]').getByRole("button", { name: "Open surprise: Corner surprise" });
    expect(apart((await cornerBadge.boundingBox())!, (await slide.locator(".review-callouts").boundingBox())!)).toBe(true);
    await cornerBadge.click();
    await expect(page.locator("#review-drawer .review-item-panel h2")).toHaveText("Corner surprise");
    await page.keyboard.press("Escape");

    // D: on a short element the badge stays clear of the Item's controls.
    const smallBadge = slide.locator('.callout-hotspot[data-element-id="small"]').getByRole("button", { name: "Open surprise: Small surprise" });
    const smallControls = slide.locator('.landmark-hotspot:not(.callout-hotspot)[data-element-id="small"] > .landmark-affordance');
    expect(apart((await smallBadge.boundingBox())!, (await smallControls.boundingBox())!)).toBe(true);

    // D: after Escape, leaving a note hotspot and returning reopens its note.
    await page.mouse.move(5, 5);
    await slide.locator(".fragment").focus();
    for (let step = 0; step < 40 && !(await upper.evaluate(node => node === document.activeElement)); step++) await page.keyboard.press("Tab");
    await expect(upper).toBeFocused();
    await expect(popover).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(popover).toBeHidden();
    await page.keyboard.press("Shift+Tab");
    await expect(upper).not.toBeFocused();
    await page.keyboard.press("Tab");
    await expect(upper).toBeFocused();
    await expect(popover.getByText("Upper note with")).toBeVisible();
  } finally {
    await stopSagaServer(running);
  }
});
