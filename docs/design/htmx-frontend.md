# The reviewer frontend on htmx

Status: in progress on `feature/htmx-frontend`.

## Why

Every page of the reviewer was a whole document. On `app.saga` that was about
2.5 MB of HTML per page. 2.4 MB of it was the deck viewer: every slide of every
deck, hidden, with 1,604 `<template>` elements, 107 slide iframes and about 16k
elements. On top of that came 90 KB of inline CSS and a 152 KB `app.js` served
`no-store`. Following a link re-downloaded, re-parsed and re-initialised all of
it:

- about 205 requests, of which 107 were iframes and about 95 were the same SVGs
  fetched again for hotspot measurement;
- about 1.2 s of main-thread work;
- the load event at about 1 s.

Yet the part of the page that actually differs between two pages is a few KB.
The server renders any page in about 20 ms, so nearly all of that time was spent
in the browser.

The reviewer becomes an htmx application:

- **The shell and everything every page shares load once per session.** That
  covers the stylesheet, the scripts, the icon sprite, the sidebar and the deck
  viewer.
- **Following a link swaps only what the new page changes.** That is the
  content, the sidebar's state, the header's tabs and the title.

Every page is still a real URL that renders completely on its own. Links stay
`<a href>` links, and with JavaScript off the reviewer works as it did before.

### Why htmx, and why vendored

htmx is one file of about 52 KB minified, with no dependencies and no build step.
It is BSD Zero Clause licensed. It works with server-rendered HTML, which is what
this server already produces through `html/template`. So the server stays the
only renderer, and there is no second copy of the markup in JavaScript (which a
React or other client-side renderer would need).

CONTRIBUTING asks that the core stay dependency-light and local:

- htmx is vendored into the binary, like the Go font and the Lucide icons
  (`internal/server/assets/htmx/`, with its `LICENSE` and a `VERSION` file).
- It is served from the reviewer's own origin. There is no CDN, and nothing is
  fetched at runtime.

The only extension used is `preload`, from the same project and under the same
license, vendored beside htmx.

## The persistent shell

A request without `HX-Request` gets the full document, rendered by the `page`
layout. Only the first load of a session receives this.

```
<head>  title · meta htmx-config · /assets/<v>/app.css · /assets/<v>/theme.js (blocking)
        /assets/<v>/htmx.min.js, preload.min.js, app.js (defer)
<body hx-boost="true" hx-target="#page" hx-swap="innerHTML" hx-history="false" hx-ext="preload">
  icon sprite
  <header class="topbar"> brand · opening badge · #page-tabs (swapped) </header>
  <div class="shell" data-shell>
    <aside class="sidebar"> #sidebar-nav: every section, every feature's subtree,
                            both sides' second sections · #nav-state (swapped) </aside>
    <main class="content">
      <div id="page" hx-history-elt> page-content (swapped) </div>
      <div id="view-slides"> #decks-<fingerprint>: the deck viewer, loaded after first paint, hx-preserve </div>
    </main>
  </div>
  #page-surfaces (swapped): the Change and Coverage views of this page
  drawer · #page-announcer (aria-live)
</body>
```

### The sidebar no longer depends on the page

The sidebar's structure used to change from page to page:

- Only the feature being read had its subtree.
- The Review side swapped the Features section for Reviews.
- In-page anchors were written `#…` on the overview and `/#…` elsewhere.

Now the sidebar is rendered the same on every page:

- Every feature carries its subtree, collapsed unless it is the page's feature.
  A closed feature now shows a disclosure. Opening it is the reader's own action
  and is never stored.
- Both sides' second sections are present. The other side's is hidden.
- Every link is page-independent (`/#target-…`, `/features/x#…`).

A page's sidebar is therefore only state:

- which rows are current (`aria-current`, `.current`);
- which places are open (`aria-expanded`, `hidden`);
- which side is shown.

The server renders that state into the markup of a full load, so the sidebar
still works with JavaScript off. The `#nav-state` element carries the same state
for a swap.

