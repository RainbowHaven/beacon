package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RainbowHaven/beacon/internal/version"
	"github.com/RainbowHaven/beacon/web"
)

func TestNavCurrent(t *testing.T) {
	if !navCurrent("/occupants", "/occupants") || !navCurrent("/occupants/new", "/occupants") {
		t.Fatal("expected occupants paths to be current")
	}
	if navCurrent("/occupants", "/expenses") || navCurrent("/", "/occupants") || navCurrent("/occupant", "/occupants") {
		t.Fatal("unexpected current match")
	}
	if !navCurrent("/admin/users/invite", "/admin/users") {
		t.Fatal("expected invite path to mark users")
	}
}

func TestCacheFingerprinted(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := cacheFingerprinted(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/css/beacon.css?v=abc", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache header %q", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/css/beacon.css", nil))
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("unversioned cache header %q", got)
	}
}

func parseLayout(t *testing.T) *template.Template {
	t.Helper()
	tmpl := template.New("").Funcs(template.FuncMap{
		"static":     func(path string) string { return "/static/" + path },
		"appVersion": version.Line,
		"navCurrent": navCurrent,
	})
	if _, err := tmpl.ParseFS(web.Templates, "templates/*.html"); err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func TestLayoutNavRenders(t *testing.T) {
	tmpl := parseLayout(t)

	var buf strings.Builder
	data := map[string]any{
		"Title": "Users",
		"Path":  "/admin/users",
		"User":  map[string]any{"Email": "rhc@example.com", "Role": "rhc_admin"},
	}
	if err := tmpl.ExecuteTemplate(&buf, "admin_users.html", data); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`class="app-frame"`,
		`class="sidebar"`,
		`class="dock dock-md no-print"`,
		`class="dock-more"`,
		`href="/occupants"`,
		`href="/expenses"`,
		`href="/operations"`,
		`href="/safeguarding"`,
		`href="/reports"`,
		`href="/admin/users" aria-current="page"`,
		`class="app-footer`,
		`class="app-version"`,
		`action="/logout"`,
		`data-theme-toggle`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}

	for _, path := range []string{"/occupants", "/expenses", "/reports"} {
		if !strings.Contains(html, `<a href="`+path+`"`) && !strings.Contains(html, `href="`+path+`"`) {
			t.Fatalf("%s should stay in the phone dock", path)
		}
	}

	panel := func(html string) string {
		_, after, ok := strings.Cut(html, `class="dock-more-panel"`)
		if !ok {
			t.Fatal("missing More panel")
		}
		before, _, _ := strings.Cut(after, "</details>")
		return before
	}
	chrome := func(html string) string {
		_, rest, ok := strings.Cut(html, `id="app-sidebar"`)
		if !ok {
			t.Fatal("missing sidebar")
		}
		aside, after, _ := strings.Cut(rest, "</aside>")
		_, dock, ok := strings.Cut(after, `class="dock`)
		if !ok {
			return aside
		}
		dockBody, _, _ := strings.Cut(dock, "</nav>")
		return aside + dockBody
	}
	if p := panel(html); !strings.Contains(p, `href="/operations"`) || !strings.Contains(p, `href="/safeguarding"`) || !strings.Contains(p, `href="/dashboard"`) || !strings.Contains(p, `href="/admin/users" aria-current="page"`) {
		t.Fatalf("admin More panel %s", p)
	}

	buf.Reset()
	data = map[string]any{
		"Title": "Dashboard",
		"Path":  "/dashboard",
		"User":  map[string]any{"Email": "rhl@example.com", "Role": "rhl_admin"},
	}
	if err := tmpl.ExecuteTemplate(&buf, "admin_users.html", data); err != nil {
		t.Fatal(err)
	}
	rhl := buf.String()
	if !strings.Contains(rhl, `href="/dashboard" aria-current="page"`) {
		t.Fatal("RHL admin should see Dashboard in the sidebar")
	}
	if !strings.Contains(rhl, `aria-current="page"`) || !strings.Contains(panel(rhl), `href="/dashboard" aria-current="page"`) {
		t.Fatalf("RHL admin More panel %s", panel(rhl))
	}
	if strings.Contains(chrome(rhl), `href="/admin/users"`) || strings.Contains(chrome(rhl), `href="/admin/houses"`) || strings.Contains(chrome(rhl), `href="/admin/audit"`) {
		t.Fatal("RHL admin nav should not link to admin pages")
	}

	buf.Reset()
	data = map[string]any{
		"Title": "Safeguarding",
		"Path":  "/safeguarding",
		"User":  map[string]any{"Email": "mgr@example.com", "Role": "safe_house_manager"},
	}
	if err := tmpl.ExecuteTemplate(&buf, "admin_users.html", data); err != nil {
		t.Fatal(err)
	}
	mgr := buf.String()
	if !strings.Contains(mgr, `<summary class="dock-active" aria-current="page">`) {
		t.Fatal("More should be current on a page listed in it")
	}
	p := panel(mgr)
	if !strings.Contains(p, `href="/operations"`) || !strings.Contains(p, `href="/safeguarding" aria-current="page"`) {
		t.Fatalf("manager More panel %s", p)
	}
	if strings.Contains(chrome(mgr), `href="/admin/`) || strings.Contains(chrome(mgr), `href="/dashboard"`) {
		t.Fatal("manager nav should not link to admin pages or the dashboard")
	}

	buf.Reset()
	if err := tmpl.ExecuteTemplate(&buf, "login.html", map[string]any{"Title": "Log in", "AuthCentered": true}); err != nil {
		t.Fatal(err)
	}
	login := buf.String()
	if strings.Contains(login, "app-frame") || strings.Contains(login, `class="sidebar"`) {
		t.Fatalf("login page should not use app chrome: %s", login[:min(400, len(login))])
	}
	if !strings.Contains(login, `auth-shell`) {
		t.Fatal("login page missing auth shell")
	}
}
