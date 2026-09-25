import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { codeLocation, git, readJSON, reviewFiles, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

// A pull request's review is a slide deck and the one place approval happens.
// Each slide records every reviewer's decision at the head it was given, and a
// push that changes a slide's code puts that slide's decisions out of date.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

const slideSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-label="Review slide"><rect id="change" x="80" y="80" width="480" height="240" fill="#dce8ff"/><text id="why" x="640" y="200">Why it changed</text></svg>\n`;

function authorReview(repositories: SagaRepositories): void {
  const { sagaRoot, sourceRepo, identity } = repositories;
  const visual = join(repositories.root, "review-slide.svg");
  writeFileSync(visual, slideSVG);
  // A realistic Saga also has living implementation decks. They must not leak
  // a second presentation control or slide surface into the PR review route.
  cli(repositories, "add-deck", "--feature", "wave-one", "--id", "living-implementation", "--title", "Living implementation", "--objective", "Explain the already-delivered feature.", sagaRoot, "Living implementation");
  cli(repositories, "add-slide", "--deck", "living-implementation", "--id", "living-overview", "--intent", "orient", "--layout", "diagram", "--title", "Living overview", "--source", visual, sagaRoot, "Living overview");
  cli(repositories, "review", "create", "--id", "pr-1", "--base", "main", "--head", "feature/wave-one", "--pr", "1", "--url", "https://example.test/acme/change-saga-demo/pull/1", "--title", "Wave one review", sagaRoot);
  for (const slide of ["greeting", "theme"]) {
    cli(repositories, "add-slide", "--review", "pr-1", "--intent", "explain", "--layout", "diagram", "--title", slide === "greeting" ? "Greeting takes a name" : "Theme colour", "--source", visual, sagaRoot, slide);
    cli(repositories, "add-item", "--review", "pr-1", "--slide", slide, "--kind", "node", "--element-id", "change", "--label", "The change", "--description", "The code this slide explains", sagaRoot);
  }
  cli(repositories, "add-item", "--review", "pr-1", "--slide", "greeting", "--kind", "statement", "--element-id", "why", "--label", "Feature", "--description", "The feature this change revises", "--record", "urn:change-saga:wave-one:feature:wave-one", sagaRoot);
  cli(repositories, "cover", "--repo", sourceRepo, "--target", "urn:change-saga:wave-one:review:pr-1:slide:greeting:item:change", "--ref", codeLocation(identity.head, "src/app.go", 4), sagaRoot);
  cli(repositories, "cover", "--repo", sourceRepo, "--target", "urn:change-saga:wave-one:review:pr-1:slide:theme:item:change", "--ref", codeLocation(identity.head, "assets/ui/theme.css", 2), sagaRoot);
  git(repositories.sagaRepo, "add", ".");
  git(repositories.sagaRepo, "commit", "-m", "Review deck for pull request 1");
}

function freezeReview(repositories: SagaRepositories): void {
  const manifestPath = join(repositories.sagaRoot, "___reviews", "pr-1.review", "review.json");
  const manifest = readJSON<Record<string, unknown>>(manifestPath);
  manifest.merged = {
    base: repositories.identity.base,
    head: repositories.identity.head,
    landed: repositories.identity.head,
    merged_at: "2026-09-24T12:00:00Z",
  };
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  git(repositories.sagaRepo, "add", ".");
  git(repositories.sagaRepo, "commit", "-m", "Freeze merged review fixture");
}

test("Code Diff gets its own full workspace and returns to the same review slide", async ({ page, sagaRepositories }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    await page.getByRole("button", { name: "Show slide: Theme colour" }).click();
    const slideHash = new URL(page.url()).hash;
    await page.getByRole("tab", { name: "Deck", exact: true }).click();
    expect(new URL(page.url()).hash).toBe(slideHash);
    await page.getByRole("tab", { name: "Code Diff", exact: true }).click();
    await expect(page.locator("#view-code [data-file-diff-status]")).toHaveText("All changed hunks");
    await page.screenshot({ path: test.info().outputPath("review-code-desktop.png") });
    const tree = page.getByRole("tree", { name: "Changed files" });
    await expect(tree).toBeVisible();
    await expect(page.locator("[data-slide-present]")).toBeHidden();
    const diff = page.locator("#view-code .file-diff");
    const desktop = await diff.boundingBox();
    expect(desktop!.width).toBeGreaterThan(800);
    expect(desktop!.x).toBeGreaterThanOrEqual(270);
    await tree.locator('[data-tree-path="src/app.go"]').click();
    await expect(diff).toHaveAttribute("data-file-path", "src/app.go");
    await expect(diff.locator("[data-file-diff-status]")).toHaveText("All changed hunks");
    await page.getByRole("tab", { name: "Deck", exact: true }).click();
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:theme"]')).toBeVisible();
    expect(new URL(page.url()).hash).toBe(slideHash);

    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("tab", { name: "Code Diff", exact: true }).click();
    const toggle = page.getByRole("button", { name: "Toggle file tree", exact: true });
    if (!(await tree.isVisible()) || await page.locator("[data-shell]").evaluate(el => el.classList.contains("tree-hidden"))) await toggle.click();
    await tree.locator('[data-tree-path="assets/ui/theme.css"]').click();
    await expect(diff).toHaveAttribute("data-file-path", "assets/ui/theme.css");
    await expect(diff.locator("[data-file-diff-status]")).toHaveText("All changed hunks");
    if (!(await page.locator("[data-shell]").evaluate(el => el.classList.contains("tree-hidden")))) await page.getByRole("button", { name: "Hide file tree", exact: true }).click();
    await expect.poll(async () => {
      const box = await page.locator("#changed-files-panel").boundingBox();
      return box!.x + box!.width;
    }).toBeLessThanOrEqual(0);
    const narrow = await diff.boundingBox();
    expect(narrow!.width).toBeGreaterThanOrEqual(380);
    expect(narrow!.x).toBe(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: test.info().outputPath("review-code-narrow.png") });
    await page.getByRole("tab", { name: "Deck", exact: true }).click();
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:theme"]')).toBeVisible();
    expect(new URL(page.url()).hash).toBe(slideHash);
  } finally {
    await stopSagaServer(running);
  }
});

test("@critical approves slide by slide and marks a decision out of date when its code changes", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    const greeting = page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"]');
    const theme = page.locator('[data-deck-slide][data-slide-target$=":slide:theme"]');
    await expect(greeting).toBeVisible();
    await greeting.locator(".landmark-menu > summary").click();
    const codeReference = greeting.locator(".landmark-list").getByRole("button", { name: "Open 1 code reference for The change" });
    await expect(codeReference).toBeVisible();
    await codeReference.click();
    // The Item's code reference is shown as a diff against the review's base.
    await expect(page.locator("#review-drawer .review-line.add").filter({ hasText: `"hello, " + name` })).toHaveCount(1);
    await expect(page.locator("#review-drawer .review-line.del").filter({ hasText: `return "hello"` })).toHaveCount(1);
    await page.locator("[data-close-drawer]").last().click();
    const affectedRecord = greeting.locator(".landmark-list").getByRole("button", { name: "Open affected documentation for Feature" });
    await expect(affectedRecord).toBeVisible();
    await affectedRecord.click();
    await expect(page.locator('#review-drawer [data-review-record="urn:change-saga:wave-one:feature:wave-one"]')).toBeVisible();
    await page.locator("[data-close-drawer]").last().click();

    await greeting.locator("[data-review-approve]").click();
    await expect(greeting.locator('[data-decision-state="approved"]')).toHaveAttribute("data-currency", "current");
    await page.locator('[data-slide-thumbnail][data-slide-target$=":slide:theme"]').click();
    await theme.locator(".review-slide-menu > summary").click();
    await theme.locator("textarea[name=body]").first().fill("Name the colour token.");
    await theme.locator("[data-review-request-changes]").click();
    await expect(theme.locator('[data-decision-state="changes_requested"]')).toHaveAttribute("data-currency", "current");
    await theme.locator(".landmark-menu > summary").click();
    await theme.locator(".landmark-list").getByRole("button", { name: "Open 1 code reference for The change" }).click();
    const commentForm = page.locator('#review-drawer [data-review-comment-form$=":item:change"]');
    await commentForm.locator("xpath=preceding-sibling::summary").click();
    await commentForm.locator("textarea").fill("Is this contrast checked?");
    await commentForm.locator("button").click();
    await expect(page.locator("#review-drawer").getByText("Is this contrast checked?")).toBeVisible();
    const reply = page.locator("#review-drawer [data-review-reply-form]");
    await reply.locator("xpath=preceding-sibling::summary").click();
    await reply.locator("textarea").fill("Yes; the token passes the contrast check.");
    await reply.getByRole("button", { name: "Reply" }).click();
    await expect(page.locator("#review-drawer").getByText("Yes; the token passes the contrast check.")).toBeVisible();

    const approvals = reviewFiles(sagaRepositories, /___reviews\/pr-1\.review\/approvals\/.+\.json$/);
    expect(approvals).toHaveLength(2);
    for (const path of approvals) {
      const record = readJSON<{ commit: string; reviewer: { kind: string } }>(path);
      expect(record.commit).toBe(sagaRepositories.identity.head);
      expect(record.reviewer.kind).toBe("human");
    }

    // A push that changes the greeting's code puts only that slide's approval
    // out of date; the other slide keeps its decision and its currency.
    git(sagaRepositories.sourceRepo, "checkout", "feature/wave-one");
    writeFileSync(join(sagaRepositories.sourceRepo, "src", "app.go"), `package demo\n\nfunc Greeting(name string) string {\n\treturn "hi, " + name\n}\n\nfunc Ready() bool {\n\treturn true\n}\n`);
    git(sagaRepositories.sourceRepo, "commit", "-am", "shorter greeting");
    await page.reload();
    await expect(greeting.locator('[data-decision-state="approved"]')).toHaveAttribute("data-currency", "out_of_date");
    await expect(greeting.locator("[data-out-of-date]")).toHaveText("Out of date");
    await expect(theme.locator('[data-decision-state="changes_requested"]')).toHaveAttribute("data-currency", "current");

    // The review index and status report the same state with no verdict.
    await page.goto(new URL("/reviews", running.baseURL).toString());
    await expect(page.locator('[data-review-summary="pr-1"] [data-review-slide-state="greeting"] [data-currency="out_of_date"]')).toBeVisible();
    const status = runCLI(sagaRepositories, ["status", "--json", "--repo", sagaRepositories.sourceRepo, sagaRepositories.sagaRoot]);
    const report = JSON.parse(status.stdout) as { reviews: Array<{ id: string; slides: Array<{ id: string; decisions: Array<{ state: string; currency: string }> }> }> };
    const slides = Object.fromEntries(report.reviews[0].slides.map((slide) => [slide.id, slide.decisions.map((decision) => `${decision.state}:${decision.currency}`)]));
    expect(slides).toEqual({ greeting: ["approved:out_of_date"], theme: ["changes_requested:current"] });
  } finally {
    await stopSagaServer(running);
  }
});

test("keeps one review slide active with durable keyboard and narrow-screen navigation", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    // The review's own deck is the page. The shell's viewer of the Saga's
    // embedded decks is kept across pages, and stays out of sight here.
    const review = page.locator("#page");
    const slides = review.locator("[data-deck-slide]");
    await expect(slides).toHaveCount(2);
    await expect(slides.filter({ visible: true })).toHaveCount(1);
    await expect(review.locator("[data-slide-position]")).toContainText("1 / 2");
    await expect(page.locator(".review-top,.review-slide-details")).toHaveCount(0);
    await expect(page.locator("[data-slide-present]")).toBeVisible();
    await expect(page.locator("#view-slides")).toBeHidden();

    await page.locator('[data-slide-thumbnail][data-slide-target$=":slide:theme"]').click();
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:theme"]')).toBeVisible();
    await expect(page).toHaveURL(/#target-.*slide-theme/);
    await page.reload();
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:theme"]')).toBeVisible();
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"]')).toBeHidden();

    await page.keyboard.press("ArrowLeft");
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"]')).toBeVisible();
    const itemID = await page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"] [data-review-item="change"]').getAttribute("id");
    expect(itemID).toBeTruthy();
    await page.goto(new URL(`/reviews/pr-1#${itemID}`, running.baseURL).toString());
    await expect(page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"]')).toBeVisible();
    await expect(page.locator(`[data-landmark-visual="${itemID}"]`)).toBeVisible();

    const slideMenu = page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"] .review-slide-menu');
    await slideMenu.locator(":scope > summary").click();
    await slideMenu.locator("textarea").first().focus();
    await page.keyboard.press("Escape");
    await expect(slideMenu).not.toHaveAttribute("open", "");
    await expect(slideMenu.locator(":scope > summary")).toBeFocused();

    await page.setViewportSize({ width: 390, height: 844 });
    await expect(review.locator("[data-slide-thumbnail]").first()).toBeVisible();
    await expect(review.locator("[data-slide-next]")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: test.info().outputPath("review-narrow.png") });
  } finally {
    await stopSagaServer(running);
  }
});

