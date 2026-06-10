// pages/settings.js — profile view + biometrics connect/disconnect.

(function () {
  'use strict';

  // Provider metadata. The backend tells us which names exist; this map tells
  // us how to render each one (label, color, status text for "not yet
  // approved" cases like Whoop's developer queue).
  const PROVIDER_META = {
    oura: {
      label: 'Oura Ring',
      tagline: 'Daily readiness scores inform tomorrow\'s session',
      accent: '#7c3aed',
    },
    whoop: {
      label: 'Whoop',
      tagline: 'Recovery score gates intensity',
      accent: '#10b981',
    },
  };

  document.addEventListener('DOMContentLoaded', async () => {
    const user = await AUTH.ensureLoggedIn();
    if (!user) return;

    renderProfile(user);
    handleFlashFromQuery();
    await loadProviders();
    await loadReadings();
    await loadDietPreferences();
    wireDietSave();
  });

  async function loadDietPreferences() {
    try {
      const data = await API.get('/v1/nutrition/preferences');
      document.getElementById('allergies-input').value = (data.allergies || []).join(', ');
      document.getElementById('dislikes-input').value = (data.disliked_foods || []).join(', ');
    } catch (e) { /* not authenticated yet — skip */ }
  }

  function wireDietSave() {
    const btn = document.getElementById('diet-save-btn');
    if (!btn) return;
    btn.addEventListener('click', async () => {
      const parse = s => s.split(',').map(x => x.trim()).filter(x => x.length > 0);
      const allergies = parse(document.getElementById('allergies-input').value);
      const disliked = parse(document.getElementById('dislikes-input').value);
      btn.disabled = true;
      btn.textContent = 'Saving…';
      try {
        await API.fetch('/v1/nutrition/preferences', {
          method: 'PATCH',
          body: JSON.stringify({ allergies: allergies, disliked_foods: disliked }),
        });
        btn.textContent = '✓ Saved';
        setTimeout(() => { btn.textContent = 'Save preferences'; btn.disabled = false; }, 1200);
      } catch (err) {
        btn.disabled = false;
        btn.textContent = 'Save preferences';
        UI.showError(document.getElementById('flash-error'), err);
      }
    });
  }

  function renderProfile(user) {
    document.getElementById('profile-email').textContent = user.email;
    const joined = UI.formatDate(user.created_at);
    document.getElementById('profile-joined').textContent = joined || '—';
    const idEl = document.getElementById('profile-id');
    if (idEl) idEl.textContent = user.id;
    const form = document.getElementById('profile-form');
    if (!form) return;

    // Set up the imperial/metric toggle
    syncProfileUnitButtons();
    document.getElementById('profile-imperial-btn').onclick = () => switchProfileUnits('imperial', user);
    document.getElementById('profile-metric-btn').onclick = () => switchProfileUnits('metric', user);

    // Populate fields with whatever the user has stored (DB is metric).
    fillProfileFields(user);

    if (user.birth_date) form.birth_date.value = String(user.birth_date).slice(0, 10);
    if (user.sex) form.sex.value = user.sex;
    updateBMI(user.height_cm, user.weight_kg);

    // Use onsubmit (rather than addEventListener) so re-rendering doesn't
    // double-bind the handler each time.
    form.onsubmit = async (e) => {
      e.preventDefault();
      const body = collectProfileBody();
      try {
        const data = await API.fetch('/v1/auth/profile', {
          method: 'PATCH', body: JSON.stringify(body),
        });
        renderProfile(data.user);
        const el = document.getElementById('flash-success');
        el.innerHTML = '<strong>Saved</strong><span>Profile updated.</span>';
        el.classList.remove('hidden');
      } catch (err) {
        UI.showError(document.getElementById('flash-error'), err);
      }
    };
  }

  function syncProfileUnitButtons() {
    const sys = UNITS.system();
    document.getElementById('profile-imperial-btn').classList.toggle('active', sys === 'imperial');
    document.getElementById('profile-metric-btn').classList.toggle('active', sys === 'metric');
    document.getElementById('height-imperial').classList.toggle('hidden', sys !== 'imperial');
    document.getElementById('height-metric').classList.toggle('hidden', sys !== 'metric');
    document.getElementById('weight-unit-label').textContent =
      sys === 'imperial' ? '(lbs)' : '(kg)';
  }

  function switchProfileUnits(sys, user) {
    UNITS.setSystem(sys);
    syncProfileUnitButtons();
    fillProfileFields(user);
  }

  function fillProfileFields(user) {
    const sys = UNITS.system();
    if (user.height_cm) {
      if (sys === 'imperial') {
        const ftIn = UNITS.cmToFtIn(user.height_cm);
        document.getElementById('height-ft').value = ftIn.ft;
        document.getElementById('height-in').value = ftIn.inches;
      } else {
        document.getElementById('height-cm').value = user.height_cm;
      }
    }
    if (user.weight_kg) {
      document.getElementById('weight-input').value = UNITS.kgToDisplay(user.weight_kg);
    }
  }

  function collectProfileBody() {
    const form = document.getElementById('profile-form');
    const fd = new FormData(form);
    const body = {};
    const sys = UNITS.system();
    if (sys === 'imperial') {
      const ft = document.getElementById('height-ft').value;
      const inches = document.getElementById('height-in').value;
      if (ft || inches) body.height_cm = UNITS.ftInToCm(ft, inches);
    } else {
      const h = parseInt(document.getElementById('height-cm').value, 10);
      if (h > 0) body.height_cm = h;
    }
    const wInput = parseFloat(document.getElementById('weight-input').value);
    if (wInput > 0) body.weight_kg = UNITS.displayToKg(wInput);
    const bd = fd.get('birth_date');
    if (bd) body.birth_date = bd;
    const sx = fd.get('sex');
    if (sx) body.sex = sx;
    return body;
  }

  function updateBMI(heightCM, weightKG) {
    const el = document.getElementById('profile-bmi');
    if (!el) return;
    if (!heightCM || !weightKG) { el.textContent = ''; return; }
    const h = heightCM / 100;
    const bmi = weightKG / (h * h);
    let cat = 'underweight';
    if (bmi >= 18.5) cat = 'normal';
    if (bmi >= 25) cat = 'overweight';
    if (bmi >= 30) cat = 'obese';
    el.textContent = 'BMI: ' + bmi.toFixed(1) + ' (' + cat + ')';
  }

  // Pull ?connected=oura or ?error=foo out of the URL and show a banner.
  // Then clean the URL so a refresh doesn't replay the flash.
  function handleFlashFromQuery() {
    const params = new URLSearchParams(window.location.search);
    const successProvider = params.get('connected');
    const errorCode = params.get('error');
    const errorProvider = params.get('provider');

    if (successProvider) {
      const meta = PROVIDER_META[successProvider] || { label: successProvider };
      const el = document.getElementById('flash-success');
      el.innerHTML =
        '<strong>Connected</strong>' +
        '<span>' + UI.escape(meta.label) + ' is linked. We\'ll start ingesting recovery data on the next sync cycle.</span>';
      el.classList.remove('hidden');
    } else if (errorCode) {
      const meta = errorProvider ? (PROVIDER_META[errorProvider] || { label: errorProvider }) : null;
      const provLabel = meta ? meta.label : 'provider';
      const msg =
        errorCode === 'missing_params' ? 'Authorization was cancelled or did not return the expected response.' :
        errorCode === 'callback_failed' ? 'The provider rejected the authorization. Try again, or check the developer app configuration.' :
        'Connection failed: ' + errorCode + '.';
      const el = document.getElementById('flash-error');
      el.innerHTML =
        '<strong>' + UI.escape(provLabel) + ' didn\'t connect</strong>' +
        '<span>' + UI.escape(msg) + '</span>';
      el.classList.remove('hidden');
    }

    if (successProvider || errorCode) {
      // Strip the query string from the URL bar without reloading the page.
      history.replaceState(null, '', window.location.pathname);
    }
  }

  async function loadProviders() {
    const container = document.getElementById('providers');
    try {
      const data = await API.get('/v1/biometrics/providers');
      const provs = data.providers || [];
      if (provs.length === 0) {
        container.innerHTML = renderEmptyState();
        return;
      }
      container.innerHTML = provs.map(renderProvider).join('') +
        renderPendingWhoopIfMissing(provs);
      attachHandlers();
    } catch (err) {
      container.innerHTML = renderApiUnavailable();
    }
  }

  function renderProvider(p) {
    const meta = PROVIDER_META[p.name] || { label: p.name, tagline: '', accent: 'var(--accent)' };
    const initial = (meta.label[0] || '?').toUpperCase();
    const status = p.connected
      ? '<span class="status status-connected">●&nbsp;connected</span>'
      : '<span class="status status-disconnected">○&nbsp;not connected</span>';
    const lastSync = p.last_synced_at
      ? '<div class="provider-sync">Last sync ' + UI.escape(UI.formatDate(p.last_synced_at)) + '</div>'
      : '';
    const button = p.connected
      ? '<button class="btn btn-ghost" data-action="sync" data-provider="' + UI.escape(p.name) + '">Sync now</button>' +
        '<button class="btn btn-ghost danger" data-action="disconnect" data-provider="' + UI.escape(p.name) + '">Disconnect</button>'
      : '<button class="btn btn-primary" data-action="connect" data-provider="' + UI.escape(p.name) + '">Connect</button>';

    return '<div class="provider-card card padded">' +
      '<div class="provider-head">' +
        '<div class="provider-avatar" style="background:' + meta.accent + '">' + UI.escape(initial) + '</div>' +
        '<div class="provider-meta">' +
          '<div class="provider-name">' + UI.escape(meta.label) + '</div>' +
          '<div class="provider-tagline">' + UI.escape(meta.tagline || '') + '</div>' +
        '</div>' +
        status +
      '</div>' +
      lastSync +
      '<div class="provider-actions">' + button + '</div>' +
    '</div>';
  }

  // Whoop registration is a multi-week approval process; if the backend
  // didn't return a "whoop" provider it means the credentials aren't set
  // yet. Show a pending-approval card so the UI tells the story.
  function renderPendingWhoopIfMissing(provs) {
    const has = provs.some(p => p.name === 'whoop');
    if (has) return '';
    const meta = PROVIDER_META.whoop;
    return '<div class="provider-card card padded provider-pending">' +
      '<div class="provider-head">' +
        '<div class="provider-avatar" style="background:' + meta.accent + ';opacity:0.5">W</div>' +
        '<div class="provider-meta">' +
          '<div class="provider-name">' + meta.label + '</div>' +
          '<div class="provider-tagline">' + meta.tagline + '</div>' +
        '</div>' +
        '<span class="status status-pending">○&nbsp;awaiting approval</span>' +
      '</div>' +
      '<div class="pending-note">Whoop\'s developer program requires a manual review (~2–6 weeks). Once approved, this tile becomes a Connect button.</div>' +
    '</div>';
  }

  function renderEmptyState() {
    return '<div class="card padded" style="grid-column: 1 / -1;">' +
      '<p style="margin: 0; color: var(--text-dim);">No integrations are enabled on the server. Set <code>OURA_CLIENT_ID</code> or <code>WHOOP_CLIENT_ID</code> in Fly secrets to enable.</p>' +
      '</div>';
  }

  function renderApiUnavailable() {
    return '<div class="card padded" style="grid-column: 1 / -1;">' +
      '<p style="margin: 0; color: var(--text-dim);">Biometrics integrations are not currently available. The server may be running without a <code>BIOMETRICS_MASTER_KEY</code>.</p>' +
      '</div>';
  }

  // ---- Latest readings ------------------------------------------------

  const READING_META = {
    recovery:      { label: 'Recovery',      pct: true,  accent: '#10b981', source: 'Whoop' },
    readiness:     { label: 'Readiness',     pct: true,  accent: '#7c3aed', source: 'Oura' },
    strain:        { label: 'Strain',        pct: false, accent: '#ef4444', source: 'Whoop' },
    sleep_score:   { label: 'Sleep score',   pct: true,  accent: '#3b82f6', source: 'wearable' },
    sleep_minutes: { label: 'Sleep',         pct: false, accent: '#3b82f6', source: 'wearable' },
  };

  async function loadReadings() {
    const container = document.getElementById('readings');
    try {
      const data = await API.get('/v1/biometrics/latest');
      const readings = data.readings || {};
      const entries = Object.entries(readings);
      if (entries.length === 0) {
        container.innerHTML =
          '<div class="card padded" style="grid-column: 1 / -1;">' +
            '<p style="margin: 0 0 6px; color: var(--text);"><strong>No readings yet.</strong></p>' +
            '<p style="margin: 0; color: var(--text-dim); font-size: 13px;">' +
              'If you just connected a provider, the first sync runs within 15 minutes. ' +
              'Make sure your wearable has synced to its app recently.' +
            '</p>' +
          '</div>';
        return;
      }
      container.innerHTML = entries.map(([kind, reading]) => {
        const meta = READING_META[kind] || { label: kind, pct: false, accent: 'var(--accent)' };
        const display = meta.pct
          ? Math.round(reading.value * 100) + '%'
          : reading.value.toFixed(1);
        return '<div class="card padded reading-card">' +
          '<div class="reading-head">' +
            '<div class="reading-label">' + UI.escape(meta.label) + '</div>' +
            '<div class="reading-source">' + UI.escape(reading.provider || meta.source) + '</div>' +
          '</div>' +
          '<div class="reading-value" style="color:' + meta.accent + '">' + display + '</div>' +
          '<div class="reading-foot">recorded ' + UI.escape(UI.formatDate(reading.recorded_at)) + '</div>' +
        '</div>';
      }).join('');
    } catch (err) {
      // Hide the entire section if the endpoint isn't available.
      document.getElementById('readings-section').classList.add('hidden');
    }
  }

  function attachHandlers() {
    document.querySelectorAll('[data-action="connect"]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const provider = btn.dataset.provider;
        btn.disabled = true;
        btn.textContent = 'Redirecting…';
        try {
          const data = await API.get('/v1/biometrics/connect/' + encodeURIComponent(provider));
          if (data && data.auth_url) {
            window.location.href = data.auth_url;
          } else {
            throw new Error('missing auth_url');
          }
        } catch (err) {
          btn.disabled = false;
          btn.textContent = 'Connect';
          const el = document.getElementById('flash-error');
          el.innerHTML = '<strong>Could not start authorization</strong><span>' + UI.escape(err.message || 'unknown error') + '</span>';
          el.classList.remove('hidden');
        }
      });
    });
    document.querySelectorAll('[data-action="sync"]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const provider = btn.dataset.provider;
        const orig = btn.textContent;
        btn.disabled = true;
        btn.textContent = 'Syncing…';
        try {
          const data = await API.post('/v1/biometrics/sync/' + encodeURIComponent(provider), {});
          const n = (data && data.readings_ingested) || 0;
          const el = document.getElementById('flash-success');
          el.innerHTML =
            '<strong>Synced</strong>' +
            '<span>' + n + ' new reading' + (n === 1 ? '' : 's') + ' ingested from ' + UI.escape(provider) + '.</span>';
          el.classList.remove('hidden');
          await loadProviders();
          await loadReadings();
        } catch (err) {
          const el = document.getElementById('flash-error');
          el.innerHTML =
            '<strong>Sync failed</strong>' +
            '<span>' + UI.escape(err.message || 'unknown error') + '</span>';
          el.classList.remove('hidden');
        } finally {
          btn.disabled = false;
          btn.textContent = orig;
        }
      });
    });
    document.querySelectorAll('[data-action="disconnect"]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const provider = btn.dataset.provider;
        if (!confirm('Disconnect ' + provider + '? Your stored readings will be deleted from the next sync onward.')) return;
        btn.disabled = true;
        btn.textContent = 'Disconnecting…';
        try {
          await API.delete('/v1/biometrics/connect/' + encodeURIComponent(provider));
          await loadProviders();
        } catch (err) {
          btn.disabled = false;
          btn.textContent = 'Disconnect';
          const el = document.getElementById('flash-error');
          el.innerHTML = '<strong>Disconnect failed</strong><span>' + UI.escape(err.message || 'unknown error') + '</span>';
          el.classList.remove('hidden');
        }
      });
    });
  }
})();
