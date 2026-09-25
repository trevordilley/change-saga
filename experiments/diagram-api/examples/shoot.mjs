// Standalone SVG screenshot QA for arbitrary SVG files, not reviewer-surface QA.
// Usage: node examples/shoot.mjs OUT_DIR file.svg... (writes NAME-1280.png, NAME-1024.png, shoot-qa.json)
import { chromium } from 'playwright';
import { readFile, writeFile } from 'node:fs/promises';
import { basename, join, resolve } from 'node:path';
const [out,...files]=process.argv.slice(2).map(p=>resolve(p));
const browser=await chromium.launch({headless:true});
const report={browser:browser.version(),network:'all requests blocked',views:[]};
try {
 for(const file of files){
  const svg=await readFile(file,'utf8');const name=basename(file,'.svg');
  for(const [width,height] of [[1280,720],[1024,576]]){
   const page=await browser.newPage({viewport:{width,height},deviceScaleFactor:1});
   await page.route('**/*',r=>r.abort());
   await page.setContent(`<style>html,body{margin:0;width:100%;height:100%}body>svg{display:block;width:100%;height:100%}</style>${svg}`);
   await page.evaluate(()=>document.fonts.ready);
   const metrics=await page.evaluate(()=>{
    const root=document.querySelector('body>svg');const ids=[...root.querySelectorAll('[id]')].map(n=>n.id);const findings=[];
    for(const e of root.querySelectorAll('[data-item-id]')){
     const b=e.getBoundingClientRect();
     if(b.x < -1||b.y < -1||b.right>innerWidth+1||b.bottom>innerHeight+1)findings.push({id:e.id,kind:'clipped'});
     if((b.width===0||b.height===0)&&!['edge','graphic','group'].includes(e.dataset.kind))findings.push({id:e.id,kind:'empty'});
    }
    return {duplicate_ids:ids.filter((v,i)=>ids.indexOf(v)!==i),findings,font_ready:document.fonts.check('22px DraftGo')};
   });
   await page.screenshot({path:join(out,`${name}-${width}.png`)});
   report.views.push({name,width,height,...metrics});await page.close();
  }
 }
 await writeFile(join(out,'shoot-qa.json'),JSON.stringify(report,null,2)+'\n');
 console.log(JSON.stringify({views:report.views.length,findings:report.views.flatMap(v=>v.findings.map(f=>({...f,view:v.name,width:v.width}))),font_ready:report.views.every(v=>v.font_ready)}));
} finally {await browser.close()}
