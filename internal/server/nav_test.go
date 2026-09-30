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
