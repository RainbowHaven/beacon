package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/server"
	"github.com/magiconair/beacon/internal/store"
)

func TestOperationalIssuesScopedFlow(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := server.New(logger, db, server.Config{
		BaseURL:           "http://localhost",
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "Beacon",
		WebAuthnRPOrigins: []string{"http://localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := srv.Store()
	rhl, houseA, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	houseB, err := st.CreateSafeHouse(ctx, rhl.ID, "Birch House", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}
	rhl2, err := st.CreateRHL(ctx, "Second RHL", true)
	if err != nil {
		t.Fatal(err)
	}
	houseC, err := st.CreateSafeHouse(ctx, rhl2.ID, "Cedar House", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	base, _ := url.Parse(ts.URL)

	login := func(email string, role domain.Role, rhlID, houseID *int64) (*http.Client, domain.User) {
		t.Helper()
		u, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{
			Email: email, DisplayName: email, Role: role, RHLID: rhlID, SafeHouseID: houseID,
		}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.ActivateUser(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
		session, err := st.CreateSession(ctx, u.ID, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		jar, _ := cookiejar.New(nil)
		jar.SetCookies(base, []*http.Cookie{{Name: "beacon_session", Value: session, Path: "/"}})
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}, u
	}
	manager, managerUser := login("ops-mgr@example.com", domain.RoleSafeHouseManager, &houseA.RHLID, &houseA.ID)
	rhlAdmin, _ := login("ops-rhl@example.com", domain.RoleRHLAdmin, &rhl.ID, nil)
	rhcAdmin, _ := login("ops-rhc@example.com", domain.RoleRHCAdmin, nil, nil)

	get := func(c *http.Client, path string) (int, string) {
		t.Helper()
		res, err := c.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	post := func(c *http.Client, path string, form url.Values) (int, string) {
		t.Helper()
		res, err := c.PostForm(ts.URL+path, form)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	issueForm := func(houseID int64, status, closed, notes string) url.Values {
		return url.Values{
			"safe_house_id": {strconv.FormatInt(houseID, 10)},
			"identified_on": {"2026-03-02"},
			"category":      {"utilities"},
			"description":   {"Water supply cut"},
			"effect":        {"No showers"},
			"action_taken":  {"Bought bottled water"},
			"rhl_request":   {"Pay for a plumber"},
			"status":        {status},
			"closed_on":     {closed},
			"closure_notes": {notes},
		}
	}

	code, html := get(manager, "/operations")
	if code != http.StatusOK || !strings.Contains(html, "Document 37") || !strings.Contains(html, `href="/operations" aria-current="page"`) {
		t.Fatalf("list status=%d body=%s", code, html)
	}
	if strings.Contains(html, `name="house"`) {
		t.Fatal("single-house manager should not see a house filter")
	}
	code, html = get(manager, "/operations/new")
	if code != http.StatusOK || !strings.Contains(html, "Document 37") || !strings.Contains(html, "Safety or security issue that is not a safeguarding concern") {
		t.Fatalf("new status=%d", code)
	}

	if code, _ := post(manager, "/operations", issueForm(houseA.ID, "open", "", "")); code != http.StatusSeeOther {
		t.Fatalf("create status %d", code)
	}
	if code, _ := post(manager, "/operations", issueForm(houseB.ID, "open", "", "")); code != http.StatusForbidden {
		t.Fatalf("create in other house status %d", code)
	}
	code, html = post(manager, "/operations", issueForm(houseA.ID, "resolved", "2026-03-01", "Fixed"))
	if code != http.StatusOK || !strings.Contains(html, "cannot be before the date identified") || !strings.Contains(html, "Pay for a plumber") {
		t.Fatalf("bad closure status=%d", code)
	}

	list, err := st.ListOperationalIssuesByHouses(ctx, []int64{houseA.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RHLRequest != "Pay for a plumber" || list[0].CreatedBy == nil || *list[0].CreatedBy != managerUser.ID {
		t.Fatalf("list=%+v", list)
	}
	issue := list[0]
	issuePath := "/operations/" + strconv.FormatInt(issue.ID, 10)

	cIssue, err := st.CreateOperationalIssue(ctx, houseC.ID, store.OperationalIssueFields{
		IdentifiedOn: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Category: "food_supplies", Description: "Cedar pantry",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cPath := "/operations/" + strconv.FormatInt(cIssue.ID, 10)
	for _, c := range []*http.Client{manager, rhlAdmin} {
		if code, _ := get(c, cPath+"/edit"); code != http.StatusForbidden {
			t.Fatalf("edit out-of-scope status %d", code)
		}
		if code, _ := post(c, cPath, issueForm(houseC.ID, "open", "", "")); code != http.StatusForbidden {
			t.Fatalf("update out-of-scope status %d", code)
		}
	}
	if code, _ := get(rhlAdmin, "/operations?house="+strconv.FormatInt(houseC.ID, 10)); code != http.StatusForbidden {
		t.Fatalf("house filter out of scope status %d", code)
	}
	if code, _ := get(rhcAdmin, cPath+"/edit"); code != http.StatusOK {
		t.Fatalf("rhc edit status %d", code)
	}
	if code, _ := post(manager, issuePath+"/delete", nil); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("delete route status %d", code)
	}

	code, html = post(rhlAdmin, issuePath, issueForm(houseA.ID, "resolved", "2026-03-05", ""))
	if code != http.StatusOK || !strings.Contains(html, "notes are required") {
		t.Fatalf("resolve without notes status=%d", code)
	}
	if code, _ := post(rhlAdmin, issuePath, issueForm(houseA.ID, "resolved", "2026-03-05", "Plumber fixed the main")); code != http.StatusSeeOther {
		t.Fatalf("resolve status %d", code)
	}
	got, err := st.GetOperationalIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "resolved" || got.ClosedOn == nil || got.ClosureNotes != "Plumber fixed the main" {
		t.Fatalf("resolved=%+v", got)
	}

	code, html = get(rhlAdmin, "/operations")
	if code != http.StatusOK || strings.Contains(html, "Water supply cut") || !strings.Contains(html, `name="house"`) {
		t.Fatalf("open list after resolve status=%d", code)
	}
	code, html = get(rhlAdmin, "/operations?status=resolved&house="+strconv.FormatInt(houseA.ID, 10))
	if code != http.StatusOK || !strings.Contains(html, "Water supply cut") {
		t.Fatalf("resolved list status=%d", code)
	}
	if _, html = get(rhlAdmin, "/operations?status=all"); !strings.Contains(html, "Water supply cut") || strings.Contains(html, "Cedar pantry") {
		t.Fatal("all list should show in-scope issues only")
	}

	if code, _ := post(manager, issuePath, issueForm(houseA.ID, "open", "2026-03-05", "Plumber fixed the main")); code != http.StatusSeeOther {
		t.Fatalf("reopen status %d", code)
	}
	got, err = st.GetOperationalIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "open" || got.ClosedOn != nil || got.ClosureNotes != "" {
		t.Fatalf("reopened=%+v", got)
	}

	events, err := st.ListAuditEvents(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	var creates, updates []domain.AuditEvent
	for _, e := range events {
		switch e.Action {
		case "operational_issue.create":
			creates = append(creates, e)
		case "operational_issue.update":
			updates = append(updates, e)
		}
		if strings.HasPrefix(e.Action, "operational_issue.") {
			for _, v := range e.Meta {
				if s, ok := v.(string); ok && (strings.Contains(s, "Water") || strings.Contains(s, "Plumber")) {
					t.Fatalf("audit meta leaked free text: %+v", e.Meta)
				}
			}
		}
	}
	if len(creates) != 1 || creates[0].Meta["category"] != "utilities" {
		t.Fatalf("create audits=%+v", creates)
	}
	if len(updates) != 2 {
		t.Fatalf("update audits=%+v", updates)
	}
	reopen, resolve := updates[0], updates[1]
	if resolve.Meta["status_from"] != "open" || resolve.Meta["status_to"] != "resolved" {
		t.Fatalf("resolve meta=%+v", resolve.Meta)
	}
	if reopen.Meta["status_from"] != "resolved" || reopen.Meta["status_to"] != "open" {
		t.Fatalf("reopen meta=%+v", reopen.Meta)
	}
	changed, _ := reopen.Meta["changed"].([]any)
	if !slices.Contains(changed, any("closure_notes")) || !slices.Contains(changed, any("status")) {
		t.Fatalf("reopen changed=%v", reopen.Meta["changed"])
	}
}
