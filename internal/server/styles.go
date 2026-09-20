package server

// darkTokens recolours the design tokens for dark mode. Only the palette
// changes: every rule in pageStyles already reads through these variables, so
// spacing, type and layout stay shared with light mode and cannot drift apart.
// The values track GitHub's dark palette because the light tokens above track
// its light one, which keeps text, diff and status colours at AA contrast.
const darkTokens = `
--bg:#0d1117;--bg-subtle:#161b22;--bg-inset:#1c2128;--ink:#e6edf3;--muted:#9198a1;--faint:#8b949e;
--line:#30363d;--line-soft:#21262d;--accent:#4493f8;--accent-soft:#121d2f;--accent-line:#316dca;
--green:#3fb950;--red:#f85149;--amber:#d29922;--sel:#132132;
--add-bg:#12261e;--add-line:#3fb950;--del-bg:#25171c;--del-line:#f85149;--code-bg:#0d1117;--code-gutter:#161b22;
--warning-bg:#2d240c;--warning-line:#9e6a03;--danger-bg:#25171c;--danger-line:#f85149;
--frosted-bg:#0d1117ed;--toolbar-bg:#0d1117ee;--accent-hover-bg:#1c2d41;--accent-hover-ink:#79c0ff;
--landmark-bg:#1c2733;--footnote-hover-bg:#1c2d41;--citation-bg:#121d2f;--landmark-affordance-bg:#161b22f2;
--button-hover:#30363d;--primary-bg:#1f6feb;--primary-hover:#1158c7;--primary-ink:#fff;
--active-fragment-line:#316dca;
--add-gutter:#142c22;--del-gutter:#321c22;--copied-bg:#30363d;--copied-ink:#e6edf3;
--syntax-keyword:#ff7b72;--syntax-string:#a5d6ff;--syntax-number:#79c0ff;--syntax-comment:#8b949e;
--syntax-type:#ffa657;--syntax-property:#d2a8ff;--syntax-punctuation:#c9d1d9;
--shadow:0 6px 24px #01040966,0 1px 3px #010409aa;color-scheme:dark
`

