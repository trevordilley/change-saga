package server

// Runs within appJavaScript's scope so citations use the existing evidence
// drawer and slide activation, including their focus and lazy-load behavior.
const deckOverviewJavaScript = `
  function setDeckFace(face, viewer = currentDeckViewer(), target = null) {
    if (!viewer || !['overview','front','back'].includes(face)) return;
    const active = q('[data-deck-slide].active', viewer);
    target = target || active?.dataset.deckTarget || q("[data-deck-overview]", viewer)?.dataset.deckOverview;
    if (!target) return;
    if (!active) face = "overview";
    viewer.dataset.overviewTarget = target;
    viewer.dataset.deckFace = face;
    if (face === 'overview') prepareOverviewReferences(viewer);
    qa('[data-deck-overview]', viewer).forEach(overview => {
      overview.hidden = face !== 'overview' || overview.dataset.deckOverview !== target;
    });
    qa('[data-deck-slide]', viewer).forEach(slide => {
      const front = q('[data-slide-front]', slide);
      const back = q('.fragment', slide);
      if (front) front.hidden = face !== 'front';
      if (back) back.hidden = face === 'front';
    });
    qa('[data-deck-face-button]', viewer).forEach(button => {
      button.setAttribute('aria-pressed', String(button.dataset.deckFaceButton === face));
      button.disabled = button.dataset.deckFaceButton !== 'overview' && !qa('[data-deck-slide]', viewer).some(slide => slide.dataset.deckTarget === target);
    });
    document.dispatchEvent(new CustomEvent('deck-face-changed', {detail:{viewer, slide:active, face}}));
    if (face === 'back') positionLandmarkHotspots();
  }

  function openDeckOverviewFromHash() {
    const id = decodeURIComponent(location.hash.slice(1));
    const overview = id ? document.getElementById(id)?.closest('[data-deck-overview]') : null;
    if (!overview) return false;
    const viewer = overview.closest('[data-deck-viewer]');
    const index = deckViewerSlides().findIndex(slide => slide.dataset.deckTarget === overview.dataset.deckOverview);
    if (index >= 0) activateDeckSlide(index, false, 'overview');
    setDeckFace('overview', viewer, overview.dataset.deckOverview);
    setView(viewer.closest('[data-view]')?.dataset.view || 'saga', false);
    return true;
  }

  let overviewPreviewTimer = 0;
  function referenceControls(ref) {
    const destination = document.getElementById(ref.dataset.overviewAnchor);
    if (ref.dataset.overviewItemTarget) return destination?.querySelector('[data-landmark-affordance-template]')?.content || destination;
    // A whole-slide reference may use the slide's aggregate controls, never
    // silently select the first Item from its marked-places menu.
    const controls = document.createDocumentFragment();
    qa(':scope > .fragment-head > .fragment-actions > button', destination || document.createElement('div')).forEach(button => controls.append(button.cloneNode(true)));
    return controls;
  }
  function prepareOverviewReferences(viewer) {
    qa('[data-overview-reference]:not([data-overview-invalid])', viewer).forEach(ref => {
      const slot = q('[data-overview-evidence-controls]', ref);
      if (!slot || slot.childElementCount) return;
      const owner = referenceControls(ref);
      for (const [kind, selector, label] of [
        ['code','[data-open-diffs],[data-target-code-href]','Code / diff'],
        ['stories','[data-open-stories]','Stories'],
        ['definition','[data-documentation-target]','Definition']
      ]) {
        const control = owner?.querySelector(selector);
        if (!control) continue;
        const button = document.createElement('button');
        button.type = 'button';
        button.dataset.overviewAction = kind;
        button.dataset.overviewReferenceId = ref.id;
        button.textContent = kind === 'code' && /details|surprise/i.test(control.title) ? 'Details' : kind === 'stories' && /affected/i.test(control.title) ? 'Documentation' : label;
        slot.append(button);
      }
    });
  }

  function overviewReference(link) {
    const overview = link?.closest('[data-deck-overview]');
    if (!overview) return null;
    if (link.hasAttribute('data-overview-open')) return link.closest('[data-overview-reference]');
    const href = link.getAttribute('href') || '';
    if (!href.startsWith('#')) return null;
    const ref = document.getElementById(decodeURIComponent(href.slice(1)));
    return ref?.matches('[data-overview-reference]') && ref.closest('[data-deck-overview]') === overview ? ref : null;
  }

  function previewOverviewReference(link) {
    clearTimeout(overviewPreviewTimer);
    const ref = overviewReference(link);
    if (!ref) return;
    const overview = ref.closest('[data-deck-overview]');
    const preview = q('[data-overview-preview]', overview);
    if (!preview) return;
    preview.replaceChildren();
    const detail = q('.overview-reference-detail,.overview-reference-problem', ref);
    if (detail) preview.append(detail.cloneNode(true));
    preview.id = overview.id + '-preview';
    link.setAttribute('aria-describedby', preview.id);
    preview.hidden = false;
  }

  function hideOverviewPreview(link) {
    const overview = link?.closest('[data-deck-overview]');
    if (!overview) return;
    const preview = q('[data-overview-preview]', overview);
    overviewPreviewTimer = setTimeout(() => { if (preview && !preview.matches(':hover') && !preview.contains(document.activeElement)) preview.hidden = true; }, 180);
    link.removeAttribute('aria-describedby');
  }

  function openOverviewReference(ref, opener, action = '') {
    if (!ref || ref.hasAttribute('data-overview-invalid')) return;
    const slides = deckViewerSlides();
    const index = slides.findIndex(slide => slide.dataset.slideTarget === ref.dataset.overviewSlideTarget);
    if (index < 0) return;
    activateDeckSlide(index, false, 'back');
    setDeckFace('back');
    const anchor = ref.dataset.overviewAnchor;
    if (anchor) history.replaceState(history.state, '', location.pathname + location.search + '#' + encodeURIComponent(anchor));
    const slide = slides[index];
    const destination = document.getElementById(anchor);
    const controls = referenceControls(ref);
    // A review Item's drawer combines its explanation, affected record and exact
    // diff. Implementation Items use the same code/story/definition controls.
    const selectors = {code:'[data-open-diffs],[data-target-code-href]', stories:'[data-open-stories]', definition:'[data-documentation-target]'};
    const control = action ? controls?.querySelector(selectors[action]) : controls?.querySelector(selectors.code) || controls?.querySelector(selectors.stories) || controls?.querySelector(selectors.definition);
    // The overview closes, so restore focus to a visible Back control.
    const restore = q('[data-deck-face-button="back"]', slide.closest('[data-deck-viewer]'));
    if (control?.dataset.openDiffs) openDrawer(control.dataset.openDiffs, restore);
    else if (control?.dataset.targetCodeHref) { void hydrateTargetCode(control, restore); }
    else if (control?.dataset.openStories) { openStoriesDrawer(control); drawerOpener = restore; }
    else if (control?.dataset.documentationTarget) { void openDocumentation(control); drawerOpener = restore; }
    else restore?.focus({preventScroll:true});
    void activateLandmark();
  }

  document.addEventListener('click', event => {
    const action = event.target.closest?.('[data-overview-action]');
    if (action) { event.preventDefault(); openOverviewReference(document.getElementById(action.dataset.overviewReferenceId), action, action.dataset.overviewAction); return; }
    const face = event.target.closest?.('[data-deck-face-button]');
    if (face) {
      event.preventDefault();
      const viewer = face.closest('[data-deck-viewer]');
      setDeckFace(face.dataset.deckFaceButton, viewer);
      const destination = face.dataset.deckFaceButton === 'overview' ? q('[data-deck-overview]:not([hidden])', viewer) : q('[data-deck-slide].active .fragment', viewer);
      if (destination?.id) history.replaceState(history.state, '', location.pathname + location.search + '#' + encodeURIComponent(destination.id));
      return;
    }
    const link = event.target.closest?.('[data-deck-overview] a');
    if (!link) return;
    const ref = overviewReference(link);
    if (ref) {
      event.preventDefault();
      if (ref.hasAttribute('data-overview-invalid')) { ref.scrollIntoView({block:'nearest'}); previewOverviewReference(link); }
      else openOverviewReference(ref, link);
      return;
    }
    if (link.hasAttribute('data-overview-slide')) {
      event.preventDefault();
      const anchor = decodeURIComponent(link.hash.slice(1));
      const slide = document.getElementById(anchor)?.closest('[data-deck-slide]');
      const index = deckViewerSlides().indexOf(slide);
      if (index >= 0) activateDeckSlide(index, true, 'back');
    }
  });
  document.addEventListener('pointerover', event => { if (event.target.closest?.('[data-overview-preview]')) clearTimeout(overviewPreviewTimer); const link = event.target.closest?.('[data-deck-overview] a'); if (link) previewOverviewReference(link); });
  document.addEventListener('focusin', event => { if (event.target.closest?.('[data-overview-preview]')) clearTimeout(overviewPreviewTimer); const link = event.target.closest?.('[data-deck-overview] a'); if (link) previewOverviewReference(link); });
  document.addEventListener('pointerout', event => { const link = event.target.closest?.('[data-deck-overview] a'); if (link && !link.contains(event.relatedTarget) && document.activeElement !== link) hideOverviewPreview(link); });
  document.addEventListener('focusout', event => { const link = event.target.closest?.('[data-deck-overview] a'); if (link) hideOverviewPreview(link); });
  document.addEventListener('keydown', event => { if (event.key === 'Escape') qa('[data-overview-preview]').forEach(preview => { preview.hidden = true; }); });
`

