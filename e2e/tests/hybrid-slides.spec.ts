import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI } from "../support/fixture-builder.js";

test("a Report Saga opens several implementation decks without paginating its documentation", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };

  run("upgrade", "--to", "3", saga.sagaRoot);
  for (const [deck, slide, title] of [
    ["request-flow", "request-change", "Request flow"],
    ["failure-path", "failure-change", "Failure path"]
  ] as const) {
    run("add-deck", "--objective", `Explain the complex ${title.toLowerCase()}.`, saga.sagaRoot, deck);
    run("add-slide", "--deck", deck, "--intent", "explain", "--layout", "diagram", "--title", title, "--takeaway", `${title} is explicit.`, saga.sagaRoot, slide);
    run("add-item", "--slide", slide, "--kind", "callout", "--id", "surprise", "--element-id", "slide-title", "--description", `The surprising part of the ${title.toLowerCase()}.`, "--body", "The implementation follows a non-obvious path.", saga.sagaRoot);
  }

  await page.reload();
  await waitForSettledSaga(page);

  await expect(page.getByRole("tablist", { name: "Workspace" }).getByRole("tab")).toHaveText([/Saga/, /Decks/, /Code Diff/, /Coverage/]);
  await expect(page.getByRole("tabpanel", { name: "Saga" })).toBeVisible();
  await expect(page.getByText("Wave 1 connects the story")).toBeVisible();

  await page.getByRole("tab", { name: "Decks" }).click();
  const deckPanel = page.getByRole("tabpanel", { name: "Decks" });
  await expect(deckPanel).toBeVisible();
  await expect(deckPanel.locator("[data-native-slide]")).toHaveCount(2);
  await expect(deckPanel.getByRole("button", { name: "Show slide: Request flow" })).toHaveAttribute("aria-current", "true");

  await deckPanel.getByRole("button", { name: "Show slide: Failure path" }).click();
  await expect(deckPanel.locator('[data-native-slide][data-slide-title="Failure path"]')).toBeVisible();
  await expect(page).toHaveURL(/[?&]view=slides/);

  // Slide navigation is scoped to the Decks surface. Returning to the report
  // and pressing an arrow key must not silently move its hidden deck.
  await page.getByRole("tab", { name: "Saga" }).click();
  await page.keyboard.press("ArrowLeft");
  await page.getByRole("tab", { name: "Decks" }).click();
  await expect(deckPanel.locator('[data-native-slide][data-slide-title="Failure path"]')).toBeVisible();
});
