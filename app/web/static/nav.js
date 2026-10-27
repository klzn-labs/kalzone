(function () {
  var key = 'kalzone-nav-resources';
  var root = document.documentElement;

  function saved() {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  function store(state) {
    try { localStorage.setItem(key, state); } catch (e) {}
  }

  function closed() {
    return root.dataset.resourcesNav === 'closed';
  }

  function sync() {
    document.querySelectorAll('[data-resources-toggle]').forEach(function (button) {
      button.setAttribute('aria-expanded', closed() ? 'false' : 'true');
    });
  }

  function toggle() {
    if (closed()) delete root.dataset.resourcesNav;
    else root.dataset.resourcesNav = 'closed';
    store(closed() ? 'closed' : 'open');
    sync();
  }

  if (saved() === 'closed') root.dataset.resourcesNav = 'closed';
  document.addEventListener('DOMContentLoaded', sync);
  document.addEventListener('click', function (e) {
    if (e.target.closest('[data-resources-toggle]')) toggle();
  });
})();
