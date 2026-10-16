import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { test, expect, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from '../support/test.js';
import { runCLI } from '../support/fixture-builder.js';

test('deprecations stay compact, reveal on hover or focus, and open through page and Saga lists', async ({page, saga}, testInfo) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const path = join(saga.root, 'deprecation-board.json');
  writeFileSync(path, JSON.stringify({version:1, width:1200, height:800, elements:[
    {id:'legacy',kind:'node',shape:'rect',label:'Legacy sharing',x:200,y:200,width:300,height:180,style:'normal'},
    {id:'current',kind:'node',shape:'rect',label:'Shared projects',x:650,y:200,width:300,height:180,style:'normal'}
  ]}));
  run('whiteboard','publish','--id','data-model','--name','Sharing model','--revision','r1','--from',path,saga.sagaRoot);
  const target = 'urn:change-saga:wave-one:whiteboard:data-model';
  run('deprecation','add','--id','legacy-sharing','--scope','content','--target',target,'--element','legacy','--reason','Use Shared Projects for new sharing. Historical records remain readable.',saga.sagaRoot);
  run('deprecation','add','--id','board-context','--scope','code','--target',target,'--ref','HEAD:src/app.go#L1','--repo',saga.sourceRepo,'--reason','The legacy sharing entry point is retained only for compatibility.',saga.sagaRoot);
  const detailRequests: string[] = [];
  page.on('request', request => { const url = new URL(request.url()); if (url.pathname === '/api/deprecations' && url.searchParams.has('deprecation')) detailRequests.push(url.href); });
  await page.emulateMedia({colorScheme:'dark'});
  await page.goto(saga.baseURL + '/technical/erd');
  await waitForSettledSaga(page);
  const board = page.locator('[data-whiteboard-view]');
  const badge = board.getByRole('button', {name:'Deprecations on this content'});
  await expect(badge).toHaveText('Deprecations · 2');
  await expect(board.locator('[data-deprecations]')).toHaveCount(0);
  const size = await badge.boundingBox(); expect(size!.height).toBeLessThan(40);
  expect(detailRequests).toHaveLength(0);
  await badge.hover(); await expect(page.getByRole('tooltip')).toContainText('2 recorded deprecations');
  await page.keyboard.press('Escape'); await expect(page.getByRole('tooltip')).toHaveCount(0);
  const marker = board.locator('[data-deprecation-marker]');
  await expect(marker).toHaveCount(1); await expect(marker).toBeVisible();
  await marker.focus(); await expect(page.getByRole('tooltip')).toContainText('Use Shared Projects');
  expect(detailRequests).toHaveLength(0);
  await page.screenshot({path:testInfo.outputPath('compact-deprecations.png')});
  await expectNoSeriousAccessibilityViolations(page, '[data-whiteboard-view]');
  await page.keyboard.press('Enter');
  const drawer = page.locator('.diff-drawer');
  await expect(drawer).toContainText('Use Shared Projects');
  await expect(drawer).toContainText('Content scope');
  await expect(drawer).toContainText('Reading saved revision r1');
  await page.keyboard.press('Escape'); await expect(drawer).not.toHaveClass(/open/);
  await expect(marker).toBeFocused();
  await badge.click(); await expect(drawer.locator('li')).toHaveCount(2);
  await drawer.getByRole('link', {name:'Show legacy', exact:true}).click();
  await expect(page).toHaveURL(/focus=legacy/);
  await expect(drawer).not.toHaveClass(/open/);
  await badge.click(); await drawer.getByRole('link', {name:'Browse all deprecations'}).click();
  await expect(page).toHaveURL(saga.baseURL + '/deprecations');
  await expect(page.getByRole('heading', {name:'Deprecations', exact:true})).toBeVisible();
  await expect(page.locator('[data-deprecation-result]')).toHaveCount(2);
  await page.getByLabel('Scope', {exact:true}).selectOption('content'); await page.getByRole('button', {name:'Filter', exact:true}).click();
  await expect(page.locator('[data-deprecation-result]')).toHaveCount(1);
  await expect(page.locator('[data-deprecation-result]')).toHaveAttribute('data-deprecation-result','legacy-sharing');
  await page.getByLabel('Search', {exact:true}).fill('not found'); await page.getByRole('button', {name:'Filter', exact:true}).click();
  await expect(page.getByText('No recorded deprecations match these filters.')).toBeVisible();
  await expectNoSeriousAccessibilityViolations(page, '.deprecation-directory');
  // Multiple assertions at one appearance share a marker; history never gets
  // silently applied to a different revision's geometry.
  run('deprecation','add','--id','second-warning','--scope','content','--target',target,'--element','legacy','--reason','Also replace the import path.',saga.sagaRoot);
  await page.goto(saga.baseURL + '/technical/erd');
  await expect(badge).toHaveText('Deprecations · 3');
  await expect(marker).toHaveCount(1); await expect(marker).toHaveText('2');
  await marker.click(); await expect(drawer.locator('li')).toHaveCount(2);
  await page.keyboard.press('Escape');
  run('whiteboard','publish','--id','data-model','--name','Sharing model','--revision','r2','--parent','r1','--from',path,saga.sagaRoot);
  await page.reload(); await expect(badge).toHaveText('Deprecations · 3');
  await expect(marker).toHaveCount(0);
  await badge.click(); await drawer.getByRole('button', {name:'Use Shared Projects for new sharing. Historical records remain readable.',exact:true}).click();
  await expect(drawer.locator('[data-target-health]')).toHaveAttribute('data-target-health','stale');

});

test('a closed deprecation drawer stays closed when its response arrives late', async ({page, saga}) => {
  const result = runCLI(saga, ['deprecation','add','--id','old-essay','--scope','content','--target','urn:change-saga:wave-one:fragment:wave-one-overview','--reason','Superseded essay.',saga.sagaRoot], saga.sagaRepo);
  expect(result.status, result.stdout + result.stderr).toBe(0);
  await page.goto(saga.featureURL);
  await page.getByRole('link', {name:'Deprecations · 1',exact:true}).click();
  await expect(page).toHaveURL(/deprecations\?page=/);
  let release!: () => void;
  const gate = new Promise<void>(resolve => {release = resolve;});
  await page.route('**/api/deprecations?deprecation=*', async route => { await gate; await route.continue(); });
  await page.getByRole('button', {name:'Superseded essay.', exact:true}).click();
  await expect(page.locator('.diff-drawer')).toHaveClass(/open/);
  await page.keyboard.press('Escape'); release();
  await page.waitForLoadState('networkidle');
  await expect(page.locator('.diff-drawer')).not.toHaveClass(/open/);
});
