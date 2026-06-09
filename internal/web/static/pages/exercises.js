// pages/exercises.js — library browser: load all exercises once, filter and
// render client-side. Filter chips for pattern and primary muscle are built
// from the data so adding a new pattern in the seed list doesn't require a
// UI change.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', async () => {
    await AUTH.ensureLoggedIn();
    const searchEl = document.getElementById('search');
    const patternEl = document.getElementById('filter-pattern');
    const muscleEl = document.getElementById('filter-muscle');
    const levelEl = document.getElementById('filter-level');
    const gridEl = document.getElementById('grid');
    const emptyEl = document.getElementById('empty');
    const countEl = document.getElementById('count');
    const errorEl = document.getElementById('error');

    let all = [];
    let filters = { search: '', pattern: '', muscle: '', level: '' };

    try {
      const data = await API.get('/v1/exercises');
      all = (data && data.exercises) || [];
    } catch (err) {
      UI.showError(errorEl, err);
      countEl.textContent = 'error';
      return;
    }

    // Build pattern + muscle filter chips from the data.
    buildChips(patternEl, uniqueSorted(all.map(e => e.pattern)));
    buildChips(muscleEl, uniqueSorted(all.map(e => e.primary_muscle)));

    // Wire chip + search interactions.
    [patternEl, muscleEl, levelEl].forEach(group => {
      group.addEventListener('click', (e) => {
        const chip = e.target.closest('.chip');
        if (!chip) return;
        group.querySelectorAll('.chip').forEach(c => c.classList.remove('active'));
        chip.classList.add('active');
        const key = group.id.replace('filter-', '');
        filters[key] = chip.dataset.value || '';
        render();
      });
    });
    searchEl.addEventListener('input', () => {
      filters.search = searchEl.value.trim().toLowerCase();
      render();
    });

    render();

    function render() {
      const filtered = all.filter(e => {
        if (filters.pattern && e.pattern !== filters.pattern) return false;
        if (filters.muscle && e.primary_muscle !== filters.muscle) return false;
        if (filters.level && e.min_level !== filters.level) return false;
        if (filters.search && !e.name.toLowerCase().includes(filters.search)) return false;
        return true;
      });
      countEl.textContent = filtered.length + ' / ' + all.length;
      if (filtered.length === 0) {
        gridEl.innerHTML = '';
        emptyEl.style.display = 'block';
        return;
      }
      emptyEl.style.display = 'none';
      gridEl.innerHTML = filtered.map(renderCard).join('');
    }
  });

  function renderCard(e) {
    const tags =
      (e.compound ? '<span class="x-tag compound">compound</span>' : '') +
      '<span class="x-tag">' + UI.escape(UI.formatTitleCase(e.pattern)) + '</span>' +
      '<span class="x-tag">' + UI.escape(UI.formatTitleCase(e.min_level)) + '</span>';

    const equipment = (e.required_equipment || []).map(UI.formatTitleCase).join(', ');
    const contras = (e.contraindications || []).map(UI.formatTitleCase).join(', ');

    return '<div class="exercise-card">' +
      '<div class="x-name">' + UI.escape(e.name) + '</div>' +
      '<div class="x-tags">' + tags + '</div>' +
      '<div class="x-meta">' +
        '<span><strong>' + UI.escape(UI.formatTitleCase(e.primary_muscle)) + '</strong></span>' +
        '<span>· ' + UI.escape(equipment) + '</span>' +
        (contras ? '<span>· avoid: ' + UI.escape(contras) + '</span>' : '') +
      '</div>' +
    '</div>';
  }

  function uniqueSorted(arr) {
    return Array.from(new Set(arr.filter(Boolean))).sort();
  }

  function buildChips(container, values) {
    const allChip = container.querySelector('.chip');
    const html = values.map(v =>
      '<button class="chip" data-value="' + UI.escape(v) + '">' +
        UI.escape(UI.formatTitleCase(v)) +
      '</button>'
    ).join('');
    container.insertAdjacentHTML('beforeend', html);
  }
})();
