// Theme preference: light, dark or system (default). The choice lives in
// localStorage; <html data-theme> always holds the resolved light|dark, which
// is what the CSS keys off. Loaded synchronously in <head> to avoid a flash.
(function () {
  var KEY = 'gloss-theme';
  var root = document.documentElement;
  var mq = window.matchMedia('(prefers-color-scheme: dark)');

  function choice() {
    try {
      var v = localStorage.getItem(KEY);
      return v === 'light' || v === 'dark' ? v : 'system';
    } catch (e) {
      return 'system';
    }
  }

  function apply() {
    var c = choice();
    root.dataset.theme = c === 'system' ? (mq.matches ? 'dark' : 'light') : c;
    document.querySelectorAll('[data-theme-choice]').forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.dataset.themeChoice === c));
    });
  }

  document.addEventListener('click', function (e) {
    var b = e.target.closest('[data-theme-choice]');
    if (!b) return;
    try {
      if (b.dataset.themeChoice === 'system') localStorage.removeItem(KEY);
      else localStorage.setItem(KEY, b.dataset.themeChoice);
    } catch (err) { /* storage blocked: choice lasts until reload */ }
    apply();
  });

  mq.addEventListener('change', apply);
  window.addEventListener('storage', function (e) { if (e.key === KEY) apply(); });
  document.addEventListener('DOMContentLoaded', apply);
  apply();
})();