test("draws, discusses, edits, and append-only deletes slide annotations", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    const slide = page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"]');
    const layer = slide.locator(".review-annotation-layer");
    await expect(layer).toBeVisible();
    const toolbar = page.getByRole("toolbar", { name: "Annotation tools" });
    await expect(toolbar).toBeHidden();
    const annotationToggle = slide.getByRole("button", { name: "Show annotation tools for Greeting takes a name" });
    await annotationToggle.click();
    await expect(toolbar).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(toolbar).toBeHidden();
    await expect(annotationToggle).toBeFocused();
    await annotationToggle.click();
    await toolbar.getByRole("button", { name: "Rectangle" }).click();
    const box = await layer.boundingBox();
    expect(box).toBeTruthy();
    await page.mouse.move(box!.x + box!.width * .32, box!.y + box!.height * .28);
    await page.mouse.down();
    await page.mouse.move(box!.x + box!.width * .62, box!.y + box!.height * .58, { steps: 8 });
    await expect(layer.locator(".review-annotation-draft rect")).toBeVisible();
    await page.mouse.up();
    const composer = page.locator(".review-annotation-compose");
    await expect(composer).toBeVisible();
    await composer.locator("textarea").fill("Clarify the transition between these states.");
    await composer.getByRole("button", { name: "Save annotation" }).click();

    const annotation = slide.locator(".review-annotation");
    await expect(annotation).toBeVisible();
    await annotation.locator(".review-annotation-bubble > summary").click();
    await expect(annotation.getByText("Clarify the transition between these states.")).toBeVisible();
    const reply = annotation.locator(".review-annotation-reply");
    await reply.locator("textarea").fill("I will add the missing state label.");
    await reply.getByRole("button", { name: "Reply" }).click();
    await expect(annotation.getByText("I will add the missing state label.")).toHaveCount(1);

    const records = () => reviewFiles(sagaRepositories, /___reviews\/pr-1\.review\/comments\/.+\.json$/);
    await expect.poll(() => records().length).toBe(2);
    type AnnotationRecord = { annotation_action: string; reply_to?: string; anchor: { coordinate_space: string; shapes: Array<{ x: number; y: number; width: number; height: number; color: string }> } };
    const root = readJSON<AnnotationRecord>(records()[0]);
    expect(root.annotation_action).toBe("create");
    expect(root.anchor.coordinate_space).toBe("normalized");
    expect(root.anchor.shapes[0]).toMatchObject({ x: expect.any(Number), y: expect.any(Number), width: expect.any(Number), height: expect.any(Number) });

    // The normalized mark keeps its relationship to the slide when the deck
    // changes size; only its rendered pixels scale.
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(annotation).toBeVisible();
    const narrow = await annotation.locator("rect").boundingBox();
    const narrowStage = await layer.boundingBox();
    expect(narrow).toBeTruthy();
    expect(narrow!.width / narrowStage!.width).toBeCloseTo(root.anchor.shapes[0].width, 1);
    await page.setViewportSize({ width: 1280, height: 800 });
    if (await toolbar.isHidden()) await slide.getByRole("button", { name: "Show annotation tools for Greeting takes a name" }).click();
    await expect(toolbar).toBeVisible();

    // Select the rectangle by its stroke, drag it, recolor it, then exercise
    // append-only undo/redo before deleting it with the keyboard.
    const mark = annotation.locator("rect");
    const markBox = await mark.boundingBox();
    await page.mouse.move(markBox!.x + 2, markBox!.y + 2);
    await page.mouse.down();
    await page.mouse.move(markBox!.x + 52, markBox!.y + 32, { steps: 5 });
    await page.mouse.up();
    await expect.poll(() => records().length).toBe(3);
    await expect(toolbar).not.toHaveAttribute("aria-busy", "true");
    const moved = readJSON<AnnotationRecord>(records()[2]);
    expect(moved.anchor.shapes[0].x).toBeGreaterThan(root.anchor.shapes[0].x);
    expect(moved.anchor.shapes[0].y).toBeGreaterThan(root.anchor.shapes[0].y);
    const handle = annotation.locator(".review-annotation-resize-handle");
    await expect(handle).toBeVisible();
    const handleBox = await handle.boundingBox();
    expect(handleBox).toBeTruthy();
    await page.mouse.move(handleBox!.x + handleBox!.width / 2, handleBox!.y + handleBox!.height / 2);
    await page.mouse.down();
    await page.mouse.move(handleBox!.x + 42, handleBox!.y + 32, { steps: 5 });
    await page.mouse.up();
    await expect.poll(() => records().length).toBe(4);
    await expect(toolbar).not.toHaveAttribute("aria-busy", "true");
    const resized = readJSON<AnnotationRecord>(records()[3]);
    expect(resized.anchor.shapes[0].width).toBeGreaterThan(moved.anchor.shapes[0].width);
    expect(resized.anchor.shapes[0].height).toBeGreaterThan(moved.anchor.shapes[0].height);
    await toolbar.locator('input[type="color"]').fill("#0969da");
    await expect.poll(() => records().length).toBe(5);
    await expect(toolbar).not.toHaveAttribute("aria-busy", "true");
    expect(readJSON<AnnotationRecord>(records()[4]).anchor.shapes[0].color).toBe("#0969da");
    await expect(toolbar.locator("[data-undo]")).toHaveAttribute("data-undo-kind", "color");
    await toolbar.locator("[data-undo]").click();
    await expect.poll(() => records().length).toBe(6);
    await expect(toolbar).not.toHaveAttribute("aria-busy", "true");
    expect(readJSON<AnnotationRecord>(records()[5]).anchor.shapes[0].color).toBe("#d04832");
    await expect(toolbar.locator("[data-redo]")).toHaveAttribute("data-redo-kind", "color");
    await toolbar.locator("[data-redo]").click();
    await expect.poll(() => records().length).toBe(7);
    await expect(toolbar).not.toHaveAttribute("aria-busy", "true");
    const redone = readJSON<AnnotationRecord>(records()[6]);
    expect(redone.anchor.shapes[0].color).toBe("#0969da");
    expect(redone.anchor.shapes[0].width).toBeCloseTo(resized.anchor.shapes[0].width);

    // The latest append-only update must project after a full reload, not
    // merely look correct in transient client state.
    await page.reload();
    await expect(annotation.locator("rect")).toHaveAttribute("stroke", "#0969da");
    await annotation.locator("rect").click({ position: { x: 2, y: 2 } });
    await page.keyboard.press("Delete");
    await expect(annotation).toHaveCount(0);
    await expect.poll(() => records().length).toBe(8);
    const deletion = readJSON<{ annotation_action: string; reply_to: string }>(records().at(-1)!);
    expect(deletion.annotation_action).toBe("delete");
    expect(deletion.reply_to).toBeTruthy();
  } finally {
    await stopSagaServer(running);
  }
});

