package server

// Viewed is an explicit, local reading aid, independent of append-only review
// decisions. Keep the identity and target in each key so renaming a title,
// moving between routes, or switching reviewers never transfers a mark.
const slideViewedJavaScript = `
;(() => {
  if (!document.body?.dataset.sagaId) return;
  const q = (selector, root = document) => root.querySelector(selector);
  const qa = (selector, root = document) => [...root.querySelectorAll(selector)];
  const prefix = 'change-saga:viewed:v1:';
  const memory = new Map();
  let storageUnavailable = false;
  let pending = false;
  const saga = () => document.body.dataset.sagaId || '';
  const identityKey = () => prefix + JSON.stringify([saga(), 'reviewer']);
  function read(key) {
    if (memory.has(key)) return memory.get(key);
    try { return localStorage.getItem(key) || ''; }
    catch { storageUnavailable = true; return ''; }
  }
  function write(key, value) {
    memory.set(key, value);
    try {
      if (value) localStorage.setItem(key, value);
      else localStorage.removeItem(key);
    } catch { storageUnavailable = true; }
  }
  function defaultReviewer() {
    const key = prefix + 'local-reader';
    let id = read(key);
    if (!id) { id = 'local:' + Array.from(crypto.getRandomValues(new Uint32Array(4)), value => value.toString(16).padStart(8, '0')).join(''); write(key, id); }
    return id;
  }
  function selectedReviewer() { return read(identityKey()) || defaultReviewer(); }
  function reviewerName(reviewer) { return reviewer === defaultReviewer() ? 'Local reader' : reviewer; }
  function markKey(slide, reviewer) {
    return prefix + JSON.stringify([saga(), reviewer, slide.dataset.deckTarget, slide.dataset.slideTarget]);
  }
  function marked(slide, reviewer) { return Boolean(reviewer && read(markKey(slide, reviewer)) === '1'); }
  function activeSlide(viewer) { return q('[data-deck-slide].active', viewer); }
  function text(node, value) { if (node.textContent !== value) node.textContent = value; }
  function mount(viewer) {
    if (q('[data-slide-viewed-controls]', viewer)) return;
    const controls = document.createElement('div');
    controls.className = 'slide-viewed-controls';
    controls.dataset.slideViewedControls = '';
    controls.innerHTML = '<details class="slide-viewed-identity"><summary data-viewed-identity-summary>Local reader</summary><form data-viewed-identity-form hx-boost="false"><label>Local reviewer name <input name="reviewer" type="text" maxlength="120" autocomplete="off" required data-viewed-reviewer></label><button type="submit">Use name</button><button type="button" data-viewed-local-reader>Use local reader</button><p>Names identify local Viewed marks only. Review decisions keep their existing author. Marks belong to this browser profile and address, including port; they are not synced or saved in the Saga.</p></form></details><label class="slide-viewed-toggle"><input type="checkbox" data-slide-viewed> Viewed</label><span data-viewed-count role="status" aria-live="polite"></span><small data-viewed-storage-note></small>';
    (q('[data-slide-viewed-host]', viewer) || viewer).append(controls);
  }
  function refresh() {
    pending = false;
    if (!saga()) return;
    const reviewer = selectedReviewer();
    const slidesByTarget = new Map();
    qa('[data-deck-viewer]').forEach(viewer => {
      mount(viewer);
      const controls = q('[data-slide-viewed-controls]', viewer);
      const slide = activeSlide(viewer);
      const slides = qa('[data-deck-slide]', viewer);
      slides.forEach(candidate => slidesByTarget.set(candidate.dataset.slideTarget, candidate));
      const checkbox = q('[data-slide-viewed]', controls);
      checkbox.disabled = !reviewer || !slide || viewer.dataset.deckFace === 'overview';
      checkbox.checked = Boolean(slide && marked(slide, reviewer));
      checkbox.title = reviewer ? 'Manually mark this slide viewed for ' + reviewerName(reviewer) : 'Choose a local reviewer name first';
      const input = q('[data-viewed-reviewer]', controls);
      if (document.activeElement !== input) input.value = reviewer === defaultReviewer() ? '' : reviewer;
      text(q('[data-viewed-identity-summary]', controls), reviewerName(reviewer));
      const deckTarget = viewer.dataset.deckFace === 'overview' ? viewer.dataset.overviewTarget : slide?.dataset.deckTarget;
      const deckSlides = [...new Map(slides.filter(candidate => candidate.dataset.deckTarget === deckTarget).map(candidate => [candidate.dataset.slideTarget, candidate])).values()];
      text(q('[data-viewed-count]', controls), 'Viewed ' + deckSlides.filter(candidate => marked(candidate, reviewer)).length + ' of ' + deckSlides.length);
      const note = q('[data-viewed-storage-note]', controls);
      text(note, storageUnavailable ? 'Storage unavailable · this page only' : 'Local only');
      note.title = 'Viewed marks belong to this browser profile and address (including port), separately from approvals. They are not synced or saved in the Saga.';
    });
    qa('[data-slide-thumbnail]').forEach(button => {
      const slide = slidesByTarget.get(button.dataset.slideTarget);
      const card = button.closest('[data-slide-thumbnail-card]');
      if (!card || !slide) return;
      const viewed = marked(slide, reviewer);
      card.toggleAttribute('data-slide-is-viewed', viewed);
      let badge = q('[data-viewed-badge]', card);
      if (!badge) {
        badge = document.createElement('span');
        badge.dataset.viewedBadge = '';
        badge.className = 'slide-viewed-badge';
        (q('.slide-thumbnail-caption', card) || card).append(badge);
      }
      text(badge, viewed ? 'Viewed' : '');
      badge.hidden = !viewed;
    });
  }
  function schedule() { if (!pending) { pending = true; queueMicrotask(refresh); } }
  document.addEventListener('submit', event => {
    const form = event.target.closest('[data-viewed-identity-form]');
    if (!form) return;
    event.preventDefault();
    const reviewer = q('[data-viewed-reviewer]', form).value.trim();
    if (!reviewer) { q('[data-viewed-reviewer]', form).focus(); return; }
    write(identityKey(), reviewer);
    form.closest('details').open = false;
    refresh();
    q('[data-slide-viewed]', form.closest('[data-slide-viewed-controls]')).focus();
  });
  document.addEventListener('keydown', event => {
    const identity = event.target.closest('.slide-viewed-identity');
    if (event.key !== 'Escape' || !identity?.open) return;
    event.preventDefault();
    identity.open = false;
    q('summary', identity).focus();
  });
  document.addEventListener('click', event => {
    const button = event.target.closest('[data-viewed-local-reader]');
    if (!button) return;
    write(identityKey(), '');
    button.closest('details').open = false;
    refresh();
  });
  document.addEventListener('change', event => {
    if (!event.target.matches('[data-slide-viewed]')) return;
    const reviewer = selectedReviewer();
    const slide = activeSlide(event.target.closest('[data-deck-viewer]'));
    if (!reviewer || !slide) { refresh(); return; }
    write(markKey(slide, reviewer), event.target.checked ? '1' : '');
    refresh();
  });
  addEventListener('storage', event => {
    if (event.key && !event.key.startsWith(prefix)) return;
    if (event.key) memory.delete(event.key); else memory.clear();
    schedule();
  });
  document.addEventListener('deck-slide-activated', schedule);
  document.addEventListener('deck-face-changed', schedule);
  // Lazy shells and history navigation can replace viewers. Watch only their
  // structural changes and active-slide state; never infer Viewed from them.
  new MutationObserver(records => {
    if (records.some(record => record.type === 'attributes'
      ? record.target.matches('[data-deck-slide],body[data-saga-id]')
      : [...record.addedNodes].some(node => node.nodeType === 1 && !node.closest('[data-slide-viewed-controls]') && (node.matches('[data-deck-viewer],[data-deck-slide],[data-slide-thumbnail]') || node.querySelector('[data-deck-viewer],[data-deck-slide],[data-slide-thumbnail]'))))) schedule();
  }).observe(document.body, {childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'hidden', 'data-saga-id']});
  refresh();
})();`
