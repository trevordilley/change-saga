package server

const appJavaScript = `(() => {
  const q = (selector, root = document) => root.querySelector(selector);
  const qa = (selector, root = document) => [...root.querySelectorAll(selector)];
  let activeFragment = null;
  let diffLayout = 'inline';
  let drawerOpener = null;
  let drawerRestore = null;
  const slideDiffPreviewReasons = new WeakMap();

  function slideDiffSummaryButton(node) {
    const button = node?.closest?.('.diff-button');
    const fragment = button?.closest?.('.fragment');
    if (!button || !fragment?.closest('[data-deck-slide]')) return null;
    return button.closest('.fragment-head')?.parentElement === fragment ? button : null;
  }

  function setSlideDiffPreview(button, reason, visible) {
    const fragment = slideDiffSummaryButton(button)?.closest('.fragment');
    if (!fragment) return;
    let reasons = slideDiffPreviewReasons.get(fragment);
    if (!reasons) {
      reasons = new Set();
      slideDiffPreviewReasons.set(fragment, reasons);
    }
    if (visible) reasons.add(reason);
    else reasons.delete(reason);
    fragment.classList.toggle('preview-linked-items', reasons.size > 0);
  }

  function landmarkOwnsDiffs(target) {
    const template = q('[data-landmark-affordance-template]', target);
    return Boolean(template?.content.querySelector('[data-open-diffs],[data-target-code-href]'));
  }

  function markLandmarkDiffOwnership(target, visual) {
    if (visual) visual.dataset.landmarkHasDiffs = String(landmarkOwnsDiffs(target));
  }

  function deckViewerSlides() { return qa('[data-deck-slide]'); }

  function viewerDeckSlides(slide) {
    const slides = deckViewerSlides();
    if (!slide?.dataset.deckTarget) return slides;
    return slides.filter(candidate => candidate.dataset.deckTarget === slide.dataset.deckTarget);
  }

  function deckViewerActive() {
    const surface = q('[data-deck-viewer]');
    const view = surface?.closest('[data-view]');
    return Boolean(surface && (!view || view.classList.contains('active')));
  }

  function activateDeckSlide(index, updateHash = false) {
    const slides = deckViewerSlides();
    if (!slides.length) return;
    qa('.fragment.preview-linked-items').forEach(fragment => fragment.classList.remove('preview-linked-items'));
    const bounded = Math.max(0, Math.min(slides.length - 1, index));
    slides.forEach((slide, current) => {
      const active = current === bounded;
      slide.hidden = !active;
      slide.classList.toggle('active', active);
      slide.setAttribute('aria-hidden', String(!active));
    });
    const active = slides[bounded];
    const shell = active.closest('[data-deck-viewer]');
    const position = q('[data-slide-position]', shell);
    const deckTitle = q('[data-slide-deck-title]', shell);
    const slideTitle = q('[data-current-slide-title]', shell);
    const deckSlides = viewerDeckSlides(active);
    const deckIndex = deckSlides.indexOf(active);
    if (position) position.textContent = (deckIndex + 1) + ' / ' + deckSlides.length;
    if (deckTitle) deckTitle.textContent = active.dataset.deckTitle || '';
    if (slideTitle) slideTitle.textContent = active.dataset.slideTitle || '';
    let activeThumbnail = null;
    qa('[data-slide-thumbnail]').forEach(thumbnail => {
      const selected = thumbnail.dataset.slideTarget === active.dataset.slideTarget;
      thumbnail.setAttribute('aria-current', String(selected));
      thumbnail.closest('[data-slide-thumbnail-card]')?.classList.toggle('active', selected);
      if (selected) activeThumbnail = thumbnail;
      if (selected && updateHash) thumbnail.scrollIntoView({block:'nearest'});
    });
    qa('.doc-deck>.doc-row').forEach(row => row.classList.toggle('current', Boolean(activeThumbnail && row.parentElement.contains(activeThumbnail))));
    const deckChildren = activeThumbnail?.closest('.doc-children');
    if (deckChildren?.id) setDocNodeExpandedByID(deckChildren.id, true);
    const previous = q('[data-slide-previous]', shell);
    const next = q('[data-slide-next]', shell);
    if (previous) previous.disabled = deckIndex === 0;
    if (next) next.disabled = deckIndex === deckSlides.length - 1;
    if (updateHash) {
      const anchor = q('.fragment', active)?.id;
      if (anchor) history.replaceState(history.state, '', location.pathname + location.search + '#' + encodeURIComponent(anchor));
    }
    const fragment = q('.fragment', active);
    const targetCodeButton = q(':scope > .fragment-head [data-target-code-href]', fragment);
    if (targetCodeButton) void hydrateTargetCodeSummary(targetCodeButton);
    positionLandmarkHotspots();
  }

  function stepDeckSlide(delta) {
    const slides = deckViewerSlides();
    const active = slides.find(slide => !slide.hidden);
    if (!active) return;
    const deckSlides = viewerDeckSlides(active);
    const current = deckSlides.indexOf(active);
    const target = deckSlides[Math.max(0, Math.min(deckSlides.length - 1, current + delta))];
    const index = slides.indexOf(target);
    if (index >= 0) activateDeckSlide(index, true);
  }

  function syncDeckSlideForHash() {
    const slides = deckViewerSlides();
    if (!slides.length) return;
    const id = decodeURIComponent(location.hash.replace(/^#/, ''));
    const requested = id ? document.getElementById(id)?.closest?.('[data-deck-slide]') : null;
    const view = slides[0].closest('[data-view]');
    if (view && !view.classList.contains('active') && !requested) return;
    activateDeckSlide(requested ? slides.indexOf(requested) : Math.max(0, slides.findIndex(slide => !slide.hidden)));
  }

  function syncSlidePresentation() {
    const body = document.body;
    if (!body?.classList) return;
    const active = Boolean(document.fullscreenElement) || body.classList.contains('presentation-fallback');
    body.classList.toggle('presentation-mode', active);
    qa('[data-slide-present]').forEach(button => button.setAttribute('aria-pressed', String(active)));
    const exit = q('[data-slide-exit-presentation]');
    if (exit) exit.hidden = !active;
    globalThis.requestAnimationFrame?.(positionLandmarkHotspots);
  }

  async function toggleSlidePresentation() {
    if (document.fullscreenElement) {
      await document.exitFullscreen();
      return;
    }
    if (document.body.classList.contains('presentation-fallback')) {
      document.body.classList.remove('presentation-fallback');
      syncSlidePresentation();
      return;
    }
    if (q('.diff-drawer.open')) closeDrawer();
    try {
      if (!document.documentElement.requestFullscreen) throw new Error('fullscreen unavailable');
      await document.documentElement.requestFullscreen();
    } catch (_) {
      document.body.classList.add('presentation-fallback');
      syncSlidePresentation();
    }
  }

  const languageKeywords = {
    go: new Set('break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var'.split(' ')),
    javascript: new Set('async await break case catch class const continue debugger default delete do else export extends finally for from function get if import in instanceof let new of return set static super switch this throw try typeof var void while with yield'.split(' ')),
    python: new Set('and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield'.split(' ')),
    ruby: new Set('alias and begin break case class def defined do else elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef unless until when while yield'.split(' ')),
    shell: new Set('case do done elif else esac fi for function if in select then time until while'.split(' ')),
    generic: new Set('class const enum false function interface let new null private public return static struct true type var void'.split(' '))
  };

  async function copyPermalink(button) {
    const url = new URL(location.href);
    url.hash = (button.dataset.copyLink || '').replace(/^#/, '');
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(url.toString());
    } catch (_) {
      const input = document.createElement('textarea');
      input.value = url.toString();
      input.setAttribute('readonly', '');
      input.style.position = 'fixed';
      input.style.opacity = '0';
      document.body.append(input);
      input.select();
      document.execCommand('copy');
      input.remove();
    }
    button.classList.add('copied');
    button.setAttribute('aria-label', 'Link copied');
    setTimeout(() => {
      button.classList.remove('copied');
      button.setAttribute('aria-label', 'Copy link');
    }, 1400);
  }

  function markExactText(target, exact, className = '', prefix = '', suffix = '') {
    if (!target || !exact) return null;
    const walker = document.createTreeWalker(target, NodeFilter.SHOW_TEXT);
    const nodes = [];
    let text = '';
    let node;
    while (node = walker.nextNode()) {
      nodes.push({node, start: text.length, end: text.length + node.data.length});
      text += node.data;
    }
    let offset = 0;
    while ((offset = text.indexOf(exact, offset)) >= 0) {
      const before = text.slice(0, offset);
      const after = text.slice(offset + exact.length);
      if ((!prefix || before.endsWith(prefix)) && (!suffix || after.startsWith(suffix))) {
        const first = nodes.find(item => item.end > offset);
        const last = nodes.find(item => item.end >= offset + exact.length);
        if (!first || !last) return null;
        const range = document.createRange();
        range.setStart(first.node, offset - first.start);
        range.setEnd(last.node, offset + exact.length - last.start);
        const mark = document.createElement('mark');
        if (className) mark.className = className;
        mark.append(range.extractContents());
        range.insertNode(mark);
        return mark;
      }
      offset += exact.length;
    }
    return null;
  }

  function cloneLandmarkAffordance(target) {
    return q('[data-landmark-affordance-template]', target)?.content.cloneNode(true) || null;
  }

  function normalizedMeasuredRegion(rect, rootRect) {
    if (!rect || !rootRect.width || !rootRect.height || (!rect.width && !rect.height)) return null;
    // A thin path or small glyph should still be easy to discover with a
    // pointer. Explicit --hotspot geometry remains the escape hatch when the
    // author's desired interaction area differs from the rendered bounds.
    const minimumX = 24 / rootRect.width;
    const minimumY = 24 / rootRect.height;
    const paddingX = 6 / rootRect.width;
    const paddingY = 6 / rootRect.height;
    const centerX = (rect.left + rect.right) / 2;
    const centerY = (rect.top + rect.bottom) / 2;
    const width = Math.min(1, Math.max(rect.width / rootRect.width + paddingX * 2, minimumX));
    const height = Math.min(1, Math.max(rect.height / rootRect.height + paddingY * 2, minimumY));
    const x = Math.max(0, Math.min(1 - width, (centerX - rootRect.left) / rootRect.width - width / 2));
    const y = Math.max(0, Math.min(1 - height, (centerY - rootRect.top) / rootRect.height - height / 2));
    return {x, y, width, height};
  }

  function appendAutomaticLandmarkHotspot(fragment, target, region) {
    const stage = q('.fragment-stage', fragment);
    if (!stage || q('[data-landmark-visual="' + CSS.escape(target.dataset.landmarkAnchor) + '"]', stage)) return;
    const visual = document.createElement('div');
    visual.className = 'landmark-hotspot';
    visual.dataset.landmarkVisual = target.dataset.landmarkAnchor;
    visual.dataset.autoLandmarkHotspot = 'true';
    visual.dataset.elementId = target.dataset.elementId;
    visual.dataset.x = String(region.x);
    visual.dataset.y = String(region.y);
    visual.dataset.width = String(region.width);
    visual.dataset.height = String(region.height);
    markLandmarkDiffOwnership(target, visual);
    const affordance = cloneLandmarkAffordance(target);
    if (affordance) visual.append(affordance);
    stage.append(visual);
  }

  async function prepareSVGElementHotspots(fragment) {
    const frame = q('[data-fragment-frame]', fragment);
    const targets = qa('[data-landmark-target][data-landmark-type="element"]', fragment)
      .filter(target => target.dataset.elementId && !q('[data-landmark-visual="' + CSS.escape(target.dataset.landmarkAnchor) + '"]', fragment));
    if (!frame || targets.length === 0) return;
    const sourceURL = new URL(frame.getAttribute('src'), location.href);
    // SVG fragments created by the CLI have a .svg entrypoint. The aspect
    // query is also present for viewBox-based SVGs, including renamed assets.
    if (!sourceURL.pathname.toLowerCase().endsWith('.svg') && !sourceURL.searchParams.has('saga_aspect')) return;
    sourceURL.hash = '';
    const response = await fetch(sourceURL, {credentials:'same-origin'});
    if (!response.ok) return;
    const parsed = new DOMParser().parseFromString(await response.text(), 'image/svg+xml');
    if (parsed.querySelector('parsererror') || parsed.documentElement.localName !== 'svg') return;
    const svg = document.importNode(parsed.documentElement, true);
    // Measurement never needs executable or navigable content. Keeping this
    // clone inert preserves the iframe sandbox while still letting the browser
    // account for groups, paths, text, and transforms via normal SVG layout.
    svg.querySelectorAll('script,foreignObject').forEach(node => node.remove());
    svg.querySelectorAll('*').forEach(node => [...node.attributes].forEach(attribute => {
      const name = attribute.name.toLowerCase();
      if (name.startsWith('on') || ((name === 'href' || name.endsWith(':href')) && !attribute.value.startsWith('#'))) node.removeAttribute(attribute.name);
    }));
    const viewBox = svg.viewBox?.baseVal;
    if (!viewBox || !(viewBox.width > 0) || !(viewBox.height > 0)) return;
    const measure = document.createElement('div');
    measure.setAttribute('aria-hidden', 'true');
    measure.style.cssText = 'position:absolute;left:-100000px;top:0;visibility:hidden;pointer-events:none;overflow:hidden';
    svg.removeAttribute('width');
    svg.removeAttribute('height');
    svg.style.width = '1000px';
    svg.style.height = (1000 * viewBox.height / viewBox.width) + 'px';
    const shadow = measure.attachShadow({mode:'closed'});
    shadow.append(svg);
    document.body.append(measure);
    const rootRect = svg.getBoundingClientRect();
    targets.forEach(target => {
      const element = svg.querySelector('#' + CSS.escape(target.dataset.elementId));
      const region = normalizedMeasuredRegion(element?.getBoundingClientRect(), rootRect);
      if (region) appendAutomaticLandmarkHotspot(fragment, target, region);
    });
    measure.remove();
    positionLandmarkHotspots();
  }

  function positionLandmarkHotspots() {
    qa('.fragment-stage').forEach(stage => {
      const media = q('.fragment-frame,.fragment-image', stage);
      if (!media) return;
      const stageRect = stage.getBoundingClientRect();
      const mediaRect = media.getBoundingClientRect();
      qa('.landmark-hotspot', stage).forEach(visual => {
        visual.style.left = (mediaRect.left - stageRect.left + Number(visual.dataset.x) * mediaRect.width) + 'px';
        visual.style.top = (mediaRect.top - stageRect.top + Number(visual.dataset.y) * mediaRect.height) + 'px';
        visual.style.width = (Number(visual.dataset.width) * mediaRect.width) + 'px';
        visual.style.height = (Number(visual.dataset.height) * mediaRect.height) + 'px';
      });
    });
  }

  // within lets a preparation pass run over one hydrated fragment as well as
  // over the whole page, including when the root is the fragment itself.
  function within(root, selector) {
    const scope = root || document;
    const self = scope.matches?.(selector) ? [scope] : [];
    return [...self, ...qa(selector, scope)];
  }

  function prepareLandmarks(root = document) {
    within(root, '.fragment-frame').forEach(frame => {
      const aspect = Number(new URL(frame.src, location.href).searchParams.get('saga_aspect'));
      if (aspect > 0) {
        frame.style.minHeight = '0';
        frame.style.aspectRatio = String(aspect);
      }
      frame.addEventListener('load', positionLandmarkHotspots);
    });
    within(root, '.fragment-image').forEach(image => image.addEventListener('load', positionLandmarkHotspots));
    within(root, '[data-landmark-target]').forEach(target => {
      const anchor = target.dataset.landmarkAnchor;
      const fragment = target.closest('.fragment');
      if (!anchor || !fragment) return;
      if (target.dataset.landmarkType === 'heading') {
        const heading = document.getElementById(anchor);
        if (!heading) return;
        markLandmarkDiffOwnership(target, heading);
        heading.querySelector('.heading-permalink')?.remove();
        const affordance = cloneLandmarkAffordance(target);
        if (affordance) heading.append(affordance);
      } else if (target.dataset.landmarkType === 'text') {
        const mark = markExactText(q('[data-selectable]', fragment), target.dataset.exact, 'content-landmark-text', target.dataset.prefix, target.dataset.suffix);
        if (!mark) return;
        target.removeAttribute('id');
        mark.id = anchor;
        mark.dataset.landmarkVisual = anchor;
        markLandmarkDiffOwnership(target, mark);
        const affordance = cloneLandmarkAffordance(target);
        if (affordance) mark.append(affordance);
      }
      markLandmarkDiffOwnership(target, q('[data-landmark-visual="' + CSS.escape(anchor) + '"]', fragment));
    });
    within(root, '.fragment').forEach(fragment => { void prepareSVGElementHotspots(fragment).catch(() => {}); });
    globalThis.requestAnimationFrame?.(positionLandmarkHotspots);
  }

  // Markdown citations are ordinary footnotes until their reference entry is
  // made into an exact-text landmark. When that landmark owns code evidence,
  // promote every inline citation marker into a direct diff-drawer control.
  // Footnotes without evidence keep their normal jump-to-reference behavior.
  function prepareDiffCitations(root = document) {
    within(root, 'a.footnote-ref').forEach(reference => {
      const href = reference.getAttribute('href') || '';
      if (!href.startsWith('#')) return;
      const definition = document.getElementById(decodeURIComponent(href.slice(1)));
      const diff = definition?.querySelector('[data-open-diffs]');
      if (!diff?.dataset.openDiffs) return;
      reference.dataset.openDiffs = diff.dataset.openDiffs;
      reference.classList.add('diff-citation');
      reference.setAttribute('aria-label', 'Open cited code');
      reference.setAttribute('title', 'Open cited code');
    });
  }

  async function activateLandmark() {
    qa('[data-landmark-visual].active').forEach(element => element.classList.remove('active'));
    qa('.content-landmark-active').forEach(element => element.classList.remove('content-landmark-active'));
    const id = decodeURIComponent(location.hash.replace(/^#/, ''));
    // The anchor may name something inside a chapter or an explanation that has
    // not been fetched yet, so it is resolved before it is scrolled to.
    const destination = await revealAnchor(id);
    const destinationView = destination?.closest('[data-view]')?.dataset.view;
    if (destinationView === 'saga' || destinationView === 'slides') setView(destinationView, false);
    const target = id ? q('[data-landmark-anchor="' + CSS.escape(id) + '"]') : null;
    if (!target) {
      destination?.scrollIntoView({block:'start'});
      return;
    }
    const fragment = target.closest('.fragment');
    if (!fragment) return;
    setActiveFragment(fragment);
    const visual = q('[data-landmark-visual="' + CSS.escape(id) + '"]', fragment);
    if (visual) {
      visual.classList.add('active');
      visual.classList.add('content-landmark-active');
    }
    if (target.dataset.landmarkType === 'element') {
      const frame = q('[data-fragment-frame]', fragment);
      if (!frame || !target.dataset.elementId) return;
      const base = frame.dataset.landmarkBase || frame.getAttribute('src').split('#')[0];
      frame.dataset.landmarkBase = base;
      const url = new URL(base, location.href);
      url.hash = target.dataset.elementId;
      if (frame.src !== url.toString()) frame.src = url.toString();
    }
    document.getElementById(id)?.scrollIntoView({block:'center'});
  }

  // Prose and licence files are not code. Running them through the tokeniser
  // painted every capitalised word as a type, which is exactly the decorative
  // noise a diff should not add to a sentence.
  const proseNames = new Set(['license','licence','notice','readme','contributing','changelog','authors','codeowners']);

  function languageForPath(path) {
    const name = (path || '').toLowerCase();
    const base = name.split('/').pop() || '';
    const extension = name.includes('.') ? name.split('.').pop() : '';
    if (['md','mdx','markdown','txt','text','rst','adoc'].includes(extension)) return 'prose';
    if (!name.includes('.') && proseNames.has(base)) return 'prose';
    if (extension === 'go') return 'go';
    if (['js','jsx','mjs','cjs','ts','tsx'].includes(extension)) return 'javascript';
    if (extension === 'py') return 'python';
    if (extension === 'rb') return 'ruby';
    if (['sh','bash','zsh'].includes(extension)) return 'shell';
    if (['json','yaml','yml','toml','xml','html','css','scss','sql','c','h','cc','cpp','java','rs','swift','kt'].includes(extension)) return extension;
    return 'generic';
  }

  function tokenClass(token, language) {
    if (/^(\/\/|\/\*|\*|--)/.test(token) || (token.startsWith('#') && ['python','ruby','shell','yaml','yml'].includes(language))) return 'tok-comment';
    if (/^["'\x60]/.test(token)) return 'tok-string';
    if (/^\d/.test(token)) return 'tok-number';
    if (/^[{}()[\].,:;]+$/.test(token)) return 'tok-punctuation';
    const words = languageKeywords[language] || languageKeywords.generic;
    if (words.has(token) || languageKeywords.generic.has(token)) return 'tok-keyword';
    if (/^[A-Z][A-Za-z0-9_]*$/.test(token)) return 'tok-type';
    if (/^[A-Za-z_$][\w$-]*(?=\s*:)/.test(token)) return 'tok-property';
    return '';
  }

  function highlightCode(root = document) {
    qa('[data-code]', root).forEach(code => {
      if (code.dataset.highlighted) return;
      const path = (code.closest('[data-file-path]') || {}).dataset?.filePath || '';
      const language = languageForPath(path);
      if (language === 'prose') { code.dataset.highlighted = language; return; }
      const source = code.textContent;
      const pattern = /(\/\/.*$|\/\*[\s\S]*?\*\/|--.*$|#.*$|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\x60(?:\\.|[^\x60\\])*\x60|\b\d+(?:\.\d+)?\b|[A-Za-z_$][\w$-]*|[{}()[\].,:;]+)/gm;
      let offset = 0;
      const fragment = document.createDocumentFragment();
      for (const match of source.matchAll(pattern)) {
        if (match.index > offset) fragment.append(document.createTextNode(source.slice(offset, match.index)));
        const span = document.createElement('span');
        span.className = tokenClass(match[0], language);
        span.textContent = match[0];
        fragment.append(span);
        offset = match.index + match[0].length;
      }
      if (offset < source.length) fragment.append(document.createTextNode(source.slice(offset)));
      code.replaceChildren(fragment);
      code.dataset.highlighted = language;
    });
  }

  function prepareContext(root = document) {
    within(root, '[data-diff-body]').forEach(body => {
      qa(':scope > .context-expander', body).forEach(button => button.remove());
      qa(':scope > [data-context-row]', body).forEach(row => { row.hidden = false; });
      const rows = [...body.children].filter(row => row.matches('.diff-row'));
      const firstChange = rows.findIndex(row => !row.matches('[data-context-row]'));
      const lastChange = rows.findLastIndex(row => !row.matches('[data-context-row]'));
      const hidden = rows.map((row, index) => {
        if (!row.matches('[data-context-row]')) return false;
        let before = Infinity, after = Infinity;
        for (let i = index - 1; i >= 0; i--) if (!rows[i].matches('[data-context-row]')) { before = index - i; break; }
        for (let i = index + 1; i < rows.length; i++) if (!rows[i].matches('[data-context-row]')) { after = i - index; break; }
        return before > 3 && after > 3;
      });
      for (let index = 0; index < rows.length;) {
        if (!hidden[index]) { index++; continue; }
        const start = index;
        const group = [];
        while (index < rows.length && hidden[index]) { rows[index].hidden = true; group.push(rows[index]); index++; }
        installContextExpander(group, firstChange >= 0 && start > firstChange, lastChange >= 0 && index <= lastChange);
      }
    });
  }

  // GitHub-style context controls keep changed hunks visible while allowing a
  // reviewer to reveal unchanged lines from either edge of a collapsed gap.
  // The middle action opens the whole gap; directional actions reveal ten
  // lines at a time without discarding the reviewer's place in the diff.
  function installContextExpander(group, hasChangeBefore, hasChangeAfter) {
    let remaining = [...group];
    const step = 10;
    const control = document.createElement('div');
    control.className = 'context-expander';

    const action = (direction, glyph) => {
      const button = document.createElement('button');
      button.type = 'button';
      button.dataset.contextExpand = direction;
      button.textContent = glyph;
      button.addEventListener('click', () => reveal(direction));
      control.append(button);
      return button;
    };
    const down = hasChangeBefore ? action('down', '↓') : null;
    const all = action('all', '');
    all.className = 'context-expand-all';
    const up = hasChangeAfter ? action('up', '↑') : null;

    const refresh = () => {
      if (!remaining.length) {
        control.remove();
        return;
      }
      const count = remaining.length;
      const amount = Math.min(step, count);
      all.textContent = 'Expand all ' + count + ' unchanged lines';
      all.title = 'Expand the collapsed gap';
      if (down) {
        const label = 'Show next ' + amount + ' unchanged lines';
        down.setAttribute('aria-label', label);
        down.title = label;
      }
      if (up) {
        const label = 'Show previous ' + amount + ' unchanged lines';
        up.setAttribute('aria-label', label);
        up.title = label;
      }
      if (control.nextElementSibling !== remaining[0]) remaining[0].before(control);
    };
    const reveal = direction => {
      const acted = document.activeElement;
      const revealed = direction === 'all' ? remaining
        : direction === 'up' ? remaining.slice(-step)
        : remaining.slice(0, step);
      const revealedSet = new Set(revealed);
      revealed.forEach(row => { row.hidden = false; });
      remaining = remaining.filter(row => !revealedSet.has(row));
      refresh();
      applyDiffLayout(diffLayout);
      if (acted instanceof HTMLElement && [down, all, up].includes(acted)) {
        globalThis.requestAnimationFrame?.(() => {
          if (acted.isConnected) {
            acted.focus({preventScroll:true});
            return;
          }
          const towardPrevious = direction === 'down' || direction === 'all' && !hasChangeAfter;
          const edge = towardPrevious ? revealed[0] : revealed[revealed.length - 1];
          let neighbor = towardPrevious ? edge?.previousElementSibling : edge?.nextElementSibling;
          while (neighbor && !neighbor.matches('[data-diff-row]')) {
            neighbor = towardPrevious ? neighbor.previousElementSibling : neighbor.nextElementSibling;
          }
          neighbor?.querySelector('button,[href],[tabindex]')?.focus({preventScroll:true});
        });
      }
    };
    refresh();
  }

  function applyDiffLayout(mode) {
    diffLayout = mode === 'split' ? 'split' : 'inline';
    if (innerWidth <= 1050) diffLayout = 'inline';
    qa('[data-diff-surface]').forEach(surface => {
      surface.dataset.layout = diffLayout;
      const body = q('[data-diff-body]', surface);
      if (!body) return;
      qa(':scope > *', body).forEach(row => { row.style.gridRow = ''; });
      if (diffLayout !== 'split') return;
      const items = [...body.children].filter(item => !item.hidden);
      let gridRow = 1;
      for (let index = 0; index < items.length;) {
        const item = items[index];
        if (!item.matches('.diff-row.old')) {
          item.style.gridRow = String(gridRow++);
          index++;
          continue;
        }
        const oldRows = [];
        while (index < items.length && items[index].matches('.diff-row.old')) oldRows.push(items[index++]);
        const newRows = [];
        while (index < items.length && items[index].matches('.diff-row.new')) newRows.push(items[index++]);
        const count = Math.max(oldRows.length, newRows.length);
        oldRows.forEach((row, i) => { row.style.gridRow = String(gridRow + i); });
        newRows.forEach((row, i) => { row.style.gridRow = String(gridRow + i); });
        gridRow += count;
      }
    });
    // Scoped to the toolbar buttons: the diff surface also carries data-layout,
    // and aria-pressed is not a valid attribute on that container.
    qa('button[data-layout]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.layout === diffLayout)));
  }

  function setTreeVisible(visible) {
    const shell = q('[data-shell]');
    if (shell) shell.classList.toggle('tree-hidden', !visible);
    qa('[data-toggle-tree]').forEach(button => button.setAttribute('aria-pressed', String(visible)));
  }

  function setRelatedVisible(visible) {
    const workspace = q('[data-code-workspace]');
    if (workspace) workspace.classList.toggle('related-hidden', !visible);
    qa('[data-toggle-related]').forEach(button => button.setAttribute('aria-pressed', String(visible)));
  }

  // ----- Directories -----
  // A directory is the table of what a section holds, and the server renders
  // every row of it. Without this the filter is a form: it submits ?q= and the
  // server sends the table back filtered. With it, the same field hides the
  // rows that do not match as the reader types, and the submit button steps
  // out of the way. Both paths compare the same thing, the text the row shows,
  // so a reader who turns JavaScript off sees exactly the rows they had.
  function filterDirectory(directory) {
    const input = q('[data-directory-filter]', directory);
    const needle = (input?.value || '').trim().toLowerCase();
    const rows = qa('[data-directory-row]', directory);
    let shown = 0;
    rows.forEach(row => {
      const matches = !needle || (row.dataset.directoryText || '').toLowerCase().includes(needle);
      row.hidden = !matches;
      if (matches) shown += 1;
    });
    const none = q('[data-directory-none]', directory);
    if (none) none.hidden = shown > 0;
    // Detail a directory summarises, such as a review's per-slide decisions,
    // is marked with the row's own key and follows the table's filter. A page
    // has one directory, so this is scoped by key rather than by container.
    const matched = new Set(rows.filter(row => !row.hidden).map(row => row.dataset.directoryRow));
    qa('[data-directory-linked]').forEach(detail => {
      detail.hidden = !matched.has(detail.dataset.directoryLinked);
    });
    const caption = q('[data-directory-caption]', directory);
    if (caption) {
      const total = Number(directory.dataset.directoryTotal || rows.length);
      const noun = total === 1 && !needle ? directory.dataset.directoryNoun : directory.dataset.directoryNouns;
      caption.textContent = needle ? shown + ' of ' + total + ' ' + noun : total + ' ' + noun;
    }
  }

  function prepareDirectories() {
    qa('[data-directory]').forEach(directory => {
      const input = q('[data-directory-filter]', directory);
      if (!input) return;
      // The submit button is the no-JavaScript path; typing has replaced it.
      q('[data-directory-submit]', directory)?.setAttribute('hidden', '');
      input.addEventListener('input', () => filterDirectory(directory));
    });
  }

  function filterTree() {
    const filter = (q('[data-file-filter]')?.value || '').trim().toLowerCase();
    const files = qa('[data-tree-file]');
    files.forEach(file => { file.hidden = !file.dataset.treePath.toLowerCase().includes(filter); });
    qa('[data-tree-folder]').reverse().forEach(folder => { folder.hidden = !q('[data-tree-file]:not([hidden])', folder); });
    const empty = q('[data-tree-empty]');
    if (empty) empty.hidden = files.some(file => !file.hidden);
  }

  function setDocNodeExpandedByID(id, expanded) {
    const children = document.getElementById(id);
    if (!children) return;
    children.hidden = !expanded;
    qa('[aria-controls]').filter(control => control.getAttribute('aria-controls') === id).forEach(control => {
      control.setAttribute('aria-expanded', String(expanded));
    });
  }

  function toggleDocNode(button) {
    const id = button.getAttribute('aria-controls');
    const children = document.getElementById(id);
    if (!children) return;
    setDocNodeExpandedByID(id, children.hidden);
  }

  // Opening a chapter is what fetches it. The disclosure state is applied at
  // once so the control never feels unresponsive, and the body arrives from
  // /api/section behind the placeholder the shell rendered in its place.
  async function setChapterOpen(chapter, open) {
    if (!chapter) return;
    const body = q('[data-chapter-body]', chapter);
    const toggle = q('[data-chapter-toggle]', chapter);
    if (!body || !toggle) return;
    body.hidden = !open;
    toggle.setAttribute('aria-expanded', String(open));
    toggle.setAttribute('aria-label', (open ? 'Close ' : 'Open ') + (q('.chapter-head h2', chapter)?.textContent.trim() || 'chapter'));
    chapter.classList.toggle('open', open);
    if (!open) return;
    await hydrateChapter(chapter);
    positionLandmarkHotspots();
  }

  function toggleChapter(button) {
	const chapter = button.closest('[data-chapter]');
	void setChapterOpen(chapter, button.getAttribute('aria-expanded') !== 'true');
  }

  // Code Diff, Coverage, Change, and History are deliberately absent from the root document.
  // Their endpoints return bounded HTML fragments, and a cold comparison may
  // answer 202 while its snapshot is still being built. Per-surface request
  // generations keep a late response for one file from replacing a newer deep
  // link, while the URL remains the source of truth throughout retries.
  const reviewSurfaceRequests = new Map();
  const reviewSurfaceRetries = new Map();
  const reviewFileRequests = new WeakMap();
  const continuousCoverageLoads = new WeakMap();
  let relatedOwnersRequest = null;

  function reviewSurfaceURL(name, explicitHref = '') {
    const surface = q('[data-review-surface="'+name+'"]');
    const url = new URL(explicitHref || surface?.dataset.surfaceHref || '', location.href);
    if (!explicitHref) {
      const current = new URL(location.href);
      ['file', 'ref', 'mode'].forEach(key => {
        if (current.searchParams.has(key)) url.searchParams.set(key, current.searchParams.get(key));
      });
    }
    return url;
  }

  function surfaceStatus(surface, state, title, detail, retry = false) {
    if (!surface) return;
    surface.dataset.surfaceState = state;
    surface.replaceChildren();
    const status = document.createElement('div');
    status.className = 'surface-placeholder ' + state;
    status.dataset.surfaceStatus = '';
    status.setAttribute('role', state === 'error' ? 'alert' : 'status');
    status.setAttribute('aria-live', 'polite');
    if (state === 'loading' || state === 'building') {
      const spinner = document.createElement('span');
      spinner.className = 'surface-spinner';
      spinner.setAttribute('aria-hidden', 'true');
      status.append(spinner);
    }
    const heading = document.createElement('strong');
    heading.textContent = title;
    status.append(heading);
    if (detail) {
      const message = document.createElement('span');
      message.textContent = detail;
      status.append(message);
    }
    if (retry) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'btn-primary';
      button.dataset.retrySurface = surface.dataset.reviewSurface;
      button.textContent = 'Try again';
      status.append(button);
    }
    surface.append(status);
  }

  function retryDelay(response) {
    const value = response.headers.get('Retry-After');
    const seconds = Number(value);
    if (Number.isFinite(seconds) && seconds >= 0) return Math.min(10_000, Math.max(250, seconds * 1000));
    const date = Date.parse(value || '');
    if (Number.isFinite(date)) return Math.min(10_000, Math.max(250, date - Date.now()));
    return 1000;
  }

  function prepareReviewSurface(name, root) {
    if (name !== 'manifest') {
      highlightCode(root);
      prepareContext(root);
      applyDiffLayout(diffLayout);
    }
    if (name === 'manifest') {
      const requested = new URL(location.href).searchParams.get('mode');
      const current = q('[data-manifest-mode][aria-pressed="true"]')?.dataset.manifestMode;
      setManifestMode(requested === 'saga' && q('[data-manifest-panel="saga"]') ? 'saga'
        : requested === 'code' && q('[data-manifest-panel="code"]') ? 'code'
        : current && q('[data-manifest-panel="'+current+'"]') ? current
        : q('[data-manifest-panel="code"]') ? 'code' : 'saga');
    }
    if (name === 'code') {
      const meta = q('[data-code-meta]');
      const content = q('[data-code-meta-content]', root);
      if (meta && content) meta.textContent = content.textContent;
      within(root, '[data-file-diff-href]').forEach(file => { void hydrateReviewFile(file); });
      void hydrateRelatedOwners(root);
    }
    const id = decodeURIComponent(location.hash.replace(/^#/, ''));
    const destination = id ? document.getElementById(id) : null;
    if (destination?.closest('[data-review-surface="'+name+'"]')) {
      globalThis.requestAnimationFrame?.(() => destination.scrollIntoView({block:'center'}));
    }
  }

  async function hydrateRelatedOwners(root) {
    const panel = q('#related-saga-panel', root);
    const file = q('[data-file-diff-href]', root);
    const filePath = file?.dataset.filePath;
    if (!panel || !filePath) return;
    const key = filePath;
    if (panel.dataset.relatedOwnersLoaded === key) return;
    relatedOwnersRequest?.controller.abort();
    const controller = new AbortController();
    const request = {key, controller};
    relatedOwnersRequest = request;
    try {
      const response = await fetch('/api/file-owners?file=' + encodeURIComponent(filePath), {
        headers:{Accept:'text/html','X-Change-Saga-Async':'true'}, credentials:'same-origin', signal:controller.signal
      });
      if (!response.ok) throw new Error('explanations request failed');
      const content = q('[data-file-owners-response]', parseShellHTML(await response.text()));
      if (!content) throw new Error('explanations response was incomplete');
      if (relatedOwnersRequest !== request || !panel.isConnected || q('[data-file-diff-href]', root)?.dataset.filePath !== filePath) return;
      panel.replaceChildren(...Array.from(content.childNodes));
      panel.dataset.relatedOwnersLoaded = key;
    } catch (error) {
      if (error.name !== 'AbortError' && panel.isConnected) panel.innerHTML = '<p>Explanations could not be loaded.</p>';
    } finally {
      if (relatedOwnersRequest === request) relatedOwnersRequest = null;
    }
  }

  // Code Diff and narrative-linked code are the same review surface. Both use
  // the same markup, row endpoint, cache, and bounded-page stream; only the
  // surrounding navigation differs. Context is prepared after the final page
  // arrives so a collapsed unchanged gap can span an endpoint boundary.
  function reviewFileIsActive(file) {
    return Boolean(file?.isConnected && (!file.matches('details') || file.open));
  }

  function cancelReviewFile(file) {
    reviewFileRequests.get(file)?.controller.abort();
  }

  async function hydrateReviewFile(file, options = {}) {
    const href = file?.dataset.fileDiffHref;
    const destination = q('[data-file-diff-rows]', file);
    const status = q('[data-file-diff-status]', file);
    if (!href || !destination) return file || null;
    const key = new URL(href, location.href).toString();
    const previous = reviewFileRequests.get(file);
    if (!options.force && previous?.key === key && !previous.controller.signal.aborted) return previous.promise;
    if (!options.force && file.dataset.fileDiffLoaded === key) return file;
    previous?.controller.abort();
    const controller = new AbortController();
    delete file.dataset.fileDiffLoaded;
    file.dataset.fileDiffLoading = 'true';
    q('[data-diff-surface]', file)?.classList.add('loading');
    const linkedContext = file.matches('.attached-file');
    if (status) status.textContent = linkedContext ? 'Loading full file diff; linked lines will be highlighted…' : 'Loading every changed hunk…';
    const visited = new Set();
    const promise = (async () => {
      let nextHref = href;
      let first = true;
      let loaded = 0;
      let total = 0;
      while (nextHref && reviewFileIsActive(file)) {
        const pageHref = new URL(nextHref, location.href).toString();
        if (visited.has(pageHref) || visited.size >= 10_000) throw new Error('diff cursor did not advance');
        visited.add(pageHref);
        const result = await fetchFileDiff(nextHref, {signal:controller.signal});
        if (!reviewFileIsActive(file)) {
          controller.abort();
          return file;
        }
        const wrapper = parseShellHTML(result.html);
        const page = q('[data-file-diff-page]', wrapper) || wrapper;
        const items = q('[data-page-items="lines"]', page) || page;
        const rows = qa('.diff-row', items);
        if (first) destination.replaceChildren(...rows); else destination.append(...rows);
        first = false;
        loaded += rows.length;
        total = result.total || total;
        if (status) status.textContent = total > 0
          ? 'Loaded ' + loaded + ' of ' + total + ' diff lines…'
          : 'Loaded ' + loaded + ' diff lines…';
        highlightCode(destination);
        nextHref = continuedPageURL(href, result.next || page.dataset?.nextCursor);
        if (nextHref) await new Promise(resolve => setTimeout(resolve, 0));
      }
      if (!reviewFileIsActive(file)) return file;
      file.dataset.fileDiffLoaded = key;
      if (status) status.textContent = linkedContext ? 'All changed hunks · linked lines highlighted' : 'All changed hunks';
      prepareContext(destination);
      applyDiffLayout(diffLayout);
      if (!location.hash) {
        const selected = q('.diff-row.selected', destination);
        globalThis.requestAnimationFrame?.(() => selected?.scrollIntoView({block:'center'}));
      }
      return file;
    })().catch(error => {
      if (error.name === 'AbortError') return file;
      visited.forEach(pageHref => fileDiffCache.delete(pageHref));
      destination.innerHTML = '<p class="diff-placeholder">This file could not be loaded. <button type="button" data-retry-file>Try again</button></p>';
      if (status) status.textContent = 'Could not load every changed hunk';
      return file;
    }).finally(() => {
      delete file.dataset.fileDiffLoading;
      q('[data-diff-surface]', file)?.classList.remove('loading');
      if (reviewFileRequests.get(file)?.promise === promise) reviewFileRequests.delete(file);
    });
    reviewFileRequests.set(file, {key, controller, promise});
    return promise;
  }

  function surfaceNextButton(name, cursor, pageKey = '') {
    if (!cursor) return null;
    const url = reviewSurfaceURL(name);
    url.searchParams.set('cursor', cursor);
    const button = document.createElement('button');
    button.type = 'button';
    button.dataset.surfaceNext = url.pathname + url.search;
    if (pageKey) button.dataset.pageTarget = pageKey;
    button.textContent = name === 'manifest' ? 'Loading more coverage…' : 'Load more';
    button.setAttribute('aria-label', name === 'manifest' ? 'Loading more coverage' : 'Load the next page');
    return button;
  }

  async function streamCoveragePages(surface, token) {
    let button = q('[data-surface-next]', surface);
    while (button && button.isConnected && continuousCoverageLoads.get(surface) === token) {
      button.disabled = true;
      button.setAttribute('aria-busy', 'true');
      const next = await loadReviewSurfacePage(button);
      if (!next) break;
      button = next;
      // Yield after every append so the browser paints useful coverage while
      // the following page is in flight instead of presenting one huge swap.
      await new Promise(resolve => setTimeout(resolve, 0));
    }
  }

  function beginContinuousCoverageLoad(surface) {
    if (!surface) return;
    const token = {};
    continuousCoverageLoads.set(surface, token);
    void streamCoveragePages(surface, token);
  }

  function installReviewSurface(name, surface, html, headerCursor = '') {
    const wrapper = parseShellHTML(html);
    const response = q('[data-review-surface-response="'+name+'"]', wrapper) || q('[data-view="'+name+'"]', wrapper);
    if (!response) throw new Error('surface response was incomplete');
    const next = surfaceNextButton(name, headerCursor || response.dataset.nextCursor, response.dataset.pageKey || (name === 'code' ? 'files' : ''));
    if (name === 'code') {
      const sidebarResponse = q('[data-code-sidebar-content]', response);
      const panelResponse = q('[data-code-panel-content]', response) || response;
      const sidebar = q('[data-code-sidebar]');
      if (!sidebarResponse || !sidebar) throw new Error('code response was incomplete');
      within(surface, '[data-file-diff-href]').forEach(cancelReviewFile);
      sidebar.replaceChildren(...Array.from(sidebarResponse.childNodes));
      surface.replaceChildren(...Array.from(panelResponse.childNodes));
    } else {
      surface.replaceChildren(...Array.from(response.childNodes));
    }
    if (next) surface.append(next);
    if (response.dataset.returned) surface.dataset.returned = response.dataset.returned;
    surface.dataset.surfaceState = 'ready';
    prepareReviewSurface(name, surface);
    if (name === 'manifest') beginContinuousCoverageLoad(surface);
  }

  const surfaceLabels = {
    code:{title:'Code Diff', loading:'Loading changed files.'},
    manifest:{title:'Coverage', loading:'Loading files and explanations.'},
    change:{title:'Change', loading:'Loading what this change edited and affected.'},
    history:{title:'History', loading:'Loading the commits that changed this record.'}
  };

  async function hydrateReviewSurface(name, options = {}) {
    const surface = q('[data-review-surface="'+name+'"]');
    if (!surface) return null;
    const url = reviewSurfaceURL(name, options.href || '');
    const requestKey = url.toString();
    if (!options.force && surface.dataset.surfaceLoaded === requestKey) return surface;
    const previous = reviewSurfaceRequests.get(name);
    if (!options.force && previous?.key === requestKey) return previous.promise;
    previous?.controller.abort();
    clearTimeout(reviewSurfaceRetries.get(name));
    const controller = new AbortController();
    const label = surfaceLabels[name] || surfaceLabels.manifest;
    surfaceStatus(surface, 'loading', 'Loading ' + label.title + '…', label.loading);
    const promise = fetch(url, {headers:{Accept:'text/html','X-Change-Saga-Async':'true'},credentials:'same-origin',signal:controller.signal}).then(async response => {
      if (response.status === 202) {
        const delay = retryDelay(response);
        surfaceStatus(surface, 'building', 'Building the source comparison…', 'This view will update automatically when it is ready.', true);
        const timer = setTimeout(() => {
          if (q('[data-view="'+name+'"]').classList.contains('active')) void hydrateReviewSurface(name, {force:true});
        }, delay);
        reviewSurfaceRetries.set(name, timer);
        return surface;
      }
      if (!response.ok) throw new Error((await response.text()).trim() || 'request failed');
      installReviewSurface(name, surface, await response.text(), response.headers.get('X-Change-Saga-Next-Cursor') || '');
      surface.dataset.surfaceLoaded = requestKey;
      return surface;
    }).catch(error => {
      if (error.name === 'AbortError') return surface;
      surfaceStatus(surface, 'error', label.title + ' could not be loaded.', error.message, true);
      return surface;
    }).finally(() => {
      if (reviewSurfaceRequests.get(name)?.promise === promise) reviewSurfaceRequests.delete(name);
    });
    reviewSurfaceRequests.set(name, {key:requestKey, controller, promise});
    return promise;
  }

  async function loadReviewSurfacePage(button) {
    const surface = button.closest('[data-review-surface]');
    const name = surface?.dataset.reviewSurface;
    const href = button.dataset.surfaceNext || button.getAttribute('href');
    if (!surface || !name || !href || button.dataset.pageLoading === 'true') return null;
    button.dataset.pageLoading = 'true';
    button.setAttribute('aria-busy', 'true');
    try {
      const response = await fetch(reviewSurfaceURL(name, href), {headers:{Accept:'text/html','X-Change-Saga-Async':'true'},credentials:'same-origin'});
      if (!response.ok) throw new Error('page request failed');
      if (!button.isConnected || button.closest('[data-review-surface]') !== surface) return null;
      const wrapper = parseShellHTML(await response.text());
      const page = q('[data-review-surface-page]', wrapper) || q('[data-review-surface-response="'+name+'"]', wrapper) || wrapper;
      const key = page.dataset?.pageKey || button.dataset.pageTarget || '';
      const groups = within(page, '[data-page-items]');
      let appended = 0;
      for (const items of groups) {
        const groupKey = items.dataset.pageItems;
        if (!groupKey) continue;
        const sidebar = q('[data-code-sidebar]');
        const destination = q('[data-page-items="'+CSS.escape(groupKey)+'"]', surface) || (sidebar ? q('[data-page-items="'+CSS.escape(groupKey)+'"]', sidebar) : null);
        if (!destination || destination === items) continue;
        destination.append(...Array.from(items.childNodes));
        appended++;
      }
      if (!appended) {
        const destinationRoot = name === 'code' && key === 'files' ? q('[data-code-sidebar]') : surface;
        const destination = key ? q('[data-page-items="'+CSS.escape(key)+'"]', destinationRoot) : q('[data-page-items]', destinationRoot);
        const items = key ? q('[data-page-items="'+CSS.escape(key)+'"]', page) : q('[data-page-items]', page);
        if (!destination || !items) throw new Error('page response was incomplete');
        destination.append(...Array.from(items.childNodes));
      }
      const next = q('[data-surface-next]', page) || surfaceNextButton(name, response.headers.get('X-Change-Saga-Next-Cursor') || page.dataset?.nextCursor, key);
      if (next) button.replaceWith(next); else button.remove();
      prepareReviewSurface(name, surface);
      return next;
    } catch (_) {
      button.textContent = name === 'manifest' ? 'Coverage paused — try again' : 'Could not load more — try again';
      delete button.dataset.pageLoading;
      button.removeAttribute('aria-busy');
      button.disabled = false;
      button.setAttribute('aria-label', name === 'manifest' ? 'Resume loading coverage' : 'Load the next page');
      return null;
    }
  }

  function setView(name, updateURL = true) {
    if (!q('[data-view="'+name+'"]')) name = 'saga';
    qa('[data-view]').forEach(view => view.classList.toggle('active', view.dataset.view === name));
    qa('[data-view-tab]').forEach(tab => {
      const selected = tab.dataset.viewTab === name || (name === 'slides' && tab.dataset.viewTab === 'saga');
      tab.classList.toggle('active', selected);
      tab.setAttribute('aria-selected', String(selected));
      // Roving focus: only the selected tab is in the sequential tab order.
      tab.tabIndex = selected ? 0 : -1;
    });
    const sagaSide = q('.saga-side');
    const codeSide = q('.code-side');
    const codeMeta = q('.top-meta');
    if (sagaSide) sagaSide.hidden = name !== 'saga' && name !== 'slides';
    if (codeSide) codeSide.hidden = name !== 'code';
    if (codeMeta) codeMeta.hidden = name !== 'code';
    const shell = q('[data-shell]');
    if (shell) {
      shell.classList.toggle('code-mode', name === 'code');
      shell.classList.toggle('slide-mode', name === 'slides');
    }
    const slideView = q('[data-view="slides"]') ? 'slides' : 'saga';
    qa('[data-slide-present]').forEach(button => { button.hidden = name !== slideView; });
    // A hidden view measures as zero, so hotspots are placed once the saga
    // view is actually on screen.
    if (name === 'saga' || name === 'slides') globalThis.requestAnimationFrame?.(positionLandmarkHotspots);
    if (updateURL) {
      const url = new URL(location.href);
      if (name === 'saga') url.searchParams.delete('view'); else url.searchParams.set('view', name);
      history.pushState({view: name}, '', url);
    }
    if (name === 'code' || name === 'manifest' || name === 'change') void hydrateReviewSurface(name);
  }

  function filterManifest() {
    const input = q('[data-manifest-filter]');
    const query = (input?.value || '').trim().toLowerCase();
    qa('[data-manifest-panel]').forEach(panel => {
      let visible = 0;
      qa('[data-manifest-search]', panel).forEach(group => {
        const match = !query || group.dataset.manifestSearch.toLowerCase().includes(query);
        group.hidden = !match;
        if (match) visible++;
      });
      // Folders are structure, not matches: hide the ones whose files all went
      // away so the tree never leaves an empty branch behind.
      qa('[data-manifest-folder]', panel).reverse().forEach(folder => {
        folder.hidden = !q('[data-manifest-search]:not([hidden])', folder);
        if (query && !folder.hidden) folder.open = true;
      });
      const empty = q('[data-manifest-empty]', panel);
      if (empty) empty.hidden = visible !== 0 || !query;
    });
  }

  function setManifestMode(mode) {
    qa('[data-manifest-mode]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.manifestMode === mode)));
    qa('[data-manifest-panel]').forEach(panel => panel.hidden = panel.dataset.manifestPanel !== mode);
    filterManifest();
  }

  function activateManifestMode(mode) {
    const current = new URL(location.href);
    current.searchParams.set('mode', mode);
    history.pushState({view:'manifest', mode}, '', current);
    if (q('[data-manifest-panel="'+mode+'"]')) {
      setManifestMode(mode);
      return Promise.resolve();
    }
    const url = reviewSurfaceURL('manifest');
    url.searchParams.set('mode', mode);
    return hydrateReviewSurface('manifest', {force:true, href:url.toString()});
  }

  function setActiveFragment(fragment) {
    if (!fragment || fragment === activeFragment) return;
    if (activeFragment) activeFragment.classList.remove('active-fragment');
    activeFragment = fragment;
    activeFragment.classList.add('active-fragment');
    // Pointing at an explanation is the clearest signal that it is about to be
    // read, so it is fetched now rather than when it scrolls.
    if (fragment.dataset.fragmentHref) void hydrateFragment(fragment);
  }

  function configureDrawer(mode, title) {
    const drawer = q('.diff-drawer');
    if (!drawer) return;
    drawer.dataset.drawerMode = mode;
    const labels = {fragment:'Related explanation', history:'History', code:'Linked code'};
    const icons = {fragment:'#i-book', history:'#i-clock', code:'#i-diff'};
    const label = labels[mode] || labels.code;
    drawer.setAttribute('aria-label', label);
    const heading = q('.drawer-head strong', drawer);
    if (heading) heading.textContent = title;
    const icon = q('.drawer-head .i use', drawer);
    if (icon) icon.setAttribute('href', icons[mode] || icons.code);
    const close = q('[data-close-drawer]', drawer);
    if (close) {
      close.setAttribute('aria-label', 'Close ' + label.toLowerCase());
      close.title = 'Close';
    }
  }

  function restoreDrawerContent() {
    const body = q('.drawer-body');
    if (body) within(body, '[data-file-diff-href]').forEach(cancelReviewFile);
    if (drawerRestore) {
      if (drawerRestore.placeholder.isConnected) drawerRestore.placeholder.replaceWith(drawerRestore.fragment);
      drawerRestore = null;
    }
    if (body) body.replaceChildren();
  }

  function showDrawer(opener) {
    drawerOpener = opener instanceof HTMLElement && opener.isConnected
      ? opener
      : (document.activeElement instanceof HTMLElement ? document.activeElement : null);
    const drawer = q('.diff-drawer');
    drawer.classList.add('open');
    drawer.removeAttribute('inert');
    drawer.setAttribute('aria-hidden', 'false');
    q('.drawer-backdrop').classList.add('open');
    document.body.style.overflow = 'hidden';
    q('[data-close-drawer]', drawer).focus();
  }

  function openDrawer(templateID, opener) {
    const lazy = qa('[data-target-code-template]').find(candidate => candidate.dataset.targetCodeTemplate === templateID);
    if (lazy) { void hydrateTargetCode(lazy); return; }
    const source = document.getElementById(templateID);
    if (!source) return;
    // WebKit does not consistently move document.activeElement to a button
    // before dispatching its click event. Preserve the event's explicit opener
    // so Escape always restores focus to the control the reviewer activated.
    const returnOpener = drawerRestore ? drawerOpener : opener;
    restoreDrawerContent();
    const body = q('.drawer-body');
    body.innerHTML = source.innerHTML;
    const attached = q('[data-attached-title]', body);
    configureDrawer('code', attached?.dataset.attachedTitle ? 'Linked code · ' + attached.dataset.attachedTitle : 'Linked code');
    highlightCode(body);
    showDrawer(returnOpener);
  }

  // A record's history opens in the review drawer: when it was introduced,
  // what it replaced, and every commit that changed it.
  async function openHistoryDrawer(href, opener) {
    const drawer = q('.diff-drawer');
    const returnOpener = drawer?.classList.contains('open') ? drawerOpener : opener;
    restoreDrawerContent();
    const surface = document.createElement('div');
    surface.className = 'history-drawer-surface';
    surface.dataset.reviewSurface = 'history';
    surface.dataset.surfaceHref = href;
    q('.drawer-body')?.append(surface);
    configureDrawer('history', 'History');
    showDrawer(returnOpener);
    return hydrateReviewSurface('history', {href:new URL(href, location.href).toString(), force:true});
  }

  // Compare mode highlights the records this change edited or affected;
  // everything else stays reachable but quiet. The Saga is documentation, so
  // neither mode offers approvals or comments on it.
  const layerState = {ready:false, changed:new Set(), affected:new Set(), dom:new Map()};
  function applyLayers(root = document) {
    if (!layerState.ready) return;
    const layerOf = id => layerState.changed.has(id) ? 'changed' : layerState.affected.has(id) ? 'affected' : '';
    const byDOM = new Map();
    layerState.dom.forEach((dom, urn) => byDOM.set(dom, layerOf(urn)));
    within(root, '[data-target]').forEach(element => {
      if (element.closest('[data-review-surface]')) return;
      const layer = layerOf(element.dataset.target);
      element.classList.toggle('layer-changed', layer === 'changed');
      element.classList.toggle('layer-affected', layer === 'affected');
      element.classList.toggle('layer-quiet', !layer);
    });
    within(root, 'nav a[href^="#target-"], .nav a[href^="#target-"]').forEach(link => {
      const layer = byDOM.get(link.getAttribute('href').slice(1)) || '';
      link.classList.toggle('layer-changed', layer === 'changed');
      link.classList.toggle('layer-affected', layer === 'affected');
      link.classList.toggle('layer-quiet', !layer);
    });
  }
  async function loadLayers() {
    if (q('[data-opening]')?.dataset.opening !== 'compare') return;
    for (let attempt = 0; attempt < 120; attempt++) {
      const response = await fetch('/api/layers', {headers:{Accept:'application/json'}, credentials:'same-origin'});
      if (response.status === 202) { await new Promise(resolve => setTimeout(resolve, retryDelay(response))); continue; }
      if (!response.ok) return;
      const layers = await response.json();
      layerState.changed = new Set(layers.changed || []);
      layerState.affected = new Set(layers.affected || []);
      layerState.dom = new Map(Object.entries(layers.dom || {}));
      layerState.ready = true;
      document.body.dataset.layersReady = 'true';
      applyLayers(document);
      new MutationObserver(records => records.forEach(record => record.addedNodes.forEach(node => {
        if (node.nodeType === 1) applyLayers(node);
      }))).observe(document.body, {childList:true, subtree:true});
      return;
    }
  }

  // One changed file's diff body is fetched the first time a reviewer opens it
  // and then reused for the rest of the session. The page deliberately ships no
  // diff bodies: a large comparison would otherwise appear in the document once
  // per narrative target that explains it and again in the coverage audit, as
  // markup that stays inside a closed disclosure until it is asked for.
  const fileDiffCache = new Map();

  async function fetchFileDiff(href, options = {}) {
    const key = new URL(href, location.href).toString();
    const cached = fileDiffCache.get(key);
    if (cached) return cached;
    const signal = options.signal || new AbortController().signal;
    while (true) {
      const response = await fetch(href, {headers:{Accept:'text/html'}, signal});
      if (response.status === 202) {
        await abortableDelay(retryDelay(response), signal);
        continue;
      }
      if (!response.ok) throw new Error('diff request failed');
      const result = {
        html:await response.text(),
        next:response.headers.get('X-Change-Saga-Next-Cursor') || '',
        total:Number(response.headers.get('X-Change-Saga-Total')) || 0,
        returned:Number(response.headers.get('X-Change-Saga-Returned')) || 0
      };
      fileDiffCache.set(key, result);
      return result;
    }
  }

  // The page ships the saga as a shell: saga identity, coverage totals, the
  // overview's explanations as descriptors, one summary per chapter, and the
  // navigation outline. Chapter bodies and explanation content arrive from
  // /api/section and /api/fragment as a reviewer reaches them, so first load
  // stays proportional to what is on screen rather than to the whole story.
  const shellCache = new Map();

  function fetchShell(href) {
    let request = shellCache.get(href);
    if (!request) {
      const load = () => fetch(href, {headers:{Accept:'text/html'}}).then(async response => {
        if (response.status === 202) {
          await new Promise(resolve => setTimeout(resolve, retryDelay(response)));
          return load();
        }
        if (!response.ok) throw new Error('shell request failed');
        return response.text();
      });
      request = load();
      shellCache.set(href, request);
      request.catch(() => shellCache.delete(href));
    }
    return request;
  }

  function parseShellHTML(html) {
    const wrapper = document.createElement('div');
    wrapper.innerHTML = html;
    return wrapper;
  }

  // Linked-code summaries are small enough to anticipate but expensive enough
  // that a pointer sweep must not fan out across the story. Two speculative
  // requests run at once; clicks promote their request above hover work, and a
  // bounded LRU keeps completed summaries useful without retaining the saga.
  const maxConcurrentTargetCodeLoads = 2;
  const targetCodeCacheLimit = 64;
  const targetCodeResponses = new Map();
  const targetCodeJobs = new Map();
  const targetCodeQueue = [];
  let activeTargetCodeLoads = 0;
  let targetCodeJobOrder = 0;

  function cachedTargetCode(href) {
    const html = targetCodeResponses.get(href);
    if (html === undefined) return null;
    targetCodeResponses.delete(href);
    targetCodeResponses.set(href, html);
    return html;
  }

  function rememberTargetCode(href, html) {
    targetCodeResponses.delete(href);
    targetCodeResponses.set(href, html);
    while (targetCodeResponses.size > targetCodeCacheLimit) {
      targetCodeResponses.delete(targetCodeResponses.keys().next().value);
    }
  }

  function requestAbortError() {
    return new DOMException('Request was cancelled', 'AbortError');
  }

  function abortableDelay(milliseconds, signal) {
    return new Promise((resolve, reject) => {
      if (signal.aborted) { reject(requestAbortError()); return; }
      const timer = setTimeout(resolve, milliseconds);
      signal.addEventListener('abort', () => { clearTimeout(timer); reject(requestAbortError()); }, {once:true});
    });
  }

  async function loadTargetCodeJob(job) {
    while (true) {
      const response = await fetch(job.href, {headers:{Accept:'text/html'}, credentials:'same-origin', signal:job.controller.signal});
      if (response.status === 202) {
        await abortableDelay(retryDelay(response), job.controller.signal);
        continue;
      }
      if (!response.ok) throw new Error('linked-code request failed');
      return response.text();
    }
  }

  function pumpTargetCodeQueue() {
    targetCodeQueue.sort((left, right) => right.priority-left.priority || left.order-right.order);
    while (activeTargetCodeLoads < maxConcurrentTargetCodeLoads && targetCodeQueue.length) {
      const job = targetCodeQueue.shift();
      if (job.state !== 'queued') continue;
      job.state = 'running';
      job.controller = new AbortController();
      activeTargetCodeLoads++;
      void loadTargetCodeJob(job).then(html => {
        if (job.state === 'cancelled') throw requestAbortError();
        rememberTargetCode(job.href, html);
        installTargetCodeResponse(job.href, html);
        job.resolve(html);
      }).catch(error => job.reject(error)).finally(() => {
        if (targetCodeJobs.get(job.href) === job) targetCodeJobs.delete(job.href);
        activeTargetCodeLoads--;
        pumpTargetCodeQueue();
      });
    }
  }

  function preemptForInteractiveTargetCode(interactiveJob) {
    if (interactiveJob.state !== 'queued' || activeTargetCodeLoads < maxConcurrentTargetCodeLoads) return;
    const victim = Array.from(targetCodeJobs.values())
      .filter(job => job !== interactiveJob && job.state === 'running' && !job.interactive)
      .sort((left, right) => left.priority-right.priority || right.order-left.order)[0];
    victim?.controller.abort();
  }

  function requestTargetCode(href, options = {}) {
    const cached = cachedTargetCode(href);
    if (cached !== null) return Promise.resolve(cached);
    let job = targetCodeJobs.get(href);
    if (!job) {
      let resolve, reject;
      const promise = new Promise((accept, decline) => { resolve = accept; reject = decline; });
      job = {href, promise, resolve, reject, state:'queued', controller:null, scopes:new Set(), interactive:false, priority:0, order:targetCodeJobOrder++};
      targetCodeJobs.set(href, job);
      targetCodeQueue.push(job);
    }
    if (options.scope) job.scopes.add(options.scope);
    if (options.interactive) job.interactive = true;
    job.priority = Math.max(job.priority, options.interactive ? 100 : Number(options.priority || 0));
    preemptForInteractiveTargetCode(job);
    pumpTargetCodeQueue();
    return job.promise;
  }

  function cancelTargetCodeScope(scope) {
    targetCodeJobs.forEach(job => {
      if (!job.scopes.delete(scope) || job.interactive || job.scopes.size) return;
      if (job.state === 'running') {
        job.state = 'cancelled';
        targetCodeJobs.delete(job.href);
        job.controller.abort();
      } else if (job.state === 'queued') {
        job.state = 'cancelled';
        targetCodeJobs.delete(job.href);
        job.reject(requestAbortError());
      }
    });
  }

  const fragmentPrefetchScopes = new WeakMap();

  function fragmentPrefetchScope(fragment) {
    let scope = fragmentPrefetchScopes.get(fragment);
    if (!scope) {
      scope = {fragment, reasons:new Set(), timer:null, started:false};
      fragmentPrefetchScopes.set(fragment, scope);
    }
    return scope;
  }

  function fragmentTargetCodeHrefs(fragment) {
    const direct = q(':scope > .fragment-head [data-target-code-href]', fragment)?.dataset.targetCodeHref || '';
    const seen = new Set();
    const hrefs = [];
    const add = href => { if (href && !seen.has(href)) { seen.add(href); hrefs.push(href); } };
    add(direct);
    within(fragment, '[data-target-code-href]').forEach(control => add(control.dataset.targetCodeHref));
    return {direct, hrefs};
  }

  function beginFragmentPrefetch(fragment, reason) {
    if (!fragment) return;
    const scope = fragmentPrefetchScope(fragment);
    scope.reasons.add(reason);
    if (scope.timer || scope.started) return;
    scope.timer = setTimeout(async () => {
      scope.timer = null;
      if (!scope.reasons.size) return;
      scope.started = true;
      if (fragment.dataset.fragmentHref) await hydrateFragment(fragment);
      if (!scope.reasons.size || !fragment.isConnected) { scope.started = false; return; }
      const targets = fragmentTargetCodeHrefs(fragment);
      targets.hrefs.forEach(href => {
        void requestTargetCode(href, {scope, priority:href === targets.direct ? 10 : 1}).catch(() => {});
      });
    }, 160);
  }

  function endFragmentPrefetch(fragment, reason) {
    const scope = fragmentPrefetchScopes.get(fragment);
    if (!scope) return;
    scope.reasons.delete(reason);
    if (scope.reasons.size) return;
    clearTimeout(scope.timer);
    scope.timer = null;
    scope.started = false;
    cancelTargetCodeScope(scope);
  }

  function installTargetCodeResponse(href, html, preferredButton = null) {
    const response = q('[data-target-code-response]', parseShellHTML(html));
    if (!response) throw new Error('linked-code response was incomplete');
    const readyButton = q('[data-open-diffs]', response);
    const readyTemplate = q('template', response);
    const controls = qa('[data-target-code-href]').filter(candidate => candidate.dataset.targetCodeHref === href);
    const templateID = readyTemplate?.id || controls.find(candidate => candidate.dataset.targetCodeTemplate)?.dataset.targetCodeTemplate || '';
    const staleButtons = templateID ? qa('button[data-open-diffs]').filter(candidate =>
      candidate.dataset.openDiffs === templateID && candidate.getAttribute('aria-label') === 'Open related code') : [];
    const landmarkMarker = ':landmark:';
    const landmarkAt = (response.dataset.targetCodeTarget || '').lastIndexOf(landmarkMarker);
    if (landmarkAt >= 0) {
      const fragmentTarget = response.dataset.targetCodeTarget.slice(0, landmarkAt);
      const landmarkID = response.dataset.targetCodeTarget.slice(landmarkAt + landmarkMarker.length);
      const fragment = qa('.fragment').find(candidate => candidate.dataset.target === fragmentTarget);
      if (fragment) within(fragment, '.landmark-list > div').forEach(row => {
        const anchor = q('a[href^="#"]', row)?.getAttribute('href')?.slice(1) || '';
        const button = q('button[data-open-diffs]', row);
        if (button && anchor.endsWith('--' + landmarkID) && !staleButtons.includes(button)) staleButtons.push(button);
      });
    }
    if (!readyButton || !readyTemplate) {
      controls.forEach(candidate => candidate.remove());
      staleButtons.forEach(candidate => candidate.remove());
      prepareDiffCitations();
      return null;
    }
    const existingTemplate = document.getElementById(readyTemplate.id);
    if (existingTemplate) existingTemplate.replaceWith(readyTemplate);
    else document.body.append(readyTemplate);
    let opener = null;
    controls.forEach(candidate => {
      const replacement = readyButton.cloneNode(true);
      if (candidate === preferredButton) opener = replacement;
      candidate.replaceWith(replacement);
    });
    staleButtons.forEach(candidate => {
      const replacement = readyButton.cloneNode(true);
      if (candidate === preferredButton) opener = replacement;
      candidate.replaceWith(replacement);
    });
    prepareDiffCitations();
    opener ||= qa('button[data-open-diffs]').find(candidate => candidate.dataset.openDiffs === readyButton.dataset.openDiffs) || null;
    return {templateID:readyButton.dataset.openDiffs, opener};
  }

  async function hydrateTargetCode(button) {
    const href = button?.dataset.targetCodeHref;
    if (!href || button.dataset.targetCodeLoading === 'true') return;
    button.dataset.targetCodeLoading = 'true';
    button.setAttribute('aria-busy', 'true');
    try {
      const installed = installTargetCodeResponse(href, await requestTargetCode(href, {interactive:true}), button);
      if (installed) openDrawer(installed.templateID, installed.opener);
    } catch (_) {
      delete button.dataset.targetCodeLoading;
      button.removeAttribute('aria-busy');
      button.title = 'Linked code could not be loaded — try again';
    }
  }

  async function hydrateTargetCodeSummary(button) {
    const href = button?.dataset.targetCodeHref;
    if (!href) return;
    try {
      installTargetCodeResponse(href, await requestTargetCode(href, {priority:20}), button);
    } catch (_) {
      button.title = 'Linked code could not be loaded — try again';
    }
  }

  function installAuxiliaryDiffNext(container, href, cursor) {
    q('[data-aux-file-next]', container)?.remove();
    if (!cursor) return;
    const url = new URL(href, location.href);
    url.searchParams.set('cursor', cursor);
    const button = document.createElement('button');
    button.type = 'button';
    button.dataset.auxFileNext = url.pathname + url.search;
    button.textContent = 'Load more lines';
    button.setAttribute('aria-label', 'Load the next file chunk');
    container.append(button);
  }

  async function appendAuxiliaryDiff(button) {
    const container = button.parentElement;
    const href = button.dataset.auxFileNext;
    if (!container || !href || button.dataset.loading === 'true') return;
    button.dataset.loading = 'true';
    try {
      const result = await fetchFileDiff(href);
      const wrapper = parseShellHTML(result.html);
      const items = q('[data-page-items="lines"]', wrapper);
      const rows = items ? qa('.diff-row', items) : [];
      button.before(...rows);
      installAuxiliaryDiffNext(container, href, result.next);
      highlightCode(container);
      prepareContext(container);
      applyDiffLayout(diffLayout);
    } catch (_) {
      button.textContent = 'Could not load more — try again';
      delete button.dataset.loading;
    }
  }

  async function hydrateChapter(chapter) {
    const href = chapter?.dataset.sectionHref;
    const body = chapter ? q('[data-chapter-body]', chapter) : null;
    if (!href || !body) return;
    if (chapter.dataset.sectionLoading === 'true') { await fetchShell(href).catch(() => {}); return; }
    chapter.dataset.sectionLoading = 'true';
    try {
      const wrapper = parseShellHTML(await fetchShell(href));
      body.replaceChildren(...Array.from(wrapper.childNodes));
      delete chapter.dataset.sectionHref;
      observeDeferredFragments(body);
    } catch (_) {
      const placeholder = q('[data-section-placeholder]', body);
      if (placeholder) placeholder.textContent = 'This chapter could not be loaded. Close and reopen to try again.';
    } finally {
      delete chapter.dataset.sectionLoading;
    }
  }

  function installFragmentContents(article, replacement) {
    const wasActive = article.classList.contains('active-fragment');
    for (const attribute of Array.from(article.attributes)) {
      if (!replacement.hasAttribute(attribute.name)) article.removeAttribute(attribute.name);
    }
    for (const attribute of Array.from(replacement.attributes)) article.setAttribute(attribute.name, attribute.value);
    if (wasActive) article.classList.add('active-fragment');
    article.removeAttribute('data-fragment-href');
    delete article.dataset.fragmentLoading;
    article.replaceChildren(...Array.from(replacement.childNodes));
    prepareLandmarks(article);
    prepareDiffCitations(article);
    highlightCode(article);
    positionLandmarkHotspots();
    return article;
  }

  async function hydrateFragment(article) {
    const href = article?.dataset.fragmentHref;
    if (!href) return article || null;
    if (article.dataset.fragmentLoading === 'true') { await fetchShell(href).catch(() => {}); return article; }
    article.dataset.fragmentLoading = 'true';
    try {
      const replacement = q('.fragment', parseShellHTML(await fetchShell(href)));
      if (!replacement) throw new Error('explanation response was incomplete');
      // The article itself is never swapped out. A reviewer can be part way
      // through clicking a descriptor's controls when its content arrives, and
      // replacing the element under the pointer loses that click: the detached
      // node no longer reaches the document that handles it. Filling the article
      // in place keeps its head where it was and every live control attached,
      // and keeps this explanation the active one without re-selecting it.
      return installFragmentContents(article, replacement);
    } catch (_) {
      delete article.dataset.fragmentLoading;
      const placeholder = q('[data-fragment-placeholder]', article);
      if (placeholder) placeholder.textContent = 'This explanation could not be loaded. Reload the page to try again.';
      return article;
    }
  }

  // A fragment is fetched when it is close enough to be read. Chapter bodies
  // stay hidden until they are opened, so nothing inside a closed chapter is
  // observed and nothing inside it is fetched.
  const fragmentObserver = typeof IntersectionObserver === 'function' ? new IntersectionObserver(entries => {
    entries.forEach(entry => {
      if (!entry.isIntersecting) return;
      fragmentObserver.unobserve(entry.target);
      void hydrateFragment(entry.target);
    });
  }, {rootMargin:'400px'}) : null;

  // Returns when everything this call decided to fetch has arrived, so the page
  // can say when it has finished filling itself in.
  function observeDeferredFragments(root = document) {
    const arriving = [];
    within(root, '[data-fragment-href]').forEach(article => {
      if (!fragmentObserver) { arriving.push(hydrateFragment(article)); return; }
      // What is already on screen is fetched now rather than one frame later.
      // The observer's first callback costs a frame the reviewer would spend
      // looking at a placeholder, and reflowing under a pointer that has
      // already arrived is worse than fetching a little too eagerly.
      if (article.getBoundingClientRect().top <= innerHeight + 400) { arriving.push(hydrateFragment(article)); return; }
      fragmentObserver.observe(article);
    });
    return Promise.all(arriving);
  }

  // A permalink can name a heading, a marked place, or a comment inside a
  // chapter nobody has opened yet. The server answers where one anchor lives;
  // shipping the same answer as an index would put every anchor in the document
  // into every first load, which is the cost this shell exists to remove.
  const anchorPlaces = new Map();

  function locateAnchor(id) {
    let request = anchorPlaces.get(id);
    if (!request) {
      request = fetch('/api/locate?anchor=' + encodeURIComponent(id), {headers:{Accept:'application/json'}})
        .then(response => response.ok ? response.json() : null)
        .catch(() => null);
      anchorPlaces.set(id, request);
    }
    return request;
  }

  async function revealAnchor(id) {
    if (!id) return null;
    let element = document.getElementById(id);
    const pending = !element ||
      element.closest('[data-section-href]') !== null ||
      element.closest('[data-fragment-href]') !== null;
    if (pending) {
      const place = await locateAnchor(id);
      if (place?.chapter) await hydrateChapter(document.getElementById(place.chapter));
      if (place?.fragment) await hydrateFragment(document.getElementById(place.fragment));
      element = document.getElementById(id);
    }
    const destination = document.getElementById(id);
    const chapter = destination?.closest('[data-chapter]');
    if (chapter) await setChapterOpen(chapter, true);
    return document.getElementById(id);
  }

  // Coverage shows the same bodies to answer a different question, so it uses
  // the same per-file endpoint and the same cache. Only the disclosure that
  // owns a surface hydrates it, so opening one narrative target does not pull
  // in every file underneath it.
  async function hydrateManifestDiff(surface) {
    if (surface.dataset.manifestDiffLoaded === 'true' || surface.dataset.manifestDiffLoading === 'true') return;
    const href = surface.dataset.manifestDiffHref;
    const rows = q('[data-manifest-diff-rows]', surface);
    if (!href || !rows) return;
    surface.dataset.manifestDiffLoading = 'true';
    surface.classList.add('loading');
    try {
      const wrapper = document.createElement('div');
      const result = await fetchFileDiff(href);
      wrapper.innerHTML = result.html;
      rows.replaceChildren(...Array.from(wrapper.childNodes));
	  installAuxiliaryDiffNext(rows, href, result.next);
      surface.dataset.manifestDiffLoaded = 'true';
      highlightCode(rows);
    } catch (_) {
      const placeholder = q('[data-diff-placeholder]', rows);
      if (placeholder) placeholder.textContent = 'This file diff could not be loaded. Close and reopen to try again.';
    } finally {
      delete surface.dataset.manifestDiffLoading;
      surface.classList.remove('loading');
    }
  }

  function continuedPageURL(href, cursor) {
    if (!cursor) return '';
    const url = new URL(href, location.href);
    url.searchParams.set('cursor', cursor);
    return url.pathname + url.search;
  }

  async function hydrateCoverageFile(details) {
    if (!details?.open || details.dataset.coverageFileLoaded === 'true' || details.dataset.coverageFileLoading === 'true') return;
    let href = details.dataset.coverageFileHref;
    const destination = q('[data-coverage-file-mappings]', details);
    if (!href || !destination) return;
    details.dataset.coverageFileLoading = 'true';
    let first = true;
    try {
      while (href && details.isConnected) {
        const response = await fetch(href, {headers:{Accept:'text/html','X-Change-Saga-Async':'true'},credentials:'same-origin'});
        if (response.status === 202) {
          await new Promise(resolve => setTimeout(resolve, retryDelay(response)));
          continue;
        }
        if (!response.ok) throw new Error('file coverage request failed');
        const page = q('[data-coverage-file-response]', parseShellHTML(await response.text()));
        if (!page) throw new Error('file coverage response was incomplete');
        const items = q('[data-page-items="coverage-file"]', page);
        if (!items) throw new Error('file coverage response was incomplete');
        const inserted = Array.from(items.childNodes);
        if (first) destination.replaceChildren(...inserted); else destination.append(...inserted);
        first = false;
        href = continuedPageURL(href, response.headers.get('X-Change-Saga-Next-Cursor') || page.dataset.nextCursor);
        await new Promise(resolve => setTimeout(resolve, 0));
      }
      details.dataset.coverageFileLoaded = 'true';
      if (!destination.childNodes.length) destination.innerHTML = '<p class="diff-placeholder">This file is not explained yet.</p>';
    } catch (_) {
      const placeholder = q('.diff-placeholder', destination);
      if (placeholder) placeholder.textContent = 'Coverage details could not be loaded. Close and reopen to try again.';
    } finally {
      delete details.dataset.coverageFileLoading;
    }
  }

  async function hydrateCoverageTarget(details) {
    if (!details?.open || details.dataset.coverageTargetLoaded === 'true' || details.dataset.coverageTargetLoading === 'true') return;
    let href = details.dataset.coverageTargetHref;
    const destination = q('[data-coverage-target-files]', details);
    if (!href || !destination) return;
    details.dataset.coverageTargetLoading = 'true';
    let first = true;
    try {
      while (href && details.isConnected) {
        const response = await fetch(href, {headers:{Accept:'text/html','X-Change-Saga-Async':'true'},credentials:'same-origin'});
        if (response.status === 202) {
          await new Promise(resolve => setTimeout(resolve, retryDelay(response)));
          continue;
        }
        if (!response.ok) throw new Error('target coverage request failed');
        const page = q('[data-coverage-target-response]', parseShellHTML(await response.text()));
        if (!page) throw new Error('target coverage response was incomplete');
        const items = q('[data-page-items="target-files"]', page);
        if (!items) throw new Error('target coverage response was incomplete');
        const inserted = Array.from(items.childNodes);
        if (first) destination.replaceChildren(...inserted); else destination.append(...inserted);
        first = false;
        href = continuedPageURL(href, response.headers.get('X-Change-Saga-Next-Cursor') || page.dataset.nextCursor);
        filterManifest();
        await new Promise(resolve => setTimeout(resolve, 0));
      }
      details.dataset.coverageTargetLoaded = 'true';
      if (!destination.childNodes.length) destination.innerHTML = '<p class="diff-placeholder">This part of the story has no linked files.</p>';
    } catch (_) {
      const placeholder = q('.diff-placeholder', destination);
      if (placeholder) placeholder.textContent = 'Linked files could not be loaded. Close and reopen to try again.';
    } finally {
      delete details.dataset.coverageTargetLoading;
    }
  }

  function hydrateOpenedManifestDiffs(details) {
    if (!details?.open) return;
    qa('[data-manifest-diff-href]', details)
      .filter(surface => surface.closest('details') === details)
      .forEach(hydrateManifestDiff);
  }

  async function openFragmentDrawer(anchor, opener) {
	anchor = decodeURIComponent(String(anchor || '').replace(/^#/, ''));
    const destination = await revealAnchor(anchor);
    let fragment = destination?.matches('.fragment') ? destination : destination?.closest('.fragment');
    let borrowed = Boolean(fragment);
    if (!fragment) {
      // The explanation belongs to another page, such as its feature's: fetch it
      // for the drawer instead of moving it out of this page.
      const place = await locateAnchor(anchor);
      if (!place?.target) return;
      try {
        fragment = q('.fragment', parseShellHTML(await fetchShell('/api/fragment?target=' + encodeURIComponent(place.target))));
      } catch (_) { fragment = null; }
      if (!fragment) return;
    }
    restoreDrawerContent();
    if (borrowed) {
      const placeholder = document.createComment('change-saga fragment drawer');
      fragment.replaceWith(placeholder);
      drawerRestore = {fragment, placeholder};
    }
    q('.drawer-body').append(fragment);
    if (!borrowed) { prepareLandmarks(fragment); highlightCode(fragment); }
    configureDrawer('fragment', fragment.dataset.fragmentTitle || 'Related explanation');
    setActiveFragment(fragment);
    showDrawer(opener);
    positionLandmarkHotspots();
    requestAnimationFrame(() => {
      const visual = q('[data-landmark-visual="' + CSS.escape(anchor) + '"]', fragment);
      (visual || destination).scrollIntoView({block:'center'});
    });
  }

  function closeDrawer() {
    const drawer = q('.diff-drawer');
    if (!drawer) return;
    const wasOpen = drawer.classList.contains('open');
    drawer.classList.remove('open');
    drawer.setAttribute('aria-hidden', 'true');
    // Return focus before the drawer becomes inert. WebKit does not always make
    // a clicked close button active, so do not condition this on activeElement
    // still being inside the drawer.
    if (wasOpen && drawerOpener?.isConnected) drawerOpener.focus();
    drawer.setAttribute('inert', '');
    q('.drawer-backdrop').classList.remove('open');
    document.body.style.overflow = '';
    restoreDrawerContent();
    drawerOpener = null;
    configureDrawer('code', 'Linked code');
  }

  document.addEventListener('pointerover', event => {
    const slideDiffButton = slideDiffSummaryButton(event.target);
    if (event.pointerType !== 'touch' && slideDiffButton && !(event.relatedTarget instanceof Node && slideDiffButton.contains(event.relatedTarget))) {
      setSlideDiffPreview(slideDiffButton, 'pointer', true);
    }
    const fragment = event.target.closest('.fragment');
    if (fragment) setActiveFragment(fragment);
    if (event.pointerType !== 'touch' && fragment && !(event.relatedTarget instanceof Node && fragment.contains(event.relatedTarget))) {
      beginFragmentPrefetch(fragment, 'pointer');
    }
  });

  document.addEventListener('pointerout', event => {
    const slideDiffButton = slideDiffSummaryButton(event.target);
    if (event.pointerType !== 'touch' && slideDiffButton && !(event.relatedTarget instanceof Node && slideDiffButton.contains(event.relatedTarget))) {
      setSlideDiffPreview(slideDiffButton, 'pointer', false);
    }
    const fragment = event.target.closest('.fragment');
    if (event.pointerType !== 'touch' && fragment && !(event.relatedTarget instanceof Node && fragment.contains(event.relatedTarget))) {
      endFragmentPrefetch(fragment, 'pointer');
    }
  });

  document.addEventListener('focusin', event => {
    const slideDiffButton = slideDiffSummaryButton(event.target);
    if (slideDiffButton && !(event.relatedTarget instanceof Node && slideDiffButton.contains(event.relatedTarget))) {
      setSlideDiffPreview(slideDiffButton, 'focus', true);
    }
    const fragment = event.target.closest('.fragment');
    if (fragment) setActiveFragment(fragment);
    if (fragment && !(event.relatedTarget instanceof Node && fragment.contains(event.relatedTarget))) beginFragmentPrefetch(fragment, 'focus');
  });

  document.addEventListener('focusout', event => {
    const slideDiffButton = slideDiffSummaryButton(event.target);
    if (slideDiffButton && !(event.relatedTarget instanceof Node && slideDiffButton.contains(event.relatedTarget))) {
      setSlideDiffPreview(slideDiffButton, 'focus', false);
    }
    const fragment = event.target.closest('.fragment');
    if (fragment && !(event.relatedTarget instanceof Node && fragment.contains(event.relatedTarget))) endFragmentPrefetch(fragment, 'focus');
  });

  document.addEventListener('click', event => {
    const slideThumbnail = event.target.closest?.('[data-slide-thumbnail]');
    if (slideThumbnail) {
      const slides = deckViewerSlides();
      const index = slides.findIndex(slide => slide.dataset.slideTarget === slideThumbnail.dataset.slideTarget);
      if (index >= 0) {
        if (q('[data-view="slides"]')) setView('slides');
        activateDeckSlide(index, true);
      }
      return;
    }
    if (event.target.closest?.('[data-slide-present],[data-slide-exit-presentation]')) {
      void toggleSlidePresentation();
      return;
    }
    const slideDirection = event.target.closest?.('[data-slide-previous],[data-slide-next]');
    if (slideDirection) {
      stepDeckSlide(slideDirection.matches('[data-slide-next]') ? 1 : -1);
      return;
    }
    const retryFile = event.target.closest?.('[data-retry-file]');
    if (retryFile) {
      void hydrateReviewFile(retryFile.closest('[data-file-diff-href]'), {force:true});
      return;
    }
    const nextAuxiliaryPage = event.target.closest?.('[data-aux-file-next]');
    if (nextAuxiliaryPage) {
      void appendAuxiliaryDiff(nextAuxiliaryPage);
      return;
    }
    const retrySurface = event.target.closest?.('[data-retry-surface]');
    if (retrySurface) {
      void hydrateReviewSurface(retrySurface.dataset.retrySurface, {force:true});
      return;
    }
    const nextSurfacePage = event.target.closest?.('[data-surface-next]');
    if (nextSurfacePage) {
      event.preventDefault();
      const surface = nextSurfacePage.closest('[data-review-surface]');
      if (surface?.dataset.reviewSurface === 'manifest') beginContinuousCoverageLoad(surface);
      else void loadReviewSurfacePage(nextSurfacePage);
      return;
    }
    const boundedLink = event.target.closest?.('a[href]');
    if (boundedLink && !boundedLink.hasAttribute('data-open-fragment') && !boundedLink.getAttribute('href')?.startsWith('#') && !event.defaultPrevented && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey && (!boundedLink.target || boundedLink.target === '_self')) {
      const destination = new URL(boundedLink.href, location.href);
      const view = destination.searchParams.get('view');
      if (destination.origin === location.origin && destination.pathname === location.pathname && (view === 'code' || view === 'manifest')) {
        event.preventDefault();
        history.pushState({view}, '', destination);
        setView(view, false);
        if (boundedLink.closest('.diff-drawer.open')) closeDrawer();
        return;
      }
    }
    const fragmentDrawerLink = event.target.closest?.('[data-open-fragment]');
    if (fragmentDrawerLink) {
      event.preventDefault();
      openFragmentDrawer(fragmentDrawerLink.dataset.openFragment, fragmentDrawerLink);
      return;
    }
    const sagaLink = event.target.closest?.('a[href^="#"]');
    // A hydrated Markdown citation is still an in-page link, but linked-code
    // activation takes precedence over its original footnote navigation.
    if (sagaLink && !sagaLink.hasAttribute('data-open-diffs')) {
      event.preventDefault();
      const id = decodeURIComponent(sagaLink.getAttribute('href').slice(1));
      const sagaURL = new URL(location.href);
      ['view', 'file', 'ref', 'mode'].forEach(key => sagaURL.searchParams.delete(key));
      sagaURL.hash = id;
      history.pushState({view:'saga'}, '', sagaURL);
      if (sagaLink.closest('.diff-drawer.open')) closeDrawer();
      // pushState deliberately does not dispatch hashchange or perform native
      // anchor scrolling. Run the same lazy reveal, view switch, highlight,
      // and scroll path used for initial and browser-history navigation.
      if (id) void activateLandmark();
      else setView('saga', false);
      return;
    }
    const permalink = event.target.closest('[data-copy-link]');
    if (permalink) { copyPermalink(permalink); return; }
    const docTwisty = event.target.closest('[data-doc-twisty]');
    if (docTwisty) { toggleDocNode(docTwisty); return; }
    const deckToggle = event.target.closest('[data-deck-toggle]');
    if (deckToggle) { toggleDocNode(deckToggle); return; }
    const docToggle = event.target.closest('[data-doc-toggle]');
    if (docToggle) { toggleDocNode(docToggle); return; }
    const reportNav = event.target.closest('[data-report-nav]');
    if (reportNav && q('[data-view="slides"].active')) setView('saga');
    const chapterToggle = event.target.closest('[data-chapter-toggle]');
    if (chapterToggle) { toggleChapter(chapterToggle); return; }
    const viewTab = event.target.closest('[data-view-tab]');
    if (viewTab) { setView(viewTab.dataset.viewTab); return; }
    const historyButton = event.target.closest('[data-open-history]');
    if (historyButton) { event.preventDefault(); void openHistoryDrawer(historyButton.dataset.historyHref, historyButton); return; }
    const manifestMode = event.target.closest('[data-manifest-mode]');
    if (manifestMode) { void activateManifestMode(manifestMode.dataset.manifestMode); return; }
    const treeToggle = event.target.closest('[data-toggle-tree]');
    if (treeToggle) {
      const hidden = q('[data-shell]')?.classList.contains('tree-hidden');
      setTreeVisible(Boolean(hidden));
      if (hidden) setTimeout(() => q('[data-file-filter]')?.focus(), 0);
      else q('.code-toolbar [data-toggle-tree]')?.focus();
      return;
    }
    const relatedToggle = event.target.closest('[data-toggle-related]');
    if (relatedToggle) {
      const hidden = q('[data-code-workspace]')?.classList.contains('related-hidden');
      setRelatedVisible(Boolean(hidden));
      if (!hidden) q('.code-toolbar [data-toggle-related]')?.focus();
      return;
    }
    // Scoped to the toolbar buttons: a diff surface also carries data-layout.
    const layout = event.target.closest('button[data-layout]');
    if (layout) { applyDiffLayout(layout.dataset.layout); return; }
    const targetCodeButton = event.target.closest('[data-target-code-href]');
    if (targetCodeButton) { event.preventDefault(); void hydrateTargetCode(targetCodeButton); return; }
    const drawerButton = event.target.closest('[data-open-diffs]');
    if (drawerButton) { event.preventDefault(); openDrawer(drawerButton.dataset.openDiffs, drawerButton); return; }
    if (event.target.closest('[data-close-drawer]')) { closeDrawer(); return; }
    const fragment = event.target.closest('.fragment');
    if (fragment) setActiveFragment(fragment);
  });

  // An observed Coverage row renders its record's code only when opened.
  async function hydrateLazyDetails(details) {
    if (details.dataset.lazyLoaded === 'true' || details.dataset.lazyLoading === 'true') return;
    const body = q('[data-lazy-body]', details);
    if (!body) return;
    details.dataset.lazyLoading = 'true';
    try {
      const response = await fetch(details.dataset.lazyHref, {headers:{Accept:'text/html'},credentials:'same-origin'});
      if (!response.ok) throw new Error('request failed');
      body.innerHTML = await response.text();
      details.dataset.lazyLoaded = 'true';
      highlightCode(body);
    } catch (_) {
      body.innerHTML = '<p class="diff-placeholder">This code could not be loaded. Close and reopen to try again.</p>';
    } finally {
      delete details.dataset.lazyLoading;
    }
  }

  // The overview's coverage line starts as a placeholder and is replaced by
  // the totals once they can be read. A comparison that is still building
  // answers 202, and the line asks again; a failure leaves a quiet pointer
  // to the Coverage tab instead of a spinner that never ends.
  async function loadCoverageTotals() {
    const line = q('[data-coverage-loading]');
    const href = line?.dataset.totalsHref;
    if (!line || !href) return;
    for (let attempt = 0; attempt < 120 && line.isConnected; attempt++) {
      let response;
      try {
        response = await fetch(href, {headers:{Accept:'text/html','X-Change-Saga-Async':'true'},credentials:'same-origin'});
      } catch (_) { break; }
      if (response.status === 202) {
        await new Promise(resolve => setTimeout(resolve, retryDelay(response)));
        continue;
      }
      if (!response.ok) break;
      const replacement = q('[data-coverage-totals]', parseShellHTML(await response.text()));
      if (replacement) { line.replaceWith(replacement); return; }
      break;
    }
    if (line.isConnected) {
      line.classList.remove('loading');
      line.removeAttribute('data-coverage-loading');
      line.textContent = 'Coverage is on the Coverage tab.';
    }
  }

  document.addEventListener('toggle', event => {
    const details = event.target.closest?.('details');
    if (!details) return;
    if (!details.open) {
      if (details.dataset.fileDiffHref) cancelReviewFile(details);
      return;
    }
    if (details.dataset.fileDiffHref) void hydrateReviewFile(details);
    if (details.dataset.coverageFileHref) void hydrateCoverageFile(details);
    if (details.dataset.coverageTargetHref) void hydrateCoverageTarget(details);
    if (details.dataset.lazyHref) void hydrateLazyDetails(details);
    hydrateOpenedManifestDiffs(details);
  }, true);

  document.addEventListener('input', event => {
    if (event.target.matches?.('[data-file-filter]')) filterTree();
    if (event.target.matches?.('[data-manifest-filter]')) filterManifest();
  });

  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && document.body.classList.contains('presentation-fallback')) {
      document.body.classList.remove('presentation-fallback');
      syncSlidePresentation();
      return;
    }
    if ((event.key === 'ArrowLeft' || event.key === 'ArrowRight') && deckViewerActive() && deckViewerSlides().length && !event.target.matches?.('input,textarea,select,[contenteditable="true"]')) {
      event.preventDefault();
      stepDeckSlide(event.key === 'ArrowRight' ? 1 : -1);
      return;
    }
    const workspaceTab = event.target.closest?.('[data-view-tab]');
    if (workspaceTab && ['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
      const tabs = qa('[data-view-tab]');
      const index = tabs.indexOf(workspaceTab);
      const step = event.key === 'ArrowLeft' ? -1 : 1;
      const next = event.key === 'Home' ? tabs[0]
        : event.key === 'End' ? tabs[tabs.length - 1]
        : tabs[(index + step + tabs.length) % tabs.length];
      event.preventDefault();
      setView(next.dataset.viewTab);
      next.focus();
      return;
    }
    const treeItem = event.target.closest('.file-tree [role=treeitem]');
    if (treeItem && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      const items = qa('.file-tree [role=treeitem]').filter(item => !item.hidden && !item.closest('[hidden]'));
      const index = items.indexOf(treeItem);
      const next = items[index + (event.key === 'ArrowDown' ? 1 : -1)];
      if (next) next.focus();
      event.preventDefault();
      return;
    }
    if (event.key === 'Escape') closeDrawer();
  });

  prepareLandmarks();
  prepareDiffCitations();
  const shellArriving = observeDeferredFragments();

  const firstFragment = q('.fragment');
  if (firstFragment) setActiveFragment(firstFragment);
  q('[data-file-filter]')?.addEventListener('input', filterTree);
  prepareDirectories();
  q('[data-manifest-filter]')?.addEventListener('input', filterManifest);
  prepareContext();
  syncSlidePresentation();
  highlightCode();
  applyDiffLayout('inline');
  addEventListener('resize', () => { applyDiffLayout(diffLayout); positionLandmarkHotspots(); });
  document.addEventListener('fullscreenchange', syncSlidePresentation);
  const requestedView = new URL(location.href).searchParams.get('view');
  const initialView = requestedView === 'code' || requestedView === 'manifest' || requestedView === 'slides' || requestedView === 'change' ? requestedView : 'saga';
  setView(initialView, false);
  setManifestMode('code');
  const anchorResolving = initialView === 'saga' || initialView === 'slides'
    ? activateLandmark()
    : hydrateReviewSurface(initialView);
  syncDeckSlideForHash();
  // The page arrives as a shell and fills in what is on screen. Saying when
  // that has finished is the difference between a reviewer who can see the
  // page has settled and automation that would otherwise have to guess.
  void Promise.all([shellArriving, anchorResolving]).then(() => {
    document.body.dataset.shellReady = 'true';
  });
  void loadLayers();
  void loadCoverageTotals();
  positionLandmarkHotspots();
  globalThis.requestAnimationFrame?.(positionLandmarkHotspots);
  addEventListener('hashchange', () => {
    syncDeckSlideForHash();
    const view = new URL(location.href).searchParams.get('view');
    if (view === 'code' || view === 'manifest') void hydrateReviewSurface(view);
    else void activateLandmark();
  });
  addEventListener('popstate', () => {
    const view = new URL(location.href).searchParams.get('view');
    setView(view === 'code' || view === 'manifest' || view === 'slides' || view === 'change' ? view : 'saga', false);
  });
})();`
