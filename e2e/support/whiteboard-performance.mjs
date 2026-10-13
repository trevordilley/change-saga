// Read-only browser workload against an already-running whiteboard preview.
// No fixture creation, server restart, publication, or application writes.
import {createRequire} from 'node:module';
import {mkdir, writeFile} from 'node:fs/promises';
import {resolve, dirname} from 'node:path';
import {createHash} from 'node:crypto';
import {readFileSync} from 'node:fs';

const require = createRequire(process.env.WHITEBOARD_PLAYWRIGHT_PACKAGE || new URL('../package.json', import.meta.url));
const {chromium} = require('@playwright/test');
const url = process.env.WHITEBOARD_PERF_URL || 'http://127.0.0.1:63540/technical/erd';
const output = resolve(process.env.WHITEBOARD_PERF_OUTPUT || '/tmp/whiteboard-performance');
const runs = Number(process.env.WHITEBOARD_PERF_RUNS || 3);
if (!Number.isInteger(runs) || runs < 1 || runs > 20) throw new Error('WHITEBOARD_PERF_RUNS must be 1–20');
const viewport = {width:1440, height:1000};
const focusPaths = ['local-cloud-overview', 'local-capture-delivery', 'cloud-source-lineage', 'cloud-corpus', 'cloud-project-authority', 'system-return'];
await mkdir(dirname(output), {recursive:true});
const browser = await chromium.launch({headless:true});
const result = {version:1, url, viewport, runs, timestamp:new Date().toISOString(), browser:browser.version(),
  node:process.version, platform:process.platform, architecture:process.arch,
  harnessSha256:createHash('sha256').update(readFileSync(new URL(import.meta.url))).digest('hex'),
  workload:{pan:'two 240×100 px opposing drags, 60 mousemove steps each', zoom:'24 Control+wheel events: 12 × -80 then 12 × +80, 16ms between events', focus:focusPaths, settleMs:250},
  instrumentation:'Same prototype counters, RAF observer and CDP trace in every run; no CPU throttling. Counts include automation geometry reads. Trace durations may nest; do not sum categories.', samples:[]};