// pageStyles is the whole renderer stylesheet. The design target is a quiet
// developer tool: system UI type for chrome, monospace for code and code-shaped
// metadata, hairline separators instead of cards, and controls that stay
// invisible until the reviewer hovers or focuses the thing they belong to.
const pageStyles = `
:root{
--bg:#ffffff;--bg-subtle:#f6f8fa;--bg-inset:#eef1f4;--ink:#1f2328;--muted:#59636e;--faint:#59636e;
--line:#d1d9e0;--line-soft:#e7ebef;--accent:#0969da;--accent-soft:#ddf4ff;--accent-line:#54aeff;
--green:#116329;--red:#a40e26;--amber:#9a6700;--sel:#eaf3fe;
--add-bg:#e6ffec;--add-line:#2da44e;--del-bg:#ffebe9;--del-line:#cf222e;--code-bg:#ffffff;--code-gutter:#f6f8fa;
--warning-bg:#fff8e6;--warning-line:#e0c98a;--danger-bg:#fff5f4;--danger-line:#e5b3ae;
--frosted-bg:#ffffffed;--toolbar-bg:#ffffffee;--accent-hover-bg:#cfeaff;--accent-hover-ink:#0550ae;
--landmark-bg:#f5f9ff;--footnote-hover-bg:#dbeafe;--citation-bg:#e8f2ff;--landmark-affordance-bg:#fffffff2;
--button-hover:#e2e6ea;--primary-bg:#0969da;--primary-hover:#0860c4;--primary-ink:#fff;
--active-fragment-line:#c8e1ff;
--add-gutter:#d9f6e0;--del-gutter:#ffdcd8;--copied-bg:#1f2328;--copied-ink:#fff;
--syntax-keyword:#cf222e;--syntax-string:#0a3069;--syntax-number:#0550ae;--syntax-comment:#6e7781;
--syntax-type:#953800;--syntax-property:#8250df;--syntax-punctuation:#57606a;
--ui:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,"Helvetica Neue",Arial,sans-serif;
--mono:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,"Liberation Mono",monospace;
--top:44px;--shadow:0 6px 24px #1f232814,0 1px 3px #1f23281f;--radius:6px;color-scheme:light
}
/* Dark mode. The OS preference decides unless the reviewer has pinned a theme,
   which is why the media rule excuses an explicit light choice. ------------ */
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){` + darkTokens + `}}
:root[data-theme=dark]{` + darkTokens + `}
*{box-sizing:border-box}
html{scroll-behavior:smooth}
body{margin:0;background:var(--bg);color:var(--ink);font:13px/1.55 var(--ui);-webkit-font-smoothing:antialiased}
button,input,textarea,select{font:inherit;color:inherit}
button{cursor:pointer}
a{color:var(--accent)}
:focus-visible{outline:2px solid var(--accent);outline-offset:1px;border-radius:3px}
.icon-sprite{display:block}
.i{width:16px;height:16px;flex:none;display:block}
.ficon{width:14px;height:14px;flex:none}

/* Deck viewer ---------------------------------------------------- */
.slide-present{margin-left:auto;border:1px solid var(--line);border-radius:6px;height:30px;align-self:center;padding-inline:12px}
.slide-thumbnail-list{display:grid;grid-template-columns:minmax(0,1fr);min-width:0;gap:10px}
.slide-section-divider{display:flex;align-items:center;gap:7px;min-width:0;margin:3px 0 -2px 22px;color:var(--faint);font:600 9px/1.2 var(--ui);letter-spacing:.045em;text-transform:uppercase}
.slide-section-divider::after{content:'';height:1px;min-width:12px;flex:1;background:var(--line)}
.slide-thumbnail-card{position:relative;min-width:0;counter-increment:slide-thumbnail;padding-left:22px;color:var(--muted)}
.slide-thumbnail-card::before{content:counter(slide-thumbnail);position:absolute;left:0;top:4px;width:17px;text-align:right;color:var(--faint);font:10px/1 var(--mono)}
.slide-thumbnail-preview{position:relative;width:100%;aspect-ratio:16/9;overflow:hidden;border:2px solid var(--line);border-radius:5px;background:#fff;box-shadow:0 1px 2px #1f23281f;transition:border-color .12s,box-shadow .12s}
.slide-thumbnail-preview iframe,.slide-thumbnail-preview img{display:block;width:100%;height:100%;border:0;object-fit:contain;pointer-events:none}
.slide-thumbnail-caption{display:flex;align-items:center;gap:5px;margin-top:4px}
.slide-thumbnail-title{display:block;min-width:0;flex:1;color:inherit;font:11.5px/1.3 var(--ui);overflow-wrap:anywhere}
.slide-thumbnail-hit{position:absolute;z-index:2;inset:0;width:100%;padding:0;border:0;border-radius:5px;background:transparent}
.slide-thumbnail-hit:hover,.slide-thumbnail-hit:active{background:transparent}
.slide-thumbnail-hit:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.slide-thumbnail-card:hover .slide-thumbnail-preview{border-color:var(--faint)}
.slide-thumbnail-card.active{color:var(--ink);font-weight:600}
.slide-thumbnail-card.active .slide-thumbnail-preview{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
.deck-viewer{display:grid;place-items:center;width:100%;height:100%;overflow:hidden;background:#111}
.sidebar-slide-surface{width:100%;height:100%;min-height:0;background:#111}
.deck-viewer-stage{position:relative;width:min(100%,calc(177.7778vh - 78.2222px));aspect-ratio:16/9;overflow:hidden;background:var(--bg);box-shadow:var(--shadow)}
.deck-viewer-slide{position:absolute;z-index:1;inset:0;overflow:hidden;background:var(--bg)}
.deck-viewer-slide[hidden]{display:none}.deck-viewer-slide.active{display:block}
.deck-viewer-slide .fragment{margin:0;border:0;border-radius:0;height:100%;min-height:100%;background:transparent}
.deck-viewer-slide .fragment-head{position:absolute;z-index:7;right:12px;top:42px;border:0;background:var(--frosted-bg);border-radius:8px}
.deck-viewer-slide .fragment-head::before{content:'Implementation slide';align-self:center;padding-left:8px;color:var(--muted);font:600 10px/1 var(--ui);letter-spacing:.025em;text-transform:uppercase}
.deck-viewer-slide[data-deck-role=onboarding] .fragment-head::before{content:'Onboarding slide'}
.deck-viewer-slide[data-deck-role=ux] .fragment-head::before{content:'UX flow slide'}
.deck-viewer-slide .fragment-stage{height:100%;min-height:100%;display:grid;place-items:center;padding:0}
.deck-viewer-slide .fragment-frame{width:100%;height:100%;min-height:0;border:0;border-radius:0}
.deck-viewer-slide .fragment-image{display:block;width:100%;height:100%;max-height:none;object-fit:contain}
.deck-viewer-header{position:absolute;z-index:6;left:12px;right:12px;top:10px;display:flex;align-items:center;justify-content:flex-end;gap:16px;pointer-events:none;color:var(--muted);text-shadow:0 1px 2px var(--bg)}
.deck-viewer-header>div{display:flex;align-items:baseline;gap:9px;min-width:0}
.deck-viewer-header strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--ink)}
.deck-viewer-header [data-slide-deck-title]{flex:none;font-size:11px;text-transform:uppercase;letter-spacing:.04em}
.deck-viewer-header [data-slide-position]{flex:none;font:11px var(--mono)}
.deck-viewer-controls{position:absolute;z-index:8;inset:0;pointer-events:none}
.slide-step{position:absolute;top:50%;display:grid;place-items:center;width:38px;height:56px;transform:translateY(-50%);border:1px solid var(--line);border-radius:8px;background:var(--frosted-bg);color:var(--ink);box-shadow:var(--shadow);font:34px/1 var(--ui);pointer-events:auto;opacity:.15;transition:opacity .14s,background .14s}
.slide-step:hover,.slide-step:focus-visible{opacity:1;background:var(--bg)}
.slide-step:disabled{visibility:hidden}.slide-step[data-slide-previous]{left:12px}.slide-step[data-slide-next]{right:12px}
.slide-exit-presentation{position:absolute;z-index:10;right:16px;bottom:16px;padding:7px 11px;border:1px solid #ffffff55;border-radius:6px;background:#111b;color:#fff;opacity:0;transition:opacity .15s}
body.presentation-mode{overflow:hidden;background:#000}
body.presentation-mode>.topbar,body.presentation-mode .manifest-view,body.presentation-mode .change-view{display:none}
body.presentation-mode .diff-drawer,body.presentation-mode .drawer-backdrop{display:none}
body.presentation-mode .deck-viewer-stage{width:min(100vw,177.7778vh);height:auto;max-height:100vh;box-shadow:none}
body.presentation-mode .deck-viewer-header,body.presentation-mode .deck-viewer-slide .fragment-head{opacity:0;pointer-events:none}
body.presentation-mode .deck-viewer-slide .landmark-hotspot{display:none}
body.presentation-mode .deck-viewer-stage:hover .slide-step,body.presentation-mode .slide-step:focus-visible{opacity:.6}
body.presentation-mode .deck-viewer-stage:hover .slide-exit-presentation,body.presentation-mode .slide-exit-presentation:focus-visible{opacity:1}
@media(max-width:780px){.deck-viewer-header{left:8px;right:8px}.deck-viewer-header strong{display:none}.slide-step{width:30px;height:46px}.slide-step[data-slide-previous]{left:6px}.slide-step[data-slide-next]{right:6px}}

/* Top bar ---------------------------------------------------------------- */
.topbar{position:sticky;top:0;z-index:30;height:var(--top);display:flex;align-items:center;gap:14px;padding:0 12px;background:var(--bg);border-bottom:1px solid var(--line)}
.brand{display:flex;align-items:center;gap:6px;color:var(--muted);font:600 11px var(--mono);letter-spacing:.04em}
.brand .i{width:14px;height:14px}
.view-tabs{display:flex;align-self:stretch;gap:2px}
.view-tab{display:flex;align-items:center;gap:6px;border:0;border-bottom:2px solid transparent;border-radius:0;padding:0 10px;background:transparent;color:var(--muted);font-size:12.5px}
.view-tab:hover{color:var(--ink);background:var(--bg-subtle)}
.view-tab.active{color:var(--ink);border-color:var(--accent);font-weight:600}
.view-tab.reviews-link.current{color:var(--ink);border-color:var(--accent);font-weight:600;text-decoration:none}
.top-meta{margin-left:auto;color:var(--faint);font:11px var(--mono)}
.top-meta[hidden]{display:none}
.theme-toggle{margin-left:8px}

/* Shell ------------------------------------------------------------------ */
.shell{display:grid;grid-template-columns:264px minmax(0,1fr);min-height:calc(100vh - var(--top))}
.shell.code-mode{grid-template-columns:280px minmax(0,1fr)}
.shell.tree-hidden{grid-template-columns:0 minmax(0,1fr)}
.sidebar{position:sticky;top:var(--top);height:calc(100vh - var(--top));overflow:auto;padding:10px 8px 40px;background:var(--bg-subtle);border-right:1px solid var(--line)}
.tree-hidden .sidebar{overflow:hidden;padding-inline:0;border:0}
.sidebar-title{display:flex;align-items:center;gap:7px;padding:6px 8px;margin-bottom:4px;color:var(--ink);text-decoration:none;font-weight:600;font-size:13px;border-radius:var(--radius)}
.sidebar-title:hover{background:var(--bg-inset)}
.sidebar-title .i{color:var(--muted)}
.side-label{margin:14px 6px 4px;color:var(--faint);font:600 11px var(--ui);letter-spacing:.02em}

/* Documentation navigation tree ------------------------------------------ */
.doc-tree{display:block}
.doc-row{display:flex;align-items:center;min-height:26px;border-radius:var(--radius)}
.doc-row:hover{background:var(--bg-inset)}
.doc-row.current{background:var(--sel)}
.doc-row.current>.doc-link{color:var(--accent);font-weight:600}
.doc-twisty{display:grid;place-items:center;width:20px;height:24px;flex:none;padding:0;border:0;background:transparent;color:var(--faint);border-radius:3px}
.doc-twisty:hover{color:var(--ink)}
.doc-twisty .i{width:13px;height:13px;transition:transform .12s ease}
.doc-twisty[aria-expanded=true] .i{transform:rotate(90deg)}
.doc-twisty.placeholder{visibility:hidden}
.doc-link{min-width:0;flex:1;padding:4px 8px 4px 0;color:var(--ink);text-decoration:none;font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.doc-deck{margin-top:3px}
.doc-deck-link{display:flex;align-items:center;gap:6px;border:0;background:transparent;text-align:left;font:inherit;cursor:pointer}
.doc-deck-link .i{width:14px;height:14px;flex:none;color:var(--muted)}
.doc-deck-link[aria-expanded=true]{font-weight:600}
.doc-link[aria-current=page]{color:var(--accent);font-weight:600}
.doc-deck>.doc-children{margin:0 0 8px;padding:7px 0 2px;border-left:0;counter-reset:slide-thumbnail}
.doc-slide-thumbnail{margin:0 0 10px;min-width:0}
.doc-slide-thumbnail .slide-thumbnail-card{padding-left:22px}
.doc-slide-thumbnail .slide-thumbnail-preview{border-width:1px;background:#fff}
.doc-slide-thumbnail .slide-thumbnail-card.active .slide-thumbnail-preview{border-width:2px}
.doc-children{margin-left:10px;border-left:1px solid var(--line);padding-left:4px}
.doc-children[hidden]{display:none}
.doc-children .doc-link{font-size:12.5px;color:var(--muted)}
.doc-children .doc-row:hover .doc-link{color:var(--ink)}
.doc-requirement>.doc-row>.doc-link{display:flex;align-items:center;gap:6px}
.doc-requirement>.doc-row>.doc-link .i{width:14px;height:14px;flex:none;color:var(--accent)}
.doc-tree>.doc-requirement{margin:5px 0;padding:3px 0;border-block:1px solid var(--line-soft)}
.doc-tree>.doc-requirement>.doc-row{min-height:32px}
.doc-tree>.doc-requirement>.doc-row>.doc-link{font-weight:650}
.doc-requirement .doc-requirement .doc-link .i{width:12px;height:12px;color:var(--faint)}
.doc-group-link{min-width:0;border:0;background:transparent;text-align:left;font:inherit;cursor:pointer}
.doc-group>.doc-row>.doc-link{display:flex;align-items:center;gap:6px}
.doc-group>.doc-row>.doc-link .i{width:14px;height:14px;flex:none;color:var(--muted)}
.doc-tree>.doc-group>.doc-row{min-height:32px}
.doc-tree>.doc-group>.doc-row>.doc-link{font-weight:650}
.doc-static{display:flex;align-items:center;gap:6px;cursor:default}
.doc-gap>.doc-row>.doc-link{color:var(--faint);font-weight:400}
.doc-gap>.doc-row>.doc-link .i{color:var(--faint)}
.doc-note{flex:none;padding:0 8px 0 4px;color:var(--faint);font-size:11px;white-space:nowrap}
.doc-row:has(>.doc-note){flex-wrap:wrap}
.doc-row:has(>.doc-note)>.doc-link{flex:0 0 auto;max-width:calc(100% - 24px)}
.doc-row:has(>.doc-note)>.doc-note{flex:1 0 auto;text-align:right}
/* Documentation titles wrap rather than truncate: a sidebar that cuts a name
   short makes the reader open it to learn what it is. */
.doc-tree .doc-link{white-space:normal;overflow-wrap:anywhere;line-height:1.35}
.doc-tree a.doc-link:has(>.i){display:flex;align-items:flex-start;gap:6px}
.doc-tree a.doc-link>.i{flex:none;width:13px;height:13px;margin-top:2px}

/* The current epic, its picker, and the full list ------------------------ */
/* The picker panel is absolutely positioned so opening it never pushes the
   epic's four places off screen; the sidebar is the scroll container, so a
   panel taller than the remaining space scrolls into view rather than being
   lost. */
.doc-epic-current{margin-top:6px;padding-top:6px;border-top:1px solid var(--line-soft)}
/* The panel hangs from the epic's row, not from the node: the node is as tall
   as the epic's four places, which would drop the panel below them. */
.doc-epic-current>.doc-row{position:relative}
.doc-epic-current>.doc-row>.doc-link{font-weight:650}
/* The epic's own title is what the row says, so the row cannot also say it is
   an epic. The eyebrow does, once, above it. */
.doc-epic-eyebrow{margin:0 0 1px 24px;color:var(--faint);font:650 9px/1.2 var(--ui);letter-spacing:.045em;text-transform:uppercase}
.epic-picker{flex:none}
.epic-picker-summary{display:grid;place-items:center;width:22px;height:22px;border-radius:3px;color:var(--faint);cursor:pointer;list-style:none}
.epic-picker-summary::-webkit-details-marker{display:none}
.epic-picker-summary:hover{background:var(--bg-inset);color:var(--ink)}
.epic-picker-summary .i{width:12px;height:12px;transform:rotate(90deg);transition:transform .12s ease}
.epic-picker[open]>.epic-picker-summary .i{transform:rotate(-90deg)}
.epic-picker[open]>.epic-picker-summary{background:var(--bg-inset);color:var(--ink)}
.epic-picker-panel{position:absolute;z-index:5;left:0;right:0;top:100%;margin-top:2px;padding:6px;border:1px solid var(--line);border-radius:var(--radius);background:var(--bg);box-shadow:var(--shadow)}
.epic-picker-search{position:relative;display:flex;align-items:center;margin-bottom:6px}
.epic-picker-search .i{position:absolute;left:7px;width:13px;height:13px;color:var(--faint);pointer-events:none}
.epic-picker-search input{min-width:0;width:100%;height:26px;padding:0 8px 0 25px;border:1px solid var(--line);border-radius:var(--radius);background:var(--bg);color:var(--ink);font-size:12px}
.epic-picker-search input::-webkit-search-cancel-button{-webkit-appearance:none}
.epic-picker-options{display:block;max-height:250px;overflow:auto}
.epic-option{display:block;padding:4px 7px;border-radius:var(--radius);color:var(--ink);text-decoration:none;font-size:12.5px}
.epic-option[hidden]{display:none}
.epic-option:hover,.epic-option.active{background:var(--bg-inset)}
.epic-option.current{color:var(--accent);font-weight:650}
.epic-option-title{display:block;overflow-wrap:anywhere}
.epic-option-id{display:block;color:var(--faint);font:10.5px var(--mono);overflow-wrap:anywhere}
.epic-picker-empty{margin:6px 7px;color:var(--muted);font-size:12px}
.epic-picker-index{display:block;margin-top:4px;padding:5px 7px;border-top:1px solid var(--line-soft);color:var(--accent);text-decoration:none;font-size:12px}
.doc-all-epics{margin-top:2px}
.epic-list-summary{display:flex;align-items:center;gap:4px;min-height:26px;padding-right:8px;border-radius:var(--radius);color:var(--muted);cursor:pointer;font-size:12.5px;list-style:none}
.epic-list-summary::-webkit-details-marker{display:none}
.epic-list-summary:hover{background:var(--bg-inset);color:var(--ink)}
.epic-list-summary .twisty{width:13px;height:13px;flex:none;margin:0 4px 0 3px;color:var(--faint);transition:transform .12s ease}
.epic-list[open]>.epic-list-summary .twisty{transform:rotate(90deg)}
.epic-list-open{display:none}
.epic-list[open]>.epic-list-summary>.epic-list-shut{display:none}
.epic-list[open]>.epic-list-summary>.epic-list-open{display:inline}
.epic-list-panel{margin-left:10px;padding-left:4px;border-left:1px solid var(--line)}
.epic-list-link{display:flex;align-items:flex-start;gap:6px;min-height:26px;padding:4px 8px 4px 0;color:var(--muted);text-decoration:none;font-size:12.5px;overflow-wrap:anywhere}
.epic-list-link .i{flex:none;width:13px;height:13px;margin-top:2px;color:var(--faint)}
.epic-list-link:hover{color:var(--ink)}
.epic-list-link.current{color:var(--accent);font-weight:650}
.epic-list-index{display:block;padding:4px 0;color:var(--accent);text-decoration:none;font-size:12px}

/* The epics index -------------------------------------------------------- */
.app-lede{margin:6px 0 0;color:var(--muted);font-size:13px;max-width:60ch}
.app-empty{color:var(--muted)}
.epic-index{display:grid;gap:10px;margin:0;padding:0;list-style:none}
.epic-index-row{padding:12px 14px;border:1px solid var(--line);border-radius:var(--radius);background:var(--bg-subtle)}
.epic-index-row.current{border-color:var(--accent-line)}
.epic-index-link{font:650 15px/1.3 var(--ui);color:var(--accent);text-decoration:none}
.epic-index-badge{margin-left:8px;color:var(--faint);font-size:11px}
.epic-index-counts{margin:4px 0 0;color:var(--faint);font:11px var(--mono)}
.epic-index-description{margin:6px 0 0;color:var(--muted);font-size:13px}

/* Changed-file tree ------------------------------------------------------ */
.tree-tools{display:flex;align-items:center;gap:4px;padding:2px 4px 6px}
.tree-search{position:relative;flex:1;display:flex;align-items:center}
.tree-search .i{position:absolute;left:7px;width:13px;height:13px;color:var(--faint);pointer-events:none}
.tree-tools input[type=search]{min-width:0;width:100%;height:26px;padding:0 8px 0 25px;border:1px solid var(--line);border-radius:var(--radius);background:var(--bg);font-size:12px}
.tree-tools input[type=search]::-webkit-search-cancel-button{-webkit-appearance:none}
.tree-summary{margin:0 6px 6px;color:var(--faint);font:11px var(--mono)}
.tree-empty{margin:8px 6px;color:var(--muted);font-size:12px}
.file-tree{display:block;max-width:100%;overflow-x:auto;overscroll-behavior-x:contain;scrollbar-width:thin}
.file-tree summary,.file-tree a{display:flex;align-items:center;gap:6px;width:max-content;min-width:100%;min-height:24px;padding:0 6px 0 calc(4px + var(--depth,0) * 12px);border-radius:var(--radius);color:var(--ink);text-decoration:none;font:12px var(--mono);white-space:nowrap}
.file-tree summary{list-style:none;cursor:pointer;color:var(--muted)}
.file-tree summary::-webkit-details-marker{display:none}
.file-tree summary:hover,.file-tree a:hover{background:var(--bg-inset)}
.file-tree .twisty{width:12px;height:12px;flex:none;color:var(--faint);transition:transform .12s ease}
.file-tree details[open]>summary .twisty{transform:rotate(90deg)}
.file-tree .tree-name{min-width:max-content;overflow:visible;text-overflow:clip}
.file-tree .selected{background:var(--sel);box-shadow:inset 2px 0 var(--accent);color:var(--accent);font-weight:600}
.file-tree .reviewed .tree-name{color:var(--muted)}
.file-tree .counts{margin-left:auto;padding-left:8px;color:var(--faint);font:11px var(--mono)}

/* Content ---------------------------------------------------------------- */
.content{width:min(1080px,100%);padding:26px clamp(16px,3vw,40px) 96px}
.shell.slide-mode{height:calc(100vh - var(--top));min-height:0;overflow:hidden}
.shell.slide-mode>.sidebar{position:relative;top:auto;height:100%}
.shell.slide-mode>.content{width:100%;height:100%;padding:0;overflow:hidden}
.shell.slide-mode #view-slides{height:100%}
.code-mode .content{width:100%;padding:0 0 40px}
.view{display:none}
.view.active{display:block}
.page-heading{margin:0 0 18px}
.page-heading h2{margin:0;font:600 22px/1.25 var(--ui);letter-spacing:-.01em}
.coverage-totals{margin:6px 0 0;color:var(--faint);font:11px var(--mono)}
.coverage-totals .gap{color:var(--red);font-weight:600}
.fragment-placeholder,.section-placeholder{margin:0;padding:8px 10px;color:var(--faint);font:11px var(--mono)}
.breadcrumbs{display:flex;align-items:center;gap:6px;margin:0 0 14px;color:var(--muted);font-size:12px}
.breadcrumbs a{color:var(--muted);text-decoration:none}
.breadcrumbs a:hover{color:var(--accent);text-decoration:underline}
.alert{display:flex;gap:9px;align-items:flex-start;margin:0 0 20px;padding:10px 12px;border:1px solid var(--warning-line);border-left:3px solid var(--amber);border-radius:var(--radius);background:var(--warning-bg);font-size:12.5px}
.alert .i{color:var(--amber);margin-top:1px}
.alert strong{display:block;margin-bottom:2px}
.remaining{margin:0;padding:24px;color:var(--muted);text-align:center;font-size:12.5px}

/* Requirements ---------------------------------------------------------- */
.requirements-page{max-width:940px;margin:0 auto}
.terms-page{max-width:940px;margin:0 auto}
.app-page{display:grid;grid-template-columns:minmax(0,1fr);gap:22px;max-width:940px;margin:0 auto}
.app-page .term-code{overflow-x:auto}
.test-case-page .term-code-list>h3::first-letter{text-transform:uppercase}
.app-page-kind{margin:0 0 4px;color:var(--muted);font:650 11px/1.3 var(--ui);letter-spacing:.04em;text-transform:uppercase}
.app-page-section>h2,.requirement-trace>h2{margin:0 0 10px;color:var(--muted);font:650 12px/1.3 var(--ui);letter-spacing:.04em;text-transform:uppercase}
.app-page-section>h3,.requirement-trace h3{margin:14px 0 6px;font:650 14px/1.3 var(--ui)}
.trace-links{display:grid;gap:8px;margin:0;padding:0;list-style:none}
.trace-links>li{padding:10px 14px;border:1px solid var(--line-soft);border-radius:8px;background:var(--bg);font:14px/1.45 var(--ui)}
.trace-links a{color:var(--accent);text-decoration:none;font-weight:600}
.trace-links a:hover{text-decoration:underline}
.trace-kind{margin-left:6px;padding:1px 7px;border-radius:999px;background:var(--bg-inset);color:var(--muted);font:600 11px var(--ui)}
.trace-note{margin-left:6px;color:var(--muted);font:12px var(--ui)}
.trace-rationale{margin:5px 0 0;color:var(--muted);font:13px/1.5 var(--ui)}
.epic-summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px;margin:0}
.epic-summary>div{padding:10px 14px;border:1px solid var(--line-soft);border-radius:8px;background:var(--bg-subtle)}
.epic-summary dt{color:var(--muted);font:650 11px/1.3 var(--ui);letter-spacing:.04em;text-transform:uppercase}
.epic-summary dd{margin:4px 0 0;font:600 16px/1.4 var(--ui)}
.epic-summary dd small{color:var(--muted);font:12px var(--ui)}
.test-steps{display:grid;gap:8px;margin:0;padding-left:22px;font:14px/1.5 var(--ui)}
.test-steps p{margin:0}
.test-runs{display:grid;gap:8px;margin:0;padding:0;list-style:none}
.test-run{padding:10px 14px;border:1px solid var(--line-soft);border-left:3px solid var(--line);border-radius:8px;font:14px/1.45 var(--ui)}
.test-run.passed{border-left-color:var(--green)}.test-run.failed{border-left-color:var(--red)}.test-run.blocked,.test-run.skipped{border-left-color:var(--amber)}
.test-run p{margin:5px 0 0}
.requirement-trace{display:grid;gap:4px}
.requirements-epic{margin:0 0 28px}
.requirements-epic-head h2{margin:0 0 4px;font:650 18px/1.3 var(--ui)}
.requirements-epic-head h2 a{color:var(--ink);text-decoration:none}
.requirements-epic-head h2 a:hover{color:var(--accent)}
.requirements-epic-head p{margin:0 0 12px;color:var(--muted);font:13.5px/1.55 var(--ui)}
.observe-coverage-note{margin:0 0 12px;color:var(--muted);font:13px/1.5 var(--ui)}
.observe-coverage-empty{color:var(--muted);font:13px var(--ui)}
.observe-references{display:grid;gap:4px;margin:0 0 10px;padding:0;list-style:none;font:12px var(--mono)}
.observe-references li{display:flex;align-items:center;gap:6px;flex-wrap:wrap}
.observe-references .gap{color:var(--red);font:600 12px var(--ui)}
.criterion-page h1{font-size:24px;line-height:1.35}
.criterion-trace{margin-top:8px;font:12.5px/1.45 var(--ui);color:var(--muted)}
.criterion-trace a{color:var(--accent);text-decoration:none}
.terms-lede{margin:0 0 18px;color:var(--muted);font:14px/1.55 var(--ui)}
.terms-list{display:grid;gap:10px;margin:0}
.terms-entry{padding:13px 16px;border:1px solid var(--line);border-radius:9px;background:var(--bg)}
.terms-entry.retired{opacity:.7}
.terms-entry dt{font:650 16px/1.3 var(--ui)}
.terms-entry dt a{color:inherit;text-decoration:none}
.terms-entry dt a:hover{color:var(--accent)}
.terms-entry dt small{margin-left:4px;color:var(--muted);font:500 12px var(--ui)}
.terms-entry dd{margin:6px 0 0;color:var(--muted);font:13.5px/1.55 var(--ui)}
.term-page{display:grid;gap:22px}
.term-page h2,.requirement-terms h2{margin:0 0 8px;color:var(--muted);font:650 12px/1.3 var(--ui);letter-spacing:.04em;text-transform:uppercase}
.term-definition p{max-width:800px;margin:0;font:500 19px/1.5 var(--ui);white-space:pre-line}
.term-state{color:var(--amber);font:600 12px var(--ui)}
.term-links{display:flex;flex-wrap:wrap;gap:7px;margin:0;padding:0;list-style:none}
.term-links li{padding:4px 10px;border:1px solid var(--line-soft);border-radius:999px;background:var(--bg-subtle);font:500 12.5px var(--ui)}
.term-links a{color:var(--accent);text-decoration:none}
.term-empty{margin:0;color:var(--muted);font:13px var(--ui)}
.term-code{margin:0 0 12px;overflow:hidden;border:1px solid var(--line);border-radius:8px;background:var(--bg)}
.term-code.stale{border-color:var(--warning-line)}
.term-code figcaption{display:flex;align-items:center;gap:7px;padding:8px 12px;border-bottom:1px solid var(--line-soft);background:var(--bg-subtle);font:12px var(--ui)}
.term-code figcaption code{font:11.5px var(--mono);overflow-wrap:anywhere}
.term-code-note{margin:0;padding:8px 12px;border-bottom:1px solid var(--line-soft);background:var(--warning-bg);color:var(--amber);font:12.5px/1.45 var(--ui)}
.term-code-lines{width:100%;border-collapse:collapse;font:12.5px/1.6 var(--mono)}
.term-code-lines th{width:1%;padding:0 10px;color:var(--faint);font-weight:400;text-align:right;user-select:none}
.term-code-lines td{padding:0 12px 0 4px;white-space:pre}
.term-code-lines tr.referenced{background:var(--accent-soft)}
.term-code-lines tr.referenced th{color:var(--accent)}
.requirement-terms{margin:0}
.file-terms{margin-top:12px}
.file-terms h3{margin:0 0 6px;color:var(--muted);font:650 11px/1.3 var(--ui);letter-spacing:.04em;text-transform:uppercase}
.requirements-header{margin:0 0 20px;padding-bottom:14px;border-bottom:1px solid var(--line)}
.requirements-header h1,.requirement-story-hero h1{margin:0;color:var(--ink);font:650 28px/1.15 var(--ui);letter-spacing:-.025em}
.requirements-story-list{display:grid;gap:10px}
.requirements-story-card{overflow:hidden;border:1px solid var(--line);border-radius:9px;background:var(--bg);transition:border-color .14s}
.requirements-story-card:hover{border-color:var(--accent-line)}
.requirements-story-card>header{padding:16px 17px 0}
.requirements-story-card h2{margin:0;font:650 17px/1.3 var(--ui)}
.requirements-story-card h2 a{color:inherit;text-decoration:none}
.requirements-story-card h2 a:hover{color:var(--accent)}
.requirement-story-statement{margin:9px 17px 14px;color:var(--muted);font:13.5px/1.55 var(--ui)}
.requirements-criteria-preview{border-top:1px solid var(--line-soft);background:var(--bg-subtle)}
.requirements-criteria-preview>summary{display:flex;align-items:center;gap:6px;padding:9px 15px;list-style:none;color:var(--muted);cursor:pointer;font:600 11px var(--ui)}
.requirements-criteria-preview>summary::-webkit-details-marker{display:none}
.requirements-criteria-preview>summary .twisty{width:12px;height:12px;transition:transform .12s}
.requirements-criteria-preview[open]>summary .twisty{transform:rotate(90deg)}
.requirements-criteria-preview ol{display:grid;gap:5px;margin:0;padding:0 12px 12px;list-style:none}
.requirements-criteria-preview li a{display:grid;grid-template-columns:48px minmax(0,1fr);align-items:baseline;gap:9px;padding:8px 9px;border:1px solid var(--line-soft);border-radius:7px;background:var(--bg);color:inherit;text-decoration:none}
.requirements-criteria-preview li a:hover{border-color:var(--accent-line);background:var(--accent-soft)}
.requirements-criteria-preview li span{color:var(--accent);font:700 9.5px var(--ui);letter-spacing:.05em;text-transform:uppercase}
.requirements-criteria-preview li strong{font:500 12.5px/1.45 var(--ui)}
.requirements-empty{display:grid;gap:4px;padding:30px;border:1px dashed var(--line);border-radius:10px;color:var(--muted);text-align:center}
.requirements-breadcrumbs{display:flex;align-items:center;gap:7px;margin:0 0 18px;color:var(--faint);font-size:12px}
.requirements-breadcrumbs a{color:var(--accent);text-decoration:none}
.requirements-breadcrumbs strong{color:var(--ink)}
.requirement-story-page{display:grid;gap:20px}
.requirement-story-hero{padding-bottom:18px;border-bottom:1px solid var(--line)}
.requirements-conflict{display:flex;align-items:flex-start;gap:8px;padding:10px 12px;border:1px solid var(--warning-line);border-radius:8px;background:var(--warning-bg);color:var(--amber)}
.requirements-conflict .i{margin-top:2px}
.requirement-story-details{overflow:hidden;border:1px solid var(--line-soft);border-radius:8px;background:var(--bg)}
.requirement-story-details>summary{display:flex;align-items:center;gap:7px;padding:11px 14px;list-style:none;color:var(--muted);cursor:pointer;font:650 12px var(--ui)}
.requirement-story-details>summary::-webkit-details-marker{display:none}
.requirement-story-details>summary .twisty{width:13px;height:13px;transition:transform .12s}
.requirement-story-details[open]>summary .twisty{transform:rotate(90deg)}
.requirement-story-details-content{display:grid;gap:10px;padding:0 14px 14px}
.requirement-need>p:last-child{max-width:800px;margin:0;font:500 20px/1.5 var(--ui)}
.requirement-characterization{padding:14px;border:1px solid var(--line-soft);border-radius:7px;background:var(--bg-subtle)}
.requirement-characterization dl{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;margin:0}
.requirement-characterization dl>div{display:grid;gap:2px;padding-top:9px;border-top:1px solid var(--line-soft)}
.requirement-characterization dt{color:var(--faint);font:700 9px var(--ui);letter-spacing:.06em;text-transform:uppercase}
.requirement-characterization dd{margin:0;color:var(--ink);font:12.5px var(--ui)}
.requirement-characterization dd small{display:block;margin-top:2px;color:var(--muted);font:11px/1.4 var(--ui)}
.requirement-characterization code{font:11px var(--mono)}
.requirement-history-lists{display:grid;grid-template-columns:1fr 1fr;gap:9px}
.requirement-history-lists details{overflow:hidden;border:1px solid var(--line-soft);border-radius:8px;background:var(--bg)}
.requirement-history-lists summary{display:flex;align-items:center;gap:6px;padding:9px 10px;list-style:none;color:var(--muted);cursor:pointer;font:600 11px var(--ui)}
.requirement-history-lists summary::-webkit-details-marker{display:none}
.requirement-history-lists summary .twisty{width:12px;height:12px;transition:transform .12s}
.requirement-history-lists details[open] summary .twisty{transform:rotate(90deg)}
.requirement-history-lists summary span{margin-left:auto;font:10px var(--mono)}
.requirement-history-lists ol{display:grid;gap:1px;margin:0;padding:0 8px 8px;list-style:none}
.requirement-history-lists li{display:grid;gap:3px;min-width:0;padding:8px;border-radius:6px;background:var(--bg-subtle)}
.requirement-history-lists li.current{box-shadow:inset 2px 0 var(--accent);background:var(--accent-soft)}
.requirement-history-lists li strong{font:600 11px var(--ui)}
.requirement-history-lists li p{margin:0;color:var(--muted);font:11px/1.4 var(--ui)}
.requirement-history-lists li code{overflow:hidden;text-overflow:ellipsis;color:var(--faint);font:9.5px var(--mono)}
.requirement-history-lists li time{color:var(--faint);font:9px var(--mono)}
.requirement-criteria>header{margin-bottom:10px}
.requirement-criteria h2{margin:0;font:650 18px/1.3 var(--ui);letter-spacing:-.01em}
.requirement-criteria-list{display:grid;gap:9px}
.requirement-criterion{scroll-margin-top:calc(var(--top) + 14px);padding:14px 15px;border:1px solid var(--line);border-left:4px solid var(--accent-line);border-radius:9px;background:var(--bg);transition:border-color .14s,box-shadow .14s,transform .14s}
.requirement-criterion:hover{border-color:var(--accent-line);transform:translateY(-1px);box-shadow:0 4px 14px #1f232810}
.requirement-criterion.selected{border-color:var(--accent);background:var(--accent-soft);box-shadow:0 0 0 2px color-mix(in srgb,var(--accent) 18%,transparent)}
.requirement-criterion>header{display:flex;align-items:center;gap:12px}
.criterion-label{display:flex;align-items:center;gap:6px;color:var(--accent);font:750 10.5px var(--ui);letter-spacing:.06em;text-decoration:none;text-transform:uppercase}
.requirement-criterion>p{margin:9px 0 0;font:500 14px/1.55 var(--ui)}
.opening-badge{font:600 12px/1 var(--ui);padding:5px 9px;border-radius:999px;border:1px solid var(--line);color:var(--muted);background:var(--bg-subtle);white-space:nowrap;margin-right:8px}
.opening-badge.compare{color:var(--accent);border-color:var(--accent-line);background:var(--accent-soft)}
.layer-quiet{opacity:.55;transition:opacity .15s}.layer-quiet:hover,.layer-quiet:focus-within{opacity:1}
.fragment.layer-changed,.section.layer-changed{box-shadow:inset 3px 0 0 var(--accent)}.fragment.layer-affected,.section.layer-affected{box-shadow:inset 3px 0 0 var(--amber)}
a.layer-changed{font-weight:600}a.layer-affected{font-style:italic}
.history-button{color:var(--muted)}
.change-view{position:fixed;z-index:20;inset:var(--top) 0 0;background:var(--bg);overflow:auto}.change-wrap{max-width:1100px;margin:0 auto;padding:24px 28px 64px;font:14px/1.5 var(--ui);color:var(--ink)}
.change-heading h1{margin:4px 0 6px;font-size:22px}.change-heading p{margin:0;color:var(--muted)}.change-note{color:var(--amber)!important;margin-top:6px!important}
.change-layer{margin-top:28px}.change-layer h2{font-size:17px;margin:0 0 4px;display:flex;gap:8px;align-items:baseline}.change-layer h2 span{color:var(--muted);font-weight:500}.change-layer-hint{color:var(--muted);margin:0 0 12px}
.change-record,.change-code-group{border:1px solid var(--line);border-radius:var(--radius);padding:12px 14px;margin:0 0 10px;background:var(--bg)}
.change-record header,.change-code-group header{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.change-badge{font:600 11px/1 var(--ui);text-transform:uppercase;letter-spacing:.04em;padding:3px 6px;border-radius:4px;background:var(--bg-inset);color:var(--muted)}
.change-badge.added{color:var(--green)}.change-badge.revised,.change-badge.changed{color:var(--accent)}.change-badge.retired{color:var(--red)}.change-badge.affected{color:var(--amber)}
.change-kind{color:var(--faint);font-size:12px}.change-title{font-weight:600;color:var(--ink)}.change-removed{color:var(--red);font-size:12px}
.change-pair{margin:8px 0 0;padding:6px 10px;border-radius:6px;background:var(--accent-soft)}.change-pair.ambiguous{background:var(--warning-bg)}
.before-after{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:10px;margin-top:10px}.before-after h3{font-size:12px;color:var(--muted);margin:0 0 4px;text-transform:uppercase}
.before-after pre,.change-hunk pre{margin:0;padding:8px 10px;background:var(--bg-subtle);border-radius:6px;font:12px/1.45 var(--mono);white-space:pre-wrap;overflow-wrap:anywhere}
.before-after .before pre{background:var(--del-bg)}.before-after .after pre{background:var(--add-bg)}
.change-causes{margin:8px 0 0;padding-left:18px}.change-cause-kind{font:600 11px/1 var(--ui);text-transform:uppercase;color:var(--amber);margin-right:4px}
.change-reasons{list-style:none;margin:10px 0 0;padding:8px 10px;border-left:3px solid var(--line);background:var(--bg-subtle);border-radius:0 6px 6px 0}.change-reasons li+li{margin-top:6px}.change-reasons p,.history-event p{margin:4px 0 0;white-space:pre-wrap;color:var(--muted)}
.change-reason-meta{color:var(--faint);font-size:12px}.change-collapsed{margin:4px 0 0;padding-left:16px}
.change-hunk{margin-top:8px}.change-hunk-path{font:12px var(--mono);color:var(--muted);margin-bottom:3px}.change-line{display:block}.change-line.old{background:var(--del-bg)}.change-line.new{background:var(--add-bg)}
.change-code-group.unreferenced{border-color:var(--warning-line)}
.history-events{padding-left:18px}.history-event{margin:10px 0}.history-open code{font-size:12px;overflow-wrap:anywhere}
@media(max-width:780px){.requirement-characterization dl,.requirement-history-lists{grid-template-columns:1fr}.requirements-criteria-preview li a{grid-template-columns:42px minmax(0,1fr)}}

/* Chapter list ----------------------------------------------------------- */
.chapter-index{margin-top:28px;border-top:1px solid var(--line-soft);padding-top:14px}
.chapter-index>h2{margin:0 0 4px;font:600 12px var(--ui);color:var(--muted)}
.chapter-pages{border-bottom:1px solid var(--line-soft)}
.chapter-pages>.chapter{margin:0;padding:0;border-top:1px solid var(--line-soft)}
.chapter>.chapter-head{min-height:44px;padding:4px 2px}
.chapter-head h2{min-width:0;flex:1;font-size:15px}
.chapter-head h2 a{color:inherit;text-decoration:none}
.chapter-head h2 a:hover{color:var(--accent)}
.chapter-toggle{display:grid;place-items:center;width:26px;height:26px;padding:0;border:0;border-radius:4px;background:transparent;color:var(--faint)}
.chapter-toggle:hover{background:var(--bg-subtle);color:var(--ink)}
.chapter-toggle .twisty{transition:transform .14s ease}
.chapter.open>.chapter-head .chapter-toggle .twisty{transform:rotate(90deg)}
.chapter-body{padding:2px 0 22px 28px}
.chapter-body[hidden]{display:none}

/* Sections and fragments ------------------------------------------------- */
.section{scroll-margin-top:calc(var(--top) + 12px);margin:22px 0}
.section .section{margin-left:0;padding-left:14px;border-left:1px solid var(--line-soft)}
.section-head{display:flex;justify-content:space-between;align-items:center;gap:12px}
.section h2{margin:0;font:600 17px/1.35 var(--ui);letter-spacing:-.01em}
.section .section h2{font-size:14.5px;color:var(--muted)}
.section-actions,.fragment-actions{display:flex;align-items:center;gap:2px}
.section-actions>.diff-button,.section-actions>.permalink,.fragment-actions>.diff-button,.fragment-actions>.landmark-menu,.fragment-actions>.permalink{opacity:0;transition:opacity .12s}
.section:hover>.section-actions>.diff-button,.section:hover>.section-actions>.permalink,.section:hover>.section-head>.section-actions>.diff-button,.section:hover>.section-head>.section-actions>.permalink,.section-head:focus-within>.section-actions>.diff-button,.section-head:focus-within>.section-actions>.permalink,.fragment:hover>.fragment-head>.fragment-actions>.diff-button,.fragment:hover>.fragment-head>.fragment-actions>.landmark-menu,.fragment:hover>.fragment-head>.fragment-actions>.permalink,.fragment-head:focus-within>.fragment-actions>.diff-button,.fragment-head:focus-within>.fragment-actions>.landmark-menu,.fragment-head:focus-within>.fragment-actions>.permalink{opacity:1}
.fragment{position:relative;scroll-margin-top:calc(var(--top) + 12px);margin:14px 0;padding:0 0 0 12px;border-left:2px solid transparent;outline:0}
.fragment.active-fragment{border-left-color:var(--active-fragment-line)}
.fragment-head{position:relative;z-index:20;display:flex;justify-content:flex-end;gap:8px;align-items:center;min-height:22px}
.fragment-stage{position:relative;min-height:40px}
.fragment-frame{display:block;width:100%;min-height:380px;border:1px solid var(--line-soft);border-radius:var(--radius);background:var(--bg)}
.fragment-image{display:block;max-width:100%;height:auto}
.fragment-markdown{font:14px/1.65 var(--ui);color:var(--ink)}
.fragment-markdown>:first-child{margin-top:0}
.fragment-markdown h1,.fragment-markdown h2,.fragment-markdown h3,.fragment-markdown h4{margin:1.5em 0 .4em;font-weight:600;letter-spacing:-.01em;line-height:1.3}
.fragment-markdown h1{font-size:18px}
.fragment-markdown h2{font-size:16px}
.fragment-markdown h3{font-size:14px}
.fragment-markdown h4{font-size:13px;color:var(--muted)}
.fragment-markdown code{padding:.15em .35em;border-radius:4px;background:var(--bg-inset);font:12px var(--mono)}
.fragment-markdown pre,.plain{overflow:auto;padding:10px 12px;border:1px solid var(--line-soft);border-radius:var(--radius);background:var(--bg-subtle);font:12px/1.5 var(--mono)}
.fragment-markdown pre code{padding:0;background:transparent}
.fragment-markdown blockquote{margin:1em 0;padding-left:12px;border-left:2px solid var(--line);color:var(--muted)}
.fragment-markdown table{border-collapse:collapse;font-size:12.5px}
.fragment-markdown th,.fragment-markdown td{padding:5px 9px;border:1px solid var(--line-soft);text-align:left}
.fragment-markdown th{background:var(--bg-subtle)}
.fragment-markdown .footnote-ref{display:inline-flex;align-items:center;justify-content:center;min-width:1.25em;height:1.25em;margin:0 .08em;padding:0 .25em;border-radius:999px;background:var(--bg-inset);color:var(--muted);font:600 10px/1 var(--mono);text-decoration:none;vertical-align:super}
.fragment-markdown .footnote-ref:hover,.fragment-markdown .footnote-ref:focus-visible{background:var(--footnote-hover-bg);color:var(--accent)}
.fragment-markdown .footnote-ref.diff-citation{background:var(--citation-bg);color:var(--accent);cursor:pointer}
.fragment-markdown .footnotes{margin-top:24px;color:var(--muted);font-size:12.5px}
.fragment-markdown .footnotes hr{height:1px;margin:0 0 10px;border:0;background:var(--line-soft)}
.fragment-markdown .footnotes ol{margin:0;padding-left:24px}
.fragment-markdown .footnotes li{padding:2px 0 2px 4px}
.fragment-markdown .footnotes p{margin:.35em 0}
.fragment-markdown .footnote-backref{color:var(--muted);text-decoration:none}
.fragment-markdown .footnotes .content-landmark-text{background:var(--landmark-bg)}

/* Quiet controls --------------------------------------------------------- */
.btn,button{font:12px var(--ui);border:1px solid transparent;border-radius:var(--radius);padding:4px 9px;background:var(--bg-inset);color:var(--ink)}
.btn:hover,button:hover{background:var(--button-hover)}
.btn-primary{background:var(--primary-bg);border-color:var(--primary-bg);color:var(--primary-ink)}
.btn-primary:hover{background:var(--primary-hover)}
.icon-button{display:inline-grid;place-items:center;width:24px;height:24px;padding:0;border:0;background:transparent;color:var(--muted)}
.icon-button:hover{background:var(--bg-inset);color:var(--ink)}
.icon-button[aria-pressed=true]{background:var(--accent-soft);color:var(--accent)}
.diff-button{display:inline-flex;align-items:center;gap:4px;width:auto;padding:0 6px;font:11px var(--mono)}
.diff-counts{display:inline-flex;gap:5px;font-weight:600}
.diff-counts .add{color:var(--green)}
.diff-counts .del{color:var(--red)}
.permalink{position:relative;display:inline-grid;place-items:center;width:24px;height:24px;padding:0;border:0;background:transparent;color:var(--faint)}
.permalink:hover{background:var(--bg-inset);color:var(--accent)}
.permalink.copied:after{position:absolute;right:0;bottom:calc(100% + 4px);z-index:5;content:'Copied';padding:2px 6px;border-radius:4px;background:var(--copied-bg);color:var(--copied-ink);font:11px var(--ui);white-space:nowrap}
.fragment-heading{display:flex;align-items:center;gap:4px;scroll-margin-top:calc(var(--top) + 12px)}
.fragment-heading .heading-permalink{display:inline-grid;place-items:center;width:22px;height:20px;opacity:0;color:var(--faint);text-decoration:none}
.fragment-heading:hover .heading-permalink,.fragment-heading:focus-within .heading-permalink,.heading-permalink:focus-visible{opacity:1}

/* Landmarks -------------------------------------------------------------- */
.landmark-target{position:absolute;top:0;left:0;width:1px;height:1px;scroll-margin-top:calc(var(--top) + 12px)}
.landmark-menu{position:relative}
.landmark-menu>summary{display:grid;place-items:center;width:24px;height:24px;list-style:none;border-radius:var(--radius);color:var(--muted);cursor:pointer}
.landmark-menu>summary:hover{background:var(--bg-inset);color:var(--ink)}
.landmark-menu>summary::-webkit-details-marker{display:none}
.landmark-list{position:absolute;z-index:20;right:0;top:calc(100% + 4px);width:min(280px,75vw);padding:4px;background:var(--bg);border:1px solid var(--line);border-radius:var(--radius);box-shadow:var(--shadow)}
.landmark-list>div{display:flex;align-items:center;gap:2px;border-radius:4px}
.landmark-list>div:hover{background:var(--bg-subtle)}
.landmark-list a:first-child{min-width:0;flex:1;padding:5px 7px;color:var(--ink);text-decoration:none;font-size:12.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.content-landmark-region{pointer-events:none;fill:transparent;stroke:transparent;stroke-width:5;vector-effect:non-scaling-stroke}
.content-landmark-region.active{fill:#f2bd4b33;stroke:#d39418}
.content-landmark-active{background:#f2bd4b40;outline:2px solid #d39418}
.landmark-affordance{display:inline-flex;align-items:center;gap:1px;padding:1px;border-radius:var(--radius);background:var(--landmark-affordance-bg);border:1px solid var(--line);box-shadow:var(--shadow);opacity:0;transition:opacity .12s}
.fragment-heading:hover>.landmark-affordance,.fragment-heading:focus-within>.landmark-affordance,.fragment.preview-linked-items .fragment-heading[data-landmark-has-diffs="true"]>.landmark-affordance,.content-landmark-text:hover>.landmark-affordance,.content-landmark-text:focus-within>.landmark-affordance,.fragment.preview-linked-items .content-landmark-text[data-landmark-has-diffs="true"]>.landmark-affordance{opacity:1}
.content-landmark-text{position:relative;display:inline;border-radius:3px;background:transparent;color:inherit}
.content-landmark-text>.landmark-affordance{position:absolute;z-index:4;left:calc(100% + 4px);top:50%;transform:translateY(-50%);white-space:nowrap}
.content-landmark-text.content-landmark-active,.fragment.preview-linked-items .content-landmark-text[data-landmark-has-diffs="true"]{background:#f2bd4b40;outline:2px solid #d39418}
.landmark-hotspot{position:absolute;z-index:3;border:1px solid transparent;border-radius:4px;pointer-events:auto}
.landmark-hotspot>.landmark-affordance{position:absolute;right:3px;top:3px}
.landmark-hotspot:hover,.landmark-hotspot:focus-within,.landmark-hotspot.active,.fragment.preview-linked-items .landmark-hotspot[data-landmark-has-diffs="true"]{border-color:#d39418;background:transparent}
.landmark-hotspot:hover>.landmark-affordance,.landmark-hotspot:focus-within>.landmark-affordance,.landmark-hotspot.active>.landmark-affordance,.fragment.preview-linked-items .landmark-hotspot[data-landmark-has-diffs="true"]>.landmark-affordance{opacity:1}

/* Dialogs, drawers, composers -------------------------------------------- */
.diff-drawer{position:fixed;z-index:80;inset:var(--top) 0 0 auto;width:min(1100px,92vw);background:var(--bg);border-left:1px solid var(--line);box-shadow:-12px 0 40px #1f23281f;transform:translateX(105%);transition:transform .2s ease;display:flex;flex-direction:column}
.diff-drawer.open{transform:none}
.drawer-backdrop{position:fixed;z-index:70;inset:var(--top) 0 0;background:#1f232833;display:none}
.drawer-backdrop.open{display:block}
.drawer-head{min-height:38px;display:flex;align-items:center;gap:8px;padding:5px 10px;border-bottom:1px solid var(--line);background:var(--bg)}
.drawer-head strong{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:600 12.5px var(--ui)}
.drawer-head .icon-button{margin-left:auto}
.drawer-body{overflow:auto;padding-bottom:60px;background:var(--bg)}
.diff-drawer[data-drawer-mode=fragment] .drawer-body{padding:10px clamp(16px,4vw,54px) 72px}
.diff-drawer[data-drawer-mode=fragment] .fragment{max-width:980px;margin:0 auto;padding-left:0}
.diff-drawer[data-drawer-mode=fragment] .fragment.active-fragment{border-left-color:transparent}
.diff-drawer[data-drawer-mode=history]{width:min(620px,92vw)}
.diff-drawer[data-drawer-mode=history] .drawer-body{padding-bottom:0}
.history-drawer-surface{min-height:100%}
.drawer-body .file-head{top:0}
.drawer-body .diff-column-head{top:38px}

/* Linked code (drawer contents) ------------------------------------------ */
.attached-code-summary{display:flex;align-items:center;gap:10px;padding:7px 12px;border-bottom:1px solid var(--line-soft);background:var(--bg-subtle);color:var(--muted);font:11px var(--mono)}
.attached-code-scope{margin-left:auto;color:var(--faint);font-family:var(--ui)}
.attached-file-list{padding:8px 10px}
.attached-file{margin:6px 0;border:1px solid var(--line);border-radius:var(--radius);overflow:hidden;background:var(--bg)}
.attached-file>summary{display:flex;align-items:center;gap:8px;padding:7px 10px;list-style:none;cursor:pointer}
.attached-file>summary::-webkit-details-marker{display:none}
.attached-file>summary:hover{background:var(--bg-subtle)}
.attached-file>summary .twisty{width:12px;height:12px;flex:none;color:var(--faint);transition:transform .12s}
.attached-file[open]>summary .twisty{transform:rotate(90deg)}
.attached-file-main{display:flex;min-width:0;flex:1;flex-direction:column}
.attached-file-main code{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:600 12px var(--mono)}
.attached-file-note{overflow:hidden;color:var(--muted);font-size:12px;text-overflow:ellipsis;white-space:nowrap}
.attached-file-note.missing{color:var(--amber)}
.attached-file-counts{margin-left:auto;color:var(--faint);font:11px var(--mono);white-space:nowrap}
.attached-file>.diff-surface{border-top:1px solid var(--line-soft);overflow:auto}
.review-file-diff-actions{display:flex;align-items:center;gap:8px;padding:5px 10px;background:var(--bg-subtle);color:var(--faint);font:11px var(--mono);border-bottom:1px solid var(--line-soft)}
.review-file-diff-actions a{display:inline-flex;align-items:center;gap:4px;margin-left:auto;color:var(--accent);text-decoration:none}
.attached-file .diff-column-head{position:static!important;top:auto!important}
.attached-file .diff-row.linked-evidence{box-shadow:inset 3px 0 var(--accent)}
.attached-file .diff-row.linked-evidence .line-no:first-child{color:var(--accent)}
.diff-surface.loading [data-file-diff-rows]>:not([data-diff-placeholder]){opacity:.55}
.manifest-file-diff.loading [data-manifest-diff-rows]>:not([data-diff-placeholder]){opacity:.55}
.diff-placeholder{margin:0;padding:8px 10px;color:var(--faint);font:11px var(--mono)}

/* Code workspace --------------------------------------------------------- */
.tool-divider{width:1px;height:18px;margin:0 3px;background:var(--line)}
.code-toolbar{position:sticky;top:var(--top);z-index:6;display:flex;align-items:center;gap:2px;padding:5px clamp(8px,1.4vw,14px);background:var(--bg);border-bottom:1px solid var(--line)}
.code-toolbar .spacer{flex:1}
.code-toolbar .metric{color:var(--faint);font:11px var(--mono);padding-right:6px}
.code-workspace{display:grid;grid-template-columns:minmax(0,1fr) minmax(240px,300px);gap:0;align-items:start}
.code-workspace.related-hidden{grid-template-columns:minmax(0,1fr) 0}
.code-workspace.related-hidden .related-saga{visibility:hidden;overflow:hidden;padding:0;border:0}
.code-main{min-width:0}
.related-saga{position:sticky;top:calc(var(--top) + 35px);max-height:calc(100vh - var(--top) - 35px);overflow:auto;padding:10px 12px;border-left:1px solid var(--line)}
.related-head{display:flex;align-items:center;gap:6px;margin-bottom:6px}
.related-head h2{margin:0;font:600 11px var(--ui);letter-spacing:.02em;color:var(--muted)}
.related-head .icon-button{margin-left:auto}
.related-chapter{border-top:1px solid var(--line-soft);padding-top:8px;margin-top:8px}
.related-chapter:first-of-type{border-top:0;margin-top:0;padding-top:0}
.related-chapter-link{display:block;color:var(--muted);text-decoration:none;font:11px var(--mono)}
.related-chapter h3.related-chapter-link{margin:0;text-transform:uppercase;letter-spacing:.035em}
.related-chapter-link:hover{color:var(--accent)}
.related-group-items{display:grid;gap:10px}
.related-fragment{display:block;margin-top:6px;padding:6px 8px;border-radius:var(--radius);background:var(--bg-subtle);color:var(--ink);text-decoration:none}
.related-fragment:hover{background:var(--sel)}
.related-fragment strong{display:block;font-size:12.5px}
.related-fragment span{display:block;margin-top:2px;color:var(--muted);font-size:11.5px;line-height:1.45}
.related-slide{display:block;margin-top:7px;color:var(--ink);text-decoration:none}
.related-slide-preview{display:block;aspect-ratio:16/9;overflow:hidden;border:2px solid var(--line);border-radius:5px;background:#fff;box-shadow:0 1px 2px #1f23281f;transition:border-color .12s,box-shadow .12s}
.related-slide-preview iframe,.related-slide-preview img{display:block;width:100%;height:100%;border:0;object-fit:contain;pointer-events:none}
.related-slide-caption{display:flex;align-items:center;gap:5px;margin-top:5px}
.related-slide-caption strong{min-width:0;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12.5px}
.related-slide>small{display:block;margin-top:2px;color:var(--muted);font:10.5px var(--mono)}
.related-slide:hover .related-slide-preview{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
.related-saga>p{color:var(--muted);font-size:12px}

/* Diff surface ----------------------------------------------------------- */
.file-diff{scroll-margin-top:calc(var(--top) + 36px);margin:0;background:var(--code-bg)}
.file-head{position:sticky;top:calc(var(--top) + 35px);z-index:5;display:flex;align-items:center;gap:8px;padding:6px clamp(8px,1.4vw,14px);background:var(--bg-subtle);border-bottom:1px solid var(--line)}
.file-head code{font:600 12.5px var(--mono);overflow-wrap:anywhere}
.file-head .counts{margin-left:auto;font:11px var(--mono);color:var(--faint)}
.diff-surface{overflow:auto;background:var(--code-bg)}
.diff-column-head{display:none;grid-template-columns:1fr 1fr;position:sticky;top:calc(var(--top) + 68px);z-index:4;min-width:640px;background:var(--bg-subtle);color:var(--muted);border-bottom:1px solid var(--line-soft);font:10px var(--mono);letter-spacing:.04em;text-transform:uppercase}
.diff-column-head span{padding:3px 10px}
.diff-lines{min-width:640px}
.diff-row{display:grid;grid-template-columns:46px 46px 18px minmax(240px,1fr) 58px;align-items:start;min-height:20px;font:12px/1.5 var(--mono);position:relative;background:var(--code-bg)}
.diff-row[hidden]{display:none}
.diff-row.context{color:var(--ink)}
.diff-row.new{background:var(--add-bg);box-shadow:inset 2px 0 var(--add-line)}
.diff-row.old{background:var(--del-bg);box-shadow:inset 2px 0 var(--del-line)}
.diff-row.event{background:var(--bg-subtle);color:var(--muted)}
.diff-row.selected{outline:2px solid var(--accent);outline-offset:-2px;z-index:1}
.line-no{display:block;padding:1px 8px;text-align:right;color:var(--faint);user-select:none;background:var(--code-gutter);border-right:1px solid var(--line-soft)}
.diff-row.new .line-no{background:var(--add-gutter)}
.diff-row.old .line-no{background:var(--del-gutter)}
.sign{padding:1px 4px;color:var(--muted);text-align:center;user-select:none}
.code-line{padding:1px 10px;white-space:pre;overflow:visible;tab-size:4}
.context-expander{display:grid;grid-template-columns:auto minmax(180px,1fr) auto;align-items:stretch;width:100%;padding:0;background:var(--bg-inset);color:var(--accent);border-block:1px solid var(--line-soft);font:11px var(--mono)}
.context-expander button{min-height:26px;padding:3px 10px;border:0;border-radius:0;background:transparent;color:inherit;font:inherit}
.context-expander button:hover,.context-expander button:focus-visible{background:var(--accent-soft)}
.context-expander .context-expand-all{grid-column:2}
.context-expander button[data-context-expand=down]{grid-column:1;grid-row:1}
.context-expander button[data-context-expand=up]{grid-column:3;grid-row:1}
.diff-surface[data-layout=split] .diff-column-head{display:grid}
.diff-surface[data-layout=split] .diff-lines{display:grid;grid-template-columns:minmax(320px,1fr) minmax(320px,1fr);align-items:stretch}
.diff-surface[data-layout=split] .diff-row{grid-template-columns:46px 46px 18px minmax(180px,1fr);grid-column:auto}
.diff-surface[data-layout=split] .diff-row.context,.diff-surface[data-layout=split] .diff-row.event,.diff-surface[data-layout=split] .context-expander{grid-column:1/-1}
.diff-surface[data-layout=split] .diff-row.old{grid-column:1}
.diff-surface[data-layout=split] .diff-row.new{grid-column:2}
.tok-keyword{color:var(--syntax-keyword)}
.tok-string{color:var(--syntax-string)}
.tok-number{color:var(--syntax-number)}
.tok-comment{color:var(--syntax-comment)}
.tok-type{color:var(--syntax-type)}
.tok-property{color:var(--syntax-property)}
.tok-punctuation{color:var(--syntax-punctuation)}
.replacement{display:none;grid-column:1/-1}

/* Deferred review surfaces ---------------------------------------------- */
.surface-placeholder{display:flex;min-height:240px;align-items:center;justify-content:center;flex-direction:column;gap:7px;padding:28px;color:var(--muted);text-align:center;font-size:12.5px}
.surface-placeholder strong{color:var(--ink);font-size:14px}
.surface-placeholder.compact{min-height:120px;padding:18px 8px}
.surface-placeholder.error{color:var(--red)}
.surface-placeholder.error strong{color:var(--red)}
.surface-placeholder .btn-primary{margin-top:6px}
.surface-spinner{width:18px;height:18px;border:2px solid var(--line);border-top-color:var(--accent);border-radius:50%;animation:surface-spin .8s linear infinite}
@keyframes surface-spin{to{transform:rotate(360deg)}}
[data-surface-next]{display:flex;margin:14px auto;padding:6px 12px;border:1px solid var(--line);border-radius:var(--radius);background:var(--bg);color:var(--accent);font:600 12px var(--ui);text-decoration:none}
[data-surface-next]:hover,[data-surface-next]:focus-visible{border-color:var(--accent);background:var(--accent-soft)}
[data-surface-next][aria-busy=true]{cursor:progress;opacity:.65}

/* Coverage view ---------------------------------------------------------- */
.manifest-view{position:fixed;z-index:20;inset:var(--top) 0 0;background:var(--bg);overflow:auto}
.manifest-wrap{width:min(1180px,100%);margin:auto;padding:0 clamp(12px,2.5vw,28px) 64px}
.manifest-tools{position:sticky;top:0;z-index:4;display:flex;gap:8px;align-items:center;padding:8px 0;background:var(--toolbar-bg);backdrop-filter:blur(6px);border-bottom:1px solid var(--line-soft)}
.manifest-modes{display:flex;gap:2px;padding:2px;border-radius:var(--radius);background:var(--bg-inset)}
.manifest-modes button{background:transparent;color:var(--muted);padding:3px 9px;font-size:12px}
.manifest-modes button[aria-pressed=true]{background:var(--bg);color:var(--ink);box-shadow:0 1px 2px #1f232826}
.manifest-tools .manifest-metric{color:var(--faint);font:11px var(--mono)}
.manifest-tools .tree-search{max-width:280px;margin-left:auto;flex:1}
.manifest-tools input{min-width:0;width:100%;height:26px;padding:0 8px 0 25px;border:1px solid var(--line);border-radius:var(--radius);background:var(--bg);font-size:12px}
.manifest-alert{display:flex;gap:9px;align-items:flex-start;margin:12px 0;padding:10px 12px;border:1px solid var(--danger-line);border-left:3px solid var(--red);border-radius:var(--radius);background:var(--danger-bg);font-size:12.5px}
.manifest-alert .i{color:var(--red);margin-top:1px}
.manifest-alert strong{display:block;margin-bottom:2px}
.manifest-panel{padding-top:6px}
.mtree{padding:2px 0}
.mtree details{display:block}
.mtree summary{display:flex;align-items:center;gap:6px;min-height:24px;padding:0 8px 0 calc(6px + var(--depth,0) * 13px);list-style:none;cursor:pointer;border-radius:var(--radius);color:var(--muted);font:12px var(--mono)}
.mtree summary::-webkit-details-marker{display:none}
.mtree summary:hover{background:var(--bg-subtle)}
.mtree .twisty{width:12px;height:12px;flex:none;color:var(--faint);transition:transform .12s}
.mtree details[open]>summary .twisty{transform:rotate(90deg)}
.manifest-file{border-bottom:1px solid var(--line-soft)}
.manifest-file>summary{color:var(--ink)}
.manifest-file>summary .mfile-name{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.manifest-file.has-gap>summary{background:var(--danger-bg)}
.manifest-file.has-gap>summary .mfile-name{color:var(--red)}
.manifest-file-stats{margin-left:auto;padding-left:10px;color:var(--faint);font:11px var(--mono);white-space:nowrap}
.manifest-file-stats .gap{color:var(--red);font-weight:600}
.manifest-file-detail{margin-left:calc(20px + var(--depth,0) * 13px);border-left:1px solid var(--line-soft)}
.manifest-file-diff{max-height:min(54vh,620px);overflow:auto;border-block:1px solid var(--line-soft)}
.manifest-file-diff .diff-lines{min-width:580px}
.manifest-file-diff .diff-row{grid-template-columns:46px 46px 18px minmax(240px,1fr) 0}
.manifest-map-heading{padding:5px 10px;border-bottom:1px solid var(--line-soft);background:var(--bg-subtle);color:var(--faint);font:600 10px var(--ui);letter-spacing:.045em;text-transform:uppercase}
.manifest-file-detail>.manifest-rows{padding-left:0}
.manifest-rows{padding-left:calc(20px + var(--depth,0) * 13px);border-left:0}
.manifest-row{display:grid;grid-template-columns:minmax(220px,.85fr) minmax(280px,1.15fr);border-top:1px solid var(--line-soft)}
.manifest-row.unmapped{background:var(--danger-bg)}
.manifest-range{display:grid;grid-template-columns:62px minmax(0,1fr);align-items:center;gap:8px;padding:5px 10px;color:var(--ink);text-decoration:none}
.manifest-range:hover{background:var(--sel)}
.manifest-range>span{color:var(--muted);font:11px var(--mono)}
.manifest-range small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--faint);font:11px var(--ui)}
.manifest-owners{display:flex;align-items:center;gap:5px;flex-wrap:wrap;padding:4px 10px;border-left:1px solid var(--line-soft)}
.manifest-owners>a{display:inline-flex;align-items:center;gap:5px;max-width:100%;padding:2px 7px;border:1px solid var(--line);border-radius:99px;background:var(--bg);color:var(--ink);text-decoration:none;font-size:11.5px}
.manifest-owners>a:hover{border-color:var(--accent);color:var(--accent)}
.manifest-owners>a .i{width:12px;height:12px;color:var(--faint)}
.manifest-owners>a strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:500}
.manifest-owners>a small{color:var(--faint)}
.manifest-owners>a.manifest-slide-owner{display:grid;grid-template-columns:72px minmax(0,1fr);gap:7px;padding:3px;border-radius:5px}
.manifest-slide-preview,.manifest-target-slide-preview{display:block;aspect-ratio:16/9;overflow:hidden;border:1px solid var(--line);border-radius:3px;background:#fff}
.manifest-slide-preview iframe,.manifest-slide-preview img,.manifest-target-slide-preview iframe,.manifest-target-slide-preview img{display:block;width:100%;height:100%;border:0;object-fit:contain;pointer-events:none}
.manifest-slide-copy{display:grid;min-width:0;align-content:center}
.manifest-owners>a .manifest-slide-copy strong,.manifest-owners>a .manifest-slide-copy small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.manifest-owner-missing,.manifest-gap{color:var(--red);font-size:11.5px}
.manifest-target-title{display:flex;min-width:0;align-items:baseline;gap:7px}
.manifest-target-title strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:500 12.5px var(--ui)}
.manifest-target-title small{color:var(--faint);font:11px var(--mono)}
.manifest-target{border-bottom:1px solid var(--line-soft)}
.manifest-target>summary{color:var(--ink)}
.manifest-target-slide-preview{width:64px;flex:none}
.manifest-target-files{padding-left:20px}
.manifest-target-file{border-top:1px solid var(--line-soft)}
.manifest-target-file>summary{padding-left:8px;color:var(--ink)}
.manifest-target-file>summary code{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:11.5px var(--mono)}
.manifest-target-file-detail{margin-left:20px;border-left:1px solid var(--line-soft)}
.manifest-target-links{display:flex;align-items:center;gap:5px;min-height:30px;padding:4px 8px;border-top:1px solid var(--line-soft);color:var(--faint);font:11px var(--ui)}
.manifest-target-links>a{padding:2px 6px;border:1px solid var(--line);border-radius:99px;color:var(--muted);text-decoration:none;font:11px var(--mono)}
.manifest-target-links>a:hover{border-color:var(--accent);color:var(--accent)}
.manifest-target-links .manifest-full-diff{display:inline-flex;align-items:center;gap:4px;margin-left:auto;border:0;color:var(--accent);font-family:var(--ui)}
.manifest-target-links .manifest-full-diff .i{width:12px;height:12px}
.manifest-open-saga{display:inline-flex;align-items:center;gap:5px;padding:6px 10px;color:var(--accent);text-decoration:none;font-size:12px}
.manifest-empty,.manifest-filter-empty{padding:18px;color:var(--muted);text-align:center;font-size:12.5px}
.manifest-orphans{margin-top:20px;padding-top:14px;border-top:1px solid var(--line)}
.manifest-orphans h2{margin:0;font:600 13px var(--ui)}
.manifest-orphans>p{margin:2px 0 8px;color:var(--muted);font-size:12px}
.manifest-orphan{display:flex;align-items:center;gap:8px;margin-top:6px;padding:8px 10px;border:1px solid var(--danger-line);border-radius:var(--radius);background:var(--danger-bg);font-size:12.5px}
.manifest-orphan>.i{color:var(--red);flex:none}
.manifest-orphan strong,.manifest-orphan small{display:block}
.manifest-orphan small{color:var(--muted);font-size:11.5px}
.manifest-orphan a{margin-left:auto;color:var(--accent);text-decoration:none;white-space:nowrap}

/* History ---------------------------------------------------------------- */
.activity-wrap{width:100%;padding:20px 16px 72px}
.activity-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:24px;padding-bottom:18px;border-bottom:1px solid var(--line)}
.activity-heading .eyebrow{margin:0 0 2px;color:var(--faint);font:600 10px var(--mono);letter-spacing:.08em;text-transform:uppercase}
.activity-heading h1{margin:0;font-size:25px;line-height:1.2;letter-spacing:-.025em}
.activity-heading p:not(.eyebrow){margin:5px 0 0;color:var(--muted);font-size:13px}

/* Responsive ------------------------------------------------------------- */
@media(max-width:1050px){
.code-workspace{grid-template-columns:1fr}
.related-saga{position:static;max-height:none;border-left:0;border-top:1px solid var(--line)}
.code-workspace.related-hidden{grid-template-columns:1fr}
.diff-surface[data-layout=split] .diff-lines{display:block}
.diff-surface[data-layout=split] .diff-row{grid-template-columns:46px 46px 18px minmax(240px,1fr) 58px}
.diff-surface[data-layout=split] .diff-row,.diff-surface[data-layout=split] .diff-row.old,.diff-surface[data-layout=split] .diff-row.new{grid-column:auto}
.diff-surface[data-layout=split] .diff-column-head{display:none}
}
@media(max-width:780px){
.topbar{padding:0 8px;gap:6px}
.brand span{display:none}
.view-tab{padding:0 8px}
.shell,.shell.code-mode{display:block}
.sidebar{position:static;height:auto;max-height:42vh;padding:8px}
.shell.code-mode .sidebar{position:fixed;z-index:25;top:var(--top);bottom:0;left:0;width:min(300px,86vw);max-height:none;height:auto;box-shadow:12px 0 30px #1f232826;transform:none;transition:transform .18s}
.shell.code-mode.tree-hidden .sidebar{transform:translateX(-105%);padding:8px}
.content{padding:16px 12px 90px}
.code-mode .content{padding:0 0 40px}
.chapter-body{padding-left:10px}
.section .section{padding-left:8px}
.file-head{top:calc(var(--top) + 35px);flex-wrap:wrap}
.drawer-body .file-head{top:0}
.code-toolbar{overflow-x:auto}
.diff-row{grid-template-columns:38px 38px 16px minmax(200px,1fr) 52px}
.code-toolbar .metric{display:none}
.attached-file>summary{align-items:flex-start;flex-wrap:wrap}
.attached-file-note{white-space:normal}
.attached-file-counts{width:100%;margin-left:26px}
.manifest-tools{align-items:stretch;flex-direction:column}
.manifest-tools .tree-search{max-width:none;margin:0}
.manifest-row{grid-template-columns:1fr}
.manifest-owners{border-left:0;border-top:1px solid var(--line-soft)}
.manifest-target-files{padding-left:8px}
.manifest-target-links{align-items:flex-start;flex-wrap:wrap}
.manifest-target-links .manifest-full-diff{width:100%;margin-left:0}
.activity-wrap{padding:20px 12px 64px}
.activity-heading{align-items:flex-start;flex-direction:column;gap:8px}
.diff-drawer{width:100vw}
}
`
