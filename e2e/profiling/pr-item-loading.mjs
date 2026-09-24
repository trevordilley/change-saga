// Read-only paired diagnostic. Both owned servers must use the same disposable
// copied Saga and exact source refs; no service is started or stopped here.
import {chromium} from '@playwright/test';
import {writeFile} from 'node:fs/promises';
const urls={eager:process.env.PR_ITEM_EAGER_URL,lazy:process.env.PR_ITEM_LAZY_URL};
if(process.env.PR_ITEM_ONLY_EAGER==='yes')delete urls.lazy;
if(!urls.eager||(!urls.lazy&&process.env.PR_ITEM_ONLY_EAGER!=='yes'))throw new Error('Set PR_ITEM_EAGER_URL and PR_ITEM_LAZY_URL');
const browser=await chromium.launch(),samples=[];
try {
 for(let index=0;index<5;index++)for(const [mode,url] of Object.entries(urls)) {
  const page=await browser.newPage({viewport:{width:1440,height:1000}});
  const requests=[];page.on('request',request=>requests.push({url:request.url(),method:request.method()}));
  const response=await page.goto(url,{waitUntil:'domcontentloaded'});
  await page.locator('.review-deck-slide.active .landmark-hotspot [data-open-diffs]').first().waitFor({state:'visible'});
  await page.frameLocator('.review-deck-slide.active iframe').locator('svg').waitFor({state:'visible'});
  const usableMs=await page.evaluate(()=>performance.now());await page.waitForLoadState('networkidle');
  const bytes=(await response.body()).length,actions=[];
  for(const kind of ['first','largest']) {
   const button=kind==='first' ? page.locator('.review-deck-slide.active .landmark-list [data-open-diffs]').first() : page.locator('.landmark-list [data-open-diffs][aria-label$="for On-slide evidence controls"]');
   const target=await button.evaluate(node=>node.closest('.review-deck-slide').dataset.slideTarget);
   await page.locator('[data-slide-thumbnail]').evaluateAll((nodes,target)=>nodes.find(n=>n.dataset.slideTarget===target).click(),target);
   const slide=page.locator('.review-deck-slide.active');
   const menu=slide.locator('.landmark-menu');if(!(await menu.evaluate(node=>node.open)))await menu.locator(':scope > summary').click();
   const from=requests.length,start=performance.now();await button.click();
   await page.locator('#review-drawer .review-line').first().waitFor({state:'visible'});
   const firstRowMs=performance.now()-start;
   if(mode==='lazy')await page.locator('.review-item-surface[data-state="ready"]').waitFor();
   const completeMs=performance.now()-start;
   const evidence=await page.locator('#review-drawer').evaluate(async node=>{
     const rows=[...node.querySelectorAll('.review-line')].map(row=>[row.className,row.textContent]);
     const data=new TextEncoder().encode(JSON.stringify(rows));const hash=[...new Uint8Array(await crypto.subtle.digest('SHA-256',data))].map(n=>n.toString(16).padStart(2,'0')).join('');
     return {rows:rows.length,hash};
   });
   actions.push({kind,firstRowMs,completeMs,...evidence,requests:requests.slice(from)});
   await page.locator('[data-close-drawer]').last().click();
  }
  samples.push({index,mode,bytes,usableMs,actions});console.log(JSON.stringify({index,mode,bytes,usableMs,actions:actions.map(({requests,...action})=>({...action,requests:requests.length}))}));await page.close();
 }
 if(urls.lazy)for(let index=0;index<5;index++) {
  const eager=samples.find(s=>s.index===index&&s.mode==='eager'),lazy=samples.find(s=>s.index===index&&s.mode==='lazy');
  for(let action=0;action<2;action++)if(eager.actions[action].hash!==lazy.actions[action].hash || eager.actions[action].rows!==lazy.actions[action].rows)throw new Error('Exact displayed rows differ');
 }
 await writeFile(process.env.PR_ITEM_OUTPUT||'/tmp/pr-item-loading.json',JSON.stringify({urls,samples},null,2)+'\n');
}finally{await browser.close();}
