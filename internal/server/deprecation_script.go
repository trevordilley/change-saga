package server

// Uses the shared, focus-managed drawer. Merely hovering never fetches target
// health or code evidence; late responses cannot reopen a closed drawer.
const deprecationJavaScript = `
  let deprecationRequest = 0;
  let deprecationTooltip = null;
 let deprecationTooltipOwner = null;
  function hideDeprecationPreview() {
    deprecationTooltip?.remove(); deprecationTooltip = null;
 deprecationTooltipOwner?.removeAttribute("aria-describedby"); deprecationTooltipOwner = null;
  }
  function previewDeprecation(button) {
    hideDeprecationPreview();
    if (!button?.dataset.deprecationPreview) return;
    const tip = document.createElement('div');
    tip.className = 'deprecation-tooltip'; tip.id = 'deprecation-preview'; tip.setAttribute('role','tooltip');
    tip.textContent = button.dataset.deprecationPreview;
    document.body.append(tip); deprecationTooltip = tip;
    tip.addEventListener('pointerleave', event => { if (!button.contains(event.relatedTarget)) hideDeprecationPreview(); });
    button.setAttribute('aria-describedby', tip.id); deprecationTooltipOwner = button;
    const box = button.getBoundingClientRect();
    tip.style.left = Math.max(8, Math.min(innerWidth - 328, box.left)) + 'px';
    tip.style.top = Math.max(8, Math.min(innerHeight - tip.offsetHeight - 8, box.bottom + 6)) + 'px';
  }
  async function openDeprecation(button) {
    const opener = q('.diff-drawer.open') ? drawerOpener : button;
    const request = ++deprecationRequest;
    hideDeprecationPreview(); restoreDrawerContent();
    configureDrawer('deprecations', 'Deprecation details');
    const body = q('.drawer-body'); body.textContent = 'Loading deprecation…';
    showDrawer(opener);
    try {
      const response = await fetch(button.dataset.deprecationDetail, {credentials:'same-origin', signal:pageScope.signal});
      const html = await response.text();
      if (request !== deprecationRequest || q('.diff-drawer.open')?.dataset.drawerMode !== 'deprecations') return;
      if (!response.ok) throw new Error('The deprecation could not be loaded.');
      body.innerHTML = html;
    } catch (error) {
      if (error.name === 'AbortError' || request !== deprecationRequest || q('.diff-drawer.open')?.dataset.drawerMode !== 'deprecations') return;
      body.textContent = 'The deprecation could not be loaded. Close this panel and try again.';
    }
  }
  function showDeprecationList(button) {
    const summary = button.closest('[data-deprecation-summary]') || q('[data-deprecation-summary]', button.closest('[data-whiteboard-view]'));
    const ids = button.dataset.deprecationIds?.split(' ');
    const entries = qa('[data-deprecation-entry]', summary).filter(entry => !ids || ids.includes(entry.dataset.deprecationId));
    deprecationRequest++; hideDeprecationPreview(); restoreDrawerContent();
    configureDrawer('deprecations', 'Deprecations on this content');
    const body = q('.drawer-body');
    const list = document.createElement('ul'); list.className = 'deprecation-results';
    const board = summary.closest('[data-whiteboard-view]');
    for (const entry of entries) {
      const row = document.createElement('li');
      row.append(...Array.from(entry.children).filter(child => !child.hasAttribute('data-deprecation-target')).map(child => child.cloneNode(true)));
      for (const target of qa('[data-deprecation-target]', entry)) {
        if (board && target.dataset.deprecationElement && target.dataset.deprecationRevision?.endsWith(':revision:' + board.dataset.whiteboardRevision)) {
          const link = document.createElement('a');
          const url = new URL(board.dataset.whiteboardUrl, location.origin); url.searchParams.set('focus', target.dataset.deprecationElement);
          link.href = url.pathname + url.search; link.textContent = 'Show ' + target.dataset.deprecationElement;
          row.append(link);
        } else if (!board) {
          const node = document.getElementById(target.dataset.deprecationAnchor || '');
          if (node) { const link = document.createElement('a'); link.href = '#' + node.id; link.textContent = 'Show affected content'; row.append(link); }
        }
      }
      list.append(row);
    }
    body.append(list);
    const all = document.createElement('a'); all.href = '/deprecations'; all.textContent = 'Browse all deprecations'; body.append(all);
    showDrawer(button);
  }
  const preparedDeprecationSummaries = new WeakSet();
  function prepareDeprecations(root) {
    for (const summary of within(root, '[data-deprecation-summary]')) {
      if (preparedDeprecationSummaries.has(summary)) continue;
      preparedDeprecationSummaries.add(summary);
      const entries = qa('[data-deprecation-entry]', summary);
      const badge = q('[data-deprecation-list]', summary);
      if (badge) badge.dataset.deprecationPreview = entries.length === 1 ? entries[0].dataset.deprecationState + ': ' + entries[0].dataset.deprecationPreview : entries.length + ' recorded deprecations. Open to inspect each target.';
      const board = summary.closest('[data-whiteboard-view]');
      if (board) { installWhiteboardDeprecations(board, entries); continue; }
      const fragment = summary.closest('.fragment');
      if (!fragment) continue;
      for (const entry of entries) for (const target of qa('[data-deprecation-target]', entry)) {
        const anchor = qa('[data-landmark-target]', fragment).find(node => node.dataset.landmarkAnchor && node.dataset.landmarkAnchor === target.dataset.deprecationAnchor);
        const owner = anchor && (q('[data-landmark-visual="'+CSS.escape(anchor.dataset.landmarkAnchor)+'"]', fragment) || document.getElementById(anchor.dataset.landmarkAnchor));
        if (owner instanceof HTMLElement && !owner.querySelector('[data-deprecation-marker]')) owner.append(deprecationMarker(entry));
      }
    }
  }
  function deprecationMarker(entry) {
    const button = document.createElement('button'); button.type = 'button'; button.className = 'deprecation-marker';
    button.dataset.deprecationMarker = entry.dataset.deprecationId;
    button.dataset.deprecationDetail = entry.dataset.deprecationHref;
    button.dataset.deprecationPreview = entry.dataset.deprecationState + ': ' + entry.dataset.deprecationPreview;
    button.dataset.deprecationState = entry.dataset.deprecationState;
    button.setAttribute('aria-label', 'Recorded deprecation: ' + entry.dataset.deprecationPreview);
    button.textContent = '!'; return button;
  }
  function installWhiteboardDeprecations(board, entries) {
    const metadata = q('[data-whiteboard-metadata]', board);
    const viewport = q('[data-whiteboard-viewport]', board), svg = q('svg', viewport);
    if (!metadata || !svg) return;
    const geometry = new Map(JSON.parse(metadata.dataset.whiteboardMetadata).map(item => [item.path,item]));
    const layer = document.createElement('div'); layer.className = 'deprecation-marker-layer'; board.append(layer);
    const markers = [];
    const byNode = new Map();
    for (const entry of entries) for (const target of qa('[data-deprecation-target]', entry)) {
      const boardTarget = 'urn:change-saga:' + target.dataset.deprecationTarget.split(':')[2] + ':whiteboard:' + board.dataset.whiteboardId;
      let nodes = [];
      if (target.dataset.deprecationTarget === boardTarget && target.dataset.deprecationElement) {
        if (!target.dataset.deprecationRevision.endsWith(':revision:' + board.dataset.whiteboardRevision)) continue;
        nodes = qa('[data-instance-path]', svg).filter(node => node.dataset.instancePath === target.dataset.deprecationElement);
      } else if (target.dataset.deprecationTarget !== boardTarget) {
        nodes = qa('[data-instance-path][data-documentation-target]', svg).filter(node => node.dataset.documentationTarget === target.dataset.deprecationTarget);
      }
      for (const node of nodes) {
        const box = geometry.get(node.dataset.instancePath)?.bounds;
        if (!box) continue;
        const previous = byNode.get(node);
        if (previous) {
          previous.ids.add(entry.dataset.deprecationId);
          if (previous.ids.size > 1) {
            delete previous.button.dataset.deprecationDetail; previous.button.dataset.deprecationList = ''; previous.button.dataset.deprecationState = 'multiple';
            previous.button.dataset.deprecationIds = [...previous.ids].join(' ');
            previous.button.textContent = String(previous.ids.size);
            previous.button.dataset.deprecationPreview = previous.ids.size + ' recorded deprecations for this element.';
            previous.button.setAttribute('aria-label', previous.button.dataset.deprecationPreview);
          }
          continue;
        }
        const button = deprecationMarker(entry); layer.append(button);
        const marker = {button,box,node,ids:new Set([entry.dataset.deprecationId])}; markers.push(marker); byNode.set(node,marker);
      }
    }
    const paint = (view, rect) => {
      const scale = Math.min(rect.width / view.width, rect.height / view.height);
      for (const {button,box,node} of markers) {
        const x = (rect.width-view.width*scale)/2 + (box.x+box.width-view.x)*scale;
        const y = (rect.height-view.height*scale)/2 + (box.y-view.y)*scale;
        button.hidden = x < 12 || y < 12 || x > rect.width-12 || y > rect.height-12 || Boolean(node.closest('[aria-hidden="true"],[hidden]'));
        button.style.left = x + 'px'; button.style.top = y + 'px';
      }
    };
    paint(svg.viewBox.baseVal, viewport.getBoundingClientRect());
    board.addEventListener('whiteboard:paint', event => {
      if (board.contains(deprecationTooltipOwner)) hideDeprecationPreview();
      paint(event.detail.view, event.detail.rect);
    }, {signal:pageScope.signal});
  }
  document.addEventListener('htmx:afterSwap', event => prepareDeprecations(event.detail.target));
  document.addEventListener('pointerover', event => { const button = event.target.closest?.('[data-deprecation-preview]:not([data-deprecation-entry])'); if (button && !button.contains(event.relatedTarget)) previewDeprecation(button); });
  document.addEventListener('pointerout', event => { if (event.target.closest?.('[data-deprecation-preview]:not([data-deprecation-entry])') && !deprecationTooltip?.contains(event.relatedTarget)) hideDeprecationPreview(); });
  document.addEventListener('focusin', event => { if (event.target.matches?.('[data-deprecation-preview]:not([data-deprecation-entry])')) previewDeprecation(event.target); });
  document.addEventListener('focusout', hideDeprecationPreview);
  document.addEventListener('keydown', event => { if (event.key === 'Escape') hideDeprecationPreview(); });
  document.addEventListener('click', event => {
    const detail = event.target.closest?.('[data-deprecation-detail]');
    if (detail) { event.preventDefault(); event.stopPropagation(); void openDeprecation(detail); return; }
    const list = event.target.closest?.('[data-deprecation-list]');
    if (list) { event.preventDefault(); showDeprecationList(list); return; }
    if (event.target.closest?.('.drawer-body a') && q('.diff-drawer.open')?.dataset.drawerMode === 'deprecations') closeDrawer();
  });
`
