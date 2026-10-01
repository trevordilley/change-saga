package server

// reviewLinesJavaScript lets a reviewer comment on a line of a diff inside a
// review: an Item's linked code in its drawer, or a file of the Code Diff.
// It is included in the page script's closure and made for each review page.
//
// Diff pages arrive lazily and are cached, so they carry no discussion. When
// a diff's first lines arrive, its threads are read once from
// /reviews/<id>/line-threads and each is placed under its line; lines that
// arrive on a later page take their threads as they come, and threads whose
// line is not shown stay listed below the diff. Every line gets a "+" button
// in its gutter, shown on hover or focus, that opens a composer under it;
// Shift-click on another line of the same side extends the range.
const reviewLinesJavaScript = `
  let reviewLines = null;
  function makeReviewLines(signal) {
    const deck = q('#page [data-review-deck]');
    if (!deck) return null;
    const reviewID = deck.dataset.review;
    const rowSelector = 'tr.review-line.add,tr.review-line.del,tr.review-line.ctx,.diff-row.new,.diff-row.old,.diff-row.context';
    const containerSelector = '.drawer-body .review-item-panel[data-review-target],[data-review-surface="code"] article.file-diff[data-file-path]';
    const holderSelector = '.review-line-thread-row,.diff-thread-row';
    const entries = new Map();
    let lastComposer = null;

    function containerFor(node) {
      return node?.closest?.(containerSelector) || null;
    }
    // A row's line: the new side's number for added and unchanged lines,
    // the old side's for deleted ones. It is read once, before the row gets
    // its button.
    function rowLine(row) {
      if (row.dataset.lineComment) {
        const [side, line] = row.dataset.lineComment.split(':');
        return {side, line:Number(line), path:row.dataset.lineCommentPath};
      }
      const table = row.matches('tr');
      const numbers = table ? row.querySelectorAll(':scope > td.review-lineno') : row.querySelectorAll(':scope > .line-no');
      const old = row.classList.contains(table ? 'del' : 'old');
      const line = parseInt((old ? numbers[0] : numbers[1])?.textContent || '', 10);
      const path = table ? row.closest('[data-review-diff-path]')?.dataset.reviewDiffPath : (row.dataset.path || row.closest('[data-file-path]')?.dataset.filePath);
      if (!path || !(line > 0)) return null;
      row.dataset.lineComment = (old ? 'old' : 'new') + ':' + line;
      row.dataset.lineCommentPath = path;
      return {side:old ? 'old' : 'new', line, path};
    }
    function rowsOf(container) {
      return qa(rowSelector, container).filter(row => !row.closest('[data-review-line-threads]'));
    }
    function lineName(side, start, end, path) {
      return (start === end ? 'line ' + start : 'lines ' + start + '–' + end) + ' of ' + path + (side === 'old' ? ' (deleted side)' : '');
    }
    function load(container) {
      let entry = entries.get(container);
      if (entry) return entry.ready;
      const query = container.matches('.review-item-panel') ? 'target=' + encodeURIComponent(container.dataset.reviewTarget) : 'path=' + encodeURIComponent(container.dataset.filePath);
      entry = {threads:new Map(), section:null, composer:null};
      entry.ready = fetch('/reviews/' + encodeURIComponent(reviewID) + '/line-threads?' + query, {headers:{Accept:'text/html'}, credentials:'same-origin', signal})
        .then(response => { if (!response.ok) throw new Error('line comments unavailable'); return response.text(); })
        .then(html => {
          if (!container.isConnected) return null;
          const template = document.createElement('template');
          template.innerHTML = html;
          const section = template.content.querySelector('[data-review-line-threads]');
          if (!section) return null;
          entry.section = section;
          entry.composer = q('template[data-review-line-composer]', section);
          qa('[data-review-line-thread]', section).forEach(thread => entry.threads.set(thread.dataset.reviewLineThread, thread));
          container.append(section);
          return entry;
        })
        .catch(() => { entries.delete(container); return null; });
      entries.set(container, entry);
      return entry.ready;
    }
    // below puts node in a full-width row under row, after the threads and
    // composers already there.
    function below(row, node) {
      let anchor = row;
      while (anchor.nextElementSibling?.matches(holderSelector)) anchor = anchor.nextElementSibling;
      let holder;
      if (row.matches('tr')) {
        holder = document.createElement('tr');
        holder.className = 'review-line-thread-row';
        const cell = document.createElement('td');
        cell.colSpan = 3;
        cell.append(node);
        holder.append(cell);
      } else {
        holder = document.createElement('div');
        holder.className = 'diff-thread-row';
        holder.append(node);
      }
      anchor.after(holder);
      return holder;
    }
    function holderOf(node) {
      return node.parentElement?.closest(holderSelector) || null;
    }
    function decorate(container, entry) {
      const writable = Boolean(entry.composer);
      for (const row of rowsOf(container)) {
        const line = rowLine(row);
        if (!line || !writable || row.querySelector(':scope [data-review-line-add]')) continue;
        const cells = row.matches('tr') ? row.querySelectorAll(':scope > td.review-lineno') : row.querySelectorAll(':scope > .line-no');
        const cell = cells[1] || cells[0];
        if (!cell) continue;
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'review-line-add';
        button.dataset.reviewLineAdd = '';
        button.textContent = '+';
        button.setAttribute('aria-label', 'Comment on ' + lineName(line.side, line.line, line.line, line.path));
        button.title = 'Comment on this line; Shift-click another line to comment on a range';
        cell.append(button);
      }
    }
    function place(container, entry) {
      const rows = new Map();
      for (const row of rowsOf(container)) {
        const line = rowLine(row);
        const key = line && line.side + ':' + line.path + ':' + line.line;
        if (key && !rows.has(key)) rows.set(key, row);
      }
      const unplaced = q('[data-review-line-unplaced]', entry.section);
      for (const thread of entry.threads.values()) {
        const holder = holderOf(thread);
        if (holder?.isConnected && container.contains(holder)) continue;
        holder?.remove();
        const row = rows.get(thread.dataset.lineSide + ':' + thread.dataset.linePath + ':' + thread.dataset.lineEnd);
        if (row) below(row, thread);
        else if (thread.parentElement !== unplaced) unplaced.append(thread);
      }
      const heading = q('[data-review-line-unplaced-heading]', entry.section);
      if (heading) heading.hidden = !unplaced.children.length;
    }
    async function refresh(container) {
      const entry = await load(container);
      if (!entry || !container.isConnected) return;
      if (!entry.section.isConnected) container.append(entry.section);
      decorate(container, entry);
      place(container, entry);
    }
    const pending = new Set();
    let scheduled = false;
    function schedule(container) {
      pending.add(container);
      if (scheduled) return;
      scheduled = true;
      queueMicrotask(() => {
        scheduled = false;
        const containers = [...pending];
        pending.clear();
        containers.forEach(container => { if (container.isConnected) void refresh(container); });
      });
    }
    const observer = new MutationObserver(records => {
      for (const record of records) for (const node of record.addedNodes) {
        if (!(node instanceof Element) || node.matches(holderSelector) || node.closest('[data-review-line-threads]')) continue;
        const container = containerFor(node);
        if (container) {
          if (node.matches(rowSelector) || node.querySelector(rowSelector)) schedule(container);
          continue;
        }
        qa(containerSelector, node).forEach(schedule);
      }
    });
    observer.observe(document.body, {childList:true, subtree:true});
    signal.addEventListener('abort', () => observer.disconnect());
    qa(containerSelector).forEach(schedule);

    function composerLabel(form) {
      const label = q('[data-review-line-composer-label]', form);
      const start = Number(form.elements.line.value), end = Number(form.elements.end_line.value || start);
      if (label) label.textContent = 'Comment on ' + lineName(form.elements.side.value, start, end, form.elements.path.value);
    }
    function openComposer(button, extend) {
      const row = button.closest(rowSelector);
      const container = containerFor(row);
      const entry = container && entries.get(container);
      const line = row && rowLine(row);
      if (!entry?.composer || !line) return;
      if (extend && lastComposer?.isConnected && container.contains(lastComposer) && lastComposer.elements.path.value === line.path && lastComposer.elements.side.value === line.side) {
        const form = lastComposer;
        const start = Math.min(Number(form.elements.line.value), line.line);
        const end = Math.max(Number(form.elements.end_line.value || form.elements.line.value), line.line);
        form.elements.line.value = String(start);
        form.elements.end_line.value = end === start ? '' : String(end);
        const endRow = rowsOf(container).find(candidate => { const at = rowLine(candidate); return at && at.side === line.side && at.path === line.path && at.line === end; });
        if (endRow) { const holder = holderOf(form); below(endRow, form); holder?.remove(); }
        composerLabel(form);
        q('textarea', form)?.focus();
        return;
      }
      const existing = qa('form[data-review-line-composer-form]', container).find(form => form.elements.path.value === line.path && form.elements.side.value === line.side && Number(form.elements.line.value) === line.line);
      if (existing) { lastComposer = existing; q('textarea', existing)?.focus(); return; }
      const form = entry.composer.content.firstElementChild.cloneNode(true);
      form.elements.path.value = line.path;
      form.elements.side.value = line.side;
      form.elements.line.value = String(line.line);
      form.dataset.opener = row.dataset.lineComment;
      composerLabel(form);
      below(row, form);
      lastComposer = form;
      q('textarea', form)?.focus();
    }
    function closeComposer(form) {
      const container = containerFor(form);
      const opener = container && rowsOf(container).find(row => row.dataset.lineComment === form.dataset.opener && row.dataset.lineCommentPath === form.elements.path.value);
      const holder = holderOf(form);
      (holder || form).remove();
      if (lastComposer === form) lastComposer = null;
      q('[data-review-line-add]', opener || document.createElement('div'))?.focus();
    }
    document.addEventListener('click', event => {
      const add = event.target.closest?.('[data-review-line-add]');
      if (add) { event.preventDefault(); openComposer(add, event.shiftKey); return; }
      const cancel = event.target.closest?.('[data-review-line-cancel]');
      if (cancel) { event.preventDefault(); closeComposer(cancel.closest('form')); }
    }, {signal});
    document.addEventListener('keydown', event => {
      const form = event.target.closest?.('form.review-line-comment-form');
      if (!form) return;
      if (event.key === 'Escape' && form.matches('[data-review-line-composer-form]')) {
        event.preventDefault();
        event.stopPropagation();
        closeComposer(form);
      } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        form.requestSubmit(q('button[type=submit]', form));
      }
    }, {signal, capture:true});

    function updateCounts(counts) {
      const roots = [document, ...qa('template[id^="review-item-"]').map(template => template.content)];
      for (const [target, count] of Object.entries(counts || {})) {
        for (const root of roots) for (const node of qa('[data-review-line-count-for]', root).filter(node => node.dataset.reviewLineCountFor === target)) {
          node.hidden = !count;
          const number = q('[data-review-line-count]', node);
          if (number) number.textContent = String(count);
          const label = q('[data-review-line-count-label]', node);
          if (label) label.textContent = count === 1 ? 'open line comment' : 'open line comments';
        }
      }
    }
    document.addEventListener('submit', async event => {
      const form = event.target;
      if (!(form instanceof HTMLFormElement) || !form.matches('form.review-line-comment-form')) return;
      event.preventDefault();
      if (form.dataset.saving) return;
      const fields = new URLSearchParams(new FormData(form));
      if (event.submitter?.name) fields.set(event.submitter.name, event.submitter.value);
      const status = q('.review-line-form-status', form);
      const say = (message, failed = false) => {
        if (!status) return;
        status.textContent = message;
        status.hidden = !message;
        status.classList.toggle('failed', failed);
      };
      if (!(fields.get('body') || '').trim() && !fields.get('state')) { say('Write a comment first.', true); q('textarea', form)?.focus(); return; }
      const container = containerFor(form);
      const entry = container && entries.get(container);
      const controls = qa('button,textarea', form);
      form.dataset.saving = 'true';
      form.setAttribute('aria-busy', 'true');
      controls.forEach(control => { control.disabled = true; });
      say('Saving…');
      let unlock = true;
      try {
        const response = await fetch(form.action, {method:'POST', body:fields, redirect:'error', credentials:'same-origin', headers:{Accept:'application/json', 'Content-Type':'application/x-www-form-urlencoded'}});
        if (!response.ok) {
          say((await response.text()).trim() || 'The comment was refused. Your draft is retained.', true);
          return;
        }
        const result = await response.json();
        if (result.saved !== true || !result.event_id) throw new Error('missing receipt');
        updateCounts(result.counts);
        if (!result.thread) { say(result.warning || 'Saved. Reload to see it.'); unlock = false; return; }
        const template = document.createElement('template');
        template.innerHTML = result.thread;
        const fresh = template.content.firstElementChild;
        const id = fresh.dataset.reviewLineThread;
        const previous = entry?.threads.get(id) || document.getElementById('line-thread-' + id);
        if (previous?.isConnected) previous.replaceWith(fresh);
        else form.replaceWith(fresh);
        if (form.matches('[data-review-line-composer-form]') && lastComposer === form) lastComposer = null;
        entry?.threads.set(id, fresh);
        unlock = false;
        fresh.focus({preventScroll:true});
      } catch (_) {
        say('The comment may have been saved, but its confirmation was lost. Reload before submitting again. Your draft is retained.', true);
      } finally {
        delete form.dataset.saving;
        form.removeAttribute('aria-busy');
        if (unlock) controls.forEach(control => { control.disabled = false; });
      }
    }, {signal});
    return {refresh};
  }
`

