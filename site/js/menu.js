// Closes the mobile nav menu on an outside click, Escape, or choosing a link.
(function () {
  var menu = document.querySelector('details[data-menu]');
  if (!menu) return;
  document.addEventListener('click', function (e) {
    if (menu.open && (!menu.contains(e.target) || e.target.closest('a'))) menu.open = false;
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') menu.open = false;
  });
})();
