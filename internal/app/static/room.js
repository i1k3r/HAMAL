/*
 * HAMAL Light Table: room pages (creator and participant).
 *
 * Runs only on pages whose <body> has the class "lt". site.js handles the theme, the home page and
 * the share page, and hands its translation table over through window.HAMAL_I18N.
 *
 * Uses only the existing API:
 *   GET  /api/v1/rooms/{token}            room status, presence, time left
 *   GET  /api/v1/rooms/{token}/files      ready files plus capacity
 *   GET  /api/v1/rooms/{token}/texts      notes (X-Client-Session-ID marks your own)
 *   POST /api/v1/rooms/{token}/files      upload (multipart, ?filename=)
 *   POST /api/v1/rooms/{token}/texts      send a note
 *   POST /api/v1/rooms/{token}/auth/pin   participant PIN
 *   POST /api/v1/rooms/{token}/close      creator closes the room
 *   POST /api/v1/rooms/{token}/files/{id}/share  creator share link (global sharing)
 */
(() => {
  const body = document.body;
  if (!body || !body.classList.contains('lt')) return;

  const page = body.dataset.page;
  const isCreator = page === 'creator';
  const token = body.dataset.token || '';
  const globalShareEnabled = body.dataset.globalShareEnabled === 'true';
  const maxTextSize = parseInt(body.dataset.maxTextSize || '', 10) || 65536;
  const API = `/api/v1/rooms/${encodeURIComponent(token)}`;
  const $ = (id) => document.getElementById(id);

  // ---------------------------------------------------------------------------------------------
  // Language: English with per-key fallback for the 20 locales site.js knows
  // ---------------------------------------------------------------------------------------------
  const I18N = window.HAMAL_I18N || { table: { en: {} }, locale: 'en' };
  const T = Object.assign({}, I18N.table.en || {}, I18N.table[I18N.locale] || {});
  const LOCALE = I18N.locale || 'en';
  document.documentElement.lang = LOCALE;
  const L = {
    copy: T.copyText || 'Copy',
    copied: T.copiedText || 'Copied!',
    exec: T.executableWarning || 'Potentially executable file. Only open or install files you trust.',
  };

  // ---------------------------------------------------------------------------------------------
  // Small helpers
  // ---------------------------------------------------------------------------------------------
  const store = {
    get(area, key) { try { return window[area].getItem(key); } catch (_) { return null; } },
    set(area, key, val) { try { window[area].setItem(key, val); } catch (_) {} },
  };

  function el(tag, attrs, children) {
    const n = document.createElement(tag);
    if (attrs) {
      for (const k of Object.keys(attrs)) {
        const v = attrs[k];
        if (v === undefined || v === null || v === false) continue;
        if (k === 'class') n.className = v;
        else if (k === 'text') n.textContent = v;
        else if (k === 'dataset') Object.assign(n.dataset, v);
        else n.setAttribute(k, v === true ? '' : String(v));
      }
    }
    if (children) for (const c of children) if (c) n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    return n;
  }

  function svg(markup) {
    const s = document.createElement('span');
    s.setAttribute('aria-hidden', 'true');
    s.style.display = 'inline-flex';
    s.innerHTML = markup; // static icon markup only, never user content
    return s;
  }
  const ICON_PLAY = '<svg width="40" height="40" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z"></path></svg>';
  const ICON_WARN = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>';
  const ICON_ALERT = '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="7" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>';

  function formatBytes(bytes) {
    if (!bytes) return '0 B';
    const k = 1024, sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(sizes.length - 1, Math.floor(Math.log(bytes) / Math.log(k)));
    const v = bytes / Math.pow(k, i);
    return (i === 0 ? String(bytes) : (v >= 100 ? v.toFixed(0) : parseFloat(v.toFixed(1)).toString())) + ' ' + sizes[i];
  }
  function clock(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d)) return '';
    const hm = d.toLocaleTimeString(LOCALE, { hour: '2-digit', minute: '2-digit' });
    const today = new Date();
    if (d.toDateString() === today.toDateString()) return hm;
    return d.toLocaleDateString(LOCALE, { month: 'short', day: 'numeric' }) + ' ' + hm;
  }
  function mmss(sec) {
    sec = Math.max(0, Math.floor(sec));
    const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
    const p = (n) => String(n).padStart(2, '0');
    return h > 0 ? `${p(h)}:${p(m)}:${p(s)}` : `${p(m)}:${p(s)}`;
  }
  function stamp() {
    const d = new Date();
    const p = (n) => String(n).padStart(2, '0');
    return `${p(d.getHours())}-${p(d.getMinutes())}`;
  }
  function announce(msg, assertive) {
    const r = $(assertive ? 'live-assertive' : 'live-polite');
    if (!r) return;
    r.textContent = '';
    setTimeout(() => { r.textContent = msg; }, 30);
  }
  const fileURL = (id) => `${API}/files/${encodeURIComponent(id)}`;

  function getClientSessionId() {
    const key = `hamal_client_session_${token}`;
    let sid = store.get('sessionStorage', key) || '';
    if (!sid) {
      if (window.crypto && typeof crypto.getRandomValues === 'function') {
        const arr = new Uint8Array(16);
        crypto.getRandomValues(arr);
        sid = 'cs_' + Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
      } else {
        sid = 'cs_' + Date.now().toString(36) + Math.random().toString(36).slice(2);
      }
      store.set('sessionStorage', key, sid);
    }
    return sid;
  }

  /** Copies text. Resolves true only when the browser confirmed the copy. */
  async function copyText(text) {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      try { await navigator.clipboard.writeText(text); return true; } catch (_) { /* fall through */ }
    }
    try {
      const ta = el('textarea', { 'aria-hidden': 'true', readonly: true });
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand && document.execCommand('copy') === true;
      ta.remove();
      return ok;
    } catch (_) {
      return false;
    }
  }

  // ---------------------------------------------------------------------------------------------
  // Kinds
  // ---------------------------------------------------------------------------------------------
  const EXT = {
    image: ['jpg', 'jpeg', 'png', 'webp', 'gif', 'bmp', 'tiff', 'tif', 'svg', 'ico', 'heic', 'avif'],
    video: ['mp4', 'mkv', 'avi', 'mov', 'webm', 'm4v', 'wmv'],
    audio: ['mp3', 'wav', 'flac', 'm4a', 'aac', 'ogg', 'wma', 'opus'],
    document: ['pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'odt', 'ods', 'odp', 'rtf', 'txt', 'md', 'csv', 'json', 'log', 'xml', 'yaml', 'yml', 'conf', 'ini', 'html', 'htm'],
    archive: ['zip', '7z', 'rar', 'tar', 'gz', 'tgz', 'bz2', 'xz', 'zst', 'iso', 'img', 'dmg'],
  };
  // Media the browser can preview from the download URL (the same lists as 6765e5e).
  const PREVIEW = {
    image: ['jpg', 'jpeg', 'png', 'webp', 'gif', 'bmp', 'tiff', 'tif', 'svg', 'ico'],
    video: ['mp4', 'mkv', 'avi', 'mov', 'webm', 'm4v', 'wmv'],
    audio: ['mp3', 'wav', 'flac', 'm4a', 'aac', 'ogg', 'wma'],
  };
  const EXEC = ['exe', 'msi', 'com', 'scr', 'bat', 'cmd', 'ps1', 'vbs', 'sh', 'jar', 'apk', 'aab', 'appimage', 'deb', 'rpm'];
  const KIND_LABEL = { image: 'Image', video: 'Video', audio: 'Audio', document: 'Document', archive: 'Archive', other: 'File', note: 'Note' };
  const BIG_IMAGE = 25 * 1024 * 1024;

  function extOf(name) {
    const lower = (name || '').toLowerCase();
    for (const multi of ['tar.gz', 'tar.bz2', 'tar.xz', 'tar.zst']) if (lower.endsWith('.' + multi)) return multi;
    const i = lower.lastIndexOf('.');
    return i > 0 && i < lower.length - 1 ? lower.slice(i + 1) : '';
  }
  function kindOf(name) {
    const ext = extOf(name);
    const last = ext.split('.').pop();
    for (const k of ['image', 'video', 'audio', 'document', 'archive']) if (EXT[k].includes(last)) return k;
    return 'other';
  }
  function previewKind(name) {
    const ext = extOf(name);
    for (const k of ['image', 'video', 'audio']) if (PREVIEW[k].includes(ext)) return k;
    return '';
  }
  const isExec = (name) => EXEC.includes(extOf(name));

  // ---------------------------------------------------------------------------------------------
  // State
  // ---------------------------------------------------------------------------------------------
  const state = {
    files: [],
    texts: [],
    room: null,
    capacity: null,
    ended: false,
    locked: page === 'participant' && body.dataset.pinRequired === 'true' && body.dataset.pinAuthenticated !== 'true',
    kind: 'all',
    query: '',
    sort: 'newest',
    view: 'table',
    selected: new Set(),
    lastBodies: { room: '', files: '', texts: '' },
    deadline: 0,
    ttl: 0,
    seenSeats: new Set(),
    seatsInitialised: false,
  };

  const downloadedKey = `hamal_downloaded_${token}`;
  let downloaded = new Set();
  try { downloaded = new Set(JSON.parse(store.get('sessionStorage', downloadedKey) || '[]')); } catch (_) {}
  function markDownloaded(id) {
    downloaded.add(id);
    store.set('sessionStorage', downloadedKey, JSON.stringify(Array.from(downloaded)));
  }

  // Time left: the server's remaining_seconds anchors a monotonic deadline, so clock skew between
  // devices never matters. The page's expires_at is the starting point before the first poll.
  (function initDeadline() {
    const exp = body.dataset.expires ? Date.parse(body.dataset.expires) : NaN;
    if (!isNaN(exp)) setRemaining((exp - Date.now()) / 1000);
  })();
  function setRemaining(sec) {
    state.deadline = performance.now() + Math.max(0, sec) * 1000;
    // The lamp's full length is the room's lifespan. The API doesn't return it, so it is the largest
    // remaining time seen, rounded up to the nearest lifespan the home page offers.
    const key = `hamal_ttl_${token}`;
    const known = parseInt(store.get('sessionStorage', key) || '0', 10) || 0;
    const options = [300, 900, 1800, 3600, 21600, 43200, 86400];
    const est = options.find((o) => o >= sec - 2) || Math.ceil(sec);
    const ttl = Math.max(known, est);
    if (ttl !== known) store.set('sessionStorage', key, String(ttl));
    state.ttl = ttl;
  }
  const remaining = () => Math.max(0, (state.deadline - performance.now()) / 1000);

  // ---------------------------------------------------------------------------------------------
  // Dialogs: one helper for QR, viewer, close confirm, share, shortcuts
  // ---------------------------------------------------------------------------------------------
  const modalStack = [];
  const frame = $('room-active-card');
  function setInert(on) {
    for (const n of [frame, $('upload-tray')]) {
      if (!n) continue;
      if ('inert' in HTMLElement.prototype) n.inert = on;
      else if (on) n.setAttribute('aria-hidden', 'true'); else n.removeAttribute('aria-hidden');
    }
  }
  function openModal(node, opener, focusEl) {
    if (!node) return;
    if (node.classList.contains('lt-viewer')) node.classList.remove('hidden');
    node.hidden = false;
    modalStack.push({ node, opener: opener || document.activeElement });
    setInert(true);
    document.body.style.overflow = 'hidden';
    setTimeout(() => { (focusEl || node.querySelector('button, [href], input, select, textarea'))?.focus(); }, 0);
  }
  function closeModal(node, restoreTo) {
    const idx = modalStack.findIndex((m) => m.node === node);
    if (idx === -1) return;
    const [entry] = modalStack.splice(idx, 1);
    if (node.classList.contains('lt-viewer')) node.classList.add('hidden');
    else node.hidden = true;
    if (!modalStack.length) {
      setInert(false);
      document.body.style.overflow = '';
    }
    const target = restoreTo || entry.opener;
    if (target && document.contains(target) && typeof target.focus === 'function') target.focus();
  }
  function closeAllModals() {
    while (modalStack.length) closeModal(modalStack[modalStack.length - 1].node, null);
  }
  document.addEventListener('keydown', (e) => {
    if (!modalStack.length) return;
    const top = modalStack[modalStack.length - 1].node;
    if (e.key === 'Escape') {
      e.preventDefault();
      if (top === viewer) closeViewer(); else closeModal(top);
      return;
    }
    if (e.key === 'Tab') {
      const f = Array.from(top.querySelectorAll('button:not([disabled]), [href], input:not([disabled]):not([type="hidden"]), select, textarea, video, audio, [tabindex]:not([tabindex="-1"])'))
        .filter((n) => n.offsetParent !== null || n === document.activeElement);
      if (!f.length) return;
      const first = f[0], last = f[f.length - 1];
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
      else if (!top.contains(document.activeElement)) { e.preventDefault(); first.focus(); }
    }
  });

  // ---------------------------------------------------------------------------------------------
  // Timeline: files and notes together, newest first
  // ---------------------------------------------------------------------------------------------
  function timeline() {
    const items = [];
    state.files.forEach((f, i) => {
      items.push({
        ord: i, key: 'f:' + f.file_id, type: 'file', id: f.file_id, name: f.filename,
        size: f.size_bytes || 0, at: f.created_at, kind: kindOf(f.filename), preview: previewKind(f.filename), raw: f,
      });
    });
    state.texts.forEach((t, i) => {
      if (t.sender_type === 'server') return; // system messages go to the room log
      items.push({
        ord: i, key: 't:' + t.id, type: 'note', id: t.id, name: (t.content || '').split('\n')[0].slice(0, 80),
        size: t.size_bytes || 0, at: t.created_at, kind: 'note', raw: t,
      });
    });
    return items;
  }
  function norm(s) {
    try { return (s || '').normalize('NFC').toLocaleLowerCase(LOCALE); } catch (_) { return (s || '').toLowerCase(); }
  }
  function visibleItems() {
    const q = norm(state.query.trim());
    let items = timeline().filter((it) => {
      if (state.kind !== 'all' && it.kind !== state.kind) return false;
      if (!q) return true;
      return norm(it.type === 'note' ? it.raw.content : it.name).includes(q);
    });
    // Timestamps have one-second resolution; within a second the server's own order (oldest first) decides.
    const byTime = (a, b) => (Date.parse(b.at) || 0) - (Date.parse(a.at) || 0) || (a.key[0] === b.key[0] ? b.ord - a.ord : (a.key < b.key ? 1 : -1));
    if (state.sort === 'name') items.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }) || byTime(a, b));
    else if (state.sort === 'size') items.sort((a, b) => b.size - a.size || byTime(a, b));
    else items.sort(byTime);
    return items;
  }

  // ---------------------------------------------------------------------------------------------
  // Item builders (Table view: prints, slips, index cards; List view: rows)
  // ---------------------------------------------------------------------------------------------
  function metaLine(it) {
    if (it.type === 'note') return '';
    const label = KIND_LABEL[it.kind] || 'File';
    const parts = [`${label}, ${formatBytes(it.size)}`];
    const c = clock(it.at);
    if (c) parts.push(c);
    return parts.join(' · ');
  }

  function fileActions(it, compact) {
    const wrap = el('div', { class: 'lt-actions' });
    if (isExec(it.name)) {
      const w = el('span', { class: 'lt-warn-mark badge-exec-warning', tabindex: '0', role: 'note', title: L.exec, 'aria-label': L.exec });
      w.appendChild(svg(ICON_WARN));
      w.appendChild(document.createTextNode(compact ? '' : 'Check'));
      wrap.appendChild(w);
    }
    if (it.preview) {
      wrap.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-soft', dataset: { action: 'view' }, 'aria-label': `View ${it.name}`, text: it.preview === 'image' ? 'View' : 'Play' }));
    }
    if (globalShareEnabled && isCreator) {
      wrap.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-line btn-share-link', dataset: { action: 'share', fileId: it.id, fileName: it.name }, 'aria-label': `Share link for ${it.name}`, text: 'Share Link' }));
    }
    const started = downloaded.has(it.id);
    wrap.appendChild(el('a', {
      class: 'lt-ink-btn btn-download' + (started ? ' is-done' : ''),
      href: fileURL(it.id), download: it.name, dataset: { action: 'save' },
      'aria-label': `Save ${it.name}`, title: started ? 'Download started earlier' : null,
      text: started ? 'Save again' : 'Save',
    }));
    return wrap;
  }

  function nameBlock(it) {
    return el('div', { class: 'lt-item-names' }, [
      el('h3', { class: 'lt-item-name', title: it.name, dir: 'auto', text: it.name }),
      el('span', { class: 'lt-item-meta', text: metaLine(it) }),
    ]);
  }

  function buildFileCard(it) {
    const li = el('li', { class: 'lt-item file-row', dataset: { key: it.key, fileId: it.id, kind: it.kind } });
    const big = it.kind === 'image' && it.size > BIG_IMAGE;
    if (it.kind === 'image' && it.preview === 'image' && !big) {
      const img = el('img', { src: fileURL(it.id), alt: it.name, loading: 'lazy', decoding: 'async' });
      const media = el('button', { type: 'button', class: 'lt-print-media', dataset: { action: 'view' }, 'aria-hidden': 'true', tabindex: '-1' }, [img]);
      // If the picture can't be loaded (removed meanwhile, unreadable format), show what it is
      // instead of the browser's broken-image icon.
      img.addEventListener('error', () => {
        media.classList.add('is-missing');
        media.replaceChildren(el('span', { class: 'lt-slip-ext', text: extOf(it.name).toUpperCase() || 'IMAGE' }), el('span', { class: 'lt-missing-note', text: 'Preview unavailable' }));
      }, { once: true });
      li.appendChild(el('article', { class: 'lt-print' }, [media, el('div', { class: 'lt-item-foot' }, [nameBlock(it), fileActions(it)])]));
    } else if (it.kind === 'video') {
      const tile = el('button', { type: 'button', class: 'lt-print-media is-video', dataset: { action: it.preview ? 'view' : 'save-direct' }, 'aria-hidden': 'true', tabindex: '-1' }, [svg(ICON_PLAY)]);
      li.appendChild(el('article', { class: 'lt-print' }, [tile, el('div', { class: 'lt-item-foot' }, [nameBlock(it), fileActions(it)])]));
    } else {
      const ext = extOf(it.name).toUpperCase() || KIND_LABEL[it.kind];
      li.appendChild(el('article', { class: 'lt-slip' + (it.kind === 'audio' ? ' is-audio' : '') }, [
        el('span', { class: 'lt-slip-hole', 'aria-hidden': 'true' }),
        el('span', { class: 'lt-slip-ext', 'aria-hidden': 'true', text: ext }),
        el('div', { class: 'lt-item-foot' }, [nameBlock(it), fileActions(it)]),
      ]));
    }
    return li;
  }

  function noteContentNode(content) {
    const trimmed = content.trim();
    if (/^https?:\/\/\S+$/i.test(trimmed) && !/\s/.test(trimmed)) {
      return el('a', { href: trimmed, target: '_blank', rel: 'noopener noreferrer', text: trimmed });
    }
    return document.createTextNode(content);
  }

  function noteWho(t) {
    if (t.sender_type === 'server') return 'HAMAL';
    return t.is_self ? 'You' : 'Note';
  }

  function buildNoteCard(it) {
    const t = it.raw;
    const isServer = t.sender_type === 'server';
    const content = t.content || '';
    const lines = content.split('\n').length;
    const strip = lines === 1 && content.length < 120;
    const li = el('li', {
      class: `lt-item text-item-card sender-${isServer ? 'server' : 'client'} ${isServer ? 'sender-server' : (t.is_self ? 'client-self' : 'client-peer')}`,
      dataset: { key: it.key, textId: t.id, kind: 'note' },
    });
    const who = el('span', { class: 'lt-card-who', title: formatBytes(it.size) }, [el('b', { text: noteWho(t) }), `, ${clock(t.created_at)}`]);
    const head = el('div', { class: 'lt-card-head' }, [who]);
    if (!isServer) {
      const copyBtn = el('button', { type: 'button', class: 'lt-ink-btn is-line btn-copy-text', dataset: { action: 'copy' }, 'aria-label': `Copy note from ${clock(t.created_at)}` }, [
        el('span', { class: 'copy-label', text: L.copy }),
      ]);
      head.appendChild(copyBtn);
      const more = el('div', { class: 'lt-more-menu' }, [
        el('button', { type: 'button', class: 'lt-ink-btn is-soft', dataset: { action: 'more' }, 'aria-label': `More for note from ${clock(t.created_at)}`, 'aria-expanded': 'false', text: '⋯' }),
      ]);
      head.appendChild(more);
    }
    const pre = el('pre', { class: 'lt-card-body text-item-content', dir: 'auto' }, [noteContentNode(content)]);
    const card = el('article', { class: 'lt-card' + (strip ? ' is-strip' : '') + (isServer ? ' is-system' : '') }, [head, pre]);
    if (lines > 8 || content.length > 900) {
      card.appendChild(el('button', { type: 'button', class: 'lt-card-more', dataset: { action: 'expand' }, text: `Show all ${lines.toLocaleString(LOCALE)} line${lines === 1 ? '' : 's'}` }));
    }
    li.appendChild(card);
    return li;
  }

  function buildRow(it) {
    const isNote = it.type === 'note';
    const li = el('li', {
      class: 'lt-item ' + (isNote ? `text-item-card ${it.raw.sender_type === 'server' ? 'sender-server' : (it.raw.is_self ? 'client-self' : 'client-peer')}` : 'file-row'),
      dataset: isNote ? { key: it.key, textId: it.id, kind: 'note' } : { key: it.key, fileId: it.id, kind: it.kind },
    });
    const row = el('div', { class: 'lt-row' });
    const sel = el('span', { class: 'lt-row-sel' });
    if (!isNote) {
      const cb = el('input', { type: 'checkbox', 'aria-label': `Select ${it.name}`, dataset: { action: 'select' } });
      cb.checked = state.selected.has(it.id);
      sel.appendChild(cb);
    }
    row.appendChild(sel);
    let thumb;
    if (it.kind === 'image' && it.preview === 'image' && it.size <= BIG_IMAGE) {
      thumb = el('img', { class: 'lt-row-thumb', src: fileURL(it.id), alt: '', loading: 'lazy', decoding: 'async' });
      const tag = el('span', { class: 'lt-row-tag', 'aria-hidden': 'true', text: extOf(it.name).toUpperCase().slice(0, 6) });
      thumb.addEventListener('error', () => thumb.replaceWith(tag), { once: true });
    }
    else if (!thumb) thumb = el('span', { class: 'lt-row-tag' + (isNote ? ' is-note' : '') + (it.kind === 'video' ? ' is-video' : ''), 'aria-hidden': 'true', text: isNote ? '' : (extOf(it.name).toUpperCase().slice(0, 6) || '—') });
    const nameCell = el('span', { class: 'lt-row-name' }, [thumb]);
    if (isNote) nameCell.appendChild(el('span', { class: 'lt-row-note text-item-content', dir: 'auto', title: it.raw.content, text: it.raw.content }));
    else nameCell.appendChild(el('h3', { class: 'lt-item-name', dir: 'auto', text: it.name }));
    row.appendChild(nameCell);
    const meta = el('span', { class: 'lt-row-meta' }, [
      el('span', { class: 'lt-row-cell', text: isNote ? noteWho(it.raw) : KIND_LABEL[it.kind] }),
      el('span', { class: 'lt-row-cell', text: formatBytes(it.size) }),
      el('span', { class: 'lt-row-cell', text: clock(it.at) }),
    ]);
    row.appendChild(meta);
    if (isNote) {
      const acts = el('div', { class: 'lt-actions' });
      if (it.raw.sender_type !== 'server') {
        acts.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-line btn-copy-text', dataset: { action: 'copy' }, 'aria-label': `Copy note from ${clock(it.at)}` }, [el('span', { class: 'copy-label', text: L.copy })]));
      }
      acts.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-soft', dataset: { action: 'expand' }, 'aria-label': `Read note from ${clock(it.at)}`, text: 'Read' }));
      row.appendChild(acts);
    } else {
      row.appendChild(fileActions(it, true));
    }
    li.appendChild(row);
    return li;
  }

  // ---------------------------------------------------------------------------------------------
  // Keyed rendering. A poll never rebuilds the list: unchanged items keep their DOM nodes (so focus,
  // images and scroll position survive), new items are inserted, gone items are removed.
  // ---------------------------------------------------------------------------------------------
  const list = $('file-list');
  const records = new Map(); // key -> { el, sig }
  let lastOrderSig = '';

  function signature(it) {
    const base = [state.view, it.type, it.name, it.size, it.at];
    if (it.type === 'file') base.push(downloaded.has(it.id) ? 1 : 0, state.view === 'list' ? (state.selected.has(it.id) ? 1 : 0) : 0);
    else base.push(it.raw.content, it.raw.is_self ? 1 : 0, it.raw.sender_type);
    return base.join('\u0001');
  }

  function cleanup(node) {
    node.querySelectorAll('video, audio').forEach((m) => { try { m.pause(); m.removeAttribute('src'); m.load(); } catch (_) {} });
  }

  function tableScroller() {
    const sc = $('table-scroll');
    if (sc && getComputedStyle(sc).overflowY !== 'visible' && sc.scrollHeight > sc.clientHeight) return sc;
    return window;
  }
  const scrollTopOf = (sc) => (sc === window ? window.scrollY : sc.scrollTop);

  function render(opts) {
    if (!list) return;
    const userReorder = opts && opts.reorder;
    const items = visibleItems();
    const wantKeys = items.map((i) => i.key);
    const orderSig = wantKeys.join('|');
    const active = document.activeElement;
    const activeItem = active && list.contains(active) ? active.closest('[data-key]') : null;
    const activeKey = activeItem ? activeItem.dataset.key : null;
    const activeAction = active && active.dataset ? active.dataset.action : null;

    // Scroll anchor: the first item whose top is in view, so new items landing above it don't move
    // what the reader is looking at. At the very top, new items simply appear (newest first).
    const scroller = tableScroller();
    const atTop = scrollTopOf(scroller) <= 2;
    let anchor = null;
    if (!atTop && !userReorder) {
      const viewTop = scroller === window ? 0 : scroller.getBoundingClientRect().top;
      for (const [key, rec] of records) {
        const r = rec.el.getBoundingClientRect();
        if (r.height > 0 && r.bottom > viewTop + 1) {
          if (!anchor || r.top < anchor.top) anchor = { key, top: r.top };
        }
      }
    }

    // Remove items that are gone (or filtered out).
    const want = new Set(wantKeys);
    for (const [key, rec] of records) {
      if (!want.has(key)) {
        if (activeKey === key) {
          const next = rec.el.nextElementSibling || rec.el.previousElementSibling;
          (next ? (next.querySelector('button, a') || next) : $('table-heading'))?.focus();
        }
        cleanup(rec.el);
        rec.el.remove();
        records.delete(key);
      }
    }

    const isFirst = !list.dataset.rendered;
    // Create or patch
    for (const it of items) {
      const sig = signature(it);
      const rec = records.get(it.key);
      if (rec && rec.sig === sig) continue;
      const node = state.view === 'list' ? buildRow(it) : (it.type === 'note' ? buildNoteCard(it) : buildFileCard(it));
      if (!isFirst && !rec && !userReorder) node.classList.add('is-new');
      if (rec) {
        const hadFocus = rec.el.contains(document.activeElement);
        cleanup(rec.el);
        rec.el.replaceWith(node);
        if (hadFocus) (node.querySelector(`[data-action="${activeAction}"]`) || node.querySelector('button, a'))?.focus();
      }
      records.set(it.key, { el: node, sig });
    }

    // Place nodes. On a poll only new nodes are inserted, in front of their next existing sibling;
    // existing nodes move only after a user action (sort, filter, view switch).
    if (userReorder || orderSig !== lastOrderSig) {
      let nextSibling = null;
      for (let i = items.length - 1; i >= 0; i--) {
        const node = records.get(items[i].key).el;
        const inPlace = node.parentNode === list && node.nextSibling === nextSibling;
        if (!inPlace && (userReorder || node.parentNode !== list)) list.insertBefore(node, nextSibling);
        nextSibling = node;
      }
      lastOrderSig = orderSig;
    }
    if (userReorder && activeKey && records.has(activeKey)) {
      const n = records.get(activeKey).el;
      (n.querySelector(`[data-action="${activeAction}"]`) || n.querySelector('button, a'))?.focus();
    }
    list.dataset.rendered = '1';
    if (anchor && records.has(anchor.key)) {
      const d = records.get(anchor.key).el.getBoundingClientRect().top - anchor.top;
      if (Math.abs(d) > 0.5) { if (scroller === window) window.scrollBy(0, d); else scroller.scrollTop += d; }
    }

    // Empty states, counts
    const all = timeline();
    const total = all.length;
    $('table-empty') && ($('table-empty').hidden = total !== 0);
    $('no-match') && ($('no-match').hidden = !(total > 0 && items.length === 0));
    list.hidden = total === 0;
    const counts = { all: total, image: 0, video: 0, audio: 0, document: 0, archive: 0, note: 0, other: 0 };
    for (const it of all) counts[it.kind] = (counts[it.kind] || 0) + 1;
    for (const k of Object.keys(counts)) {
      const c = k === 'note' ? $('text-count') : $('count-' + k);
      if (c) c.textContent = String(counts[k]);
    }
    const heading = $('table-heading');
    if (heading) heading.textContent = `Room open, ${state.files.length} file${state.files.length === 1 ? '' : 's'} and ${state.texts.length} note${state.texts.length === 1 ? '' : 's'} on the table`;
    updateListChrome();
    if (viewerKey) refreshViewerAfterPoll();
  }

  // ---------------------------------------------------------------------------------------------
  // Views: Table (default up to 24 items on wide screens) and List
  // ---------------------------------------------------------------------------------------------
  let autoSwitchAnnounced = false;
  function storedView() {
    const v = store.get('localStorage', 'hamal_view');
    return v === 'table' || v === 'list' ? v : null;
  }
  function autoView() {
    if (window.innerWidth < 640) return 'list';
    return timeline().length > 24 ? 'list' : 'table';
  }
  function applyView(view, why) {
    if (view === state.view && list && list.dataset.view === view) return;
    state.view = view;
    if (list) {
      list.dataset.view = view;
      list.classList.toggle('is-list', view === 'list');
      list.setAttribute('aria-label', view === 'list' ? 'Files and notes as a list' : 'Files and notes, newest first');
    }
    $('view-table')?.setAttribute('aria-pressed', String(view === 'table'));
    $('view-list')?.setAttribute('aria-pressed', String(view === 'list'));
    if (why === 'auto-count' && !autoSwitchAnnounced) {
      autoSwitchAnnounced = true;
      announce('Showing the list, because the room has more than 24 items.');
    }
    render({ reorder: true });
  }
  function evaluateView(reason) {
    const s = storedView();
    const v = s || autoView();
    if (v !== state.view) applyView(v, !s && reason === 'poll' ? 'auto-count' : reason);
  }
  $('view-table')?.addEventListener('click', () => { store.set('localStorage', 'hamal_view', 'table'); applyView('table', 'user'); });
  $('view-list')?.addEventListener('click', () => { store.set('localStorage', 'hamal_view', 'list'); applyView('list', 'user'); });
  let lastNarrow = window.innerWidth < 640;
  window.addEventListener('resize', () => {
    const narrow = window.innerWidth < 640;
    if (narrow !== lastNarrow) { lastNarrow = narrow; if (!storedView()) evaluateView('resize'); }
  });

  function updateListChrome() {
    const isList = state.view === 'list' && timeline().length > 0;
    $('list-bar') && ($('list-bar').hidden = !isList);
    $('list-head') && ($('list-head').hidden = !isList || window.innerWidth < 760);
    const btn = $('save-selected-btn');
    // Drop selections of files that are gone.
    const ids = new Set(state.files.map((f) => f.file_id));
    for (const id of Array.from(state.selected)) if (!ids.has(id)) state.selected.delete(id);
    if (btn) { btn.textContent = `Save selected (${state.selected.size})`; btn.disabled = state.selected.size === 0; }
    const all = $('select-all');
    if (all) {
      const vis = visibleItems().filter((i) => i.type === 'file');
      const n = vis.filter((i) => state.selected.has(i.id)).length;
      all.checked = vis.length > 0 && n === vis.length;
      all.indeterminate = n > 0 && n < vis.length;
    }
  }
  $('select-all')?.addEventListener('change', (e) => {
    const vis = visibleItems().filter((i) => i.type === 'file');
    if (e.target.checked) vis.forEach((i) => state.selected.add(i.id)); else vis.forEach((i) => state.selected.delete(i.id));
    render();
  });
  $('save-selected-btn')?.addEventListener('click', () => {
    const files = state.files.filter((f) => state.selected.has(f.file_id));
    files.forEach((f, i) => setTimeout(() => {
      const a = el('a', { href: fileURL(f.file_id), download: f.filename });
      document.body.appendChild(a); a.click(); a.remove();
      markDownloaded(f.file_id);
      logActivity(`Download started: ${f.filename}`);
      if (i === files.length - 1) { $('bulk-note').hidden = false; render(); }
    }, i * 400));
  });

  // Toolbar: search, kind chips (and their select on very narrow screens), sort
  let searchTimer = null;
  $('search-input')?.addEventListener('input', (e) => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => { state.query = e.target.value; render({ reorder: true }); }, 120);
  });
  function setKind(kind) {
    state.kind = kind;
    document.querySelectorAll('#kind-chips .lt-chip').forEach((c) => c.setAttribute('aria-pressed', String(c.dataset.kind === kind)));
    if ($('kind-select')) $('kind-select').value = kind;
    render({ reorder: true });
  }
  $('kind-chips')?.addEventListener('click', (e) => { const c = e.target.closest('.lt-chip'); if (c) setKind(c.dataset.kind); });
  $('kind-select')?.addEventListener('change', (e) => setKind(e.target.value));
  $('sort-select')?.addEventListener('change', (e) => { state.sort = e.target.value; render({ reorder: true }); });

  // ---------------------------------------------------------------------------------------------
  // Item actions (one delegated listener for the whole list)
  // ---------------------------------------------------------------------------------------------
  function itemByKey(key) { return timeline().find((i) => i.key === key); }
  let openMore = null;
  function closeMore() { if (openMore) { openMore.pop.remove(); openMore.btn.setAttribute('aria-expanded', 'false'); openMore = null; } }
  document.addEventListener('click', (e) => { if (openMore && !openMore.pop.contains(e.target) && e.target !== openMore.btn) closeMore(); });

  if (list && !list.dataset.bound) {
    list.dataset.bound = '1';
    list.addEventListener('click', async (e) => {
      const actionEl = e.target.closest('[data-action]');
      if (!actionEl || !list.contains(actionEl)) return;
      const host = actionEl.closest('[data-key]');
      const it = host ? itemByKey(host.dataset.key) : null;
      if (!it) return;
      const action = actionEl.dataset.action;
      if (action === 'view' || action === 'expand') {
        openViewer(it.key, actionEl);
      } else if (action === 'save') {
        markDownloaded(it.id);
        logActivity(`Download started: ${it.name}`);
        setTimeout(() => render(), 0);
      } else if (action === 'save-direct') {
        actionEl.closest('.lt-item').querySelector('a[download]')?.click();
      } else if (action === 'share') {
        openShare(it.id, it.name, actionEl);
      } else if (action === 'copy') {
        const ok = await copyText(it.raw.content || '');
        const label = actionEl.querySelector('.copy-label');
        if (label) label.textContent = ok ? L.copied : "Couldn't copy";
        actionEl.classList.toggle('is-done', ok);
        if (!ok) {
          const pre = host.querySelector('.text-item-content');
          if (pre) { const r = document.createRange(); r.selectNodeContents(pre); const s = getSelection(); s.removeAllRanges(); s.addRange(r); }
          announce("Couldn't copy. Select the text and copy it manually.", true);
        }
        setTimeout(() => { if (label) label.textContent = L.copy; actionEl.classList.remove('is-done'); }, 2000);
      } else if (action === 'more') {
        if (openMore && openMore.btn === actionEl) { closeMore(); return; }
        closeMore();
        const content = it.raw.content || '';
        const pop = el('div', { class: 'lt-more-pop', role: 'menu' }, [
          el('button', { type: 'button', role: 'menuitem', dataset: { action: 'expand' }, text: 'Read in full' }),
          el('a', { role: 'menuitem', href: 'data:text/plain;charset=utf-8,' + encodeURIComponent(content), download: `note-${stamp()}.txt`, text: 'Download .txt' }),
        ]);
        actionEl.parentNode.appendChild(pop);
        actionEl.setAttribute('aria-expanded', 'true');
        openMore = { pop, btn: actionEl };
        pop.querySelector('button').focus();
      } else if (action === 'select') {
        if (actionEl.checked) state.selected.add(it.id); else state.selected.delete(it.id);
        updateListChrome();
      }
    });
  }

  // ---------------------------------------------------------------------------------------------
  // Viewer (replaces the 6765e5e preview overlay, keeps its ids and its Escape / close button /
  // backdrop dismissal). Steps through the viewable items of the current filter and sort.
  // ---------------------------------------------------------------------------------------------
  const viewer = $('file-preview-overlay');
  const vBody = $('preview-body');
  const vTitle = $('preview-filename');
  let viewerKey = null;
  let viewerOpener = null;

  function viewable() { return visibleItems().filter((i) => i.type === 'note' || i.preview); }
  function stopMedia() {
    if (!vBody) return;
    vBody.querySelectorAll('video, audio').forEach((m) => { try { m.pause(); m.removeAttribute('src'); m.load(); } catch (_) {} });
  }
  function showInViewer(key) {
    const seq = viewable();
    const idx = seq.findIndex((i) => i.key === key);
    const it = idx >= 0 ? seq[idx] : null;
    stopMedia();
    vBody.replaceChildren();
    $('viewer-note').textContent = '';
    viewerKey = key;
    if (!it) { showGone(); return; }
    vTitle.textContent = it.type === 'note' ? `Note, ${clock(it.at)}` : it.name;
    $('viewer-count').textContent = `${idx + 1} of ${seq.length}`;
    if (it.type === 'note') {
      vBody.appendChild(el('pre', { dir: 'auto', tabindex: '0' }, [noteContentNode(it.raw.content || '')]));
    } else if (it.preview === 'image') {
      vBody.appendChild(el('img', { src: fileURL(it.id), alt: it.name }));
      const next = seq[idx + 1];
      if (next && next.preview === 'image') { const pre = new Image(); pre.src = fileURL(next.id); }
    } else if (it.preview === 'video') {
      const v = el('video', { src: fileURL(it.id), controls: true, preload: 'metadata', playsinline: true });
      v.addEventListener('error', () => { if (viewerKey === key && !v.currentSrc) return; });
      vBody.appendChild(v);
    } else if (it.preview === 'audio') {
      const ext = extOf(it.name).toUpperCase();
      vBody.appendChild(el('div', { class: 'lt-viewer-audio' }, [
        el('div', { class: 'lt-slip is-audio' }, [el('span', { class: 'lt-slip-hole', 'aria-hidden': 'true' }), el('span', { class: 'lt-slip-ext', text: ext }), el('span', { class: 'lt-item-name', text: it.name })]),
        el('audio', { src: fileURL(it.id), controls: true, preload: 'metadata' }),
      ]));
    }
    $('viewer-prev').disabled = idx <= 0;
    $('viewer-next').disabled = idx >= seq.length - 1;
    $('viewer-save').disabled = false;
    $('viewer-save').textContent = it.type === 'note' ? L.copy : 'Save';
  }
  function showGone() {
    stopMedia();
    vBody.replaceChildren(el('p', { class: 'lt-viewer-gone', text: 'This item is no longer in the room.' }));
    $('viewer-save').disabled = true;
    const seq = viewable();
    $('viewer-count').textContent = `${seq.length} on the table`;
  }
  function openViewer(key, opener) {
    viewerOpener = opener || document.activeElement;
    showInViewer(key);
    openModal(viewer, viewerOpener, $('preview-close-btn'));
  }
  function closeViewer() {
    stopMedia();
    vBody.replaceChildren();
    const key = viewerKey;
    viewerKey = null;
    const rec = key && records.get(key);
    const back = rec ? (rec.el.querySelector('[data-action="view"]:not([tabindex="-1"])') || rec.el.querySelector('[data-action="expand"]') || rec.el.querySelector('button:not([tabindex="-1"]), a')) : null;
    closeModal(viewer, back || viewerOpener);
    if (back) back.scrollIntoView({ block: 'nearest' });
  }
  function step(d) {
    const seq = viewable();
    const idx = seq.findIndex((i) => i.key === viewerKey);
    const n = seq[idx + d];
    if (n) showInViewer(n.key);
  }
  function refreshViewerAfterPoll() {
    if (!viewerKey) return;
    const seq = viewable();
    const idx = seq.findIndex((i) => i.key === viewerKey);
    if (idx === -1) { if (!vBody.querySelector('.lt-viewer-gone')) showGone(); return; }
    // Keep the media element (and playback); only the position and buttons update.
    $('viewer-count').textContent = `${idx + 1} of ${seq.length}`;
    $('viewer-prev').disabled = idx <= 0;
    $('viewer-next').disabled = idx >= seq.length - 1;
  }
  if (viewer) {
    $('preview-close-btn')?.addEventListener('click', closeViewer);
    $('viewer-prev')?.addEventListener('click', () => step(-1));
    $('viewer-next')?.addEventListener('click', () => step(1));
    $('viewer-save')?.addEventListener('click', () => viewerSave());
    // Backdrop: a click on the dark surround (not on the media, title or buttons) closes.
    viewer.addEventListener('click', (e) => {
      const t = e.target;
      if (t === viewer || t === vBody || t.classList.contains('lt-viewer-head') || t.classList.contains('lt-viewer-foot') || t.classList.contains('lt-viewer-audio')) closeViewer();
    });
    viewer.addEventListener('keydown', (e) => {
      if (e.target.closest && e.target.closest('video, audio')) return;
      if (e.key === 'ArrowLeft') { e.preventDefault(); step(-1); }
      else if (e.key === 'ArrowRight') { e.preventDefault(); step(1); }
      else if ((e.key === 's' || e.key === 'S') && !e.ctrlKey && !e.metaKey && !e.altKey) { e.preventDefault(); viewerSave(); }
    });
    let sx = null, sy = null;
    vBody.addEventListener('pointerdown', (e) => { if (e.target.closest('video, audio')) { sx = null; return; } sx = e.clientX; sy = e.clientY; });
    vBody.addEventListener('pointerup', (e) => {
      if (sx === null) return;
      const dx = e.clientX - sx, dy = e.clientY - sy;
      sx = null;
      if (Math.abs(dx) >= 50 && Math.abs(dx) > Math.abs(dy)) step(dx < 0 ? 1 : -1);
    });
  }
  async function viewerSave() {
    const it = viewerKey && itemByKey(viewerKey);
    if (!it) return;
    const note = $('viewer-note');
    if (it.type === 'note') {
      const ok = await copyText(it.raw.content || '');
      note.textContent = ok ? L.copied : "Couldn't copy. Select the text and copy it manually.";
      if (!ok) { const pre = vBody.querySelector('pre'); if (pre) { const r = document.createRange(); r.selectNodeContents(pre); const s = getSelection(); s.removeAllRanges(); s.addRange(r); } }
      return;
    }
    try {
      const res = await fetch(fileURL(it.id), { method: 'HEAD', cache: 'no-store' });
      if (res.status === 410) { endRoom('gone'); return; }
      if (!res.ok) { note.textContent = 'This file is no longer in the room.'; return; }
    } catch (_) {
      note.textContent = "Couldn't reach HAMAL. Check your connection and try again.";
      return;
    }
    const a = el('a', { href: fileURL(it.id), download: it.name });
    document.body.appendChild(a); a.click(); a.remove();
    markDownloaded(it.id);
    logActivity(`Download started: ${it.name}`);
    note.textContent = "Download started. Check your browser's downloads.";
    setTimeout(() => { if (note.textContent.startsWith('Download started')) note.textContent = ''; }, 4000);
    render();
  }

  // ---------------------------------------------------------------------------------------------
  // Upload queue: one file at a time, every state explicit, "Not sent" separate from "Room ended"
  // ---------------------------------------------------------------------------------------------
  const MAX_UPLOAD_RETRIES = 5;
  const uploads = new Map();
  const order = [];
  let seq = 0;
  let currentId = null;
  let trayCollapseTimer = null;
  const LEGAL = {
    queued: ['sending', 'removed', 'ended'],
    sending: ['sent', 'waiting', 'failed', 'ended', 'removed'],
    waiting: ['sending', 'removed', 'ended'],
    failed: ['queued', 'removed', 'ended'],
    sent: [],
    ended: ['removed'],
  };
  function transition(item, to, cause) {
    if (!LEGAL[item.state] || !LEGAL[item.state].includes(to)) { console.debug('upload: ignored', item.state, '->', to, cause); return false; }
    if (item.retryTimer) { clearTimeout(item.retryTimer); item.retryTimer = null; }
    const from = item.state;
    item.state = to;
    if (from === 'sending' && currentId === item.localId) { currentId = null; setTimeout(pump, 0); }
    if (to === 'removed') {
      uploads.delete(item.localId);
      const i = order.indexOf(item.localId);
      if (i >= 0) order.splice(i, 1);
    }
    if (to === 'sent') announce(`${item.name} sent`);
    if (to === 'failed') announce(`${item.name} not sent. ${item.error || ''}`, true);
    renderTray();
    return true;
  }
  function enqueue(files) {
    if (state.ended || !files || !files.length) return;
    for (const f of Array.from(files)) {
      const localId = 'u' + (++seq);
      uploads.set(localId, { localId, file: f, name: f.name, size: f.size, state: 'queued', loaded: 0, startedAt: 0, bytesPerSec: 0, attempts: 0, attempt: 0, error: '', retryTimer: null, xhr: null, wait: 0 });
      order.push(localId);
    }
    expandTray();
    renderTray();
    pump();
  }
  function pump() {
    if (state.ended || currentId) return;
    const id = order.find((x) => uploads.get(x) && uploads.get(x).state === 'queued');
    if (!id) return;
    const item = uploads.get(id);
    currentId = id;
    transition(item, 'sending', 'pump');
    send(item);
  }
  function parseErr(xhr) {
    try { const d = JSON.parse(xhr.responseText); if (d && d.error) return d.error; } catch (_) {}
    return `HTTP ${xhr.status}`;
  }
  function send(item) {
    item.attempt++;
    const attempt = item.attempt;
    item.loaded = 0;
    item.startedAt = performance.now();
    const xhr = new XMLHttpRequest();
    item.xhr = xhr;
    const live = () => item.attempt === attempt && item.state === 'sending' && uploads.has(item.localId);
    xhr.open('POST', `${API}/files?filename=${encodeURIComponent(item.name)}`);
    xhr.setRequestHeader('X-Client-Session-ID', getClientSessionId());
    xhr.upload.addEventListener('progress', (e) => {
      if (!live() || !e.lengthComputable) return;
      item.loaded = e.loaded;
      const secs = (performance.now() - item.startedAt) / 1000;
      if (secs > 0.3) item.bytesPerSec = e.loaded / secs;
      renderTray();
    });
    xhr.addEventListener('load', () => {
      if (!live()) return;
      const st = xhr.status;
      if (st === 200 || st === 201) {
        let data = null;
        try { data = JSON.parse(xhr.responseText); } catch (_) {}
        transition(item, 'sent', 'load');
        logActivity(`You laid down ${item.name}`);
        if (data && data.file_id && !state.files.some((f) => f.file_id === data.file_id)) {
          state.files.push(data);
          evaluateView('poll');
          render();
        }
        fetchFiles();
        return;
      }
      if (st === 429 && item.attempts < MAX_UPLOAD_RETRIES && !state.ended) {
        item.attempts++;
        let wait = parseInt(xhr.getResponseHeader('Retry-After') || '', 10);
        if (!Number.isFinite(wait) || wait <= 0) wait = 2;
        wait = Math.min(wait, 60);
        item.wait = wait;
        transition(item, 'waiting', '429');
        const captured = item.attempt;
        item.retryTimer = setTimeout(() => {
          if (state.ended || item.state !== 'waiting' || item.attempt !== captured) return;
          item.retryTimer = null;
          // Resume this file before anything else in the queue.
          if (currentId) { item.state = 'queued'; renderTray(); return; }
          currentId = item.localId;
          transition(item, 'sending', 'retry-after');
          send(item);
        }, wait * 1000);
        return;
      }
      const msg = parseErr(xhr);
      if (st === 401) { item.error = 'this device is no longer signed in to the room. Reload to enter the PIN again.'; transition(item, 'ended', '401'); endQueue(item.error); return; }
      if (st === 404 || st === 409 || st === 410) { item.error = msg; transition(item, 'ended', String(st)); endQueue(msg); if (st === 410 || st === 404) endRoom('gone'); return; }
      item.error = st === 429 ? 'the server is busy (' + msg + ')' : msg;
      transition(item, 'failed', String(st));
    });
    const transport = () => {
      if (!live()) return;
      item.error = 'connection lost';
      transition(item, 'failed', 'transport');
    };
    xhr.addEventListener('error', transport);
    xhr.addEventListener('timeout', transport);
    const fd = new FormData();
    fd.append('file', item.file, item.name);
    xhr.send(fd);
  }
  // A refusal that means no further upload can succeed now: the rest of the queue stops too.
  function endQueue(reason) {
    for (const id of order) {
      const it = uploads.get(id);
      if (it && it.state === 'queued') { it.error = reason; transition(it, 'ended', 'queue-stopped'); }
    }
  }
  function cancelUpload(item) {
    const xhr = item.xhr;
    item.attempt++;
    transition(item, 'removed', 'cancel');
    if (xhr && xhr.readyState !== 4) { try { xhr.abort(); } catch (_) {} }
  }
  function retryUpload(item, btn) {
    if (btn) btn.disabled = true;
    const startedAt = item.failedAt || 0;
    const dup = state.files.find((f) => f.filename === item.name && f.size_bytes === item.size && Date.parse(f.created_at) >= startedAt - 5000);
    if (dup && !window.confirm('A file with this name and size arrived in the room. Send it again?')) { if (btn) btn.disabled = false; return; }
    item.error = '';
    if (transition(item, 'queued', 'retry')) pump();
  }

  function statusText(item) {
    switch (item.state) {
      case 'queued': return item.attempts ? 'Waiting to retry' : 'Queued';
      case 'sending': {
        const pct = item.size ? Math.round((item.loaded / item.size) * 100) : 0;
        let s = `Sending ${Math.min(100, pct)}%`;
        if (item.bytesPerSec > 0 && item.size) {
          const left = (item.size - item.loaded) / item.bytesPerSec;
          s += ` · ${formatBytes(item.bytesPerSec)}/s · ${Math.max(0, Math.ceil(left))} s left`;
        }
        return s;
      }
      case 'waiting': return `Waiting: server busy, retrying in ${item.wait}s`;
      case 'failed': return `Not sent: ${item.error}`;
      case 'sent': return 'Sent';
      case 'ended': return item.error && item.error.startsWith('this device') ? `Not sent: ${item.error}` : `The room ended before this finished${item.error ? ` (${item.error})` : ''}`;
      default: return '';
    }
  }

  const tray = $('upload-tray');
  const trayList = $('tray-list');
  const trayRecords = new Map();
  function expandTray() {
    if (!tray) return;
    tray.classList.remove('is-collapsed');
    $('tray-toggle').textContent = 'Hide';
    $('tray-toggle').setAttribute('aria-expanded', 'true');
  }
  $('tray-toggle')?.addEventListener('click', () => {
    const collapsed = tray.classList.toggle('is-collapsed');
    $('tray-toggle').textContent = collapsed ? 'Show' : 'Hide';
    $('tray-toggle').setAttribute('aria-expanded', String(!collapsed));
  });
  function renderTray() {
    if (!tray) return;
    const items = order.map((id) => uploads.get(id)).filter(Boolean);
    tray.hidden = items.length === 0;
    const n = items.length;
    const sent = items.filter((i) => i.state === 'sent').length;
    const failed = items.filter((i) => i.state === 'failed').length;
    const ended = items.filter((i) => i.state === 'ended').length;
    const open = items.filter((i) => ['queued', 'sending', 'waiting'].includes(i.state)).length;
    let summary;
    if (open) summary = `Sending ${Math.min(n, sent + 1)} of ${n}`;
    else if (failed || ended) summary = `${sent} of ${n} sent`;
    else summary = `All ${n} sent`;
    if (failed) summary += ` · ${failed} not sent`;
    if (ended) summary += ` · ${ended} stopped`;
    $('tray-summary').textContent = summary;

    // Keyed rows
    const keep = new Set(items.map((i) => i.localId));
    for (const [id, rec] of trayRecords) if (!keep.has(id)) { rec.el.remove(); trayRecords.delete(id); }
    let prev = null;
    for (const item of items) {
      let rec = trayRecords.get(item.localId);
      if (!rec) {
        const li = el('li', { class: 'lt-up', dataset: { id: item.localId } }, [
          el('span', { class: 'lt-up-name', text: item.name }),
          el('span', { class: 'lt-up-status' }),
          el('div', { class: 'lt-actions' }),
          el('span', { class: 'lt-up-bar', 'aria-hidden': 'true' }),
        ]);
        rec = { el: li, state: '' };
        trayRecords.set(item.localId, rec);
      }
      const li = rec.el;
      li.className = 'lt-up is-' + item.state;
      li.querySelector('.lt-up-status').textContent = statusText(item);
      li.querySelector('.lt-up-bar').style.width = item.state === 'sending' && item.size ? `${Math.round((item.loaded / item.size) * 100)}%` : (item.state === 'sent' ? '100%' : '0');
      if (rec.state !== item.state) {
        rec.state = item.state;
        const acts = li.querySelector('.lt-actions');
        acts.replaceChildren();
        if (item.state === 'failed') {
          acts.appendChild(el('button', { type: 'button', class: 'lt-ink-btn', dataset: { up: 'retry' }, 'aria-label': `Retry ${item.name}`, text: 'Retry' }));
          acts.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-soft', dataset: { up: 'remove' }, 'aria-label': `Remove ${item.name}`, text: 'Remove' }));
          item.failedAt = item.failedAt || Date.now();
        } else if (item.state === 'ended') {
          acts.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-soft', dataset: { up: 'remove' }, 'aria-label': `Remove ${item.name}`, text: 'Remove' }));
        } else if (['queued', 'sending', 'waiting'].includes(item.state)) {
          acts.appendChild(el('button', { type: 'button', class: 'lt-ink-btn is-line', dataset: { up: 'cancel' }, 'aria-label': `Cancel ${item.name}`, text: 'Cancel' }));
        }
      }
      if (li.previousElementSibling !== prev || li.parentNode !== trayList) {
        trayList.insertBefore(li, prev ? prev.nextSibling : trayList.firstChild);
      }
      prev = li;
    }
    // Collapse to the summary 5 s after everything was sent.
    clearTimeout(trayCollapseTimer);
    if (n && !open && !failed && !ended) {
      trayCollapseTimer = setTimeout(() => {
        tray.classList.add('is-collapsed');
        $('tray-toggle').textContent = 'Show';
        $('tray-toggle').setAttribute('aria-expanded', 'false');
      }, 5000);
    }
  }
  trayList?.addEventListener('click', (e) => {
    const b = e.target.closest('[data-up]');
    if (!b) return;
    const item = uploads.get(b.closest('[data-id]').dataset.id);
    if (!item) return;
    if (b.dataset.up === 'cancel') cancelUpload(item);
    else if (b.dataset.up === 'remove') transition(item, 'removed', 'remove');
    else if (b.dataset.up === 'retry') retryUpload(item, b);
  });

  // Ways in: Add files, drop anywhere, paste an image (6765e5e behaviour)
  const fileInput = $('file-input');
  const pick = () => { if (!state.ended && fileInput) fileInput.click(); };
  $('add-files-btn')?.addEventListener('click', pick);
  $('empty-add-btn')?.addEventListener('click', pick);
  fileInput?.addEventListener('change', () => { if (fileInput.files && fileInput.files.length) enqueue(fileInput.files); fileInput.value = ''; });

  const table = $('dropzone');
  let dragDepth = 0;
  const hasFiles = (e) => e.dataTransfer && (Array.from(e.dataTransfer.types || []).includes('Files') || (e.dataTransfer.files && e.dataTransfer.files.length));
  document.addEventListener('dragenter', (e) => { if (!hasFiles(e) || state.ended || state.locked) return; dragDepth++; table?.classList.add('is-dragover'); });
  document.addEventListener('dragleave', () => { dragDepth = Math.max(0, dragDepth - 1); if (!dragDepth) table?.classList.remove('is-dragover'); });
  document.addEventListener('dragover', (e) => { if (!hasFiles(e) || state.ended || state.locked) return; e.preventDefault(); table?.classList.add('is-dragover'); });
  document.addEventListener('drop', (e) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dragDepth = 0;
    table?.classList.remove('is-dragover');
    if (state.ended || state.locked) return;
    if (e.dataTransfer.files && e.dataTransfer.files.length) enqueue(e.dataTransfer.files);
  });

  // Clipboard image paste (from 6765e5e, unchanged): Ctrl+V anywhere on the page turns a copied
  // image into an upload. Pastes into text fields are left alone, so text pastes normally.
  document.addEventListener('paste', (e) => {
    if (state.ended || state.locked) return;
    const tag = e.target && e.target.tagName ? e.target.tagName.toUpperCase() : '';
    if (tag === 'INPUT' || tag === 'TEXTAREA') return;
    const items = e.clipboardData && e.clipboardData.items;
    if (!items) return;
    const images = [];
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const ext = item.type.split('/')[1] || 'png';
        const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const blob = item.getAsFile();
        if (blob) images.push(new File([blob], `pasted-image-${ts}.${ext}`, { type: item.type }));
      }
    }
    if (images.length) { e.preventDefault(); enqueue(images); }
  });

  // ---------------------------------------------------------------------------------------------
  // Composer
  // ---------------------------------------------------------------------------------------------
  const composer = $('composer');
  const textInput = $('text-input');
  const sendBtn = $('send-text-btn');
  const counter = $('text-size-counter');
  const composeErr = $('text-compose-error');
  const overlimit = $('text-overlimit');
  const isNarrow = () => window.innerWidth < 760;
  function openComposer() {
    if (!composer || state.ended) return;
    composer.hidden = false;
    $('write-note-btn')?.setAttribute('aria-expanded', 'true');
    if (isNarrow()) $('composer-backdrop').hidden = false;
    textInput?.focus();
  }
  function closeComposer() {
    if (!composer) return;
    composer.hidden = true;
    $('composer-backdrop').hidden = true;
    $('write-note-btn')?.setAttribute('aria-expanded', 'false');
    $('write-note-btn')?.focus();
  }
  $('write-note-btn')?.addEventListener('click', () => (composer.hidden ? openComposer() : closeComposer()));
  $('composer-cancel')?.addEventListener('click', closeComposer);
  $('composer-backdrop')?.addEventListener('click', closeComposer);
  composer?.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !modalStack.length) { e.preventDefault(); closeComposer(); } });

  const bytesOf = (s) => new TextEncoder().encode(s).length;
  function updateCounter() {
    if (!textInput || !counter) return;
    const b = bytesOf(textInput.value);
    counter.textContent = `${formatBytes(b)} / ${formatBytes(maxTextSize)}`;
    const over = b > maxTextSize;
    counter.classList.toggle('is-over', over);
    if (overlimit) {
      overlimit.hidden = !over;
      if (over) $('text-overlimit-msg').textContent = `This text is ${formatBytes(b)}. Notes can be up to ${formatBytes(maxTextSize)}.`;
    }
    if (sendBtn && !sendBtn.dataset.sending) sendBtn.disabled = over || state.ended;
  }
  if (textInput) {
    updateCounter();
    textInput.addEventListener('input', () => { updateCounter(); composeErr.hidden = true; });
    const touch = ('ontouchstart' in window) || navigator.maxTouchPoints > 0;
    textInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey || (!e.shiftKey && !touch && !isNarrow()))) { e.preventDefault(); submitNote(); }
    });
    $('composer-hint').textContent = touch ? 'Long-press to paste.' : 'Enter sends. Shift+Enter adds a line.';
  }
  function composeError(msg) { composeErr.textContent = msg; composeErr.hidden = false; }
  async function submitNote() {
    if (!textInput || state.ended) return;
    const content = textInput.value; // sent exactly as typed: indentation and newlines are content
    if (!content.trim()) return;
    if (bytesOf(content) > maxTextSize) { composeError(T.textTooLarge || 'Text exceeds maximum allowed size'); return; }
    sendBtn.disabled = true;
    sendBtn.dataset.sending = '1';
    $('send-text-label').textContent = T.sendingText || 'Sending…';
    composeErr.hidden = true;
    try {
      const sid = getClientSessionId();
      const res = await fetch(`${API}/texts`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-Client-Session-ID': sid },
        body: JSON.stringify({ content, client_session_id: sid }),
      });
      if (res.status === 201) {
        textInput.value = '';
        updateCounter();
        logActivity('You wrote a note');
        await fetchTexts();
        const mine = state.texts.filter((t) => t.is_self).pop();
        const rec = mine && records.get('t:' + mine.id);
        if (rec) rec.el.scrollIntoView({ block: 'nearest' });
      } else if (res.status === 410) {
        endRoom('gone');
      } else {
        const d = await res.json().catch(() => ({}));
        composeError(d.error || 'Failed to send text');
      }
    } catch (_) {
      composeError('Network error while sending text');
    } finally {
      delete sendBtn.dataset.sending;
      $('send-text-label').textContent = 'Send note';
      updateCounter();
    }
  }
  sendBtn?.addEventListener('click', submitNote);
  $('send-as-file-btn')?.addEventListener('click', () => {
    const text = textInput.value;
    enqueue([new File([text], `note-${stamp()}.txt`, { type: 'text/plain' })]);
    textInput.value = '';
    updateCounter();
  });

  // ---------------------------------------------------------------------------------------------
  // Lamp, expiry banner, end state
  // ---------------------------------------------------------------------------------------------
  let bannerShown = false;
  let tickTimer = null;
  function lampWords(sec) {
    if (sec >= 3600) {
      const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60);
      return `${h} h ${m} min`;
    }
    if (sec >= 300) return `${Math.floor(sec / 60)} minutes`;
    return mmss(sec);
  }
  function tick() {
    if (state.ended) return;
    const sec = Math.floor(remaining());
    const cd = $('countdown');
    if (cd) cd.textContent = lampWords(sec);
    const meter = $('lamp-meter');
    if (meter) {
      meter.setAttribute('aria-valuemax', String(state.ttl || sec));
      meter.setAttribute('aria-valuenow', String(sec));
      meter.setAttribute('aria-valuetext', `${lampWords(sec)} left`);
    }
    const fill = $('lamp-fill');
    if (fill) fill.style.width = `${state.ttl ? Math.max(0, Math.min(100, (sec / state.ttl) * 100)) : 100}%`;
    $('lamp')?.classList.toggle('is-late', sec <= 300);
    frame?.classList.toggle('is-dusk', sec <= 120);
    if ($('lamp-label')) $('lamp-label').textContent = sec <= 120 ? 'Lamp off in' : 'Lamp on';
    if ($('lamp-left')) $('lamp-left').textContent = sec <= 120 ? '' : 'left';
    if ($('qr-lamp')) $('qr-lamp').textContent = `Lamp on, ${lampWords(sec)} left`;
    const closingCard = $('room-closing-card');
    if (closingCard && !closingCard.hidden) $('closing-countdown').textContent = String(sec);
    else if (sec <= 120) showBanner(sec);
    if (sec <= 0 && state.deadline) { endRoom('expired'); return; }
    tickTimer = setTimeout(tick, 1000);
  }
  function showBanner(sec) {
    const slot = $('expiry-slot');
    if (!slot) return;
    const open = Array.from(uploads.values()).filter((i) => ['queued', 'sending', 'waiting'].includes(i.state)).length;
    const visible = `This room closes in ${mmss(sec)}. Save anything you need now.${open ? ` ${open} file${open === 1 ? ' is' : 's are'} still sending and may not finish.` : ''}`;
    if (!bannerShown) {
      bannerShown = true;
      const b = el('div', { class: 'lt-banner', id: 'expiry-banner', role: 'alert' }, [
        svg(ICON_ALERT),
        el('span', {}, [el('span', { id: 'expiry-visible', 'aria-hidden': 'true', text: visible }), el('span', { class: 'lt-sr', id: 'expiry-summary', text: `This room closes in about ${Math.max(1, Math.ceil(sec / 60))} minute${Math.ceil(sec / 60) === 1 ? '' : 's'}. Save anything you need now.` })]),
      ]);
      slot.appendChild(b);
    } else {
      $('expiry-visible').textContent = visible;
    }
  }

  function endRoom(cause) {
    if (state.ended) return;
    state.ended = true;
    clearTimeout(pollTimer);
    clearTimeout(tickTimer);
    stopMedia();
    closeAllModals();
    if (composer) composer.hidden = true;
    if ($('composer-backdrop')) $('composer-backdrop').hidden = true;
    let hadOpen = false;
    for (const id of order) {
      const it = uploads.get(id);
      if (it && ['queued', 'sending', 'waiting', 'failed'].includes(it.state)) {
        hadOpen = true;
        const xhr = it.xhr;
        it.attempt++;
        transition(it, 'ended', 'room-ended');
        if (xhr && xhr.readyState !== 4) { try { xhr.abort(); } catch (_) {} }
      }
    }
    currentId = null;
    for (const [, rec] of records) cleanup(rec.el);
    const title = $('inactive-title');
    const msg = $('inactive-message');
    if (cause === 'closed-by-me') {
      title.textContent = 'You closed this room';
      msg.textContent = 'Files and notes on it were deleted. Anything you already saved is still on this device.';
    } else if (cause === 'closed') {
      title.textContent = isCreator ? 'This room was closed' : 'This room was closed by its creator';
      msg.textContent = 'Files and notes on it were deleted. Anything you already saved is still on this device.';
    } else {
      title.textContent = 'This room has ended';
      msg.textContent = 'Files and notes on it were deleted. Anything you already saved is still on this device.';
    }
    if (downloaded.size) $('inactive-downloads').hidden = false;
    if (frame) frame.hidden = true;
    if ($('pin-barrier-card')) $('pin-barrier-card').hidden = true;
    if (tray && !hadOpen) tray.hidden = true;
    $('room-inactive-card').hidden = false;
    title.focus();
    announce(title.textContent, true);
  }

  // ---------------------------------------------------------------------------------------------
  // Presence, capacity and the room log
  // ---------------------------------------------------------------------------------------------
  // Room log: what the server recorded (room opened, files laid down) plus this device's own events.
  const activities = []; // local: { at, text }
  let logClearedAt = 0;
  function logActivity(text) {
    if (!$('activity-list')) return;
    activities.unshift({ at: Date.now(), text });
    if (activities.length > 20) activities.pop();
    renderLog();
  }
  function serverLogText(t) {
    const c = (t.content || '').trim();
    let m;
    if (/^room created\b/i.test(c)) return 'Room opened';
    if ((m = /^file uploaded:\s*(.+)$/i.exec(c))) return `${m[1]} laid on the table`;
    return c;
  }
  function renderLog() {
    const ol = $('activity-list');
    if (!ol) return;
    const server = state.texts.filter((t) => t.sender_type === 'server').map((t) => ({ at: Date.parse(t.created_at) || 0, text: serverLogText(t) }));
    const local = activities.filter((a) => !/ laid down /.test(a.text));
    const all = server.concat(local).filter((a) => a.at > logClearedAt).sort((x, y) => y.at - x.at).slice(0, 8);
    const sig = all.map((a) => a.at + a.text).join('|');
    if (ol.dataset.sig === sig) return;
    ol.dataset.sig = sig;
    ol.replaceChildren(...(all.length ? all.map((a) => el('li', { text: `${a.text}, ${clock(new Date(a.at).toISOString())}` })) : [el('li', { text: 'No transfer activity yet.' })]));
    if ($('btn-clear-activity')) $('btn-clear-activity').hidden = all.length === 0;
  }
  $('btn-clear-activity')?.addEventListener('click', () => { activities.length = 0; logClearedAt = Date.now(); renderLog(); });
  renderLog();

  // Device names come from the user agent ("Apple iOS", "Android Device", "Windows Device"…).
  function seatName(n) {
    const s = (n || '').replace(/ Device$/i, '');
    if (/ios|iphone/i.test(s)) return 'iPhone';
    if (/ipad/i.test(s)) return 'iPad';
    if (/android/i.test(s)) return 'Android phone';
    if (/mac/i.test(s)) return 'Mac';
    if (/windows/i.test(s)) return 'Windows PC';
    if (/linux/i.test(s)) return 'Linux computer';
    return s || 'Device';
  }
  function renderSeats(participants, count) {
    const ul = $('participant-list');
    const names = [];
    const seen = {};
    for (const p of participants || []) {
      const base = seatName(p.name);
      seen[base] = (seen[base] || 0) + 1;
      names.push({ id: p.id || p.ip || base, label: seen[base] > 1 ? `${base} ${seen[base]}` : base });
    }
    if (ul) {
      const sig = names.map((n) => n.label).join('|');
      if (ul.dataset.sig !== sig) {
        ul.dataset.sig = sig;
        ul.replaceChildren(...names.map((n) => el('li', { class: 'lt-seat', text: n.label })));
      }
    }
    const n = typeof count === 'number' ? count : names.length;
    if ($('participant-count')) $('participant-count').textContent = String(n);
    if ($('participant-count-val')) $('participant-count-val').textContent = String(n);
    if ($('qr-seats')) $('qr-seats').textContent = n === 0 ? 'No other device at the table yet' : (n === 1 ? 'One device at the table' : `${n} devices at the table`);
    // Room log: a device sitting down
    for (const nm of names) {
      if (!state.seenSeats.has(nm.id)) {
        state.seenSeats.add(nm.id);
        if (state.seatsInitialised) logActivity(`${nm.label} sat down`);
      }
    }
    state.seatsInitialised = true;
  }
  function renderCapacity(fd) {
    const cap = $('capacity');
    if (!cap || !fd) return;
    const parts = [];
    if (fd.max_room_size) parts.push(`${formatBytes(fd.total_size_bytes || 0)} of ${formatBytes(fd.max_room_size)}`);
    if (fd.max_files) parts.push(`${fd.file_count || 0} of ${fd.max_files} files`);
    cap.textContent = parts.join(' · ');
  }

  // ---------------------------------------------------------------------------------------------
  // Polling
  // ---------------------------------------------------------------------------------------------
  let pollTimer = null;
  async function fetchFiles() {
    const res = await fetch(`${API}/files`, { cache: 'no-store' });
    if (res.status === 404 || res.status === 410) { endRoom('gone'); return false; }
    if (!res.ok) return true;
    const text = await res.text();
    if (text === state.lastBodies.files) return true;
    state.lastBodies.files = text;
    const d = JSON.parse(text);
    // Keep files added straight from an upload response until the server lists them.
    state.files = d.files || [];
    renderCapacity(d);
    evaluateView('poll');
    render();
    return true;
  }
  async function fetchTexts() {
    const res = await fetch(`${API}/texts`, { cache: 'no-store', headers: { 'X-Client-Session-ID': getClientSessionId() } });
    if (res.status === 404 || res.status === 410) { endRoom('gone'); return false; }
    if (!res.ok) return true;
    const text = await res.text();
    if (text === state.lastBodies.texts) return true;
    state.lastBodies.texts = text;
    const d = JSON.parse(text);
    state.texts = d.texts || [];
    evaluateView('poll');
    render();
    renderLog();
    return true;
  }
  async function poll() {
    if (state.ended || !token) return;
    try {
      const res = await fetch(API, { cache: 'no-store' });
      if (res.status === 404 || res.status === 410) {
        const d = await res.json().catch(() => ({}));
        endRoom(d.status === 'closed' ? 'closed' : 'gone');
        return;
      }
      if (res.ok) {
        const d = await res.json();
        state.room = d;
        if (d.status === 'closed') { endRoom('closed'); return; }
        if (d.status === 'expired' || d.remaining_seconds <= 0) { endRoom('expired'); return; }
        setRemaining(d.remaining_seconds);
        if (d.status === 'closing') {
          // The server's grace period before a close takes effect.
          setRemaining(Math.min(d.remaining_seconds, d.closing_remaining_seconds || 0));
          $('room-closing-card').hidden = false;
          $('closing-countdown').textContent = String(d.closing_remaining_seconds || 0);
        }
        if (page === 'participant') {
          const locked = d.pin_required && !d.pin_authenticated;
          if (locked !== state.locked) {
            state.locked = locked;
            $('pin-barrier-card').hidden = !locked;
            frame.hidden = locked;
          }
        }
        if (!(state.locked)) renderSeats(d.participants || [], d.participant_count);
      }
      if (!state.locked) {
        if (!(await fetchFiles())) return;
        if (!(await fetchTexts())) return;
      }
    } catch (_) {
      // A missed poll is retried on the next cycle.
    }
    pollTimer = setTimeout(poll, document.hidden ? 12000 : 3500);
  }
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden && !state.ended) { clearTimeout(pollTimer); poll(); }
  });

  // ---------------------------------------------------------------------------------------------
  // QR projection, Close room, share links, room menu, shortcuts
  // ---------------------------------------------------------------------------------------------
  const qr = $('qr-dialog');
  function openQR(opener) { if (qr && !state.ended) openModal(qr, opener, $('qr-done-btn')); }
  $('show-qr-btn')?.addEventListener('click', (e) => openQR(e.currentTarget));
  $('empty-qr-btn')?.addEventListener('click', (e) => openQR(e.currentTarget));
  $('qr-done-btn')?.addEventListener('click', () => closeModal(qr));
  $('copy-link-btn')?.addEventListener('click', async () => {
    const input = $('participant-link-input');
    const ok = await copyText(input.value);
    const toast = $('copy-toast');
    toast.textContent = ok ? 'Address copied' : "Couldn't copy. Select the address and copy it.";
    toast.classList.toggle('is-fail', !ok);
    toast.classList.add('show');
    if (!ok) input.select();
    setTimeout(() => toast.classList.remove('show'), 2500);
  });

  const closeDlg = $('modal-close-confirm');
  $('close-room-btn')?.addEventListener('click', (e) => openModal(closeDlg, e.currentTarget, $('modal-close-cancel')));
  $('modal-close-cancel')?.addEventListener('click', () => closeModal(closeDlg));
  $('confirm-close-btn')?.addEventListener('click', async () => {
    const btn = $('confirm-close-btn');
    btn.disabled = true;
    btn.textContent = 'Closing…';
    $('close-error').hidden = true;
    try {
      const res = await fetch(`${API}/close`, { method: 'POST' });
      if (res.ok || res.status === 404 || res.status === 410) { endRoom('closed-by-me'); return; }
      const d = await res.json().catch(() => ({}));
      $('close-error').textContent = d.error || 'The room could not be closed.';
      $('close-error').hidden = false;
    } catch (_) {
      // No response: the outcome is unknown, so claim neither success nor failure.
      $('close-error').textContent = 'Could not confirm that the room was closed. Check your connection and try again.';
      $('close-error').hidden = false;
    }
    btn.disabled = false;
    btn.textContent = 'Close room';
  });

  const shareDlg = $('share-modal');
  function openShare(fileId, filename, opener) {
    if (!shareDlg) return;
    $('modal-file-id').value = fileId;
    $('modal-file-subtitle').textContent = `Create a temporary public download link for "${filename}".`;
    $('share-result-box').hidden = true;
    $('share-error-box').hidden = true;
    openModal(shareDlg, opener, $('share-ttl-select'));
  }
  $('close-modal-btn')?.addEventListener('click', () => closeModal(shareDlg));
  $('create-share-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = $('generate-share-btn');
    btn.disabled = true;
    $('share-error-box').hidden = true;
    try {
      const res = await fetch(`${API}/files/${encodeURIComponent($('modal-file-id').value)}/share`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ttl_seconds: parseInt($('share-ttl-select').value, 10) || 3600 }),
      });
      if (res.ok) {
        const d = await res.json();
        $('generated-share-input').value = d.share_url;
        $('share-result-box').hidden = false;
        $('copy-share-btn').focus();
      } else {
        const d = await res.json().catch(() => ({}));
        $('share-error-box').textContent = d.error || 'Failed to create share';
        $('share-error-box').hidden = false;
      }
    } catch (_) {
      $('share-error-box').textContent = 'Network error while creating share';
      $('share-error-box').hidden = false;
    } finally {
      btn.disabled = false;
    }
  });
  $('copy-share-btn')?.addEventListener('click', async () => {
    const input = $('generated-share-input');
    const ok = await copyText(input.value);
    $('copy-share-toast').textContent = ok ? 'Link copied' : "Couldn't copy. Select the link and copy it.";
    if (!ok) input.select();
  });

  const menu = $('room-menu');
  const menuBtn = $('room-menu-btn');
  menuBtn?.addEventListener('click', () => {
    const open = menu.hidden;
    menu.hidden = !open;
    menuBtn.setAttribute('aria-expanded', String(open));
    if (open) menu.querySelector('button')?.focus();
  });
  document.addEventListener('click', (e) => {
    if (menu && !menu.hidden && !menu.contains(e.target) && !menuBtn.contains(e.target)) { menu.hidden = true; menuBtn.setAttribute('aria-expanded', 'false'); }
  });
  menu?.addEventListener('click', async (e) => {
    const b = e.target.closest('[data-menu]');
    if (!b) return;
    menu.hidden = true;
    menuBtn.setAttribute('aria-expanded', 'false');
    const what = b.dataset.menu;
    if (what === 'qr') openQR(menuBtn);
    else if (what === 'copy') { const ok = await copyText($('participant-link-input').value); announce(ok ? 'Address copied' : "Couldn't copy the address", !ok); }
    else if (what === 'theme') (document.documentElement.getAttribute('data-theme') === 'light' ? $('theme-btn-dark') : $('theme-btn-light'))?.click();
    else if (what === 'shortcuts') openModal($('shortcuts-dialog'), menuBtn, $('shortcuts-close'));
    else if (what === 'close') openModal(closeDlg, menuBtn, $('modal-close-cancel'));
  });
  $('shortcuts-close')?.addEventListener('click', () => closeModal($('shortcuts-dialog')));

  document.addEventListener('keydown', (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey || modalStack.length || state.ended || state.locked) return;
    const tag = (e.target && e.target.tagName || '').toUpperCase();
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (e.target && e.target.isContentEditable)) return;
    if ((e.key === 'q' || e.key === 'Q') && isCreator) { e.preventDefault(); openQR($('show-qr-btn')); }
    else if (e.key === '/') { e.preventDefault(); $('search-input')?.focus(); }
    else if (e.key === 'n' || e.key === 'N') { e.preventDefault(); openComposer(); }
    else if (e.key === '?') { e.preventDefault(); openModal($('shortcuts-dialog'), document.activeElement, $('shortcuts-close')); }
  });

  // ---------------------------------------------------------------------------------------------
  // PIN screen (participant)
  // ---------------------------------------------------------------------------------------------
  const pinForm = $('pin-form');
  const pinInput = $('participant-pin-input');
  const unlockBtn = $('unlock-btn');
  const pinErr = $('pin-error');
  const pinCool = $('pin-cooldown');
  function pinError(msg) { $('pin-error-text').textContent = msg; pinErr.style.display = 'flex'; }
  function lockout(seconds) {
    let rem = seconds;
    pinCool.style.display = 'flex';
    pinErr.style.display = 'none';
    pinInput.disabled = true;
    unlockBtn.disabled = true;
    $('unlock-btn-text').textContent = 'Locked';
    const show = () => { $('pin-cooldown-text').textContent = `Too many failed attempts. Cooldown active (${rem}s). Try again in ${mmss(rem).replace(/^00:/, '0:')}.`; };
    show();
    const t = setInterval(() => {
      rem--;
      if (rem <= 0) {
        clearInterval(t);
        pinCool.style.display = 'none';
        pinInput.disabled = false;
        pinInput.value = '';
        unlockBtn.disabled = false;
        $('unlock-btn-text').textContent = 'Join';
        pinInput.focus();
      } else show();
    }, 1000);
  }
  if (pinInput) {
    pinInput.addEventListener('input', () => {
      const digits = pinInput.value.replace(/[^0-9]/g, '');
      if (digits !== pinInput.value) pinInput.value = digits;
    });
    if (!$('pin-barrier-card').hidden) pinInput.focus();
    if (body.dataset.isLocked === 'true') lockout(parseInt(body.dataset.retryAfter || '30', 10) || 30);
  }
  pinForm?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const pin = pinInput.value.trim();
    if (!/^[0-9]{4,8}$/.test(pin)) { pinError('PIN must be 4 to 8 digits.'); return; }
    unlockBtn.disabled = true;
    $('unlock-btn-text').textContent = 'Checking…';
    pinErr.style.display = 'none';
    try {
      const res = await fetch(`${API}/auth/pin`, { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify({ pin }) });
      if (res.ok) {
        state.locked = false;
        body.dataset.pinAuthenticated = 'true';
        $('pin-barrier-card').hidden = true;
        frame.hidden = false;
        $('table-heading')?.focus();
        clearTimeout(pollTimer);
        poll();
        return;
      }
      const d = await res.json().catch(() => ({}));
      if (res.status === 429) { lockout(d.retry_after_seconds || 30); return; }
      let msg = "That PIN didn't match.";
      if (d.remaining_attempts !== undefined) msg += ` ${d.remaining_attempts} attempts remaining.`;
      pinError(msg);
      pinInput.value = '';
      pinInput.focus();
    } catch (_) {
      pinError('Network error. Please try again.');
    }
    unlockBtn.disabled = false;
    $('unlock-btn-text').textContent = 'Join';
  });

  // ---------------------------------------------------------------------------------------------
  // Start
  // ---------------------------------------------------------------------------------------------
  if (!$('room-inactive-card').hidden) {
    state.ended = true;
    $('inactive-title').textContent = 'This room has ended';
    return;
  }
  list?.querySelectorAll('[data-ssr]').forEach((n) => n.remove()); // server-rendered first paint
  evaluateView('load');
  applyView(state.view, 'load');
  render();
  tick();
  poll();
})();
