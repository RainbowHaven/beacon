package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magiconair/beacon/internal/version"
	"github.com/magiconair/beacon/web"
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
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/css/app.css?v=abc", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache header %q", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/css/app.css", nil))
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("unversioned cache header %q", got)
	}
}

func TestLayoutNavRenders(t *testing.T) {
	tmpl := template.New("").Funcs(template.FuncMap{
		"static":     func(path string) string { return "/static/" + path },
		"appVersion": version.Line,
		"navCurrent": navCurrent,
	})
	if _, err := tmpl.ParseFS(web.Templates, "templates/*.html"); err != nil {
		t.Fatal(err)
	}

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
		`class="app-shell"`,
		`class="app-nav"`,
		`class="nav-desktop-only"`,
		`class="nav-more"`,
		`href="/occupants"`,
		`href="/expenses"`,
		`href="/operations"`,
		`href="/safeguarding"`,
		`href="/reports"`,
		`href="/admin/users" aria-current="page"`,
		`class="app-footer`,
		`<hr>`,
		`class="app-version"`,
		`class="profile-menu"`,
		`action="/logout"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}

	// The phone bar keeps Occupants, Expenses, Reports and More; the rest is
	// sidebar-only and repeated in the More panel.
	for _, path := range []string{"/operations", "/safeguarding", "/dashboard", "/admin/users", "/admin/houses", "/admin/audit"} {
		if !strings.Contains(html, `<a class="nav-desktop-only" href="`+path+`"`) {
			t.Fatalf("%s should be sidebar-only in the bar", path)
		}
	}
	for _, path := range []string{"/occupants", "/expenses", "/reports"} {
		if !strings.Contains(html, `<a href="`+path+`"`) || strings.Contains(html, `<a class="nav-desktop-only" href="`+path+`"`) {
			t.Fatalf("%s should stay in the phone bar", path)
		}
	}

	panel := func(html string) string {
		_, after, ok := strings.Cut(html, `class="nav-more-panel"`)
		if !ok {
			t.Fatal("missing More panel")
		}
		before, _, _ := strings.Cut(after, "</details>")
		return before
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
	if !strings.Contains(rhl, `<a class="nav-desktop-only" href="/dashboard" aria-current="page">Dashboard</a>`) {
		t.Fatal("RHL admin should see Dashboard in the sidebar")
	}
	if !strings.Contains(rhl, `<summary aria-current="page">More</summary>`) || !strings.Contains(panel(rhl), `href="/dashboard" aria-current="page"`) {
		t.Fatalf("RHL admin More panel %s", panel(rhl))
	}
	if nav, _, _ := strings.Cut(rhl, "</nav>"); strings.Contains(nav, "/admin/") {
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
	if !strings.Contains(mgr, `<summary aria-current="page">More</summary>`) {
		t.Fatal("More should be current on a page listed in it")
	}
	p := panel(mgr)
	if !strings.Contains(p, `href="/operations"`) || !strings.Contains(p, `href="/safeguarding" aria-current="page"`) {
		t.Fatalf("manager More panel %s", p)
	}
	if nav, _, _ := strings.Cut(mgr, "</nav>"); strings.Contains(nav, "/admin/") || strings.Contains(nav, "/dashboard") {
		t.Fatal("manager nav should not link to admin pages or the dashboard")
	}

	buf.Reset()
	if err := tmpl.ExecuteTemplate(&buf, "login.html", map[string]any{"Title": "Log in"}); err != nil {
		t.Fatal(err)
	}
	login := buf.String()
	if strings.Contains(login, "app-shell") || strings.Contains(login, "app-nav") {
		t.Fatalf("login page should not use app chrome: %s", login[:min(400, len(login))])
	}
	if !strings.Contains(login, `href="/login"`) {
		t.Fatal("login page missing login link")
	}
}
