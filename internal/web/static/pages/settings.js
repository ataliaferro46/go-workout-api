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
    wireBodyFat(user);
    wireGoalAuto();
    wireWorkoutTime(user);
    await loadGoalTargets();
  });

  function wireWorkoutTime(user) {
    const input = document.getElementById('workout-time-input');
    if (!input) return;
    if (user.workout_time) input.value = user.workout_time;
    document.getElementById('workout-time-save-btn').addEventListener('click', async () => {
      const btn = document.getElementById('workout-time-save-btn');
      btn.disabled = true; btn.textContent = 'Saving…';
      try {
        await API.fetch('/v1/auth/profile', {
          method: 'PATCH',
          body: JSON.stringify({ workout_time: input.value }),
        });
        btn.textContent = '✓ Saved';
        setTimeout(() => { btn.textContent = 'Save'; btn.disabled = false; }, 1200);
      } catch (err) {
        btn.disabled = false; btn.textContent = 'Save';
        UI.showError(document.getElementById('flash-error'), err);
      }
    });
  }

  function wireBodyFat(user) {
    const input = document.getElementById('bf-input');
    if (!input) return;
    if (user.body_fat_percentage) input.value = user.body_fat_percentage;
    renderBFFigures();
    document.querySelectorAll('.bf-tab').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.bf-tab').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        const sex = btn.dataset.bfSex;
        document.getElementById('bf-figures-male').classList.toggle('hidden', sex !== 'male');
        document.getElementById('bf-figures-female').classList.toggle('hidden', sex !== 'female');
      });
    });
    document.getElementById('bf-save-btn').addEventListener('click', async () => {
      const val = parseFloat(input.value);
      const body = { body_fat_percentage: isNaN(val) ? null : val };
      const btn = document.getElementById('bf-save-btn');
      btn.disabled = true; btn.textContent = 'Saving…';
      try {
        await API.fetch('/v1/auth/profile', { method: 'PATCH', body: JSON.stringify(body) });
        btn.textContent = '✓ Saved';
        setTimeout(() => { btn.textContent = 'Save'; btn.disabled = false; }, 1200);
      } catch (err) {
        btn.disabled = false; btn.textContent = 'Save';
        UI.showError(document.getElementById('flash-error'), err);
      }
    });
  }

  // ---- Body fat visual silhouettes ----
  //
  // Drawn as SVG paths. Body width at the waist increases with body fat;
  // the chest and shoulders narrow slightly in higher ranges as the
  // V-taper disappears. A small set of definition lines (abs, pec
  // separation) toggles on/off based on the BF range.

  function bfMaleSVG(bf) {
    // bf 5-35 → control widths
    const t = Math.max(5, Math.min(40, bf));
    const waistW = 16 + (t - 5) * 0.9;   // 16 (lean) → 47 (obese)
    const chestW = 32 - (t - 5) * 0.3;   // taper widens slightly w/ fat
    const showAbs = t < 13;
    const showOutline = t < 18;
    const showSoft = t > 22;
    return '<svg viewBox="0 0 80 140" xmlns="http://www.w3.org/2000/svg">' +
      // head
      '<ellipse cx="40" cy="14" rx="9" ry="11" fill="currentColor"/>' +
      // neck + shoulders + torso silhouette
      '<path d="' +
        'M40 25 ' +
        // left shoulder
        'C' + (40-chestW) + ' 30 ' + (40-chestW) + ' 30 ' + (40-chestW) + ' 36 ' +
        // taper to waist
        'L' + (40-waistW/2) + ' 70 ' +
        // hip
        'L' + (40-waistW/2-2) + ' 95 ' +
        // leg
        'L' + (40-7) + ' 135 L' + (40-1) + ' 135 L' + (40-1) + ' 95 ' +
        // crotch
        'L' + (40+1) + ' 95 L' + (40+1) + ' 135 L' + (40+7) + ' 135 ' +
        'L' + (40+waistW/2+2) + ' 95 ' +
        // right waist
        'L' + (40+waistW/2) + ' 70 ' +
        'C' + (40+chestW) + ' 36 ' + (40+chestW) + ' 30 ' + (40+chestW) + ' 30 ' +
        'Z' +
      '" fill="currentColor"/>' +
      // definition lines
      (showAbs ? '<line x1="40" y1="40" x2="40" y2="65" stroke="rgba(0,0,0,0.3)" stroke-width="0.8"/>' +
                '<line x1="34" y1="48" x2="46" y2="48" stroke="rgba(0,0,0,0.25)" stroke-width="0.6"/>' +
                '<line x1="34" y1="56" x2="46" y2="56" stroke="rgba(0,0,0,0.25)" stroke-width="0.6"/>' : '') +
      (showOutline && !showAbs ? '<line x1="40" y1="42" x2="40" y2="60" stroke="rgba(0,0,0,0.15)" stroke-width="0.5"/>' : '') +
      (showSoft ? '<ellipse cx="40" cy="62" rx="' + (waistW/2-1) + '" ry="6" fill="rgba(0,0,0,0.08)"/>' : '') +
    '</svg>';
  }

  function bfFemaleSVG(bf) {
    const t = Math.max(12, Math.min(45, bf));
    const waistW = 12 + (t - 12) * 0.9;
    const chestW = 30 - (t - 12) * 0.2;
    const hipW = waistW + 8;
    const showAbs = t < 18;
    const showSoft = t > 28;
    return '<svg viewBox="0 0 80 140" xmlns="http://www.w3.org/2000/svg">' +
      '<ellipse cx="40" cy="14" rx="9" ry="11" fill="currentColor"/>' +
      '<path d="' +
        'M40 25 ' +
        'C' + (40-chestW) + ' 30 ' + (40-chestW) + ' 32 ' + (40-chestW) + ' 38 ' +
        'L' + (40-waistW/2) + ' 65 ' +
        'L' + (40-hipW/2) + ' 92 ' +
        'L' + (40-7) + ' 135 L' + (40-1) + ' 135 L' + (40-1) + ' 95 ' +
        'L' + (40+1) + ' 95 L' + (40+1) + ' 135 L' + (40+7) + ' 135 ' +
        'L' + (40+hipW/2) + ' 92 ' +
        'L' + (40+waistW/2) + ' 65 ' +
        'C' + (40+chestW) + ' 38 ' + (40+chestW) + ' 32 ' + (40+chestW) + ' 30 ' +
        'Z' +
      '" fill="currentColor"/>' +
      (showAbs ? '<line x1="40" y1="44" x2="40" y2="62" stroke="rgba(0,0,0,0.25)" stroke-width="0.6"/>' : '') +
      (showSoft ? '<ellipse cx="40" cy="58" rx="' + (waistW/2-1) + '" ry="5" fill="rgba(0,0,0,0.08)"/>' : '') +
    '</svg>';
  }

  function renderBFFigures() {
    const maleLevels  = [{ bf: 8, label: '6-9%', desc: 'visible abs, dry' }, { bf: 12, label: '10-13%', desc: 'defined abs' }, { bf: 16, label: '14-17%', desc: 'faint abs' }, { bf: 20, label: '18-22%', desc: 'soft, no abs' }, { bf: 26, label: '23-28%', desc: 'noticeable belly' }, { bf: 32, label: '29-34%', desc: 'overweight' }];
    const femaleLevels = [{ bf: 15, label: '14-17%', desc: 'visible abs, athletic' }, { bf: 20, label: '18-22%', desc: 'lean, some definition' }, { bf: 25, label: '23-27%', desc: 'fit, smooth' }, { bf: 30, label: '28-32%', desc: 'average' }, { bf: 35, label: '33-37%', desc: 'soft' }, { bf: 40, label: '38%+', desc: 'overweight' }];
    function tile(svgFn, lvl) {
      return '<button class="bf-figure" data-bf-pick="' + lvl.bf + '">' +
        '<div class="bf-figure-svg">' + svgFn(lvl.bf) + '</div>' +
        '<div class="bf-figure-label">' + lvl.label + '</div>' +
        '<div class="bf-figure-desc">' + lvl.desc + '</div>' +
      '</button>';
    }
    document.getElementById('bf-figures-male').innerHTML = maleLevels.map(l => tile(bfMaleSVG, l)).join('');
    document.getElementById('bf-figures-female').innerHTML = femaleLevels.map(l => tile(bfFemaleSVG, l)).join('');
    document.querySelectorAll('[data-bf-pick]').forEach(btn => {
      btn.addEventListener('click', () => {
        document.getElementById('bf-input').value = btn.dataset.bfPick;
        document.querySelectorAll('[data-bf-pick]').forEach(b => b.classList.remove('picked'));
        btn.classList.add('picked');
      });
    });
  }

  async function loadGoalTargets() {
    try {
      const t = await API.get('/v1/nutrition/targets');
      if (t.goal) document.getElementById('goal-quick-input').value = t.goal;
      if (t.activity_level) document.getElementById('activity-quick-input').value = t.activity_level;
      renderGoalTargetsPreview(t);
    } catch (e) { /* skip */ }
  }

  function renderGoalTargetsPreview(t) {
    const el = document.getElementById('goal-targets-preview');
    if (!el) return;
    if (!t || !t.calories) { el.textContent = ''; return; }
    let line = 'Current target: ' + t.calories + ' kcal · ' + t.protein_g + 'g P · ' + t.carbs_g + 'g C · ' + t.fat_g + 'g F.';
    if (t.tdee) line += ' (BMR ' + t.bmr + ' / TDEE ' + t.tdee + ')';
    el.textContent = line;
  }

  function wireGoalAuto() {
    const btn = document.getElementById('goal-auto-btn');
    if (!btn) return;
    btn.addEventListener('click', async () => {
      const goal = document.getElementById('goal-quick-input').value;
      const activity = document.getElementById('activity-quick-input').value;
      btn.disabled = true; btn.textContent = 'Computing…';
      try {
        const t = await API.post('/v1/nutrition/targets/auto', { goal: goal, activity_level: activity });
        renderGoalTargetsPreview(t);
        btn.textContent = '✓ Saved'; setTimeout(() => { btn.textContent = 'Auto-compute targets'; btn.disabled = false; }, 1200);
      } catch (err) {
        btn.disabled = false; btn.textContent = 'Auto-compute targets';
        UI.showError(document.getElementById('flash-error'), err);
      }
    });
  }

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