## What swaps, and how the server answers

A boosted link is `<a href>` inside `hx-boost`. Following one sends
`GET <url>` with `HX-Request: true` and `HX-Target: page`. For a history
restore, htmx sends `HX-History-Restore-Request: true` instead of a target.

`app.page` and the review handlers call the same `shell()`. Then:

- A full request executes `page`.
- An htmx request executes `page-partial`.

Both answers carry `Vary: HX-Request`, so a cache never serves one in place of
the other.

The templates follow the usual `html/template` shape: one named `{{define}}` per
region, composed by the layout. No markup appears twice:

| block           | full load                       | partial                                         |
|-----------------|---------------------------------|-------------------------------------------------|
| `title`         | inside `<title>`                | a top-level `<title>`, which htmx applies       |
| `page-content`  | inside `#page`                  | the response body, swapped into `#page`         |
| `page-tabs`     | inside the topbar               | `hx-swap-oob="true"`                            |
| `page-surfaces` | after the shell                 | `hx-swap-oob="true"`                            |
| `nav-state`     | inside the sidebar              | `hx-swap-oob="true"`                            |
| `sidebar-nav`   | inside the sidebar              | `hx-swap-oob` **only when the shell is stale**  |
| `deck-host`     | inside `#view-slides`           | `hx-swap-oob` **only when the shell is stale**  |

Each block writes its own `hx-swap-oob` attribute when `.Partial` is set. That
one flag is the only difference between the two renderings.

### Freshness

The server already checks every request against the Saga as it was when the
request arrived, and a partial is rendered by the same code, so it takes the same
check. The persistent regions (the sidebar and the deck viewer) are the new risk:
the Saga can change after they loaded. The shell therefore records the
fingerprint of the Saga state it was built from, and handles a change like this:

1. **The browser reports its fingerprint.** It is `data-saga-shell` on
   `#sidebar-nav`. `app.js` sends it with every htmx request as
   `X-Saga-Shell`, from one `htmx:configRequest` listener.
2. **The server compares it with its own.**
   - If they match, the response carries only the page's own blocks.
   - If they differ, the server also sends `sidebar-nav` and `deck-host`
     out of band. The new deck host has a new id (`decks-<fingerprint>`), so
     `hx-preserve` does not keep the old viewer in place of the new one.
3. **The browser clears its caches when the fingerprint changes.** Each page root
   carries `data-saga-state`. When it changes, `app.js` drops every response it
   cached: chapters, explanations, anchor places, linked code and file diffs.
   Those caches used to die with the page. They now live for the session, so the
   page's own fingerprint scopes them instead.

Taken together, no region shows Saga content older than the request that
produced it.

A later step can push staleness instead of waiting for the next navigation: an
SSE stream from the watcher that tells stale regions to fetch themselves (the
htmx `sse` extension). The navigation work does not depend on it.

## Heavy parts load once, early, in the background

The deck viewer is `GET /decks`, which serves the viewer HTML the server already
renders once per Saga state. The host element in the shell is:

```html
<div id="decks-<fp>" class="sidebar-slide-surface" hx-get="/decks" hx-trigger="load"
     hx-swap="innerHTML" hx-preserve data-saga-shell="<fp>">…placeholder…</div>
```

- **When it loads.** The `load` trigger fires when htmx initialises after
  `DOMContentLoaded`, so the page has already been parsed and painted. The
  download then happens in the background, and the swap lands once per session.
  The viewer is hidden until a reader opens a slide. By then it is in place, so
  no slide appears late.
- **Its slide iframes.** They load as before, eagerly and sandboxed on an opaque
  origin (`authoredContentPolicy` is unchanged), but once per session instead of
  once per navigation.
- **The hotspot SVG measurements.** `prepareSVGElementHotspots` also runs once
  per session.