test("keeps merged review annotations read-only", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  freezeReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    await expect(page.locator(".review-annotation-layer").first()).toBeVisible();
    await expect(page.getByRole("toolbar", { name: "Annotation tools" })).toHaveCount(0);
    await expect(page.locator("[data-review-decision-form], [data-review-comment-form]")).toHaveCount(0);
  } finally {
    await stopSagaServer(running);
  }
});

test("shows the living layers read-only beside the review of the compared change", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories, "main");
  try {
    // The Change view is a comparison view, so it is on the Review side.
    await page.goto(`${running.baseURL}/reviews`);
    await page.getByRole("tab", { name: "Change" }).click();
    const review = page.locator('[data-change-review] [data-review-summary="pr-1"]');
    await expect(review).toBeVisible();
    await expect(review.getByRole("link", { name: "Wave one review" })).toHaveAttribute("href", "/reviews/pr-1");
    await expect(page.locator('[data-change-layer="changed"]')).toBeVisible();
    await expect(page.locator("[data-review-decision],[data-review-comment],form[action*='/decision']")).toHaveCount(0);
  } finally {
    await stopSagaServer(running);
  }
});

// A slide visual is authored content like a fragment. Opened directly, it must
// not run on the app origin beside the review's mutation token.
test("serves a review slide visual on an opaque origin even when opened directly", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${running.baseURL}/reviews/pr-1`);
    const visual = page.locator('.review-deck-slide.active iframe.fragment-frame');
    await expect(visual.contentFrame().getByRole("img", { name: "Review slide" })).toBeVisible();
    await expect(page.locator('.review-deck-slide.active .landmark-hotspot[data-element-id="change"]')).toHaveCount(1);
    const source = new URL(await visual.getAttribute("src") ?? "", running.baseURL);
    const response = await page.goto(source.href);
    expect(response?.headers()["content-security-policy"]).toMatch(/(^|;\s*)sandbox allow-scripts(;|$)/);
    await expect(page.getByRole("img", { name: "Review slide" })).toBeVisible();
    expect(await page.evaluate(() => self.origin)).toBe("null");
  } finally {
    await stopSagaServer(running);
  }
});

test("async saves retain drafts on refusal and preserve the document, visual, drawer and slide", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${running.baseURL}/reviews/pr-1`);
    await page.waitForLoadState("networkidle");
    const slide = page.locator('.review-deck-slide.active');
    const navigations: string[] = [], fullGets: string[] = [];
    page.on('framenavigated', frame => navigations.push(frame.url()));
    page.on('request', request => {
      const path = new URL(request.url()).pathname;
      if (request.method() === 'GET' && (path === '/reviews/pr-1' || path.includes('/visual/'))) fullGets.push(path);
    });
    await slide.locator('[data-review-approve]').click();
    await expect(slide.locator('[data-decision-state="approved"]')).toHaveAttribute('data-currency','current');
    await expect(page.locator('.slide-thumbnail-status').first()).toHaveAttribute('data-review-state','approved');
    await slide.locator('.landmark-menu > summary').click();
    await slide.locator('.landmark-list [data-open-diffs]').first().click();
    const drawer=page.locator('#review-drawer');
    const form=drawer.locator('[data-review-comment-form]');
    await form.locator('xpath=preceding-sibling::summary').click();
    await form.locator('textarea').fill('Keep this Item draft');
    await page.route('**/reviews/pr-1/comment', route=>route.fulfill({status:400,body:'Refused before saving.'}),{times:1});
    await form.locator('button').click();
    await expect(form.locator('textarea')).toHaveValue('Keep this Item draft');
    await expect(form.locator('button')).toBeEnabled();
    await form.locator('button').click();
    await expect(drawer.getByText('Keep this Item draft')).toBeVisible();
    const reply=drawer.locator('[data-review-reply-form]');
    await reply.locator('xpath=preceding-sibling::summary').click();
    await reply.locator('textarea').fill('A reply without navigation');
    await reply.locator('button').click();
    await expect(drawer.getByText('A reply without navigation')).toBeVisible();
    await expect(drawer).toHaveClass(/open/);
    await page.locator('[data-close-drawer]').last().click();
    await slide.locator('[data-review-annotation-toggle]').click();
    const toolbar=page.getByRole('toolbar',{name:'Annotation tools'});
    await toolbar.getByRole('button',{name:'Rectangle',exact:true}).click();
    const layer=slide.locator('.review-annotation-layer'), box=(await layer.boundingBox())!;
    await page.mouse.move(box.x+box.width*.25,box.y+box.height*.25);await page.mouse.down();
    await page.mouse.move(box.x+box.width*.45,box.y+box.height*.45);await page.mouse.up();
    const composer=page.locator('.review-annotation-compose');
    await composer.locator('textarea').fill('Retry this mark once');
    await page.route('**/reviews/pr-1/comment',route=>route.fulfill({status:400,body:'Refused before saving.'}),{times:1});
    await composer.getByRole('button',{name:'Save annotation'}).click();
    await expect(composer.locator('textarea')).toHaveValue('Retry this mark once');
    await expect(layer.locator('.review-annotation-draft')).toBeVisible();
    await expect(composer.getByRole('button',{name:'Save annotation'})).toBeEnabled();
    await composer.getByRole('button',{name:'Save annotation'}).click();
    await expect(composer).toBeHidden();
    await expect(slide.locator('.review-annotation')).toHaveCount(1);
    expect(navigations).toEqual([]);expect(fullGets).toEqual([]);
    const records=reviewFiles(sagaRepositories,/___reviews\/pr-1\.review\/comments\/.+\.json$/);
    expect(records).toHaveLength(3);
    expect(records.map(p=>readJSON<{body:string}>(p).body).filter(body=>body==='Retry this mark once')).toHaveLength(1);
    await page.reload();
    await expect(slide.locator('.review-annotation')).toHaveCount(1);
  } finally { await stopSagaServer(running); }
});

