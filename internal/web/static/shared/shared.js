// shared/shared.js — namespace of helpers every page imports.
//
// Conventions:
//   - All API calls go through API.fetch / API.get / API.post — adds the
//     stable per-browser user id and a JSON content-type, and decodes the
//     response envelope uniformly so pages handle errors the same way.
//   - DOM helpers (escape, showError) live here so each page doesn't
//     reinvent them.
//   - No external dependencies, no build step.

(function () {
  'use strict';

  // --- user id ---------------------------------------------------------

  const UID_KEY = 'workout-api-uid';

  function getUserID() {
    let uid = localStorage.getItem(UID_KEY);
    if (!uid) {
      uid = 'demo-' + Math.random().toString(36).slice(2, 10);
      localStorage.setItem(UID_KEY, uid);
    }
    return uid;
  }

  // --- api client ------------------------------------------------------

  async function apiFetch(path, opts = {}) {
    const headers = Object.assign(
      { 'Content-Type': 'application/json', 'X-User-ID': getUserID() },
      opts.headers || {}
    );
    const resp = await fetch(path, Object.assign({}, opts, { headers }));
    let data = null;
    const ct = resp.headers.get('content-type') || '';
    if (ct.includes('application/json')) {
      try { data = await resp.json(); } catch (e) { /* empty body */ }
    }
    if (!resp.ok) {
      const code = (data && data.error && data.error.code) || 'unknown_error';
      const msg = (data && data.error && data.error.message) || resp.statusText;
      const err = new Error(msg);
      err.code = code;
      err.status = resp.status;
      throw err;
    }
    return data;
  }

  const apiGet = (path) => apiFetch(path, { method: 'GET' });
  const apiPost = (path, body) =>
    apiFetch(path, { method: 'POST', body: JSON.stringify(body) });
  const apiDelete = (path) => apiFetch(path, { method: 'DELETE' });

  // --- dom helpers -----------------------------------------------------

  function escape(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  function showError(el, err) {
    if (!el) return;
    const title = (err.code || 'error').replace(/_/g, ' ');
    const msg = err.message || 'something went wrong';
    el.innerHTML =
      '<strong>' + escape(title) + '</strong>' +
      '<span>' + escape(msg) + '</span>';
    el.classList.remove('hidden');
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }

  function hideError(el) {
    if (el) el.classList.add('hidden');
  }

  // --- shell rendering -------------------------------------------------

  // Renders the shared header/nav and footer into the placeholder divs each
  // page declares (#site-header, #site-footer). Keeping the markup here means
  // each HTML page only has page-specific content; adding a new nav link is
  // a single edit instead of one per page.
  function renderShell() {
    const headerEl = document.getElementById('site-header');
    if (headerEl) {
      headerEl.outerHTML = SHELL_HEADER;
    }
    const footerEl = document.getElementById('site-footer');
    if (footerEl) {
      footerEl.outerHTML = SHELL_FOOTER;
    }
    markActiveNav();
  }

  const NAV_LINKS = [
    { href: '/',          label: 'Generate' },
    { href: '/exercises', label: 'Exercises' },
    { href: '/plans',     label: 'Plans' },
  ];

  function navHTML(extraClass) {
    return NAV_LINKS.map(l =>
      '<a href="' + l.href + '">' + escape(l.label) + '</a>'
    ).join('');
  }

  const SHELL_HEADER =
    '<header class="site-header">' +
      '<div class="container header-row">' +
        '<a class="logo" href="/">' +
          '<div class="logo-mark">/</div>' +
          '<div class="logo-text"><strong>workout</strong><span class="dim">.api</span></div>' +
        '</a>' +
        '<nav class="nav">' + navHTML() + '</nav>' +
        '<div class="header-aside">' +
          '<a href="https://github.com/ataliaferro46/go-workout-api" target="_blank" rel="noopener">github →</a>' +
          '<span class="badge"><span class="dot"></span>live</span>' +
        '</div>' +
      '</div>' +
      '<nav class="container nav-mobile">' + navHTML() + '</nav>' +
    '</header>';

  const SHELL_FOOTER =
    '<footer class="site-footer">' +
      '<div class="container footer-row">' +
        '<div><strong>Built by Alex Taliaferro</strong> · backend portfolio piece</div>' +
        '<div class="footer-stack">Go · pgx · pgxpool · goose · embed.FS · AES-GCM · Fly.io · Neon</div>' +
      '</div>' +
    '</footer>';

  // Marks the current page's nav link with .active so the highlight reflects
  // the URL without each page having to do it manually.
  function markActiveNav() {
    let path = window.location.pathname.replace(/\/$/, '');
    if (path === '') path = '/';
    document.querySelectorAll('.nav a, .nav-mobile a').forEach(a => {
      let href = a.getAttribute('href').replace(/\/$/, '');
      if (href === '') href = '/';
      if (href === path) a.classList.add('active');
    });
  }

  // --- format helpers --------------------------------------------------

  function formatTitleCase(s) {
    return String(s).replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
  }

  function formatDate(iso) {
    if (!iso) return '';
    try {
      const d = new Date(iso);
      return d.toLocaleString(undefined, {
        month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
      });
    } catch (e) { return iso; }
  }

  // --- export ----------------------------------------------------------

  window.API = {
    getUserID,
    fetch: apiFetch,
    get: apiGet,
    post: apiPost,
    delete: apiDelete,
  };
  window.UI = {
    escape,
    showError,
    hideError,
    formatTitleCase,
    formatDate,
  };

  // Run on script load — every page includes this script in <head>.
  document.addEventListener('DOMContentLoaded', renderShell);
})();
