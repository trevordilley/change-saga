package server

// slideThemeStyles lets slide visuals follow the app theme. A frame or image
// inherits the page's color-scheme, which some browsers pass to the frame's
// prefers-color-scheme; slideSchemeBoot covers the rest through the URL. A
// visual with fixed colours stays in the light scheme and sits on the paper
// card, whose colour and inset are tokens that only show in dark mode.
const slideThemeStyles = `
/* Slides on tokens ------------------------------------------------ */
[data-slide-paper]{color-scheme:light;background:var(--slide-paper);padding:var(--slide-paper-inset);border-radius:var(--radius)}
.related-slide-preview [data-slide-paper],.manifest-slide-preview [data-slide-paper],.manifest-target-slide-preview [data-slide-paper]{padding:calc(var(--slide-paper-inset)/4);border-radius:3px}
`

// slideSchemeBoot keeps every themed slide visual's URL naming the scheme the
// reviewer pinned, when it differs from the OS preference the visual would
// otherwise follow; see slideScheme. It runs from the head, so frames the
// parser inserts are pointed at the right scheme before they load, and it
// follows the theme toggle, the OS preference, and swapped-in content.
// It follows themeBoot in theme.js, so it opens with a semicolon.
const slideSchemeBoot = `;
(()=>{const r=document.documentElement,os=matchMedia('(prefers-color-scheme:dark)'),param='` + slideSchemeParam + `';
const wanted=()=>{const t=r.dataset.theme;return t&&t!==(os.matches?'dark':'light')?t:''};
const point=(value,scheme)=>{const url=new URL(value,location.href);if((url.searchParams.get(param)||'')===scheme)return value;
if(scheme)url.searchParams.set(param,scheme);else url.searchParams.delete(param);return url.origin===location.origin?url.pathname+url.search+url.hash:url.href};
const sync=el=>{if(el.hasAttribute('data-slide-paper'))return;const scheme=wanted();
for(const name of ['src','data-frame-src']){const value=el.getAttribute(name);if(value){const next=point(value,scheme);if(next!==value)el.setAttribute(name,next)}}
if(el.dataset.landmarkBase)el.dataset.landmarkBase=point(el.dataset.landmarkBase,scheme)};
const all=()=>document.querySelectorAll('[data-slide-visual]').forEach(sync);
new MutationObserver(records=>{for(const record of records)for(const node of record.addedNodes){if(node.nodeType!==1)continue;
if(node.matches('[data-slide-visual]'))sync(node);node.querySelectorAll('[data-slide-visual]').forEach(sync)}}).observe(r,{childList:true,subtree:true});
document.addEventListener('click',e=>{if(e.target.closest?.('[data-theme-toggle]'))all()});
os.addEventListener('change',all)})()`
