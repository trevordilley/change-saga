import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI } from "../support/fixture-builder.js";

test("a Report Saga opens several implementation decks without paginating its documentation", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };

  run("upgrade", "--to", "3", saga.sagaRoot);
  for (const deck of [
    { id: "request-flow", title: "Request flow", slides: [["request-enters", "Request enters", "Ingress"], ["response-returns", "Response returns", "Egress"]] },
    { id: "failure-path", title: "Failure path", slides: [["failure-change", "Failure path", "Errors"]] }
  ] as const) {
    run("add-deck", "--objective", `Explain the complex ${deck.title.toLowerCase()}.`, saga.sagaRoot, deck.id);
    for (const [slide, title, section] of deck.slides) {
      run("add-slide", "--deck", deck.id, "--section", section, "--intent", "explain", "--layout", "diagram", "--title", title, "--takeaway", `${title} is explicit.`, saga.sagaRoot, slide);
      run("add-item", "--slide", slide, "--kind", "callout", "--id", "surprise", "--element-id", "slide-title", "--description", `The surprising part of ${title.toLowerCase()}.`, "--body", "The implementation follows a non-obvious path.", saga.sagaRoot);
    }
  }

  await page.reload();
  await waitForSettledSaga(page);

  await expect(page.getByRole("tablist", { name: "Workspace" }).getByRole("tab")).toHaveText([/Saga/, /Code Diff/, /Coverage/]);
  await expect(page.getByRole("tabpanel", { name: "Saga" })).toBeVisible();
  await expect(page.getByText("Wave 1 connects the story")).toBeVisible();

  const requestDeck = page.locator("[data-deck-toggle]", { hasText: "request flow" });
  const failureDeck = page.locator("[data-deck-toggle]", { hasText: "failure path" });
  await expect(requestDeck).toHaveAttribute("aria-expanded", "false");
  await requestDeck.click();
  await expect(requestDeck).toHaveAttribute("aria-expanded", "true");
  const requestDeckNode = requestDeck.locator("xpath=ancestor::div[contains(@class,'doc-deck')]");
  await expect(requestDeckNode.locator("[data-slide-thumbnail]")).toHaveCount(2);
  await expect(requestDeckNode.locator(".slide-thumbnail-preview img")).toHaveCount(2);
  await expect(requestDeckNode.locator("[data-slide-section]")).toHaveText(["Ingress", "Egress"]);
  await expect(requestDeckNode.locator(":scope > .doc-children")).toHaveCSS("margin-left", "0px");
  await expect(requestDeckNode.locator(":scope > .doc-children")).toHaveCSS("padding-left", "0px");
  await requestDeckNode.getByRole("button", { name: "Show slide: Request enters" }).click();

  const slidePanel = page.locator("#view-slides");
  await expect(slidePanel).toBeVisible();
  await expect(slidePanel.locator("[data-native-slide]")).toHaveCount(3);
  await expect(slidePanel.locator('[data-native-slide][data-slide-title="Request enters"]')).toBeVisible();
  await expect(slidePanel.locator("[data-slide-position]")).toHaveText("1 / 2");
  await expect(page.locator("[data-shell]")).toHaveClass(/slide-mode/);
  await expect(page.getByRole("tab", { name: "Saga" })).toHaveAttribute("aria-selected", "true");

  await slidePanel.getByRole("button", { name: "Next slide" }).click();
  await expect(slidePanel.locator('[data-native-slide][data-slide-title="Response returns"]')).toBeVisible();
  await expect(slidePanel.locator("[data-slide-position]")).toHaveText("2 / 2");
  await expect(slidePanel.locator("[data-slide-next]")).toBeDisabled();

  await failureDeck.click();
  const failureDeckNode = failureDeck.locator("xpath=ancestor::div[contains(@class,'doc-deck')]");
  await expect(failureDeckNode.locator(".slide-thumbnail-preview img")).toHaveCount(1);
  await failureDeckNode.getByRole("button", { name: "Show slide: Failure path" }).click();
  await expect(slidePanel.locator('[data-native-slide][data-slide-title="Failure path"]')).toBeVisible();
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
  await expect(slidePanel.locator('[data-native-slide][data-slide-title="Failure path"]')).toBeVisible();
});
