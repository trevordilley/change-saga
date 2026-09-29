import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, reviewFiles } from "../support/fixture-builder.js";
import type { Page } from "@playwright/test";

async function chooseReviewer(page: Page, name: string): Promise<void> {
  const controls = page.locator('[data-deck-viewer]:visible [data-slide-viewed-controls]');
  await controls.locator('[data-viewed-identity-summary]').click();
  await controls.getByRole('textbox', { name: 'Local reviewer name' }).fill(name);
  await controls.getByRole('button', { name: 'Use name' }).click();
}

test('Viewed is manual, persists through navigation and reload, and belongs to a local reviewer @critical', async ({ page, saga }) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stderr).toBe(0);
  };
  run('add-deck', '--feature', 'wave-one', '--objective', 'Explain reading progress.', saga.sagaRoot, 'reading');
  for (const slide of ['first', 'second']) {
    run('add-slide', '--deck', 'reading', '--intent', 'explain', '--layout', 'diagram', '--title', slide, '--takeaway', 'Reading is separate from approval.', saga.sagaRoot, slide);
  }
  run('add-deck', '--feature', 'wave-one', '--objective', 'Explain the other path.', saga.sagaRoot, 'other-reading');
  run('add-slide', '--deck', 'other-reading', '--intent', 'explain', '--layout', 'diagram', '--title', 'Other path', saga.sagaRoot, 'other-path');
  await page.reload();
  await waitForSettledSaga(page);
  await page.getByRole('button', { name: 'Show slide: first', exact: true }).click();
  const controls = page.locator('[data-deck-viewer]:visible [data-slide-viewed-controls]');
  const checkbox = controls.getByRole('checkbox', { name: 'Viewed', exact: true });
  const count = controls.locator('[data-viewed-count]');
  await expect(checkbox).toBeEnabled();
  await expect(count).toHaveText('Viewed 0 of 2');
  // A stable Local reader works immediately without setup. Naming a different
  // reviewer changes only the local reading profile.
  await checkbox.check();
  await expect(count).toHaveText('Viewed 1 of 2');
  await chooseReviewer(page, 'Alice');
  await expect(checkbox).not.toBeChecked();
  await page.getByRole('button', { name: 'Next slide', exact: true }).click();
  await expect(checkbox).not.toBeChecked();
  await expect(count).toHaveText('Viewed 0 of 2');
  await page.getByRole('button', { name: 'Previous slide', exact: true }).click();
  await checkbox.check();
  await expect(count).toHaveText('Viewed 1 of 2');
  const firstCard = page.locator('[data-slide-thumbnail-card]').filter({ has: page.getByRole('button', { name: 'Show slide: first', exact: true }) });
  await expect(firstCard.locator('[data-viewed-badge]')).toHaveText('Viewed');
  const keys = await page.evaluate(() => Object.keys(localStorage).filter(key => key.startsWith('change-saga:viewed:v1:')).filter(key => key !== 'change-saga:viewed:v1:local-reader').map(key => JSON.parse(key.slice('change-saga:viewed:v1:'.length))));
  expect(keys).toContainEqual(['wave-one', 'Alice', 'urn:change-saga:wave-one:deck:reading', 'urn:change-saga:wave-one:slide:first']);
  await page.getByRole('button', { name: 'Next slide', exact: true }).click();
  await expect(checkbox).not.toBeChecked();
  await expect(count).toHaveText('Viewed 1 of 2');
  await page.reload();
  await waitForSettledSaga(page);
  await expect(checkbox).not.toBeChecked();
  await expect(count).toHaveText('Viewed 1 of 2');
  await page.getByRole('button', { name: 'Previous slide', exact: true }).click();
  await expect(checkbox).toBeChecked();
  await chooseReviewer(page, 'Bob');
  await expect(checkbox).not.toBeChecked();
  await expect(count).toHaveText('Viewed 0 of 2');
  await checkbox.check();
  await chooseReviewer(page, 'Alice');
  await expect(checkbox).toBeChecked();
  await checkbox.uncheck();
  await page.reload();
  await waitForSettledSaga(page);
  await expect(checkbox).not.toBeChecked();
  await expect(count).toHaveText('Viewed 0 of 2');
  await chooseReviewer(page, 'Bob');
  await expect(checkbox).toBeChecked();
  await expect(count).toHaveText('Viewed 1 of 2');
  await page.getByRole('button', { name: 'Show slide: Other path', exact: true }).click();
  await expect(count).toHaveText('Viewed 0 of 1');
  await expect(checkbox).not.toBeChecked();
  await page.getByRole('button', { name: 'Show slide: first', exact: true }).click();
  await expect(count).toHaveText('Viewed 1 of 2');
  await controls.locator('[data-viewed-identity-summary]').click();
  await controls.getByRole('button', { name: 'Use local reader', exact: true }).click();
  await expect(checkbox).toBeChecked();
  await expect(count).toHaveText('Viewed 1 of 2');
  await page.screenshot({ path: test.info().outputPath('slide-viewed.png') });
});

