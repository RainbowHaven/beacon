(function () {
  const themeKey = "beacon-mockup-theme";
  const sidebarKey = "beacon-mockup-sidebar";
  const root = document.documentElement;
  const saved = localStorage.getItem(themeKey);
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  root.setAttribute("data-theme", saved || (prefersDark ? "dark" : "light"));

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

  applySidebar(localStorage.getItem(sidebarKey) === "collapsed");

  document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
    el.checked = root.getAttribute("data-theme") === "dark";
    el.addEventListener("change", function () {
      const theme = el.checked ? "dark" : "light";
      root.setAttribute("data-theme", theme);
      localStorage.setItem(themeKey, theme);
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
