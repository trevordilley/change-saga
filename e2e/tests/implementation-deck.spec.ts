import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI } from "../support/fixture-builder.js";

test("a Saga opens several implementation decks without paginating its documentation", async ({ page, saga, browser, browserName }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };

  for (const deck of [
    { id: "request-flow", title: "Request flow", slides: [["request-enters", "Request enters", "Ingress"], ["response-returns", "Response returns", "Egress"]] },
    { id: "failure-path", title: "Failure path", slides: [["failure-change", "Failure path", "Errors"]] }
  ] as const) {
    run("add-deck", "--feature", "wave-one", "--objective", `Explain the complex ${deck.title.toLowerCase()}.`, saga.sagaRoot, deck.id);
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
  run("cover", "--repo", saga.sourceRepo, "--against", "main", "--target", "urn:change-saga:wave-one:slide:request-enters:item:surprise", "--path", "src/app.go", "--side", "new", "--lines", "3", "--name", "request-slide-item", saga.sagaRoot);

  // Code-only and story-only elements must have independent affordances.
  const story = "urn:change-saga:wave-one:story:understand-request";
  run("story", "add", "--feature", "wave-one", "--id", "understand-request", "--revision", "r1", "--event", "proposed",
    "--title", "Understand a request", "--statement", "As a reviewer I can trace a request to its intent.",
    "--criterion", "visible=The intent is visible on the element", saga.sagaRoot);
  for (const [id, to] of [["request-story", story], ["request-criterion", `${story}:criterion:visible`]]) {
    run("relation", "add", "--feature", "wave-one", "--id", id, "--type", "explains",
      "--from", "urn:change-saga:wave-one:slide:request-enters:item:no-diff", "--to", to,
      "--rationale", "This element makes the request intent explicit.", saga.sagaRoot);
  }

  await page.reload();
  await waitForSettledSaga(page);

  await expect(page.getByRole("tablist", { name: "Documentation" }).getByRole("tab")).toHaveText([/Saga/, /Documented code/]);
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
  await linkedItem.click({ position: { x: 8, y: 8 } });
  const linkedCodeDrawer = page.getByRole("complementary", { name: "Linked code" });
  await expect(linkedCodeDrawer).toHaveAttribute("aria-hidden", "false");
  await expect(linkedCodeDrawer.getByText("src/app.go", { exact: true })).toBeVisible();
  await linkedCodeDrawer.getByRole("button", { name: "Close linked code" }).click();
  await linkedItem.locator(".diff-button").blur();

  await expect(linkedItem).toHaveAttribute("data-landmark-has-stories", "false");
  await expect(unlinkedItem).toHaveAttribute("data-landmark-has-stories", "true");
  const slideStories = activeSlide.locator(".fragment > .fragment-head > .fragment-actions > .stories-button");
  await expect(slideStories).toHaveText("1"); // Two relations, one distinct story.
  await expect(slideStories).toHaveAttribute("title", "Linked stories: Understand a request");
  await slideStories.hover();
  await expect(unlinkedItem).toHaveCSS("border-color", "rgb(211, 148, 24)");
  await expect(linkedItem).toHaveCSS("border-color", "rgba(0, 0, 0, 0)");
  await page.locator(".brand").hover();
  await expect(unlinkedItem).toHaveCSS("border-color", "rgba(0, 0, 0, 0)");
  await slideStories.focus();
  await expect(unlinkedItem).toHaveCSS("border-color", "rgb(211, 148, 24)");
  await page.keyboard.press("Enter");
  const storiesDrawer = page.getByRole("complementary", { name: "Linked stories" });
  await expect(storiesDrawer).toHaveAttribute("aria-hidden", "false");
  await expect(storiesDrawer.locator("[data-story-link]")).toHaveCount(2);
  await expect(storiesDrawer.getByRole("link", { name: "Understand a request", exact: true })).toHaveCount(2);
  await expect(storiesDrawer.getByRole("link", { name: "The intent is visible on the element" })).toHaveAttribute("href", /understand-request.*visible/);
  await expect(storiesDrawer.locator(".story-link-status")).toHaveText(["current", "current"]);
  await page.keyboard.press("Escape");
  await expect(slideStories).toBeFocused();
  await slideStories.blur();
  await unlinkedItem.hover();
  const elementStories = unlinkedItem.locator(".stories-button");
  await expect(elementStories).toBeVisible();
  await expect(unlinkedItem.locator(".diff-button")).toHaveCount(0);
  await elementStories.click();
  await expect(storiesDrawer).toHaveAttribute("aria-hidden", "false");
  await expect(storiesDrawer.locator("[data-story-link]")).toHaveCount(2);
  await storiesDrawer.getByRole("button", { name: "Close linked stories" }).click();
  await expect(elementStories).toBeFocused();

  // A deck is documentation: its slides and Items carry no approval, comment,
  // or annotation control.
  await expect(activeSlide.locator("[data-review-decision],[data-review-comment],[data-annotation-tools],[data-review-controls]")).toHaveCount(0);
  await expect(page.locator(".annotation-toolbox,form[action=\"/api/thread\"]")).toHaveCount(0);
  run("validate", saga.sagaRoot);

  run("relation", "add", "--feature", "wave-one", "--id", "both-story-and-code", "--type", "explains",
    "--from", "urn:change-saga:wave-one:slide:request-enters:item:surprise", "--to", story,
    "--rationale", "The source implements this intent.", saga.sagaRoot);

  const reload = await page.reload();
  expect(reload?.status()).toBe(200);
  await waitForSettledSaga(page);
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Request enters"]')).toBeVisible();
  await linkedItem.hover();
  await expect(linkedItem.locator(".stories-button")).toBeVisible();
  await expect(linkedItem.locator(".diff-button")).toBeVisible();
  await linkedItem.locator(".stories-button").click();
  await expect(storiesDrawer).toHaveAttribute("aria-hidden", "false");
  await expect(storiesDrawer.locator("[data-story-link]")).toHaveCount(1);
  await expect(storiesDrawer.getByText("The source implements this intent.", { exact: true })).toBeVisible();
  await expect(storiesDrawer).toHaveCSS("transform", "none");
  await page.screenshot({ path: test.info().outputPath("element-story-links.png") });
  await page.getByRole("button", { name: "Toggle dark mode" }).click();
  await page.screenshot({ path: test.info().outputPath("element-story-links-dark.png") });
  await page.getByRole("button", { name: "Toggle dark mode" }).click();
  await storiesDrawer.getByRole("button", { name: "Close linked stories" }).click();
  await linkedItem.locator(".diff-button").click();
  await expect(linkedCodeDrawer).toHaveAttribute("aria-hidden", "false");
  await linkedCodeDrawer.getByRole("button", { name: "Close linked code" }).click();

  await slidePanel.getByRole("button", { name: "Next slide" }).click();
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Response returns"]')).toBeVisible();
  const emptyStories = slidePanel.locator('[data-deck-slide][data-slide-title="Response returns"] .fragment-head .stories-button');
  await expect(emptyStories).toHaveText("0");
  await emptyStories.click();
  await expect(storiesDrawer.locator("[data-story-links-empty]")).toHaveText("No element-level story links yet.");
  await storiesDrawer.getByRole("button", { name: "Close linked stories" }).click();
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
  await page.getByRole("heading", { name: "Wave One", exact: true }).click();
  await page.keyboard.press("ArrowLeft");
  await failureDeckNode.getByRole("button", { name: "Show slide: Failure path" }).click();
  await expect(slidePanel.locator('[data-deck-slide][data-slide-title="Failure path"]')).toBeVisible();

  // Touch has no hover: story controls remain visible and can be tapped.
  if (browserName !== "firefox") {
    await requestSlide.click();
    const context = await browser.newContext({ hasTouch: true, viewport: { width: 1280, height: 900 } });
    try {
      const touchPage = await context.newPage();
      await touchPage.goto(page.url());
      await waitForSettledSaga(touchPage);
      const touchElement = touchPage.locator('[data-slide-title="Request enters"] .landmark-hotspot[data-element-id="unlinked-node"]');
      await expect(touchElement.locator(".landmark-affordance")).toHaveCSS("opacity", "1");
      await touchElement.locator(".stories-button").tap();
      const touchDrawer = touchPage.getByRole("complementary", { name: "Linked stories" });
      await expect(touchDrawer).toHaveAttribute("aria-hidden", "false");
      await touchDrawer.getByRole("link", { name: "The intent is visible on the element" }).tap();
      await expect(touchPage).toHaveURL(/understand-request.*visible/);
    } finally {
      await context.close();
    }
  }
});