- **HTTP caching.** `/decks` answers with `ETag: <fingerprint>` and
  `Cache-Control: no-cache`, so a new session that finds the Saga unchanged
  revalidates with a 304. `/f/…` files get the same validator treatment, so the
  iframes and the measurement fetches share one cached copy.
- **Keeping it.** The viewer never sits inside `#page`, so an ordinary swap never
  touches it. `hx-preserve` is there for any swap that does include it.

Sidebar thumbnails keep `loading="lazy"`. They are in the persistent sidebar, so
each thumbnail loads once when it first scrolls into view and then stays.

### Preloading the next page

The `preload` extension fetches a link's partial on `mousedown`. That gives about
the 80–120 ms between press and click as a head start.

The extension relies on the browser's HTTP cache, so partials are
`Cache-Control: private, max-age=2`. A preloaded partial is therefore reused only
within two seconds of the press that fetched it. Full pages stay uncached. If the
measurements show no gain, preload is removed.

## app.js: run-once setup and per-swap setup

`app.js` keeps its single closure, and its helpers and state are unchanged. Its
initialisation is split.

**Run once, when the script first executes:**

- the document-wide delegated listeners (`click`, `keydown`, `pointerover`,
  `focusin`, `toggle`, `input`, `submit` for review forms, `hashchange`,
  `popstate`, `resize`, `fullscreenchange`) and the layers `MutationObserver`;
- `loadLayers`, which is one request per session in compare mode;
- the htmx event hooks:
  - `htmx:configRequest` adds `X-Saga-Shell`;
  - `htmx:confirm` keeps links that only move within the current page, such as
    `?view=` or `#anchor` on the same page, as in-page actions, and keeps
    drawer-opening links from navigating;
  - `htmx:beforeRequest` clears `data-shell-ready` and records the scroll
    position;
  - `htmx:historyCacheMiss` cancels the fetch when the entry is the page that is
    already shown, for example `?view=` or a hash within one page;
  - `htmx:responseError` falls back to a full navigation.

**Per swap, through `htmx.onLoad(root => …)`.** This runs for the initial
`<body>` and then for each element a swap inserts:

- **`[data-page-root]`, the page.** This covers:
  - landmarks, diff citations, deferred fragments, directories, context and
    highlighting, ERD visuals and diff layout;
  - the initial view from `?view=`, and the hash's landmark or deck slide;
  - coverage totals, review annotations and review async state;
  - focus, scroll and the announcement;
  - finally `data-shell-ready`.
- **`[data-deck-host]`, the deck viewer once it arrives.** Landmarks, SVG
  hotspots and the slide for the current hash.
- **`#nav-state`.** It applies the sidebar state.
- **`#page-tabs` and `#page-surfaces`.** Tab and view state for the page, so
  `setView` sees the new page's views.

What was attached per page moves to one of three places:

- **Delegated listeners.** For example, the duplicate `input` listeners on the
  filters are removed.
- **Per-root setup.** For example, the per-directory filters.
- **An explicit teardown on `htmx:beforeSwap` of `#page`.** This aborts the old
  page's review surface requests, closes the drawer and clears
  `activeFragment`.

The review annotation layer is built per page, and its document-level keyboard
listener becomes a single delegated one.

## Versioned, cacheable assets

`app.css` (the former inline `<style>`), `theme.js`, `app.js`, `htmx.min.js` and
`preload.min.js` are served from `/assets/<hash>/<name>`:

- **`<hash>` is a SHA-256 prefix of the file's own bytes**, computed once at
  startup, so every build gets new URLs.
- **The response header is `Cache-Control: public, max-age=31536000, immutable`.**
  A reload or a new session costs no requests for them.
- **An unknown hash returns 404**, so a stale URL never serves new bytes under an
  old name.
- **`/app.js` and `/theme.js` stay** as `no-store` aliases for anything that
  still links them.

The CSP is unchanged: `script-src 'self'` covers the vendored scripts, and
`style-src 'self' 'unsafe-inline'` covers the stylesheet. htmx is configured
through `<meta name="htmx-config">`:

