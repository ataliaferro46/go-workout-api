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
    { href: '/nutrition', label: 'Nutrition' },
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

  // --- units / measurement system --------------------------------------
  //
  // Single source of truth for measurement preference. Default is
  // imperial (lbs / miles / ft+in) — the storage system is always metric
  // (kg, km, cm) and the UI converts on input/display. Switching
  // systems updates localStorage; pages decide whether to re-render or
  // listen for the change event.

  // One-time migration: earlier versions stored 'kg' or 'lbs' as the
  // unit preference. Map those to the new 'metric' / 'imperial' values.
  (function migrateUnitsKey() {
    const cur = localStorage.getItem('wapi.unit');
    if (cur === 'kg') localStorage.setItem('wapi.unit', 'metric');
    else if (cur === 'lbs') localStorage.setItem('wapi.unit', 'imperial');
  })();

  const UNITS = {
    system() { return localStorage.getItem('wapi.unit') === 'metric' ? 'metric' : 'imperial'; },
    setSystem(sys) {
      localStorage.setItem('wapi.unit', sys === 'metric' ? 'metric' : 'imperial');
      window.dispatchEvent(new CustomEvent('wapi:units-changed', { detail: { system: sys } }));
    },
    // --- weight ---
    weightLabel() { return UNITS.system() === 'imperial' ? 'lbs' : 'kg'; },
    kgToDisplay(kg) {
      if (kg == null || kg === 0) return 0;
      return UNITS.system() === 'imperial' ? +(kg * 2.20462).toFixed(1) : +kg.toFixed(1);
    },
    displayToKg(val) {
      if (val == null || isNaN(val)) return 0;
      return UNITS.system() === 'imperial' ? +(val / 2.20462).toFixed(2) : +val;
    },
    // --- distance ---
    distanceLabel() { return UNITS.system() === 'imperial' ? 'mi' : 'km'; },
    kmToDisplay(km) {
      if (km == null || km === 0) return 0;
      return UNITS.system() === 'imperial' ? +(km * 0.621371).toFixed(2) : +km.toFixed(2);
    },
    displayToKm(val) {
      if (val == null || isNaN(val)) return 0;
      return UNITS.system() === 'imperial' ? +(val / 0.621371).toFixed(2) : +val;
    },
    // --- height (cm <-> ft + in) ---
    cmToFtIn(cm) {
      if (!cm) return { ft: 0, inches: 0 };
      const totalIn = cm / 2.54;
      const ft = Math.floor(totalIn / 12);
      const inches = Math.round(totalIn - ft * 12);
      return { ft, inches };
    },
    ftInToCm(ft, inches) {
      const f = parseInt(ft, 10) || 0;
      const i = parseInt(inches, 10) || 0;
      return Math.round((f * 12 + i) * 2.54);
    },
  };

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
  window.UNITS = UNITS;

  // Inject the SVG favicon if the page didn't declare one. Modern browsers
  // prefer the rel="icon" + type="image/svg+xml" link element; the legacy
  // /favicon.ico request is served from the same SVG (see embed.go).
  function injectFavicon() {
    if (document.querySelector('link[rel="icon"]')) return;
    const link = document.createElement('link');
    link.rel = 'icon';
    link.type = 'image/svg+xml';
    link.href = '/favicon.svg';
    document.head.appendChild(link);
  }

  // Run on script load — every page includes this script in <head>.
  document.addEventListener('DOMContentLoaded', () => {
    injectFavicon();
    renderShell();
  });
})();