// reviewLineStyles place threads and composers under their lines, and the
// gutter button that opens a composer.
const reviewLineStyles = `
.review-lineno,.diff-row .line-no{position:relative}
.review-line-add{position:absolute;top:0;right:-9px;z-index:2;display:grid;place-items:center;width:18px;height:18px;padding:0;border:1px solid var(--accent);border-radius:4px;background:var(--accent);color:#fff;font:700 13px/1 var(--ui);cursor:pointer;opacity:0}
.review-line:hover .review-line-add,.diff-row:hover .review-line-add,.review-line-add:focus-visible{opacity:1}
.review-line-add:focus-visible{outline:2px solid var(--ink);outline-offset:1px}
@media(hover:none){.review-line-add{opacity:.55}}
.review-line-thread-row>td{padding:0 8px 0 3.5em;background:var(--bg)}
.diff-thread-row{grid-column:1/-1;padding:0 12px 0 104px;background:var(--bg);font:13px/1.5 var(--ui)}
.review-line-thread-row .review-thread,.diff-thread-row .review-thread,.review-line-threads .review-thread{font:13px/1.5 var(--ui);white-space:normal;max-width:760px}
.review-line-thread.outdated{border-color:#b45309}
.review-line-outdated{font-weight:700;color:#b45309}
.review-line-thread-state{text-transform:none}
.review-line-original table{border-collapse:collapse;font:12px/1.4 var(--mono);margin:4px 0 8px;background:var(--code-bg)}
.review-line-original .review-code code{white-space:pre}
.review-line-comment-form{margin:8px 0;max-width:760px;font:13px/1.5 var(--ui);white-space:normal}
.review-line-comment-form label>span{display:block;font-weight:600;margin-bottom:2px}
.review-line-comment-form textarea{width:100%;box-sizing:border-box;font:13px/1.45 var(--ui)}
.review-line-form-buttons{display:flex;gap:6px;margin-top:6px}
.review-line-form-status{margin:4px 0 0;font-size:12px;color:var(--muted)}.review-line-form-status.failed{color:#b45309;font-weight:600}
.review-line-threads{margin-top:12px}.review-line-threads-heading{margin:12px 0 4px;font-size:14px}
.review-line-count{display:flex;align-items:center;gap:6px;margin:8px 0;color:var(--muted);font-size:13px}.review-line-count[hidden],.review-line-count-badge[hidden]{display:none}
.review-line-count-badge{display:inline-flex;align-items:center;gap:1px;margin-left:2px;color:var(--accent);font:600 10px var(--mono)}.review-line-count-badge .i{width:11px;height:11px}
`