```json
{"allowEval":false,"allowScriptTags":false,"includeIndicatorStyles":false,
 "historyCacheSize":0,"defaultSettleDelay":0,"scrollIntoViewOnBoost":false,
 "selfRequestsOnly":true,"refreshOnHistoryMiss":false}
```

## History, focus, scroll and accessibility

**History.** Boosted links push their URL, as a normal link does. htmx never
snapshots a page into storage: `hx-history="false"` on `<body>` and a
`historyCacheSize` of 0 disable it. A 2.5 MB page overflowed the store, and a
snapshot could show content older than the Saga. So a back or forward step is a
cache miss, and htmx asks the server for the partial:

- It sends `HX-Request` with `HX-History-Restore-Request`.
- It swaps the answer into `#page`, the `hx-history-elt`, through the same
  `swap()` a link uses, so the out-of-band tabs, surfaces and sidebar state apply
  as well.

`app.js` also pushes its own entries, for `?view=` tabs and in-page anchors. It
marks them as htmx entries too, so htmx handles every entry. When the entry is
the page already shown, the `htmx:historyCacheMiss` hook cancels the fetch, and
`app.js` restores the view or anchor as before.

**Scroll.**

- A navigation starts at the top of the page, or at its hash's landmark through
  the existing lazy reveal.
- A back or forward step returns to the scroll position recorded when the page
  was left. The positions are kept in memory, per history entry.

**Focus and announcement.** After a navigation (not after the first load or a
same-page action):

- Focus moves to the new page's `h1`, which gets `tabindex="-1"`.
- `#page-announcer`, a polite live region, says the page's title.
- The `<title>` names the page as well as the Saga.
- An open drawer is closed first, and focus is never left on a removed element.

**Without JavaScript.** Nothing changes. Every URL renders the whole page,
sidebar links are plain links, and the sidebar state is in the markup.

## Links that are not pages

`hx-boost` covers every local link, so a boosted request can reach something
that is not a page, such as `/f/…` files or `/api/…`. These responses carry
`HX-Redirect` to the same URL, and the browser loads them as a normal
navigation.

Anchors that `app.js` treats as actions (fragment drawers, history, surface
paging) are recognised in `htmx:confirm`, and the boosted request is cancelled.
Review decision and comment forms keep their async submit and carry
`hx-boost="false"`. Directory filter forms are GET forms and are boosted like
links.

## How it is tested

- **Go tests (`internal/server`).**
  - A partial response:
    - holds the page's content and the out-of-band blocks;
    - has no `<html>`, no sidebar tree and no deck viewer;
    - carries `Vary: HX-Request`.
  - A stale `X-Saga-Shell` adds the sidebar and a new deck host.
  - The full page and the partial render `page-content` byte for byte the same.
  - Assets:
    - hashed assets are immutable;
    - an unknown hash returns 404;
    - `/decks` answers 304 for its ETag.
  - A boosted request for a non-page answers `HX-Redirect`.
  - The existing layout, budget and review tests move to the new structure where
    they pinned the old one.
- **E2E (Playwright).**
  - A navigation suite:
    - clicking between feature, persona, story, overview, technical and review
      pages changes the heading, the title, the sidebar's current row and open
      feature, and the header tabs;
    - `performance.timeOrigin` does not change, so no document was loaded;
    - the deck viewer and its iframes are the same nodes before and after;
    - no request for `app.js`, the stylesheet or `/decks` repeats;
    - back and forward restore the content and the sidebar;
    - focus lands on the `h1`;
    - a deep link and a refresh render the same page as the swap.
  - `feature-list.spec.ts` keeps its JavaScript-disabled test. Its "open feature"
    is now the one whose subtree is shown.
  - Review pages keep their decision, comment and annotation tests, reached both
    by deep link and by navigation.
- **Measurements.** `/tmp/browser-measure` records first paint, time from click
  to visible heading, requests per navigation and main-thread time per
  navigation, before and after, and the results are recorded here.
