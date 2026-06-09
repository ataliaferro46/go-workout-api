// pages/history.js — calendar of workouts. Renders a month grid, marks
// days that contain workouts, and shows the day's logged sets inline
// when a day is clicked.

(function () {
  'use strict';

  let viewYear, viewMonth; // viewMonth: 0-11
  let workoutsByDate = {}; // 'YYYY-MM-DD' → [workout]
  let allWorkouts = [];

  document.addEventListener('DOMContentLoaded', async () => {
    await AUTH.ensureLoggedIn();
    const now = new Date();
    viewYear = now.getFullYear();
    viewMonth = now.getMonth();

    document.getElementById('cal-prev').addEventListener('click', () => navigate(-1));
    document.getElementById('cal-next').addEventListener('click', () => navigate(1));

    // View tabs (calendar vs by-target)
    document.querySelectorAll('.mode-tab[data-view]').forEach(t => {
      t.addEventListener('click', () => {
        document.querySelectorAll('.mode-tab[data-view]').forEach(x => x.classList.remove('active'));
        t.classList.add('active');
        const view = t.dataset.view;
        document.getElementById('calendar-view').style.display = view === 'calendar' ? '' : 'none';
        document.getElementById('by-target-view').style.display = view === 'by-target' ? '' : 'none';
        if (view === 'by-target') renderByTarget();
      });
    });

    try {
      const data = await API.get('/v1/workouts');
      allWorkouts = (data && data.workouts) || [];
      workoutsByDate = bucketByDate(allWorkouts);
      render();
    } catch (err) {
      UI.showError(document.getElementById('error'), err);
    }
  });

  function navigate(deltaMonths) {
    viewMonth += deltaMonths;
    if (viewMonth < 0) { viewMonth = 11; viewYear--; }
    if (viewMonth > 11) { viewMonth = 0; viewYear++; }
    render();
  }

  function render() {
    const title = new Date(viewYear, viewMonth, 1)
      .toLocaleString(undefined, { month: 'long', year: 'numeric' });
    document.getElementById('cal-title').textContent = title;
    const grid = document.getElementById('cal-grid');
    grid.innerHTML = buildGrid();
    grid.querySelectorAll('.cal-day[data-date]').forEach(cell => {
      cell.addEventListener('click', () => showDay(cell.dataset.date));
    });
  }

  function buildGrid() {
    const firstOfMonth = new Date(viewYear, viewMonth, 1);
    const startDOW = firstOfMonth.getDay(); // 0=Sun
    const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate();
    const todayISO = isoDate(new Date());

    let html = '';
    for (let i = 0; i < startDOW; i++) {
      html += '<div class="cal-day cal-blank"></div>';
    }
    for (let d = 1; d <= daysInMonth; d++) {
      const dateISO = isoDateParts(viewYear, viewMonth, d);
      const workouts = workoutsByDate[dateISO] || [];
      const cls = ['cal-day'];
      if (dateISO === todayISO) cls.push('cal-today');
      if (workouts.length > 0) cls.push('cal-filled');
      const dot = workouts.length > 0
        ? '<span class="cal-dot" title="' + workouts.length + ' workout' + (workouts.length === 1 ? '' : 's') + '">●</span>'
        : '';
      html += '<div class="' + cls.join(' ') + '" data-date="' + dateISO + '">' +
        '<span class="cal-day-num">' + d + '</span>' + dot +
      '</div>';
    }
    return html;
  }

  async function showDay(dateISO) {
    const detail = document.getElementById('day-detail');
    const title = document.getElementById('day-detail-title');
    const body = document.getElementById('day-detail-body');
    const workouts = workoutsByDate[dateISO] || [];
    const dateLabel = new Date(dateISO + 'T12:00:00')
      .toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });
    title.textContent = dateLabel;
    detail.classList.remove('hidden');
    detail.scrollIntoView({ behavior: 'smooth', block: 'start' });

    if (workouts.length === 0) {
      body.innerHTML = '<p class="day-empty">No workouts logged on this day.</p>';
      return;
    }
    body.innerHTML = '<p class="day-loading">Loading…</p>';

    // Fetch each workout's full state so we get logged sets.
    try {
      const details = await Promise.all(workouts.map(w =>
        API.get('/v1/workouts/' + encodeURIComponent(w.id))));
      body.innerHTML = details.map(renderWorkout).join('');
    } catch (err) {
      body.innerHTML = '<p class="day-empty">Could not load workout details: ' + UI.escape(err.message || 'error') + '</p>';
    }
  }

  function renderWorkout(w) {
    const exercises = (w.exercises || []).map(ex => {
      const logged = ex.logged_sets || [];
      let setsHTML;
      if (logged.length === 0) {
        const planned = ex.sets > 0 ? ex.sets : 0;
        setsHTML = '<div class="dx-empty">' +
          (planned > 0 ? planned + ' sets prescribed, none logged' : 'No sets logged') +
        '</div>';
      } else {
        setsHTML = '<div class="dx-sets">' +
          logged.map(s => {
            // Bodyweight movements log with weight_kg = 0; show "X reps"
            // instead of "X × 0 kg" so the chip reads cleanly.
            const valStr = s.weight_kg > 0
              ? s.reps + ' × ' + s.weight_kg + ' kg'
              : s.reps + ' reps';
            return '<div class="dx-set">' +
              '<span class="dx-set-num">Set ' + s.set_number + '</span>' +
              '<span class="dx-set-vals">' + valStr + '</span>' +
            '</div>';
          }).join('') +
        '</div>';
      }
      return '<div class="dx-card">' +
        '<div class="dx-name">' + UI.escape(ex.name) + '</div>' +
        setsHTML +
      '</div>';
    }).join('');
    return '<div class="dw-card">' +
      '<div class="dw-head">' +
        '<a class="dw-name" href="/workout/' + encodeURIComponent(w.id) + '">' + UI.escape(w.name) + ' →</a>' +
        '<span class="dw-time">' + UI.escape(timeOf(w.created_at)) + '</span>' +
      '</div>' +
      exercises +
    '</div>';
  }

  function categorizeWorkout(w) {
    if ((w.type || 'strength') === 'cardio') return 'Cardio';
    const n = (w.name || '').toLowerCase();
    if (n.includes('push')) return 'Push';
    if (n.includes('pull')) return 'Pull';
    if (n.includes('legs') || n.includes('lower')) return 'Legs';
    if (n.includes('upper')) return 'Upper';
    if (n.includes('full body')) return 'Full Body';
    return 'Other / Custom';
  }

  function renderByTarget() {
    const body = document.getElementById('by-target-body');
    if (allWorkouts.length === 0) {
      body.innerHTML = '<p class="day-empty">No workouts logged yet.</p>';
      return;
    }
    const groups = {};
    allWorkouts.forEach(w => {
      const cat = categorizeWorkout(w);
      if (!groups[cat]) groups[cat] = [];
      groups[cat].push(w);
    });
    const order = ['Push', 'Pull', 'Legs', 'Upper', 'Full Body', 'Cardio', 'Other / Custom'];
    body.innerHTML = order.filter(k => groups[k] && groups[k].length).map(k =>
      '<div class="target-group">' +
        '<div class="target-head">' +
          '<h3>' + UI.escape(k) + '</h3>' +
          '<span class="target-count">' + groups[k].length + ' session' + (groups[k].length === 1 ? '' : 's') + '</span>' +
        '</div>' +
        '<div class="target-list">' +
          groups[k].map(w =>
            '<div class="target-row">' +
              '<a class="target-link" href="/workout/' + encodeURIComponent(w.id) + '">' +
                '<span class="target-name">' + UI.escape(w.name || 'Workout') + '</span>' +
                '<span class="target-when">' + UI.escape(UI.formatDate(w.created_at)) + '</span>' +
              '</a>' +
              '<button class="repeat-btn" data-id="' + UI.escape(w.id) + '">↻ repeat</button>' +
            '</div>'
          ).join('') +
        '</div>' +
      '</div>'
    ).join('');
    body.querySelectorAll('.repeat-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        const id = btn.dataset.id;
        btn.disabled = true;
        btn.textContent = 'cloning…';
        try {
          const w = await API.post('/v1/workouts/' + encodeURIComponent(id) + '/repeat', {});
          window.location.href = '/workout/' + encodeURIComponent(w.id);
        } catch (err) {
          btn.disabled = false;
          btn.textContent = '↻ repeat';
          UI.showError(document.getElementById('error'), err);
        }
      });
    });
  }

  function bucketByDate(workouts) {
    const out = {};
    workouts.forEach(w => {
      const d = isoDate(new Date(w.created_at));
      if (!out[d]) out[d] = [];
      out[d].push(w);
    });
    return out;
  }

  function isoDate(d) {
    return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate());
  }
  function isoDateParts(y, m, d) {
    return y + '-' + pad2(m + 1) + '-' + pad2(d);
  }
  function pad2(n) { return n < 10 ? '0' + n : '' + n; }
  function timeOf(iso) {
    try {
      return new Date(iso).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
    } catch (e) { return ''; }
  }
})();
