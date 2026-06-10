// pages/nutrition.js — daily food log + macro tracking against
// auto-computed (or manually set) targets.

(function () {
  'use strict';

  let dailyData = null;
  let selectedFood = null;

  document.addEventListener('DOMContentLoaded', async () => {
    await AUTH.ensureLoggedIn();
    document.getElementById('page-date').textContent =
      new Date().toLocaleDateString(undefined, {
        weekday: 'long', month: 'long', day: 'numeric',
      });
    await loadDaily();
    wireModals();
  });

  async function loadDaily() {
    try {
      dailyData = await API.get('/v1/nutrition/daily');
      renderTargets(dailyData.targets);
      renderRings(dailyData.totals, dailyData.targets, dailyData.calories_burned, dailyData.net_calories);
      renderMeals(dailyData.logs);
    } catch (err) {
      UI.showError(document.getElementById('error'), err);
    }
  }

  function renderTargets(t) {
    const body = document.getElementById('targets-body');
    if (!t || !t.calories) {
      body.innerHTML = '<p class="muted">No targets set yet. Click <strong>Edit</strong> to compute from your profile.</p>';
      return;
    }
    body.innerHTML = '<div class="target-pills">' +
      pill(t.calories + ' kcal') +
      pill('P ' + t.protein_g + 'g') +
      pill('C ' + t.carbs_g + 'g') +
      pill('F ' + t.fat_g + 'g') +
      (t.goal ? pill(t.goal) : '') +
      (t.tdee ? pill('TDEE ' + t.tdee) : '') +
    '</div>';
  }

  function pill(text) {
    return '<span class="target-pill">' + UI.escape(text) + '</span>';
  }

  function renderRings(totals, targets, burned, net) {
    const tCal = targets.calories || 2000;
    const tP = targets.protein_g || 150;
    const tC = targets.carbs_g || 220;
    const tF = targets.fat_g || 65;
    document.getElementById('rings-body').innerHTML =
      ring('Calories', totals.calories, tCal, 'kcal') +
      ring('Protein', Math.round(totals.protein_g), tP, 'g') +
      ring('Carbs',   Math.round(totals.carbs_g),   tC, 'g') +
      ring('Fat',     Math.round(totals.fat_g),     tF, 'g') +
      renderEnergyBalance();
  }

  function renderEnergyBalance() {
    if (!dailyData) return '';
    const consumed = dailyData.totals.calories;
    const cardio = dailyData.calories_burned || 0;
    const ouraBurn = dailyData.daily_burned_oura || 0;
    const surplus = dailyData.surplus || 0;
    const src = dailyData.burn_source || 'cardio';

    const labelMap = {
      oura:   'Oura TDEE',
      target: 'Target TDEE',
      cardio: 'Cardio only',
    };
    const balanceLabel = surplus >= 0 ? 'Surplus' : 'Deficit';
    const balanceCls = surplus >= 0 ? 'bal-surplus' : 'bal-deficit';
    const balanceVal = Math.abs(surplus);

    let burnLine = '';
    if (ouraBurn > 0) {
      burnLine = '<div><span class="net-label">Total burn:</span> ' + ouraBurn + ' kcal <span class="net-src">(Oura)</span></div>';
    } else if (src === 'target' && dailyData.targets.tdee) {
      burnLine = '<div><span class="net-label">Est. TDEE:</span> ' + (dailyData.targets.tdee + cardio) + ' kcal <span class="net-src">(profile)</span></div>';
    }
    return '<div class="net-row energy-row">' +
      '<div><span class="net-label">Eaten:</span> ' + consumed + ' kcal</div>' +
      (cardio > 0 ? '<div><span class="net-label">Cardio:</span> ' + cardio + ' kcal</div>' : '') +
      burnLine +
      '<div class="balance-pill ' + balanceCls + '">' +
        balanceLabel + ' ' + balanceVal + ' kcal' +
        '<span class="net-src">via ' + labelMap[src] + '</span>' +
      '</div>' +
    '</div>';
  }

  function ring(label, val, target, unit) {
    const pct = target > 0 ? Math.min(100, (val / target) * 100) : 0;
    const remaining = Math.max(0, target - val);
    return '<div class="ring-card">' +
      '<div class="ring-label">' + UI.escape(label) + '</div>' +
      '<div class="ring-bar"><div class="ring-fill" style="width:' + pct + '%"></div></div>' +
      '<div class="ring-vals">' + val + ' / ' + target + ' ' + UI.escape(unit) + '</div>' +
      '<div class="ring-rem">' + remaining + ' ' + UI.escape(unit) + ' left</div>' +
    '</div>';
  }

  function renderMeals(logs) {
    const body = document.getElementById('meals-body');
    if (logs.length === 0) {
      body.innerHTML = '<p class="muted">No food logged yet today.</p>';
      return;
    }
    const groups = { breakfast: [], lunch: [], dinner: [], snack: [] };
    logs.forEach(l => { if (groups[l.meal_type]) groups[l.meal_type].push(l); });
    body.innerHTML = ['breakfast', 'lunch', 'dinner', 'snack']
      .filter(k => groups[k].length > 0)
      .map(k => renderMealGroup(k, groups[k]))
      .join('');
    body.querySelectorAll('.log-delete').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm('Remove this entry?')) return;
        try {
          await API.delete('/v1/nutrition/log/' + encodeURIComponent(btn.dataset.id));
          await loadDaily();
        } catch (err) { UI.showError(document.getElementById('error'), err); }
      });
    });
  }

  function renderMealGroup(meal, logs) {
    const cal = logs.reduce((s, l) => s + Math.round(l.food.calories * l.servings), 0);
    return '<div class="meal-group">' +
      '<div class="meal-group-head">' +
        '<h4>' + meal[0].toUpperCase() + meal.slice(1) + '</h4>' +
        '<span class="meal-cal">' + cal + ' kcal</span>' +
      '</div>' +
      logs.map(l => {
        const f = l.food;
        const cal = Math.round(f.calories * l.servings);
        const p = (f.protein_g * l.servings).toFixed(1);
        const c = (f.carbs_g * l.servings).toFixed(1);
        const fg = (f.fat_g * l.servings).toFixed(1);
        return '<div class="log-row">' +
          '<div class="log-main">' +
            '<div class="log-name">' + UI.escape(f.name) + '</div>' +
            '<div class="log-sub">' + l.servings + ' × ' + f.serving_size + UI.escape(f.serving_unit) + ' · ' +
              cal + ' kcal · P' + p + ' C' + c + ' F' + fg + '</div>' +
          '</div>' +
          '<button class="log-delete" data-id="' + UI.escape(l.id) + '" title="Remove">×</button>' +
        '</div>';
      }).join('') +
    '</div>';
  }

  // ---- Modals ----

  function wireModals() {
    document.getElementById('add-food-btn').addEventListener('click', () => openAdd());
    document.getElementById('add-close').addEventListener('click', () => closeModal('add-modal'));
    document.getElementById('targets-edit-btn').addEventListener('click', () => openTargets());
    document.getElementById('targets-close').addEventListener('click', () => closeModal('targets-modal'));

    const search = document.getElementById('search-input');
    let searchTimeout = null;
    search.addEventListener('input', () => {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(doSearch, 200);
    });

    document.getElementById('add-confirm').addEventListener('click', confirmAdd);
    document.getElementById('auto-compute-btn').addEventListener('click', autoCompute);
    document.getElementById('save-manual-btn').addEventListener('click', saveManual);
    document.getElementById('suggest-meal-plan-btn').addEventListener('click', suggestMealPlan);
    document.getElementById('meal-plan-close').addEventListener('click', () => {
      document.getElementById('meal-plan-card').classList.add('hidden');
    });
    document.getElementById('meal-plan-save-btn').addEventListener('click', saveCurrentPlan);

    document.getElementById('my-plans-btn').addEventListener('click', openPlansList);
    document.getElementById('plans-close').addEventListener('click', () => closeModal('plans-modal'));

    document.getElementById('recipes-btn').addEventListener('click', openRecipes);
    document.getElementById('recipes-close').addEventListener('click', () => closeModal('recipes-modal'));
    document.getElementById('recipe-save-btn').addEventListener('click', saveRecipe);
    wireRecipeSearch();
  }

  // ---- Save current plan ----

  async function saveCurrentPlan() {
    if (!currentPlan) return;
    const name = prompt('Name this plan:', currentPlanMode === 'week'
      ? '7-day plan ' + new Date().toLocaleDateString()
      : 'Today ' + new Date().toLocaleDateString());
    if (!name) return;
    const items = [];
    if (currentPlanMode === 'week') {
      (currentPlan.days || []).forEach((d, di) => {
        d.meals.forEach(m => {
          (m.foods || []).forEach((fs, si) => {
            items.push({
              day_idx: di + 1, meal_type: m.meal,
              sort_order: si, food_id: fs.food_id, servings: fs.servings,
            });
          });
        });
      });
    } else {
      (currentPlan.meal_plan || []).forEach(m => {
        (m.foods || []).forEach((fs, si) => {
          items.push({
            day_idx: 1, meal_type: m.meal,
            sort_order: si, food_id: fs.food_id, servings: fs.servings,
          });
        });
      });
    }
    try {
      await API.post('/v1/nutrition/meal-plans', {
        name: name, days: currentPlanMode === 'week' ? 7 : 1,
        items: items, targets: dailyData ? dailyData.targets : {},
      });
      const btn = document.getElementById('meal-plan-save-btn');
      btn.textContent = '✓ Saved'; btn.disabled = true;
      setTimeout(() => { btn.textContent = '💾 Save plan'; btn.disabled = false; }, 1200);
    } catch (err) { UI.showError(document.getElementById('error'), err); }
  }

  // ---- My plans modal ----

  async function openPlansList() {
    document.getElementById('plans-modal').classList.remove('hidden');
    const body = document.getElementById('plans-list');
    body.innerHTML = '<p class="muted">Loading…</p>';
    try {
      const data = await API.get('/v1/nutrition/meal-plans');
      const plans = data.plans || [];
      if (plans.length === 0) {
        body.innerHTML = '<p class="muted">No saved plans yet. Generate one and click "Save plan".</p>';
        return;
      }
      body.innerHTML = plans.map(p =>
        '<div class="plan-row-saved">' +
          '<div class="log-main">' +
            '<div class="log-name">' + UI.escape(p.name) + '</div>' +
            '<div class="log-sub">' + p.days + ' day' + (p.days === 1 ? '' : 's') +
              ' · saved ' + UI.escape(UI.formatDate(p.created_at)) + '</div>' +
          '</div>' +
          '<button class="btn btn-ghost" data-open-plan="' + UI.escape(p.id) + '">Open</button>' +
          '<button class="btn btn-ghost danger" data-del-plan="' + UI.escape(p.id) + '">×</button>' +
        '</div>'
      ).join('');
      body.querySelectorAll('[data-open-plan]').forEach(b => {
        b.addEventListener('click', () => openSavedPlan(b.dataset.openPlan));
      });
      body.querySelectorAll('[data-del-plan]').forEach(b => {
        b.addEventListener('click', async () => {
          if (!confirm('Delete this plan?')) return;
          try {
            await API.delete('/v1/nutrition/meal-plans/' + encodeURIComponent(b.dataset.delPlan));
            openPlansList();
          } catch (err) { UI.showError(document.getElementById('error'), err); }
        });
      });
    } catch (err) {
      body.innerHTML = '<p class="muted">Error loading plans.</p>';
    }
  }

  async function openSavedPlan(id) {
    try {
      const plan = await API.get('/v1/nutrition/meal-plans/' + encodeURIComponent(id));
      // Group items by day / meal to render in the suggested-meals view.
      const byDay = {};
      (plan.items || []).forEach(it => {
        const k = it.day_idx;
        if (!byDay[k]) byDay[k] = {};
        if (!byDay[k][it.meal_type]) byDay[k][it.meal_type] = { meal: it.meal_type, target_calories: 0, foods: [], totals: { calories:0, protein_g:0, carbs_g:0, fat_g:0 } };
        const m = byDay[k][it.meal_type];
        m.foods.push({ food_id: it.food_id, food: it.food, servings: it.servings });
        if (it.food) {
          m.totals.calories += Math.round(it.food.calories * it.servings);
          m.totals.protein_g += it.food.protein_g * it.servings;
          m.totals.carbs_g += it.food.carbs_g * it.servings;
          m.totals.fat_g += it.food.fat_g * it.servings;
        }
      });
      // Convert byDay → meal_plan or days shape
      if (plan.days <= 1) {
        const meals = Object.values(byDay[1] || {});
        currentPlan = { meal_plan: meals };
        currentPlanMode = 'today';
      } else {
        const days = Object.keys(byDay).sort((a,b) => parseInt(a)-parseInt(b)).map(k =>
          ({ date: '', meals: Object.values(byDay[k]) }));
        currentPlan = { days: days };
        currentPlanMode = 'week';
      }
      closeModal('plans-modal');
      document.getElementById('meal-plan-card').classList.remove('hidden');
      document.getElementById('meal-plan-title').textContent = plan.name;
      if (currentPlanMode === 'today') renderTodayPlan(currentPlan);
      else renderWeekPlan(currentPlan);
      attachLogHandlers();
    } catch (err) { UI.showError(document.getElementById('error'), err); }
  }

  // ---- Recipes ----

  let recipeIngredients = [];

  async function openRecipes() {
    document.getElementById('recipes-modal').classList.remove('hidden');
    recipeIngredients = [];
    document.getElementById('recipe-name').value = '';
    document.getElementById('recipe-servings').value = '1';
    document.getElementById('recipe-search').value = '';
    document.getElementById('recipe-search-results').innerHTML = '';
    refreshRecipeIngList();
    await loadRecipesList();
  }

  async function loadRecipesList() {
    try {
      const data = await API.get('/v1/nutrition/recipes');
      const recs = data.recipes || [];
      const el = document.getElementById('recipes-list');
      if (recs.length === 0) { el.innerHTML = '<p class="muted">No recipes yet.</p>'; return; }
      el.innerHTML = '<h4 style="margin: 0 0 8px;">Your recipes</h4>' + recs.map(r =>
        '<div class="plan-row-saved">' +
          '<div class="log-main">' +
            '<div class="log-name">' + UI.escape(r.name) + '</div>' +
            '<div class="log-sub">' + r.servings_per_recipe + ' serving' + (r.servings_per_recipe === 1 ? '' : 's') + '</div>' +
          '</div>' +
          '<button class="btn btn-ghost danger" data-del-rec="' + UI.escape(r.id) + '">×</button>' +
        '</div>'
      ).join('');
      el.querySelectorAll('[data-del-rec]').forEach(b => {
        b.addEventListener('click', async () => {
          if (!confirm('Delete this recipe?')) return;
          try {
            await API.delete('/v1/nutrition/recipes/' + encodeURIComponent(b.dataset.delRec));
            loadRecipesList();
          } catch (err) { UI.showError(document.getElementById('error'), err); }
        });
      });
    } catch (e) { /* skip */ }
  }

  function wireRecipeSearch() {
    const input = document.getElementById('recipe-search');
    if (!input) return;
    let timer;
    input.addEventListener('input', () => {
      clearTimeout(timer);
      timer = setTimeout(async () => {
        const q = input.value.trim();
        const results = document.getElementById('recipe-search-results');
        if (q.length < 2) { results.innerHTML = ''; return; }
        try {
          const data = await API.get('/v1/foods/search?q=' + encodeURIComponent(q));
          const foods = data.foods || [];
          results.innerHTML = foods.slice(0, 6).map(f =>
            '<button class="food-result" data-add-ing="' + UI.escape(f.id) + '">' +
              '<div class="food-name">' + UI.escape(f.name) + '</div>' +
              '<div class="food-macros">' + f.calories + ' kcal · ' + f.serving_size + UI.escape(f.serving_unit) + '</div>' +
            '</button>').join('');
          results.querySelectorAll('[data-add-ing]').forEach(b => {
            b.addEventListener('click', () => {
              const f = foods.find(x => x.id === b.dataset.addIng);
              recipeIngredients.push({ food_id: f.id, food: f, servings: 1, sort_order: recipeIngredients.length });
              input.value = '';
              results.innerHTML = '';
              refreshRecipeIngList();
            });
          });
        } catch (e) { /* skip */ }
      }, 200);
    });
  }

  function refreshRecipeIngList() {
    const el = document.getElementById('recipe-ingredients');
    document.getElementById('recipe-save-btn').disabled = recipeIngredients.length === 0;
    el.innerHTML = recipeIngredients.length === 0
      ? '<p class="muted" style="margin: 8px 0;">No ingredients yet.</p>'
      : recipeIngredients.map((ing, i) =>
          '<div class="ing-row">' +
            '<span class="ing-name">' + UI.escape(ing.food.name) + '</span>' +
            '<input type="number" min="0.1" step="0.1" value="' + ing.servings + '" data-ing-servings="' + i + '" class="input ing-servings">' +
            '<span class="ing-unit">×</span>' +
            '<button class="btn btn-ghost danger" data-ing-remove="' + i + '">×</button>' +
          '</div>').join('');
    el.querySelectorAll('[data-ing-servings]').forEach(inp => {
      inp.addEventListener('input', () => {
        const i = parseInt(inp.dataset.ingServings, 10);
        recipeIngredients[i].servings = parseFloat(inp.value) || 0;
      });
    });
    el.querySelectorAll('[data-ing-remove]').forEach(b => {
      b.addEventListener('click', () => {
        recipeIngredients.splice(parseInt(b.dataset.ingRemove, 10), 1);
        refreshRecipeIngList();
      });
    });
  }

  async function saveRecipe() {
    const name = document.getElementById('recipe-name').value.trim();
    const sps = parseFloat(document.getElementById('recipe-servings').value) || 1;
    if (!name || recipeIngredients.length === 0) return;
    const btn = document.getElementById('recipe-save-btn');
    btn.disabled = true; btn.textContent = 'Saving…';
    try {
      await API.post('/v1/nutrition/recipes', {
        name: name, servings_per_recipe: sps,
        ingredients: recipeIngredients.map(i => ({ food_id: i.food_id, servings: i.servings, sort_order: i.sort_order })),
      });
      btn.textContent = '✓ Saved'; recipeIngredients = [];
      document.getElementById('recipe-name').value = '';
      document.getElementById('recipe-servings').value = '1';
      refreshRecipeIngList();
      loadRecipesList();
      setTimeout(() => { btn.textContent = 'Save recipe'; btn.disabled = false; }, 1000);
    } catch (err) {
      btn.disabled = false; btn.textContent = 'Save recipe';
      UI.showError(document.getElementById('error'), err);
    }
  }

  let currentPlanMode = 'today';
  let currentPlan = null; // either { meal_plan: [...] } or { days: [...] }

  async function suggestMealPlan() {
    const card = document.getElementById('meal-plan-card');
    card.classList.remove('hidden');
    card.scrollIntoView({ behavior: 'smooth', block: 'start' });

    // Wire plan-mode toggle once
    document.querySelectorAll('[data-plan-mode]').forEach(b => {
      b.addEventListener('click', () => {
        document.querySelectorAll('[data-plan-mode]').forEach(x => x.classList.remove('active'));
        b.classList.add('active');
        currentPlanMode = b.dataset.planMode;
        document.getElementById('meal-plan-title').textContent =
          currentPlanMode === 'today' ? 'Suggested meals for today' : 'Suggested 7-day meal plan';
        runPlan();
      });
    });
    await runPlan();
  }

  async function runPlan() {
    const body = document.getElementById('meal-plan-body');
    body.innerHTML = '<p class="muted">Generating…</p>';
    try {
      if (currentPlanMode === 'week') {
        currentPlan = await API.post('/v1/nutrition/meal-plan/multi-day', { days: 7 });
        renderWeekPlan(currentPlan);
      } else {
        currentPlan = await API.post('/v1/nutrition/meal-plan', {});
        renderTodayPlan(currentPlan);
      }
      attachLogHandlers();
    } catch (err) {
      body.innerHTML = '<p class="muted">' + UI.escape(err.message || 'error') + '</p>';
    }
  }

  function renderTodayPlan(data) {
    const plan = data.meal_plan || [];
    const body = document.getElementById('meal-plan-body');
    if (plan.length === 0) {
      body.innerHTML = '<p class="muted">No suggestions could be generated. Make sure your targets are set.</p>';
      return;
    }
    body.innerHTML = plan.map(slot => renderMealSuggestion(slot, null)).join('');
  }

  function renderWeekPlan(data) {
    const days = data.days || [];
    const body = document.getElementById('meal-plan-body');
    if (days.length === 0) {
      body.innerHTML = '<p class="muted">Could not build a multi-day plan.</p>';
      return;
    }
    body.innerHTML = days.map((d, i) => {
      const label = new Date(d.date + 'T12:00:00').toLocaleDateString(undefined,
        { weekday: 'short', month: 'short', day: 'numeric' });
      return '<div class="day-section">' +
        '<h4 class="day-section-head">' + UI.escape(label) + '</h4>' +
        d.meals.map(m => renderMealSuggestion(m, d.date)).join('') +
      '</div>';
    }).join('');
  }

  function attachLogHandlers() {
    document.querySelectorAll('[data-log-food]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const foodID = btn.dataset.logFood;
        const servings = parseFloat(btn.dataset.servings);
        const meal = btn.dataset.meal;
        btn.disabled = true;
        btn.textContent = 'Logging…';
        try {
          await API.post('/v1/nutrition/log', {
            food_id: foodID, servings: servings, meal_type: meal,
          });
          btn.textContent = '✓ logged';
          await loadDaily();
        } catch (err) {
          btn.disabled = false;
          btn.textContent = '+ log';
          UI.showError(document.getElementById('error'), err);
        }
      });
    });
    const logAll = document.getElementById('meal-plan-log-all-btn');
    if (logAll) logAll.onclick = logAllSuggested;
  }

  async function logAllSuggested() {
    if (!currentPlan) return;
    // Only "today" mode supports batch-log to today; week mode would
    // need date-stamped logs which the current API doesn't accept.
    if (currentPlanMode !== 'today') {
      alert('Switch to "Today" tab to log all suggestions at once.');
      return;
    }
    const plan = currentPlan.meal_plan || [];
    const logs = [];
    plan.forEach(slot => {
      (slot.foods || []).forEach(fs => {
        logs.push({
          food_id: fs.food_id, servings: fs.servings, meal_type: slot.meal,
        });
      });
    });
    if (logs.length === 0) return;
    const btn = document.getElementById('meal-plan-log-all-btn');
    btn.disabled = true; btn.textContent = 'Logging…';
    try {
      const res = await API.post('/v1/nutrition/log/batch', { logs: logs });
      btn.textContent = '✓ Logged ' + res.inserted;
      await loadDaily();
      setTimeout(() => {
        document.getElementById('meal-plan-card').classList.add('hidden');
      }, 800);
    } catch (err) {
      btn.disabled = false; btn.textContent = 'Log all';
      UI.showError(document.getElementById('error'), err);
    }
  }

  // mealAdvisory returns a small contextual note for a meal slot based
  // on the user's stored workout time. The closest meal-before-workout
  // gets a carb-forward, low-fat advisory; the meal after gets a high-
  // protein, moderate-carb advisory.
  function mealAdvisory(mealName) {
    const user = (AUTH.currentUser && AUTH.currentUser()) || {};
    const wt = user.workout_time;
    if (!wt) return '';
    const [hStr, mStr] = wt.split(':');
    const wHour = parseInt(hStr, 10);
    const wMin = parseInt(mStr, 10) || 0;
    const wAt = wHour + wMin / 60.0;
    // Conventional meal times — could later become user-settable.
    const slotHours = { breakfast: 8, lunch: 12.5, dinner: 19, snack: 15.5 };
    const mealAt = slotHours[mealName];
    if (mealAt == null) return '';
    const diff = wAt - mealAt; // hours from meal to workout
    if (diff > 0.5 && diff <= 3) {
      // Meal is 0.5–3 hours before workout → pre-workout
      return '<div class="adv adv-pre"><strong>Pre-workout</strong> · 30-50g carbs, 15-25g protein, keep fat low. Workout in ~' + Math.round(diff * 10) / 10 + ' hr</div>';
    }
    if (diff > -2.5 && diff < -0.5) {
      // Meal is 0.5–2.5 hours after workout → post-workout
      return '<div class="adv adv-post"><strong>Post-workout</strong> · 30-40g protein + 50-80g carbs to refuel + recover</div>';
    }
    return '';
  }

  function renderMealSuggestion(slot, date) {
    const totalCal = slot.totals.calories;
    return '<div class="meal-suggestion">' +
      '<div class="meal-sg-head">' +
        '<h4>' + slot.meal[0].toUpperCase() + slot.meal.slice(1) + '</h4>' +
        '<span class="meal-cal">' + totalCal + ' / ' + slot.target_calories + ' kcal</span>' +
      '</div>' +
      mealAdvisory(slot.meal) +
      slot.foods.map(fs =>
        '<div class="meal-sg-row">' +
          '<div class="log-main">' +
            '<div class="log-name">' + UI.escape(fs.food.name) + '</div>' +
            '<div class="log-sub">' + fs.servings + ' × ' + fs.food.serving_size + UI.escape(fs.food.serving_unit) + ' · ' +
              Math.round(fs.food.calories * fs.servings) + ' kcal' + '</div>' +
          '</div>' +
          '<button class="btn btn-ghost" data-log-food="' + UI.escape(fs.food_id) + '" data-servings="' + fs.servings + '" data-meal="' + UI.escape(slot.meal) + '">+ log</button>' +
        '</div>'
      ).join('') +
    '</div>';
  }

  let scanner = null;
  let scanReader = null;

  function openAdd() {
    document.getElementById('add-modal').classList.remove('hidden');
    switchAddMode('search');
    document.getElementById('search-results').innerHTML = '';
    document.getElementById('serving-row').classList.add('hidden');
    selectedFood = null;

    document.querySelectorAll('.add-tab').forEach(b => {
      b.addEventListener('click', () => switchAddMode(b.dataset.addMode), { once: false });
    });
  }

  function switchAddMode(mode) {
    document.querySelectorAll('.add-tab').forEach(b => {
      b.classList.toggle('active', b.dataset.addMode === mode);
    });
    document.getElementById('add-search-pane').classList.toggle('hidden', mode !== 'search');
    document.getElementById('add-scan-pane').classList.toggle('hidden', mode !== 'scan');
    if (mode === 'scan') {
      startBarcodeScan();
    } else {
      stopBarcodeScan();
      document.getElementById('search-input').focus();
    }
  }

  async function startBarcodeScan() {
    const status = document.getElementById('scan-status');
    if (typeof ZXing === 'undefined') {
      status.textContent = 'Scanner library failed to load.';
      return;
    }
    try {
      scanReader = new ZXing.BrowserMultiFormatReader();
      const video = document.getElementById('scanner-video');
      status.textContent = 'Starting camera…';
      scanner = await scanReader.decodeFromVideoDevice(undefined, video, async (result, err) => {
        if (result) {
          const barcode = result.getText();
          status.textContent = 'Looking up ' + barcode + '…';
          stopBarcodeScan();
          try {
            const data = await API.post('/v1/foods/barcode', { barcode: barcode });
            const f = data.food;
            // Select it as if the user searched + clicked it
            selectedFood = f;
            switchAddMode('search');
            document.getElementById('search-input').value = f.name;
            document.getElementById('search-results').innerHTML =
              '<button class="food-result selected">' +
                '<div class="food-name">' + UI.escape(f.name) + '</div>' +
                '<div class="food-macros">' + f.calories + ' kcal · ' + f.serving_size + UI.escape(f.serving_unit) +
                  ' · P' + f.protein_g + ' C' + f.carbs_g + ' F' + f.fat_g + '</div>' +
              '</button>';
            document.getElementById('serving-row').classList.remove('hidden');
          } catch (e) {
            UI.showError(document.getElementById('error'),
              { code: 'barcode_failed', message: 'Product not found in Open Food Facts.' });
          }
        }
      });
      status.textContent = 'Point camera at a barcode.';
    } catch (e) {
      status.textContent = 'Camera access denied. Grant permission in browser settings.';
    }
  }

  function stopBarcodeScan() {
    if (scanReader) {
      try { scanReader.reset(); } catch (e) {}
      scanReader = null;
    }
  }

  function openTargets() {
    document.getElementById('targets-modal').classList.remove('hidden');
    const t = (dailyData && dailyData.targets) || {};
    if (t.goal) document.getElementById('goal-input').value = t.goal;
    if (t.activity_level) document.getElementById('activity-input').value = t.activity_level;
    document.getElementById('manual-cal').value = t.calories || '';
    document.getElementById('manual-p').value = t.protein_g || '';
    document.getElementById('manual-c').value = t.carbs_g || '';
    document.getElementById('manual-f').value = t.fat_g || '';
  }

  function closeModal(id) { document.getElementById(id).classList.add('hidden'); }

  async function doSearch() {
    const q = document.getElementById('search-input').value.trim();
    if (q.length < 2) {
      document.getElementById('search-results').innerHTML = '';
      return;
    }
    try {
      const data = await API.get('/v1/foods/search?q=' + encodeURIComponent(q));
      const list = data.foods || [];
      const el = document.getElementById('search-results');
      if (list.length === 0) {
        el.innerHTML = '<p class="muted">No matches.</p>';
        return;
      }
      el.innerHTML = list.map(f =>
        '<button class="food-result" data-id="' + UI.escape(f.id) + '">' +
          '<div class="food-name">' + UI.escape(f.name) + '</div>' +
          '<div class="food-macros">' + f.calories + ' kcal · ' + f.serving_size + UI.escape(f.serving_unit) +
            ' · P' + f.protein_g + ' C' + f.carbs_g + ' F' + f.fat_g + '</div>' +
        '</button>'
      ).join('');
      el.querySelectorAll('.food-result').forEach(btn => {
        btn.addEventListener('click', () => {
          selectedFood = list.find(f => f.id === btn.dataset.id);
          document.querySelectorAll('.food-result').forEach(b => b.classList.remove('selected'));
          btn.classList.add('selected');
          document.getElementById('serving-row').classList.remove('hidden');
        });
      });
    } catch (err) { UI.showError(document.getElementById('error'), err); }
  }

  async function confirmAdd() {
    if (!selectedFood) return;
    const servings = parseFloat(document.getElementById('serving-input').value);
    const meal = document.getElementById('meal-input').value;
    if (!servings || servings <= 0) return;
    try {
      await API.post('/v1/nutrition/log', {
        food_id: selectedFood.id, servings: servings, meal_type: meal,
      });
      closeModal('add-modal');
      await loadDaily();
    } catch (err) { UI.showError(document.getElementById('error'), err); }
  }

  async function autoCompute() {
    const goal = document.getElementById('goal-input').value;
    const activity = document.getElementById('activity-input').value;
    try {
      const t = await API.post('/v1/nutrition/targets/auto', { goal: goal, activity_level: activity });
      // Reflect back in manual inputs
      document.getElementById('manual-cal').value = t.calories;
      document.getElementById('manual-p').value = t.protein_g;
      document.getElementById('manual-c').value = t.carbs_g;
      document.getElementById('manual-f').value = t.fat_g;
      await loadDaily();
    } catch (err) { UI.showError(document.getElementById('error'), err); }
  }

  async function saveManual() {
    const t = {
      calories:  parseInt(document.getElementById('manual-cal').value, 10) || 0,
      protein_g: parseInt(document.getElementById('manual-p').value, 10) || 0,
      carbs_g:   parseInt(document.getElementById('manual-c').value, 10) || 0,
      fat_g:     parseInt(document.getElementById('manual-f').value, 10) || 0,
      goal:           document.getElementById('goal-input').value,
      activity_level: document.getElementById('activity-input').value,
    };
    try {
      await API.fetch('/v1/nutrition/targets', { method: 'PUT', body: JSON.stringify(t) });
      closeModal('targets-modal');
      await loadDaily();
    } catch (err) { UI.showError(document.getElementById('error'), err); }
  }
})();
