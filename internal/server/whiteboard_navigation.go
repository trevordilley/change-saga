package server

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/twentyideas/changesaga/internal/requirements"
)

// Latest ERD links enter an authored board when that source is embedded there.
// Explicit saved revisions remain independently addressable for compatibility.
// A missing or invalid board never takes a working definition page away.
func (a *app) technicalDefinitionPage(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("kind") == requirements.KindERD && r.URL.Query().Get("revision") == "" {
		if document := a.outlineDocument(r.Context()); document != nil {
			if inventory, err := requirements.LoadInventory(a.root, document.Manifest.ID); err == nil {
				if destination := whiteboardSourceDestination(a.makeWhiteboardView(inventory), r.PathValue("id")); destination != "" {
					if r.Header.Get("HX-Boosted") == "true" {
						w.Header().Set("HX-Location", destination)
						w.WriteHeader(http.StatusNoContent)
					} else {
						http.Redirect(w, r, destination, http.StatusFound)
					}
					return
				}
			}
		}
	}
	a.page(w, r)
}

func whiteboardSourceDestination(board *whiteboardView, id string) string {
	if board == nil || board.Visual == "" {
		return ""
	}
	var paths []string
	for _, frame := range board.Frames {
		source, err := url.Parse(frame.Href)
		if err == nil && source.Path == technicalHref(requirements.KindERD, id, "") {
			paths = append(paths, frame.Path)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	sort.Strings(paths)
	href := board.Href
	if href == "" {
		href = whiteboardHref("data-model", "")
	}
	return whiteboardFocusHref(href, paths[0])
}

const whiteboardNavigationJavaScript = `
  function prepareWhiteboardNavigation(board, focus) {
    const signal = pageScope.signal;
    const boardURL = new URL(board.dataset.whiteboardUrl || location.href, location.href);
    const sameDocument = url => url.pathname === boardURL.pathname &&
      (url.searchParams.get('revision') || '') === (boardURL.searchParams.get('revision') || '');
    const entries = qa('[data-whiteboard-frame]', board).map(row => {
      const link = q('a[href]', row);
      const href = row.dataset.whiteboardSourceHref || link?.dataset.whiteboardSourceHref || link?.getAttribute('href');
      if (!href) return null;
      const source = new URL(href, location.href);
      return {path:row.dataset.whiteboardFrame, source, row};
    }).filter(Boolean).sort((a,b) => a.path < b.path ? -1 : a.path > b.path ? 1 : 0);
    const paths = new Set([...entries.map(entry => entry.path), ...qa('[data-whiteboard-section]', board).map(row => row.dataset.whiteboardSection), ...qa('[data-instance-path]', board).map(row => row.dataset.instancePath)]);
    let restoring = false;
    const destination = (path, fitAll = false) => {
      const url = new URL(location.href);
      url.pathname = boardURL.pathname;
      const revision = boardURL.searchParams.get('revision');
      if (revision) url.searchParams.set('revision', revision); else url.searchParams.delete('revision');
      if (fitAll) url.searchParams.set('view', 'all'); else url.searchParams.delete('view');
      url.hash = '';
      if (path) url.searchParams.set('focus', path); else url.searchParams.delete('focus');
      return url;
    };
    function entryFor(link) {
      let url;
      const target = link.dataset.documentationTarget?.split(':');
      if (target?.length === 5 && target[3] === 'erd') {
        url = new URL('/technical/erd/' + encodeURIComponent(target[4]), location.href);
        const revision = link.dataset.documentationRevision;
        if (revision) url.searchParams.set('revision', revision.split(':revision:').pop());
      } else {
        const href = link.dataset.whiteboardSourceHref || link.getAttribute('href');
        if (!href) return null;
        try { url = new URL(href, location.href); } catch { return null; }
      }
      if (url.origin !== location.origin) return null;
      const matches = entries.filter(entry => entry.source.pathname === url.pathname &&
        (!url.searchParams.get('revision') || entry.source.searchParams.get('revision') === url.searchParams.get('revision')));
      const owner = link.closest('[data-whiteboard-frame]')?.dataset.whiteboardFrame;
      const instance = link.dataset.instancePath;
      const enclosing = instance && matches.filter(entry => instance.startsWith(entry.path + '/')).sort((a,b) => b.path.length - a.path.length)[0];
      return matches.find(entry => entry.path === owner) || enclosing || matches[0];
    }
    // Keep source provenance on each rewritten link, including its exact pin.
    // Restoring retained sidebar links prevents stale mappings after a page swap.
    const rewritten = [];
    for (const link of qa('a[href]')) {
      const entry = entryFor(link);
      if (!entry) continue;
      const href = link.getAttribute('href');
      link.dataset.whiteboardSourceHref = href;
      link.setAttribute('href', destination(entry.path).pathname + destination(entry.path).search);
      rewritten.push({link, href});
    }
    signal.addEventListener('abort', () => {
      for (const {link, href} of rewritten) {
        link.setAttribute('href', href);
        delete link.dataset.whiteboardSourceHref;
      }
    }, {once:true});
    function selection(path) {
      board.dataset.whiteboardFocused = path;
      const selected = entries.find(entry => entry.path === path);
      // A retained sidebar may not yet contain this named board's outline.
      // Keep the document current when its focused section has no sidebar row.
      const sectionListed = path && qa('.doc-tree a.doc-link[href]').some(link => {
        const url = new URL(link.getAttribute('href'), location.href);
        return sameDocument(url) && url.searchParams.get('focus') === path;
      });
      for (const row of qa('.doc-tree [data-nav-row]')) {
        const link = q(':scope > a.doc-link', row);
        if (!link) continue;
        const entry = entryFor(link);
        const url = new URL(link.getAttribute('href'), location.href);
        if (!entry && !sameDocument(url)) continue;
        const active = entry ? selected?.source.pathname === entry.source.pathname : (url.searchParams.get('focus') || '') === (sectionListed ? path : '');
        row.classList.toggle('current', Boolean(active));
        if (active) link.setAttribute('aria-current', 'page'); else link.removeAttribute('aria-current');
      }
    }
    function remember(path, fitAll = false) {
      if (restoring || (path && !paths.has(path))) return;
      const url = destination(path, fitAll);
      if (url.href !== location.href) history.pushState({htmx:true, whiteboard:true}, '', url);
      selection(path);
    }
    function navigate(path) {
      if (path && !paths.has(path)) return;
      focus(path);
      remember(path);
    }
    function restore() {
      if (!board.isConnected || !sameDocument(new URL(location.href))) return;
      const path = new URL(location.href).searchParams.get('focus') || '';
      restoring = true;
      focus(paths.has(path) ? path : '', new URL(location.href).searchParams.get('view') === 'all');
      selection(paths.has(path) ? path : '');
      restoring = false;
    }
    board.addEventListener('whiteboard:focus', event => remember(event.detail.path, event.detail.fitted), {signal});
    document.addEventListener('click', event => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || !board.isConnected) return;
      const link = event.target.closest('a[href],[data-documentation-target]');
      if (!link || link.hasAttribute('data-whiteboard-evidence') || link.closest('.diff-drawer, [data-whiteboard-viewport]') || link.hasAttribute('download') || (link.getAttribute('target') && link.getAttribute('target') !== '_self')) return;
      const entry = entryFor(link);
      let path = entry?.path;
      if (path === undefined && link.matches('a[href]')) {
        const url = new URL(link.getAttribute('href'), location.href);
        if (url.origin === location.origin && sameDocument(url)) {
          path = url.searchParams.get('focus') || '';
          if (path && !paths.has(path)) return;
        }
      }
      if (path === undefined) return;
      event.preventDefault(); event.stopImmediatePropagation();
      navigate(path);
      q('[data-whiteboard-viewport]', board)?.focus({preventScroll:true});
    }, {capture:true, signal});
    // Run after the viewer's capture handler so a drag cannot activate a
    // source link, and a frame keeps its own repeated-instance identity.
    board.addEventListener('click', event => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const node = event.target.closest('[data-instance-path]');
      if (!node?.closest('[data-whiteboard-viewport]') || node.hasAttribute('data-frame-path') ||
          !node.hasAttribute('data-documentation-target') || node.dataset.documentationTarget.split(':')[3] !== 'erd') return;
      const entry = entryFor(node);
      if (!entry) return;
      event.preventDefault(); event.stopImmediatePropagation();
      navigate(entry.path);
    }, {signal});
    addEventListener('popstate', restore, {signal});
    restore();
  }
`