const deckOverviewStyles = `
.review-empty{height:100%;overflow:auto;background:var(--bg);color:var(--ink);padding:20px;box-sizing:border-box}.review-empty-viewer{height:60vh;margin-top:20px;background:var(--bg)}.review-empty-viewer .deck-viewer-stage{height:100%;width:100%;aspect-ratio:auto}.review-overview-link{display:block;padding:6px 12px}.deck-face-controls{display:flex;gap:2px;pointer-events:auto;background:var(--bg);border:1px solid var(--line);text-shadow:none;margin-right:auto}
.deck-face-controls button{font:inherit;padding:5px 9px;border:0;background:transparent;color:var(--muted);cursor:pointer}
.deck-face-controls button[aria-pressed=true]{color:var(--ink);background:var(--bg-inset);box-shadow:inset 0 -2px var(--accent)}
.deck-face-controls button:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
.slide-front{position:absolute;inset:0;padding:72px 9% 50px;overflow:auto;font:20px/1.65 var(--ui);color:var(--ink);background:var(--bg)}
.slide-front h2{font-size:28px;line-height:1.25;margin:0 0 25px}.slide-front li{margin:16px 0}
.slide-front[hidden],.deck-viewer-slide .fragment[hidden],.deck-overview[hidden]{display:none!important}
.deck-overview{position:absolute;inset:0;z-index:2;overflow:auto;padding:64px 8% 44px;background:var(--bg);color:var(--ink);font:15px/1.65 var(--ui);scroll-padding-top:64px}
.deck-overview h2{font-size:25px;line-height:1.3;margin:4px 0 12px}.deck-overview .eyebrow{margin:0;color:var(--muted)}
.deck-overview img{max-width:100%;height:auto}.overview-report table{border-collapse:collapse;max-width:100%;display:block;overflow:auto}.overview-report th,.overview-report td{border:1px solid var(--line);padding:6px 10px}
.overview-generated,.overview-evidence-note{font-size:13px;color:var(--muted)}.overview-directory>li{padding:12px 0;border-bottom:1px solid var(--line)}
.overview-references{border-top:1px solid var(--line);margin-top:28px;padding-top:10px}.overview-references>ul{padding:0;list-style:none}.overview-references>ul>li{padding:10px 0;border-bottom:1px solid var(--line);scroll-margin-top:64px}
.overview-reference-detail{font-size:13px;color:var(--muted)}.overview-reference-detail p{margin:3px 0}.overview-evidence-controls{display:flex;gap:8px;margin-top:6px}.overview-evidence-controls button{font:inherit;color:var(--ink);background:var(--bg-inset);border:1px solid var(--line);padding:3px 8px;cursor:pointer}.overview-reference-problem{color:var(--muted)}
.overview-preview{position:sticky;bottom:0;z-index:4;border:1px solid var(--line);padding:12px 16px;background:var(--bg);max-height:180px;overflow:auto;box-shadow:0 -3px 12px #0001}.overview-preview[hidden]{display:none}
[data-deck-face=overview] .deck-viewer-slide{visibility:hidden}[data-deck-face=overview] .deck-viewer-controls{display:none}
@media(max-width:780px){.deck-overview{padding:62px 22px 30px}.slide-front{padding:62px 35px 30px;font-size:16px}.slide-front h2{font-size:23px}}
`
