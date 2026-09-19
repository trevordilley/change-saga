import { existsSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { reviewFiles, runCLI } from "../support/fixture-builder.js";

test("a Saga opens several implementation decks without paginating its documentation", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };

  for (const deck of [
    { id: "request-flow", title: "Request flow", slides: [["request-enters", "Request enters", "Ingress"], ["response-returns", "Response returns", "Egress"]] },
    { id: "failure-path", title: "Failure path", slides: [["failure-change", "Failure path", "Errors"]] }
  ] as const) {
    run("add-deck", "--epic", "wave-one", "--objective", `Explain the complex ${deck.title.toLowerCase()}.`, saga.sagaRoot, deck.id);
    for (const [slide, title, section] of deck.slides) {
      run("add-slide", "--deck", deck.id, "--section", section, "--intent", "explain", "--layout", "diagram", "--title", title, "--takeaway", `${title} is explicit.`, saga.sagaRoot, slide);
      if (slide === "request-enters") {
        const source = join(saga.root, "request-enters.svg");
        writeFileSync(source, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><rect width="1280" height="720" fill="#f7f7f4"/><g id="linked-node"><rect x="120" y="180" width="420" height="260" rx="32" fill="#dce8ff" stroke="#3867a8" stroke-width="6"/><text x="190" y="325" font-size="44">Linked item</text></g><g id="unlinked-node"><rect x="740" y="180" width="420" height="260" rx="32" fill="#f2f2ee" stroke="#777" stroke-width="6"/><text x="790" y="325" font-size="44">No diff</text></g></svg>`);
        run("set-slide-content", "--target", slide, "--source", source, saga.sagaRoot);
      }
      run("add-item", "--slide", slide, "--kind", "callout", "--id", "surprise", "--element-id", slide === "request-enters" ? "linked-node" : "slide-title", "--description", `The surprising part of ${title.toLowerCase()}.`, "--body", "The implementation follows a non-obvious path.", saga.sagaRoot);
      if (slide === "request-enters") run("add-item", "--slide", slide, "--kind", "node", "--id", "no-diff", "--element-id", "unlinked-node", "--description", "A nearby element without exact diff evidence.", saga.sagaRoot);
    }
  }
  run("cover", "--repo", saga.sourceRepo, "--target", "urn:change-saga:wave-one:slide:request-enters:item:surprise", "--path", "src/app.go", "--side", "new", "--lines", "3", "--name", "request-slide-item", saga.sagaRoot);

  await page.reload();
  await waitForSettledSaga(page);

  await expect(page.getByRole("tablist", { name: "Workspace" }).getByRole("tab")).toHaveText([/Saga/, /Code Diff/, /Coverage/]);
  await expect(page.getByRole("tabpanel", { name: "Saga" })).toBeVisible();
  await expect(page.getByText("Wave 1 connects the story")).toBeVisible();

  const requestDeck = page.locator("[data-deck-toggle]", { hasText: "request flow" });
  const failureDeck = page.locator("[data-deck-toggle]", { hasText: "failure path" });
  // Implementation opens with its decks expanded; several decks keep their
  // rows so the reader knows which deck a slide belongs to.
  await expect(requestDeck).toHaveAttribute("aria-expanded", "true");
  await expect(failureDeck).toHaveAttribute("aria-expanded", "true");
  const requestDeckNode = requestDeck.locator("xpath=ancestor::div[contains(@class,'doc-deck')]");
  await expect(requestDeckNode.locator("[data-slide-thumbnail]")).toHaveCount(2);
  await expect(requestDeckNode.locator(".slide-thumbnail-preview img")).toHaveCount(2);
  await expect(requestDeckNode.locator("[data-slide-section]")).toHaveText(["Ingress", "Egress"]);
  await expect(requestDeckNode.locator(":scope > .doc-children")).toHaveCSS("margin-left", "0px");
  await expect(requestDeckNode.locator(":scope > .doc-children")).toHaveCSS("padding-left", "0px");
  const requestSlide = requestDeckNode.getByRole("button", { name: "Show slide: Request enters" });
  await requestSlide.hover();
  await expect(requestSlide).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await requestSlide.click();

  const slidePanel = page.locator("#view-slides");
  await expect(slidePanel).toBeVisible();
  await expect(slidePanel.locator("[data-deck-slide]")).toHaveCount(3);
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Request enters"]')).toBeVisible();
  await expect(slidePanel.locator("[data-slide-position]")).toHaveText("1 / 2");
  await expect(page.locator("[data-shell]")).toHaveClass(/slide-mode/);
  await expect(page.getByRole("tab", { name: "Saga" })).toHaveAttribute("aria-selected", "true");

  const activeSlide = slidePanel.locator('[data-deck-slide][data-slide-title="Request enters"]');
  const linkedItem = activeSlide.locator('.landmark-hotspot[data-element-id="linked-node"]');
  const unlinkedItem = activeSlide.locator('.landmark-hotspot[data-element-id="unlinked-node"]');
  await expect(linkedItem).toHaveAttribute("data-landmark-has-diffs", "true");
  await expect(unlinkedItem).toHaveAttribute("data-landmark-has-diffs", "false");
  const slideDiffs = activeSlide.locator(".fragment > .fragment-head > .fragment-actions > .diff-button").first();
  await expect(slideDiffs).toHaveAttribute("data-open-diffs", /.+/);
  await slideDiffs.hover();
  await expect(linkedItem).toHaveCSS("border-color", "rgb(211, 148, 24)");
  await expect(linkedItem.locator(".landmark-affordance")).toHaveCSS("opacity", "1");
  await expect(unlinkedItem).toHaveCSS("border-color", "rgba(0, 0, 0, 0)");
  await page.locator(".brand").hover();
  await expect(linkedItem).toHaveCSS("border-color", "rgba(0, 0, 0, 0)");

  // Review edits stay on the active slide. Item comments use the same flat
  // overlay as the embedded deck, refresh only this slide, and keep the
  // reviewer's URL, scroll position, and deck context intact.
  const reviewURL = page.url();
  const reviewScroll = await page.evaluate(() => ({ x: scrollX, y: scrollY }));
  const navigations: string[] = [];
  page.on("framenavigated", (frame) => { if (frame === page.mainFrame()) navigations.push(frame.url()); });
  const itemComment = linkedItem.locator("[data-review-comment]");
  const itemCommentBox = await itemComment.boundingBox();
  await itemComment.click();
  const composer = page.locator("form.annotation-compose.open");
  await expect(composer).toHaveClass(/anchored/);
  const composerBox = await composer.boundingBox();
  expect(itemCommentBox).not.toBeNull();
  expect(composerBox).not.toBeNull();
  expect(Math.abs(composerBox!.y - itemCommentBox!.y)).toBeLessThan(300);
  await composer.getByRole("textbox", { name: "Comment" }).fill("Keep this implementation link visible.");
  const commented = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/thread" && response.request().method() === "POST");
  await composer.getByRole("button", { name: "Comment" }).click();
  expect((await commented).status()).toBe(204);
  await expect(activeSlide.getByText("Keep this implementation link visible.", { exact: true })).toHaveCount(1);
  await expect(linkedItem.locator(".landmark-comment-count")).toHaveText("1");
  expect(page.url()).toBe(reviewURL);
  expect(await page.evaluate(() => ({ x: scrollX, y: scrollY }))).toEqual(reviewScroll);
  expect(navigations).toEqual([]);

  let thread = activeSlide.locator("article.thread").filter({ hasText: "Keep this implementation link visible." });
  await thread.getByRole("textbox", { name: "Reply" }).fill("Confirmed without leaving the slide.");
  const replied = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/reply" && response.request().method() === "POST");
  await thread.getByRole("button", { name: "Reply" }).click();
  expect((await replied).status()).toBe(204);
  await expect(activeSlide.getByText("Confirmed without leaving the slide.", { exact: true })).toHaveCount(1);
  thread = activeSlide.locator("article.thread").filter({ hasText: "Keep this implementation link visible." });
  const resolved = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/thread-state" && response.request().method() === "POST");
  await thread.getByRole("button", { name: "Resolve" }).click();
  expect((await resolved).status()).toBe(204);
  await expect(activeSlide.locator("article.thread.resolved").filter({ hasText: "Keep this implementation link visible." })).toHaveCount(1);

  const slideControls = activeSlide.locator(".fragment-head [data-review-controls]").first();
  const approved = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/review" && response.request().method() === "POST");
  await slideControls.getByRole("button", { name: "Approve Request enters" }).click();
  expect((await approved).status()).toBe(204);
  await expect(slideControls.getByRole("button", { name: /Approval recorded for Request enters/ })).toHaveAttribute("aria-pressed", "true");
  expect(page.url()).toBe(reviewURL);
  expect(navigations).toEqual([]);
  expect(reviewFiles(saga, /\/84-r-.*\.json$/)).toHaveLength(1);
  expect(existsSync(join(saga.sagaRoot, "___epics", "wave-one.epic", "___slides", "request-flow.deck", "___approvals"))).toBe(false);
  run("validate", saga.sagaRoot);

  const reload = await page.reload();
  expect(reload?.status()).toBe(200);
  await waitForSettledSaga(page);
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Request enters"]')).toBeVisible();

  await slidePanel.getByRole("button", { name: "Next slide" }).click();
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Response returns"]')).toBeVisible();
  await expect(slidePanel.locator("[data-slide-position]")).toHaveText("2 / 2");
  await expect(slidePanel.locator("[data-slide-next]")).toBeDisabled();

  const failureDeckNode = failureDeck.locator("xpath=ancestor::div[contains(@class,'doc-deck')]");
  await expect(failureDeckNode.locator(".slide-thumbnail-preview img")).toHaveCount(1);
  await failureDeckNode.getByRole("button", { name: "Show slide: Failure path" }).click();
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Failure path"]')).toBeVisible();
  await expect(slidePanel.locator("[data-slide-position]")).toHaveText("1 / 1");
  await expect(slidePanel.locator("[data-slide-previous]")).toBeDisabled();
  await expect(slidePanel.locator("[data-slide-next]")).toBeDisabled();
  await expect(page).toHaveURL(/[?&]view=slides/);

  // Slide navigation is scoped to the slide surface. Returning to the report
  // and pressing an arrow key must not silently move its hidden deck.
  await page.getByRole("tab", { name: "Saga" }).click();
  await page.getByRole("heading", { name: "Wave One Review" }).click();
  await page.keyboard.press("ArrowLeft");
  await failureDeckNode.getByRole("button", { name: "Show slide: Failure path" }).click();
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Failure path"]')).toBeVisible();
});
