package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/server"
	"github.com/RainbowHaven/beacon/internal/store"
)

func TestSafeguardingScopedAccessAndAudit(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

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
	ctx := context.Background()

	tor, err := st.CreateRHLWithCode(ctx, "Toronto", "TOR", true)
	if err != nil {
		t.Fatal(err)
	}
	van, err := st.CreateRHL(ctx, "Vancouver", true)
	if err != nil {
		t.Fatal(err)
	}
	houseA, err := st.CreateSafeHouse(ctx, tor.ID, "House A", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}
	houseB, err := st.CreateSafeHouse(ctx, van.ID, "House B", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	base, _ := url.Parse(ts.URL)

	clientFor := func(email string, role domain.Role, rhlID, houseID *int64) *http.Client {
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
		return &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	managerA := clientFor("a@example.com", domain.RoleSafeHouseManager, &tor.ID, &houseA.ID)
	rhlVan := clientFor("van@example.com", domain.RoleRHLAdmin, &van.ID, nil)
	rhc := clientFor("rhc@example.com", domain.RoleRHCAdmin, nil, nil)

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

	today := time.Now().UTC().Format("2006-01-02")
	year := strconv.Itoa(time.Now().UTC().Year())
	suggested := "Incident-TOR-" + year + "-1"

	code, body := get(managerA, "/safeguarding/new")
	if code != http.StatusOK || !strings.Contains(body, `value="`+suggested+`"`) {
		t.Fatalf("new form status=%d, suggestion %q missing", code, suggested)
	}
	code, body = get(managerA, "/safeguarding")
	if code != http.StatusOK {
		t.Fatalf("list status %d", code)
	}
	for _, want := range []string{
		"Reporting a Safeguarding Concern",
		"Complete Document 37 when a safeguarding concern or incident occurs or is reported. Download the blank form using the button below.",
		"Submit the completed form to your RHL Safeguarding Contact within 24 hours, or as soon as safely and reasonably possible. Use WhatsApp or the safest locally available method.",
		"Record only the incident identifier and basic tracking information in Beacon. Do not enter detailed information about the incident in Beacon, and do not upload the completed Document 37 to Beacon.",
		`href="/safeguarding/document-37"`,
		"Document 37 version: 12 September 2026",
		`href="/safeguarding" aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("list page missing %q", want)
		}
	}

	form := url.Values{
		"safe_house_id":      {strconv.FormatInt(houseA.ID, 10)},
		"incident_id":        {suggested},
		"reported_on":        {today},
		"occurred_precision": {"unknown"},
		"status":             {"open"},
	}
	future := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad.Set("reported_on", future)
	if code, body := post(managerA, "/safeguarding", bad); code != http.StatusOK || !strings.Contains(body, "cannot be in the future") {
		t.Fatalf("future reported: status=%d", code)
	}
	bad.Set("reported_on", today)
	bad.Set("status", "resolved")
	if code, body := post(managerA, "/safeguarding", bad); code != http.StatusOK || !strings.Contains(body, "Enter the date the concern was resolved or closed") {
		t.Fatalf("resolved without date: status=%d", code)
	}
	bad.Set("status", "open")
	bad.Set("incident_id", "line one\nline two")
	if code, _ := post(managerA, "/safeguarding", bad); code != http.StatusOK {
		t.Fatalf("multi-line identifier: status=%d", code)
	}

	if code, _ := post(managerA, "/safeguarding", form); code != http.StatusSeeOther {
		t.Fatalf("create status %d", code)
	}
	if code, body := post(managerA, "/safeguarding", form); code != http.StatusOK || !strings.Contains(body, "already recorded") || !strings.Contains(body, "Incident-TOR-"+year+"-2") {
		t.Fatalf("duplicate: status=%d", code)
	}
	other := url.Values{}
	for k, v := range form {
		other[k] = v
	}
	other.Set("safe_house_id", strconv.FormatInt(houseB.ID, 10))
	other.Set("incident_id", "Incident-VAN-"+year+"-1")
	if code, _ := post(managerA, "/safeguarding", other); code != http.StatusForbidden {
		t.Fatalf("create in other house: status %d", code)
	}
	if code, _ := post(rhlVan, "/safeguarding", other); code != http.StatusSeeOther {
		t.Fatalf("rhl admin create: status %d", code)
	}

	list, err := st.ListSafeguardingConcerns(ctx, []int64{houseA.ID}, nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	concern := list[0]
	edit := "/safeguarding/" + strconv.FormatInt(concern.ID, 10)

	if code, _ := get(rhlVan, edit+"/edit"); code != http.StatusForbidden {
		t.Fatalf("other rhl edit get: status %d", code)
	}
	if code, _ := post(rhlVan, edit, form); code != http.StatusForbidden {
		t.Fatalf("other rhl edit post: status %d", code)
	}
	if code, body := get(rhlVan, "/safeguarding?status=all"); code != http.StatusOK || strings.Contains(body, suggested) {
		t.Fatalf("other rhl list leaks concern: status %d", code)
	}
	if code, _ := get(managerA, "/safeguarding/999999/edit"); code != http.StatusNotFound {
		t.Fatalf("missing concern: status %d", code)
	}

	code, body = get(managerA, edit+"/edit")
	if code != http.StatusOK || !strings.Contains(body, `value="`+suggested+`"`) {
		t.Fatalf("edit get status %d", code)
	}
	resolved := url.Values{
		"incident_id":        {suggested},
		"reported_on":        {today},
		"occurred_on":        {today},
		"occurred_precision": {"approximate"},
		"status":             {"resolved"},
		"closed_on":          {today},
	}
	if code, _ := post(managerA, edit, resolved); code != http.StatusSeeOther {
		t.Fatalf("resolve status %d", code)
	}
	got, err := st.GetSafeguardingConcern(ctx, concern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.SafeguardingResolved || got.ClosedOn == nil || got.OccurredPrecision != domain.DateApproximate {
		t.Fatalf("after resolve %+v", got)
	}

	if _, body := get(managerA, "/safeguarding"); strings.Contains(body, suggested) {
		t.Fatal("resolved concern shown in default open list")
	}
	if _, body := get(managerA, "/safeguarding?status=resolved"); !strings.Contains(body, suggested) {
		t.Fatal("resolved concern missing from resolved list")
	}
	if _, body := get(rhc, "/safeguarding?status=all&house="+strconv.FormatInt(houseB.ID, 10)); strings.Contains(body, suggested) || !strings.Contains(body, "Incident-VAN-") {
		t.Fatal("rhc house filter")
	}

	events, err := st.ListAuditEvents(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	var created, updated map[string]any
	for _, e := range events {
		if e.SubjectID != strconv.FormatInt(concern.ID, 10) {
			continue
		}
		switch e.Action {
		case "safeguarding.create":
			created = e.Meta
		case "safeguarding.update":
			updated = e.Meta
		}
	}
	if created == nil || created["incident_id"] != suggested || created["status"] != "open" {
		t.Fatalf("create audit %v", created)
	}
	if updated == nil || updated["status_from"] != "open" || updated["status_to"] != "resolved" {
		t.Fatalf("update audit %v", updated)
	}
	changed, _ := updated["changed"].([]any)
	if len(changed) != 4 {
		t.Fatalf("changed fields %v", changed)
	}
}

func TestDocument37Download(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

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
	ctx := context.Background()
	_, house, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{
		Email: "m@example.com", DisplayName: "M", Role: domain.RoleSafeHouseManager,
		RHLID: &house.RHLID, SafeHouseID: &house.ID,
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

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	noRedirect := func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

	anon := &http.Client{CheckRedirect: noRedirect}
	res, err := anon.Get(ts.URL + "/safeguarding/document-37")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/login" {
		t.Fatalf("anonymous download: status %d", res.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	base, _ := url.Parse(ts.URL)
	jar.SetCookies(base, []*http.Cookie{{Name: "beacon_session", Value: session, Path: "/"}})
	client := &http.Client{Jar: jar, CheckRedirect: noRedirect}
	res, err = client.Get(ts.URL + "/safeguarding/document-37")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("download status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("content type %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="37_Safeguarding_and_Incident_Form.docx"` {
		t.Fatalf("content disposition %q", cd)
	}
	if len(b) < 1000 || string(b[:2]) != "PK" {
		t.Fatalf("not a docx: %d bytes", len(b))
	}

	res, err = anon.Get(ts.URL + "/static/docs/37_Safeguarding_and_Incident_Form.docx")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Fatal("Document 37 must not be served under /static/")
	}
}
