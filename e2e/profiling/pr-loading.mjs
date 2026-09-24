// Read-only diagnostic against an already-running disposable fixture server.
// PR_PERF_URL=http://127.0.0.1:PORT/reviews/ID node e2e/profiling/pr-loading.mjs
import { chromium } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
const url = process.env.PR_PERF_URL;
if (!url || !['127.0.0.1', 'localhost'].includes(new URL(url).hostname)) throw new Error('PR_PERF_URL must be loopback');
const browser = await chromium.launch();
const results = { browser: browser.version(), viewport: {width:1440,height:1000}, samples: [] };
try {
  for (let sample = 0; sample < 5; sample++) {
    const context = await browser.newContext({ viewport: results.viewport });
    const page = await context.newPage();
    const cdp = await context.newCDPSession(page);
    await cdp.send('Performance.enable');
    const requests = [];
    page.on('requestfinished', r => requests.push({url:r.url(), timing:r.timing()}));
    const pageErrors = [];
    page.on('pageerror', e => pageErrors.push(String(e)));
    await page.addInitScript(() => {
      window.perfLongTasks = [];
      new PerformanceObserver(list => window.perfLongTasks.push(...list.getEntries().map(e=>({start:e.startTime,duration:e.duration})))).observe({type:'longtask',buffered:true});
    });
    if (process.env.PR_PERF_FILE) {
      const selected = new URL(url);
      selected.searchParams.set('view','code');
      selected.searchParams.set('file',process.env.PR_PERF_FILE);
      await page.goto(selected.href,{waitUntil:'domcontentloaded'});
      await page.locator('#view-code .diff-row').first().waitFor({state:'attached'});
      const firstRowMs = await page.evaluate(()=>performance.now());
      await page.locator('#view-code [data-file-diff-loaded]').waitFor({state:'attached'});
      const completeMs = await page.evaluate(()=>performance.now());
      await page.waitForLoadState('networkidle');
      if (pageErrors.length) throw new Error(pageErrors.join('\n'));
      results.samples.push({sample,firstRowMs,completeMs,
        rows:await page.locator('#view-code .diff-row').count(),
        navigation:await page.evaluate(()=>performance.getEntriesByType('navigation')[0].toJSON()), requests});
      console.error(`sample ${sample+1}: first row=${firstRowMs.toFixed(1)}ms all hunks=${completeMs.toFixed(1)}ms`);
      await context.close();
      continue;
    }
    const response = await page.goto(url, { waitUntil:'domcontentloaded' });
    if (response.status() !== 200) throw new Error(`HTTP ${response.status()}`);
    const usable = page.locator('.review-deck-slide.active .landmark-hotspot [data-open-diffs]').first();
    await usable.waitFor({state:'visible'});
    await page.frameLocator('.review-deck-slide.active iframe').locator('svg').waitFor({state:'visible'});
    const usableMs = await page.evaluate(()=>performance.now());
    await page.waitForLoadState('networkidle');
    const initial = await page.evaluate(() => ({
      navigation: performance.getEntriesByType('navigation')[0].toJSON(),
      paint: performance.getEntriesByType('paint').map(e=>e.toJSON()),
      resources: performance.getEntriesByType('resource').map(e=>e.toJSON()),
      longTasks:window.perfLongTasks, elements:document.querySelectorAll('*').length,
      templates:[...document.querySelectorAll('template[id^="review-item-"]')].map(t=>({bytes:new TextEncoder().encode(t.innerHTML).length,rows:t.content.querySelectorAll('tr').length})),
    }));
    const documentBytes = (await response.body()).length;
    const cpu = Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(x=>[x.name,x.value]));
    async function action(name, click, ready) {
      const before = await page.evaluate(()=>performance.now());
      const n = requests.length;
      await click(); await ready();
      // Two frames approximate presentation, and include locator/automation overhead.
      await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
      return {name, ms:(await page.evaluate(()=>performance.now()))-before, requests:requests.slice(n)};
    }
    const actions = [];
    actions.push(await action('item_drawer',()=>usable.click(),()=>page.locator('.diff-drawer.open .review-item-panel').waitFor({state:'visible'})));
    await page.getByRole('button',{name:'Close linked code',exact:true}).click();
    actions.push(await action('next_slide',()=>page.getByRole('button',{name:'Next slide',exact:true}).click(),()=>page.locator('.review-deck-slide.active[data-slide-title="The picture and its proof stay connected"]').waitFor({state:'visible'})));
    actions.push(await action('previous_slide',()=>page.getByRole('button',{name:'Previous slide',exact:true}).click(),()=>usable.waitFor({state:'visible'})));
    actions.push(await action('code_tab',()=>page.getByRole('tab',{name:'Code Diff',exact:true}).click(),()=>page.locator('#view-code[data-surface-state="ready"] .diff-row').first().waitFor({state:'attached'})));
    await page.waitForLoadState('networkidle');
    actions.push(await action('coverage_tab',()=>page.getByRole('tab',{name:'Coverage',exact:true}).click(),()=>page.locator('#view-manifest[data-surface-state="ready"]').waitFor({state:'visible'})));
    actions.push(await action('code_tab_repeat',()=>page.getByRole('tab',{name:'Code Diff',exact:true}).click(),()=>page.locator('#view-code[data-surface-state="ready"]').waitFor({state:'visible'})));
    actions.push(await action('deck_return',()=>page.getByRole('tab',{name:'Deck',exact:true}).click(),()=>usable.waitFor({state:'visible'})));
    await page.reload({waitUntil:'domcontentloaded'});
    await usable.waitFor({state:'visible'});
    await page.frameLocator('.review-deck-slide.active iframe').locator('svg').waitFor({state:'visible'});
    const reloadUsableMs = await page.evaluate(()=>performance.now());
    await page.waitForLoadState('networkidle');
    const reload = await page.evaluate(()=>performance.getEntriesByType('navigation')[0].toJSON());
    if (pageErrors.length) throw new Error(pageErrors.join('\n'));
    results.samples.push({sample,usableMs,documentBytes,initial,cpu,actions,reloadUsableMs,reload,requests});
    console.error(`sample ${sample+1}: usable=${usableMs.toFixed(1)}ms reload=${reloadUsableMs.toFixed(1)}ms`);
    await context.close();
  }
} finally {
  await browser.close();
  await writeFile(process.env.PR_PERF_OUTPUT || '/tmp/pr-loading-browser.json', JSON.stringify(results,null,2)+'\n');
}
