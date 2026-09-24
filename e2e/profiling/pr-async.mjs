// Diagnostic against an explicitly supplied disposable fixture's owned server.
// This script performs review writes. Never point it at a real review.
import {chromium} from '@playwright/test';
import {writeFile} from 'node:fs/promises';
const url=process.env.PR_ASYNC_URL;
if (!url || process.env.PR_ASYNC_DISPOSABLE !== 'yes') throw new Error('Set PR_ASYNC_URL and PR_ASYNC_DISPOSABLE=yes only for a disposable copied fixture.');
const browser=await chromium.launch();
const page=await browser.newPage({viewport:{width:1440,height:1000}});
const samples=[],requests=[],navigations=[];
try {
  await page.goto(url);await page.waitForLoadState('networkidle');
  page.on('request',request=>requests.push({method:request.method(),url:request.url(),type:request.resourceType()}));
  page.on('framenavigated',frame=>navigations.push(frame.url()));
  const slide=page.locator('.review-deck-slide.active');
  async function action(kind,submit,ready) {
    const start=performance.now(),from=requests.length;
    const received=page.waitForResponse(r=>r.request().method()==='POST' && new URL(r.url()).pathname.startsWith(new URL(url).pathname+'/'));
    await submit();const response=await received;const receipt=await response.json();const receiptMs=performance.now()-start;
    if(!response.ok() || !receipt.saved || !receipt.event_id) throw new Error('Unconfirmed save');
    await ready();const projectedMs=performance.now()-start;
    await page.waitForLoadState('networkidle');
    samples.push({kind,receiptMs,projectedMs,bytes:(await response.body()).length,requests:requests.slice(from)});
  }
  for(let index=0;index<5;index++) {
    await action('decision',()=>slide.locator('[data-review-approve]').click(),()=>page.waitForFunction(()=>!document.querySelector('form[data-saving]')));
  }
  const discussion=slide.locator('.review-slide-comment');
  await discussion.locator(':scope > summary').click();
  const form=discussion.locator('[data-review-comment-form]');
  for(let index=0;index<5;index++) {
    await form.locator('xpath=preceding-sibling::summary').click();
    const body='Disposable async latency sample '+index;
    await form.locator('textarea').fill(body);
    await action('comment',()=>form.locator('button').click(),()=>discussion.getByText(body,{exact:true}).waitFor({state:'visible'}));
  }
  await discussion.locator(':scope > summary').click();
  await slide.locator('[data-review-annotation-toggle]').click();
  const toolbar=page.getByRole('toolbar',{name:'Annotation tools'}),layer=slide.locator('.review-annotation-layer');
  for(let index=0;index<5;index++) {
    await page.waitForFunction(()=>[...document.querySelectorAll('.review-annotation-status')].every(node=>node.hidden));
    await toolbar.getByRole('button',{name:'Sticky note',exact:true}).click();
    const point=await layer.evaluate(node=>{ const box=node.getBoundingClientRect(); for(let y=.04;y<.96;y+=.07) for(let x=.04;x<.96;x+=.07) { const point={x:box.x+box.width*x,y:box.y+box.height*y}; if(document.elementFromPoint(point.x,point.y)===node)return point; } return null; });
    if(!point) throw new Error('No unobscured annotation position');
    await page.mouse.click(point.x,point.y);
    const composer=page.locator('.review-annotation-compose');
    await composer.locator('textarea').fill('Disposable annotation latency sample '+index);
    await action('annotation',()=>composer.getByRole('button',{name:'Save annotation'}).click(),()=>composer.waitFor({state:'hidden'}));
  }
  const fullGets=requests.filter(r=>r.method==='GET' && (new URL(r.url).pathname===new URL(url).pathname || new URL(r.url).pathname.includes('/visual/')));
  if(navigations.length || fullGets.length) throw new Error('Save navigated or reloaded a visual');
  await writeFile(process.env.PR_ASYNC_OUTPUT||'/tmp/pr-async.json',JSON.stringify({url,samples,navigations,fullGets},null,2)+'\n');
  for(const kind of ['decision','comment','annotation']) {
    const selected=samples.filter(s=>s.kind===kind),sorted=selected.map(s=>s.receiptMs).sort((a,b)=>a-b),projected=selected.map(s=>s.projectedMs).sort((a,b)=>a-b);
    console.log(JSON.stringify({kind,medianReceiptMs:sorted[2],medianProjectedMs:projected[2],rangeMs:[sorted[0],sorted[4]],bytes:selected.map(s=>s.bytes),requests:selected.map(s=>s.requests.length)}));
  }
} finally {await browser.close();}
