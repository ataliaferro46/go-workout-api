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

  // --- auth state ------------------------------------------------------
  //
  // The session lives in an HttpOnly cookie set by /v1/auth/login. JS cannot
  // read it (deliberately — protects against XSS). We track "is the user
  // logged in" by hitting /v1/auth/me and caching the result on window.

  let currentUser = null;
  // Cache the in-flight Promise (not a boolean) so concurrent callers — e.g.
  // renderShell and a page's ensureLoggedIn, both firing on DOMContentLoaded —
  // share the same fetch. A boolean flag here causes a race: the second
  // caller short-circuits to a still-null currentUser before the first
  // caller's fetch resolves, and the page incorrectly thinks the user is
  // logged out.
  let userLoadPromise = null;

  function loadCurrentUser() {
    if (userLoadPromise) return userLoadPromise;
    userLoadPromise = (async () => {
      try {
        const resp = await fetch('/v1/auth/me', { credentials: 'same-origin' });
        if (resp.ok) {
          const j = await resp.json();
          currentUser = j.user || null;
        } else {
          currentUser = null;
        }
      } catch (e) {
        currentUser = null;
      }
      return currentUser;
    })();
    return userLoadPromise;
  }

  async function ensureLoggedIn() {
    const u = await loadCurrentUser();
    if (!u) {
      const here = encodeURIComponent(window.location.pathname + window.location.search);
      window.location.href = '/login?next=' + here;
    }
    return u;
  }

  async function redirectIfLoggedIn(to) {
    const u = await loadCurrentUser();
    if (u) window.location.href = to || '/';
  }

  async function logout() {
    try {
      await fetch('/v1/auth/logout', { method: 'POST', credentials: 'same-origin' });
    } catch (e) { /* ignore */ }
    currentUser = null;
    userLoadPromise = null; // so a subsequent loadCurrentUser refetches
    window.location.href = '/login';
  }

  // --- api client ------------------------------------------------------

  async function apiFetch(path, opts = {}) {
    const headers = Object.assign(
      { 'Content-Type': 'application/json' },
      opts.headers || {}
    );
    const resp = await fetch(path, Object.assign(
      {}, opts, { headers, credentials: 'same-origin' }
    ));
    if (resp.status === 401) {
      // Session expired or never existed — bounce to login.
      const here = encodeURIComponent(window.location.pathname + window.location.search);
      window.location.href = '/login?next=' + here;
      throw new Error('not authenticated');
    }
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
  // page declares (#site-header, #site-footer). Loads the current user
  // first so the header reflects auth state.
  async function renderShell() {
    await loadCurrentUser();
    const headerEl = document.getElementById('site-header');
    if (headerEl) {
      headerEl.outerHTML = shellHeaderHTML();
    }
    const footerEl = document.getElementById('site-footer');
    if (footerEl) {
      footerEl.outerHTML = SHELL_FOOTER;
    }
    markActiveNav();
    const logoutLink = document.getElementById('logout-link');
    if (logoutLink) {
      logoutLink.addEventListener('click', (e) => {
        e.preventDefault();
        logout();
      });
    }
    const navToggle = document.getElementById('nav-toggle');
    const navMobile = document.getElementById('nav-mobile');
    if (navToggle && navMobile) {
      navToggle.addEventListener('click', () => {
        navMobile.classList.toggle('open');
        navToggle.classList.toggle('open');
      });
    }
  }

  const NAV_LINKS = [
    { href: '/',          label: 'Generate' },
    { href: '/quick',     label: 'Quick' },
    { href: '/exercises', label: 'Exercises' },
    { href: '/plans',     label: 'Plans' },
    { href: '/history',   label: 'History' },
  ];

  function navHTML(extraClass) {
    return NAV_LINKS.map(l =>
      '<a href="' + l.href + '">' + escape(l.label) + '</a>'
    ).join('');
  }

  function shellHeaderHTML() {
    const userBlock = currentUser
      ? '<a href="/settings" class="badge badge-link" title="' + escape(currentUser.email) + ' — open settings">' + escape(shortEmail(currentUser.email)) + '</a>' +
        '<a href="#" id="logout-link">sign out</a>'
      : '<a href="/login">sign in</a>';
    return '<header class="site-header">' +
      '<div class="container header-row">' +
        '<a class="logo" href="/">' +
          '<div class="logo-mark">/</div>' +
          '<div class="logo-text"><strong>workout</strong><span class="dim">.api</span></div>' +
        '</a>' +
        '<nav class="nav">' + navHTML() + '</nav>' +
        '<div class="header-aside">' + userBlock + '</div>' +
        '<button class="nav-toggle" id="nav-toggle" aria-label="Open menu">' +
          '<span></span><span></span><span></span>' +
        '</button>' +
      '</div>' +
      '<nav class="container nav-mobile" id="nav-mobile">' + navHTML() + userBlock + '</nav>' +
    '</header>';
  }

  function shortEmail(email) {
    if (!email) return '';
    const at = email.indexOf('@');
    if (at <= 0) return email;
    return email.length > 24 ? email.slice(0, at) : email;
  }

  const SHELL_FOOTER =
    '<footer class="site-footer">' +
      '<div class="container footer-row">' +
        '<div><strong>workout</strong><span class="dim">.api</span></div>' +
        '<div class="footer-legal">' +
          '<a href="/privacy">Privacy</a>' +
          '<span class="footer-sep">·</span>' +
          '<a href="/terms">Terms</a>' +
        '</div>' +
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
    fetch: apiFetch,
    get: apiGet,
    post: apiPost,
    delete: apiDelete,
  };
  window.AUTH = {
    currentUser: () => currentUser,
    ensureLoggedIn,
    redirectIfLoggedIn,
    logout,
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
