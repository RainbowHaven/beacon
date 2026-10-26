package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
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
	nav := chrome(html)
	dash, occ := strings.Index(nav, `href="/dashboard"`), strings.Index(nav, `href="/occupants"`)
	if dash < 0 || occ < 0 || dash > occ {
		t.Fatal("Dashboard should be first in the sidebar")
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
	if !strings.Contains(login, `class="auth-product"`) || !strings.Contains(login, ">Beacon<") {
		t.Fatal("login page should show the product name Beacon")
	}
	if !strings.Contains(login, `class="link link-primary" href="/help/passkeys"`) {
		t.Fatal("login passkey help should use link link-primary")
	}
}

func TestListFilterTemplatesRender(t *testing.T) {
	tmpl := parseLayout(t)
	data := map[string]any{
		"Title":        "x",
		"Path":         "/reports",
		"User":         map[string]any{"Email": "a@b.c", "Role": "rhc_admin"},
		"Theme":        "dark",
		"Month":        "2026-10",
		"MonthChoices": []monthChoice{{Value: "2026-10", Label: "October 2026"}},
		"Houses":       []any{},
		"Filters":      []any{},
		"DocVersion":   "12 September 2026",
		"Status":       "open",
		"StatusFilter": "open",
	}
	for _, page := range []string{"reports.html", "expenses.html", "safeguarding.html", "operational_issues.html"} {
		var buf strings.Builder
		if err := tmpl.ExecuteTemplate(&buf, page, data); err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		html := buf.String()
		if !strings.Contains(html, "Light mode") || strings.Contains(html, `type="month"`) {
			t.Fatalf("%s theme/month markup unexpected", page)
		}
		if strings.Contains(html, ">Filter</button>") {
			t.Fatalf("%s still has a Filter button", page)
		}
	}
	var reports strings.Builder
	if err := tmpl.ExecuteTemplate(&reports, "reports.html", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reports.String(), "October 2026") || !strings.Contains(reports.String(), `class="select"`) {
		t.Fatal("report month select missing")
	}
	var sg strings.Builder
	if err := tmpl.ExecuteTemplate(&sg, "safeguarding.html", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sg.String(), `data-open-modal="doc37-modal"`) || !strings.Contains(sg.String(), `id="doc37-modal"`) {
		t.Fatal("Document 37 modal missing")
	}
}

func TestAdminUserEditTemplateRenders(t *testing.T) {
	tmpl := parseLayout(t)
	created := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	used := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	phone := domain.Passkey{ID: []byte{1, 2, 3}, Label: "iPhone", CreatedAt: created, LastUsedAt: &used}
	laptop := domain.Passkey{ID: []byte{4}, Label: "Laptop", CreatedAt: created}
	target := domain.User{
		ID:          2,
		Email:       "mgr@example.com",
		DisplayName: "House lead",
		Status:      domain.UserActive,
		Role:        domain.RoleSafeHouseManager,
	}
	data := map[string]any{
		"Title":         "Edit user",
		"Path":          "/admin/users/2/edit",
		"User":          domain.User{Email: "rhc@example.com", Role: domain.RoleRHCAdmin},
		"Target":        target,
		"SelectedRHL":   int64(3),
		"SelectedHouse": int64(7),
		"RHLs":          []domain.RHL{{ID: 3, Name: "North", Code: "N"}},
		"SafeHouses":    []domain.SafeHouse{{ID: 7, Name: "Haven House"}},
		"Passkeys":      []domain.Passkey{phone, laptop},
		"Error":         "safe house required",
		"Flash":         "Email updated",
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "admin_user_edit.html", data); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`<div class="page-header">`,
		`<h1>Edit user</h1>`,
		`<div class="stack">`,
		`<section class="stack stack-narrow">`,
		`<form method="post" action="/admin/users/2" class="stack">`,
		`<input class="input" type="text" name="display_name" value="House lead">`,
		`<select class="select" name="role" id="role" required>`,
		`value="safe_house_manager" selected`,
		`<select class="select" name="rhl_id">`,
		`value="3" selected>North (N)`,
		`<select class="select" name="safe_house_id">`,
		`value="7" selected>Haven House`,
		`class="btn btn-primary"`,
		`>Save</button>`,
		`class="btn btn-outline" href="/admin/users">Cancel</a>`,
		`class="alert alert-error" role="alert">safe house required`,
		`role="status">Email updated`,
		`does <strong>not</strong> require a new invite`,
		`<section class="stack stack-narrow">`,
		`<h2>Email</h2>`,
		`action="/admin/users/2/email" class="invite-row"`,
		`<input class="input" type="email" name="email" value="mgr@example.com" required aria-label="New email">`,
		`>Change email</button>`,
		`<section class="stack">`,
		`<h2>Passkeys</h2>`,
		`<ul class="record-list">`,
		`<li class="record-card">`,
		`Added Jan 2, 2026 · Last used Mar 4, 2026`,
		`Added Jan 2, 2026 · Last used never`,
		`action="/admin/users/2/passkeys/` + phone.Key() + `/delete"`,
		`action="/admin/users/2/passkeys/` + laptop.Key() + `/delete"`,
		`class="btn btn-error"`,
		`>Remove</button>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(html, "No passkeys") {
		t.Fatal("passkey list should render the credentials")
	}
	if strings.Contains(html, `style="`) || strings.Contains(html, `class="flex`) {
		t.Fatal("edit user page should use named layout classes only")
	}

	buf.Reset()
	data["Passkeys"] = []domain.Passkey{}
	data["Error"] = ""
	data["Flash"] = ""
	if err := tmpl.ExecuteTemplate(&buf, "admin_user_edit.html", data); err != nil {
		t.Fatal(err)
	}
	empty := buf.String()
	if !strings.Contains(empty, "No passkeys. They need a new invite to sign in.") {
		t.Fatal("missing empty passkey state")
	}
	if strings.Contains(empty, "safe house required") || strings.Contains(empty, "Email updated") {
		t.Fatal("empty error and flash should not render")
	}
}
