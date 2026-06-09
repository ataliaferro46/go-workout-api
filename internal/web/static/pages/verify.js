// pages/verify.js — consumes the token in ?token=…, shows success/failure.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', async () => {
    const status = document.getElementById('status');
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');

    if (!token) {
      renderError('Missing token', 'The verification link is incomplete. Try the email link again.');
      return;
    }

    try {
      await API.get('/v1/auth/verify?token=' + encodeURIComponent(token));
      // The server set our session cookie automatically. Bounce to /.
      renderSuccess();
      setTimeout(() => { window.location.href = '/'; }, 1200);
    } catch (err) {
      renderError('Verification failed', err.message || 'This link may have expired or already been used.');
    }

    function renderSuccess() {
      status.innerHTML =
        '<div class="status-icon"><div class="checkmark">✓</div></div>' +
        '<h2>You\'re verified.</h2>' +
        '<p>Redirecting you to the app…</p>';
    }
    function renderError(title, msg) {
      status.innerHTML =
        '<div class="status-icon"><div class="cross">✕</div></div>' +
        '<h2>' + UI.escape(title) + '</h2>' +
        '<p>' + UI.escape(msg) + '</p>' +
        '<div class="actions">' +
          '<a href="/signup" class="btn btn-secondary">Try signing up again</a>' +
          '<a href="/login" class="btn btn-primary">Sign in</a>' +
        '</div>';
    }
  });
})();
