(function () {
  const themeKey = "beacon-mockup-theme";
  const sidebarKey = "beacon-mockup-sidebar";
  const root = document.documentElement;
  const saved = localStorage.getItem(themeKey);
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

  function applyTheme(theme) {
    root.setAttribute("data-theme", theme);
    localStorage.setItem(themeKey, theme);
    document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
      el.checked = theme === "dark";
    });
  }

  function applySidebar(collapsed) {
    if (collapsed) {
      root.setAttribute("data-sidebar", "collapsed");
    } else {
      root.removeAttribute("data-sidebar");
    }
    document.querySelectorAll("[data-sidebar-toggle]").forEach(function (btn) {
      const label = collapsed ? "Expand sidebar" : "Collapse sidebar";
      btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
      btn.setAttribute("aria-label", label);
      btn.title = label;
    });
  }

  applyTheme(saved || (prefersDark ? "dark" : "light"));
  applySidebar(localStorage.getItem(sidebarKey) === "collapsed");

  document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
    el.addEventListener("change", function () {
      applyTheme(el.checked ? "dark" : "light");
    });
  });

  document.querySelectorAll("[data-sidebar-toggle]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      const collapsed = root.getAttribute("data-sidebar") !== "collapsed";
      applySidebar(collapsed);
      localStorage.setItem(sidebarKey, collapsed ? "collapsed" : "expanded");
    });
  });
})();
