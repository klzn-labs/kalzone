(function () {
  var key = 'kalzone-theme';
  var order = ['system', 'light', 'dark'];
  var root = document.documentElement;

  function apply(theme) {
    if (theme === 'light' || theme === 'dark') root.dataset.theme = theme;
    else delete root.dataset.theme;
  }

  function saved() {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  function next() {
    var theme = order[(order.indexOf(root.dataset.theme || 'system') + 1) % order.length];
    try { localStorage.setItem(key, theme); } catch (e) {}
    apply(theme);
  }

  apply(saved());
  document.addEventListener('click', function (e) {
    if (e.target.closest('[data-theme-toggle]')) next();
  });
})();
