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

test("@critical approves slide by slide and marks a decision out of date when its code changes", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    const greeting = page.locator('[data-deck-slide][data-slide-target$=":slide:greeting"]');
    const theme = page.locator('[data-deck-slide][data-slide-target$=":slide:theme"]');
    await expect(greeting).toBeVisible();
    await greeting.getByRole("button", { name: "Open linked evidence for The change" }).click();
    // The Item's code reference is shown as a diff against the review's base.
    await expect(page.locator("#review-drawer .review-line.add").filter({ hasText: `"hello, " + name` })).toHaveCount(1);
    await expect(page.locator("#review-drawer .review-line.del").filter({ hasText: `return "hello"` })).toHaveCount(1);
    await page.locator("[data-close-drawer]").last().click();
    await greeting.getByRole("button", { name: "Open affected documentation for Feature" }).click();
    await expect(page.locator('#review-drawer [data-review-record="urn:change-saga:wave-one:feature:wave-one"]')).toBeVisible();
    await page.locator("[data-close-drawer]").last().click();

    await greeting.locator(".review-slide-menu > summary").click();
    await greeting.locator("[data-review-approve]").click();
    await expect(greeting.locator('[data-decision-state="approved"]')).toHaveAttribute("data-currency", "current");
    await page.locator('[data-slide-thumbnail][data-slide-target$=":slide:theme"]').click();
    await theme.locator(".review-slide-menu > summary").click();
    await theme.locator("textarea[name=body]").first().fill("Name the colour token.");
    await theme.locator("[data-review-request-changes]").click();
    await expect(theme.locator('[data-decision-state="changes_requested"]')).toHaveAttribute("data-currency", "current");
    await theme.getByRole("button", { name: "Open linked evidence for The change" }).click();
    const commentForm = page.locator('#review-drawer [data-review-comment-form$=":item:change"]');
    await commentForm.locator("xpath=preceding-sibling::summary").click();
    await commentForm.locator("textarea").fill("Is this contrast checked?");
    await commentForm.locator("button").click();
    await theme.getByRole("button", { name: "Open linked evidence for The change" }).click();
    await expect(page.locator("#review-drawer").getByText("Is this contrast checked?")).toBeVisible();
    const reply = page.locator("#review-drawer [data-review-reply-form]");
    await reply.locator("xpath=preceding-sibling::summary").click();
    await reply.locator("textarea").fill("Yes; the token passes the contrast check.");
    await reply.getByRole("button", { name: "Reply" }).click();
    await theme.getByRole("button", { name: "Open linked evidence for The change" }).click();
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
    const slides = page.locator("[data-deck-slide]");
    await expect(slides).toHaveCount(2);
    await expect(slides.filter({ visible: true })).toHaveCount(1);
    await expect(page.locator("[data-slide-position]")).toContainText("1 / 2");
    await expect(page.locator(".review-top,.review-slide-details")).toHaveCount(0);

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
    await expect(page.locator("[data-slide-thumbnail]").first()).toBeVisible();
    await expect(page.locator("[data-slide-next]")).toBeVisible();
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
    const toolbar = page.getByRole("toolbar", { name: "Annotate active slide" });
    await toolbar.getByRole("button", { name: "Rectangle" }).click();
    const box = await layer.boundingBox();
    expect(box).toBeTruthy();
    await page.mouse.move(box!.x + box!.width * .32, box!.y + box!.height * .28);
    await page.mouse.down();
    await page.mouse.move(box!.x + box!.width * .62, box!.y + box!.height * .58, { steps: 8 });
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
    const root = readJSON<{ annotation_action: string; anchor: { coordinate_space: string; shapes: Array<{ x: number; y: number; width: number; height: number }> } }>(records()[0]);
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

    // Select the rectangle by its stroke, drag it, recolor it, then exercise
    // append-only undo/redo before deleting it with the keyboard.
    const mark = annotation.locator("rect");
    const markBox = await mark.boundingBox();
    await page.mouse.move(markBox!.x + 2, markBox!.y + 2);
    await page.mouse.down();
    await page.mouse.move(markBox!.x + 52, markBox!.y + 32, { steps: 5 });
    await page.mouse.up();
    await expect.poll(() => records().length).toBe(3);
    await toolbar.locator('input[type="color"]').fill("#0969da");
    await expect.poll(() => records().length).toBe(4);
    await toolbar.getByRole("button", { name: "Undo" }).click();
    await expect.poll(() => records().length).toBe(5);
    await toolbar.getByRole("button", { name: "Redo" }).click();
    await expect.poll(() => records().length).toBe(6);
    await page.keyboard.press("Delete");
    await expect(annotation).toHaveCount(0);
    await expect.poll(() => records().length).toBe(7);
    const deletion = readJSON<{ annotation_action: string; reply_to: string }>(records().at(-1)!);
    expect(deletion.annotation_action).toBe("delete");
    expect(deletion.reply_to).toBeTruthy();
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