test("lost confirmations never retry writes and new feedback cannot approve an unseen head", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${running.baseURL}/reviews/pr-1`);await page.waitForLoadState('networkidle');
    const slide=page.locator('.review-deck-slide.active');
    const shown=await slide.getAttribute('data-review-snapshot');
    await page.route('**/reviews/pr-1/decision',async route=>{
      const response=await route.fetch();
      expect(response.status()).toBe(200);
      await route.abort('failed');
    },{times:1});
    await slide.locator('[data-review-approve]').click();
    await expect(page.getByText(/confirmation was lost/)).toBeVisible();
    await expect(slide.locator('[data-review-approve]')).toBeDisabled();
    await page.getByRole('button',{name:'Check saved feedback'}).click();
    await expect(slide.locator('[data-decision-state="approved"]')).toHaveAttribute('data-currency','current');
    expect(reviewFiles(sagaRepositories,/___reviews\/pr-1\.review\/approvals\/.+\.json$/)).toHaveLength(1);
    writeFileSync(join(sagaRepositories.sourceRepo,'src/app.go'),'package main\n// unseen source change\n');
    git(sagaRepositories.sourceRepo,'add','.');git(sagaRepositories.sourceRepo,'commit','-m','Change after viewed slide');
    await page.getByRole('button',{name:'Check saved feedback'}).click();
    await expect(slide).toHaveAttribute('data-review-stale','true');
    await expect(slide).toHaveAttribute('data-review-snapshot',shown!);
    await expect(slide.locator('[data-review-approve]')).toBeDisabled();
    expect(reviewFiles(sagaRepositories,/___reviews\/pr-1\.review\/approvals\/.+\.json$/)).toHaveLength(1);
  } finally { await stopSagaServer(running); }
});

test("a refused sticky-note edit retains the editable draft and confirms exactly one update", async ({page,sagaRepositories}) => {
  authorReview(sagaRepositories);const running=await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${running.baseURL}/reviews/pr-1`);await page.waitForLoadState('networkidle');
    const slide=page.locator('.review-deck-slide.active');
    await slide.locator('[data-review-annotation-toggle]').click();
    await page.getByRole('toolbar',{name:'Annotation tools'}).getByRole('button',{name:'Sticky note',exact:true}).click();
    const layer=slide.locator('.review-annotation-layer'),box=(await layer.boundingBox())!;
    await page.mouse.click(box.x+box.width*.4,box.y+box.height*.4);
    const composer=page.locator('.review-annotation-compose');
    await composer.locator('textarea').fill('Original note');
    await composer.getByRole('button',{name:'Save annotation'}).click();
    const note=slide.locator('.review-sticky-note');await expect(note).toHaveText('Original note');
    await expect(composer).toBeHidden();await note.dblclick();
    await composer.locator('textarea').fill('Edited note retained');
    await page.route('**/reviews/pr-1/comment',route=>route.fulfill({status:400,body:'Edit refused before saving.'}),{times:1});
    await composer.getByRole('button',{name:'Save annotation'}).click();
    await expect(composer.locator('textarea')).toHaveValue('Edited note retained');
    await expect(note).toHaveText('Original note');
    await composer.getByRole('button',{name:'Save annotation'}).click();
    await expect(composer).toBeHidden();await expect(note).toHaveText('Edited note retained');
    const records=reviewFiles(sagaRepositories,/___reviews\/pr-1\.review\/comments\/.+\.json$/);
    expect(records).toHaveLength(2);
    expect(readJSON<{annotation_action:string}>(records[1]).annotation_action).toBe('update');
  } finally {await stopSagaServer(running);}
});
