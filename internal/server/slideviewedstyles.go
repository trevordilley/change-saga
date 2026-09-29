package server

const slideViewedStyles = `
/* Reading state is always an explicit control, never a faded content surface. */
.deck-viewer{position:relative}
.slide-viewed-controls{display:flex;align-items:center;flex-wrap:wrap;gap:8px 12px;pointer-events:auto;color:var(--ink);font:12px var(--ui);text-shadow:none}
.deck-viewer>.slide-viewed-controls{position:absolute;z-index:9;left:12px;bottom:12px;max-width:calc(100% - 24px);padding:6px 8px;background:var(--bg);border:1px solid var(--line)}
.slide-viewed-toggle{display:inline-flex;align-items:center;gap:4px;white-space:nowrap;cursor:pointer}
.slide-viewed-toggle input{margin:0;accent-color:var(--accent)}
.slide-viewed-controls small{color:var(--muted);font-size:11px}
.slide-viewed-identity{position:relative;max-width:100%}
.slide-viewed-identity summary{cursor:pointer;overflow-wrap:anywhere}
.slide-viewed-identity form{position:absolute;z-index:20;bottom:calc(100% + 8px);left:0;width:270px;max-width:calc(100vw - 40px);padding:12px;background:var(--bg);border:1px solid var(--line);box-shadow:var(--shadow)}
[data-slide-viewed-host] .slide-viewed-identity form{bottom:auto;top:calc(100% + 8px)}
.slide-viewed-identity label{display:grid;gap:6px}
.slide-viewed-identity input{min-width:0;width:100%;box-sizing:border-box;font:inherit;background:var(--bg);color:var(--ink);border:1px solid var(--line);padding:5px}
.slide-viewed-identity button{margin-top:8px;font:inherit;cursor:pointer}
.slide-viewed-identity p{font-size:11px;color:var(--muted);margin:8px 0 0}
.slide-viewed-badge{font:10px var(--ui);color:var(--muted);margin-left:auto;white-space:nowrap}
body.presentation-mode .slide-viewed-controls{display:none}
@media(max-width:780px){
/* Reserve two chrome rows above the visual instead of wrapping the Viewed
   count over the slide's marked-places and evidence controls. */
.deck-viewer-stage{box-sizing:content-box;padding-top:76px}
.deck-viewer-header{flex-wrap:wrap;justify-content:flex-start;gap:4px 10px}
.deck-viewer-header [data-slide-viewed-host]{flex-basis:100%}
.slide-viewed-controls{gap:6px;flex-wrap:nowrap;font-size:11px;min-width:0;max-width:100%}
.slide-viewed-identity{min-width:0;max-width:100px}
.slide-viewed-identity summary,.slide-viewed-controls small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.slide-viewed-controls small{min-width:0}
.slide-viewed-controls [data-viewed-count]{flex:none}
.deck-viewer-slide,.deck-viewer-controls{top:76px}
.deck-overview{top:76px;padding-top:16px}
.slide-front{padding-top:24px}
body.presentation-mode .deck-viewer-stage{padding-top:0}
body.presentation-mode .deck-viewer-slide,body.presentation-mode .deck-viewer-controls{top:0}
}
`