test('Viewed degrades honestly when browser storage is unavailable', async ({ page, saga }) => {
  for (const args of [
    ['add-deck', '--feature', 'wave-one', '--objective', 'Explain storage.', saga.sagaRoot, 'storage'],
    ['add-slide', '--deck', 'storage', '--intent', 'explain', '--layout', 'diagram', '--title', 'Storage', saga.sagaRoot, 'storage-slide']
  ]) expect(runCLI(saga, args, saga.sagaRepo).status).toBe(0);
  await page.addInitScript(() => {
    Storage.prototype.getItem = () => { throw new Error('Storage blocked'); };
    Storage.prototype.setItem = () => { throw new Error('Storage blocked'); };
  });
  await page.reload();
  await waitForSettledSaga(page);
  await page.getByRole('button', { name: 'Show slide: Storage', exact: true }).click();
  await chooseReviewer(page, 'Alice');
  const controls = page.locator('[data-deck-viewer]:visible [data-slide-viewed-controls]');
  await controls.getByRole('checkbox', { name: 'Viewed', exact: true }).check();
  await expect(controls.locator('[data-viewed-count]')).toHaveText('Viewed 1 of 1');
  await expect(controls.locator('[data-viewed-storage-note]')).toHaveText('Storage unavailable · this page only');
  await page.reload();
  await waitForSettledSaga(page);
  await expect(controls.getByRole('checkbox', { name: 'Viewed', exact: true })).not.toBeChecked();
});


test('Viewed and review approvals never imply one another', async ({ page, saga }) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stderr).toBe(0);
  };
  run('review', 'create', '--id', 'viewed-review', '--base', 'main', '--head', 'feature/wave-one', '--title', 'Viewed review', saga.sagaRoot);
  run('add-slide', '--review', 'viewed-review', '--intent', 'explain', '--layout', 'diagram', '--title', 'Review reading', saga.sagaRoot, 'review-reading');
  await page.goto(new URL('/reviews/viewed-review', saga.baseURL).toString());
  const controls = page.locator('[data-deck-viewer]:visible [data-slide-viewed-controls]');
  const checkbox = controls.getByRole('checkbox', { name: 'Viewed', exact: true });
  await expect(checkbox).not.toBeChecked();
  await page.getByRole('button', { name: 'Approve Review reading', exact: true }).click();
  await expect.poll(() => reviewFiles(saga, /approvals\/.+\.json$/).length).toBe(1);
  await expect(checkbox).not.toBeChecked();
  await expect(controls.locator('[data-viewed-count]')).toHaveText('Viewed 0 of 1');
  await checkbox.check();
  await expect(controls.locator('[data-viewed-count]')).toHaveText('Viewed 1 of 1');
  expect(reviewFiles(saga, /approvals\/.+\.json$/)).toHaveLength(1);
  await checkbox.uncheck();
  expect(reviewFiles(saga, /approvals\/.+\.json$/)).toHaveLength(1);
  await expect(page.locator('[data-review-state="approved"]')).toHaveCount(1);
});