function stats(values) {
  const v = [...values].sort((a,b)=>a-b);
  const percentile = p => v.length ? v[Math.min(v.length-1,Math.floor(v.length*p))] : null;
  return {count:v.length, median:percentile(.5), p95:percentile(.95), max:v.at(-1) ?? null,
    over25ms:v.filter(x=>x>25).length, over50ms:v.filter(x=>x>50).length};
}
const metricNames = ['ScriptDuration','LayoutDuration','RecalcStyleDuration','TaskDuration','LayoutCount','RecalcStyleCount'];
async function metrics(cdp) { return Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(m=>[m.name,m.value])); }
function delta(before, after) { return Object.fromEntries(metricNames.map(n=>[n,after[n]-before[n]])); }
async function stopTrace(cdp) {
  const complete = new Promise(resolve=>cdp.once('Tracing.tracingComplete', resolve));
  await cdp.send('Tracing.end');
  const {stream} = await complete;
  let raw = '';
  while (true) { const chunk = await cdp.send('IO.read',{handle:stream}); raw += chunk.base64Encoded ? Buffer.from(chunk.data,'base64').toString() : chunk.data; if(chunk.eof) break; }
  await cdp.send('IO.close',{handle:stream});
  return raw;
}
function traceSummary(raw) {
  const names = new Set(['Paint','PrePaint','Layout','UpdateLayoutTree','FunctionCall','EventDispatch','FireAnimationFrame','RunTask','RasterTask']);
  const summary = {};
  for (const event of JSON.parse(raw).traceEvents) {
    if (event.ph !== 'X' || !names.has(event.name)) continue;
    const s = summary[event.name] ||= {count:0,totalMs:0,maxMs:0};
    s.count++; s.totalMs += (event.dur || 0)/1000; s.maxMs=Math.max(s.maxMs,(event.dur || 0)/1000);
  }
  return summary;
}
try {
  for(let run=1;run<=runs;run++) {
    const context=await browser.newContext({viewport,deviceScaleFactor:1,reducedMotion:'no-preference'});
    const page=await context.newPage();
    const errors=[];page.on('pageerror',e=>errors.push(String(e)));
    const response=await page.goto(url,{waitUntil:'networkidle'});
    if (!response?.ok()) throw new Error('Preview failed: '+response?.status());
    await page.locator('[data-whiteboard-toolbar]').waitFor({state:'visible'});
    await page.waitForFunction(()=>document.querySelectorAll('[data-frame-path]').length===21);
    await page.waitForTimeout(500);
    const initial=await page.evaluate(()=>({viewBox:document.querySelector('[data-whiteboard-viewport] > svg').getAttribute('viewBox'),
      frameCount:document.querySelectorAll('[data-frame-path]').length,nodeCount:document.querySelectorAll('[data-whiteboard-view] *').length,
      title:document.title, fields:document.querySelector('[data-whiteboard-fields]')?.getAttribute('aria-pressed')}));
    const bodyHash=createHash('sha256').update(await response.body()).digest('hex');
    const assetHashes=[];
    for (const src of await page.locator('script[src]').evaluateAll(nodes=>nodes.map(n=>n.src))) {
      const asset=await context.request.get(src);assetHashes.push({url:src,sha256:createHash('sha256').update(await asset.body()).digest('hex')});
    }
    await page.evaluate(()=>{
      const counters={}; let active=false, last=null, frames=[], longTasks=[];
      const count=name=>{if(active)counters[name]=(counters[name]||0)+1;};
      for(const [proto,name] of [[Element.prototype,'getBoundingClientRect'],[Element.prototype,'getClientRects'],[SVGGraphicsElement.prototype,'getBBox'],[SVGGraphicsElement.prototype,'getCTM'],[SVGGraphicsElement.prototype,'getScreenCTM'],[Element.prototype,'setAttribute'],[Element.prototype,'removeAttribute'],[CSSStyleDeclaration.prototype,'setProperty']]) {
        const original=proto[name];if(!original)continue;
        proto[name]=function(...args){count(name);return original.apply(this,args);};
      }
      for(const [proto,name] of [[Element.prototype,'clientWidth'],[Element.prototype,'clientHeight'],[HTMLElement.prototype,'offsetWidth'],[HTMLElement.prototype,'offsetHeight']]) {
        const descriptor=Object.getOwnPropertyDescriptor(proto,name);
        if(descriptor?.get && descriptor.configurable)Object.defineProperty(proto,name,{...descriptor,get(){count(name);return descriptor.get.call(this);}});
      }
      const observer=new MutationObserver(records=>{if(active) for(const r of records)count('mutation:'+ (r.type==='attributes'?r.attributeName:r.type));});
      observer.observe(document.querySelector('[data-whiteboard-view]'),{subtree:true,attributes:true,childList:true,characterData:true});
      const longObserver=new PerformanceObserver(list=>{if(active)longTasks.push(...list.getEntries().map(e=>({start:e.startTime,duration:e.duration})));});
      longObserver.observe({type:'longtask'});
      function frame(time){if(active){if(last!==null)frames.push(time-last);last=time;}requestAnimationFrame(frame);}requestAnimationFrame(frame);
      window.__whiteboardPerf={start(name){for(const key in counters)delete counters[key];frames=[];longTasks=[];last=null;active=true;performance.mark('whiteboard-perf:'+name+':start');},stop(name){active=false;performance.mark('whiteboard-perf:'+name+':end');return {counters:{...counters},frames:[...frames],longTasks:[...longTasks]};}};
    });
    const cdp=await context.newCDPSession(page);await cdp.send('Performance.enable');
    await cdp.send('Tracing.start',{categories:'devtools.timeline,blink.user_timing,disabled-by-default-devtools.timeline',transferMode:'ReturnAsStream'});
    const phases=[];
    const canvas=page.locator('[data-whiteboard-viewport]');
    const rect=await canvas.boundingBox();
    const point={x:rect.x+rect.width*.55,y:rect.y+rect.height*.60};
    const runBefore=await metrics(cdp);
    async function phase(name,action) {
      const before=await metrics(cdp);
      await page.evaluate(name=>window.__whiteboardPerf.start(name),name);
      console.log(`run ${run}: ${name} started`);
      const started=performance.now();await action();await page.waitForTimeout(250);
      const observed=await page.evaluate(name=>window.__whiteboardPerf.stop(name),name);
      const elapsedMs=performance.now()-started;
      const after=await metrics(cdp);
      console.log(`run ${run}: ${name} completed in ${elapsedMs.toFixed(1)}ms`);
      phases.push({name,elapsedMs,metrics:delta(before,after),frameIntervals:stats(observed.frames),...observed,
        viewBox:await page.locator('[data-whiteboard-viewport] > svg').getAttribute('viewBox')});
    }
    await phase('pan',async()=>{
      await page.mouse.move(point.x,point.y);await page.mouse.down();
      await page.mouse.move(point.x+240,point.y+100,{steps:60});await page.mouse.up();
      await page.mouse.down();await page.mouse.move(point.x,point.y,{steps:60});await page.mouse.up();
    });
    await phase('zoom',async()=>{
      await page.mouse.move(point.x,point.y);await page.keyboard.down('Control');
      for(const delta of [...Array(12).fill(-80),...Array(12).fill(80)]){await page.mouse.wheel(0,delta);await page.waitForTimeout(16);}
      await page.keyboard.up('Control');
    });
    await phase('focus',async()=>{
      for(const path of focusPaths){await page.locator('[data-whiteboard-focus]').selectOption(path);await page.waitForTimeout(400);}
    });
    const runAfter=await metrics(cdp);
    const trace=await stopTrace(cdp);
    await writeFile(output+`-run${run}.trace.json`,trace);
    const sample={run,initial,bodyHash,assetHashes,metrics:delta(runBefore,runAfter),phases,trace:traceSummary(trace),errors};
    result.samples.push(sample);
    await page.screenshot({path:output+`-run${run}.png`});
    await writeFile(output+'.json',JSON.stringify(result,null,2)+'\n');
    console.log(JSON.stringify({run,metrics:sample.metrics,phases:phases.map(p=>({name:p.name,frames:p.frameIntervals,counters:p.counters})),trace:sample.trace}));
    if(errors.length)throw new Error('Browser error: '+errors.join('; '));
    await context.close();
  }
} finally {await browser.close();}
console.log('Saved '+output+'.json');
