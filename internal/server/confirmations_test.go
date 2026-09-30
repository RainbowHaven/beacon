package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/server"
	"github.com/RainbowHaven/beacon/internal/store"
)

func TestMonthConfirmation(t *testing.T) {
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
	rhl, house, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRHL(ctx, rhl.ID, rhl.Name, "TOR", true); err != nil {
		t.Fatal(err)
	}
	otherRHL, err := st.CreateRHLWithCode(ctx, "Other Local", "OTH", true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateSafeHouse(ctx, otherRHL.ID, "Other House", "USD", true)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	lastMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	monthKey := lastMonth.Format("2006-01")
	resident, err := st.CreateOccupant(ctx, store.CreateOccupantInput{
		SafeHouseID: house.ID, Nickname: "Kestrel", ArrivedAt: lastMonth.AddDate(0, 0, 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	newUser := func(email, name string, role domain.Role, rhlID, houseID *int64) domain.User {
		t.Helper()
		u, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{
			Email: email, DisplayName: name, Role: role, RHLID: rhlID, SafeHouseID: houseID,
		}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.ActivateUser(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
		return u
	}
	manager := newUser("mgr@example.com", "Alex Agent", domain.RoleSafeHouseManager, &house.RHLID, &house.ID)
	rhlAdmin := newUser("rhl@example.com", "Robin RHL", domain.RoleRHLAdmin, &house.RHLID, nil)
	rhc := newUser("rhc@example.com", "Casey RHC", domain.RoleRHCAdmin, nil, nil)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(u domain.User, method, path string, form url.Values) (int, string, string) {
		t.Helper()
		session, err := st.CreateSession(ctx, u.ID, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequest(method, ts.URL+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.AddCookie(&http.Cookie{Name: "beacon_session", Value: session})
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Location"), string(b)
	}
	get := func(u domain.User, path string) (int, string) {
		t.Helper()
		code, _, body := do(u, http.MethodGet, path, nil)
		return code, body
	}
	houseID := strconv.FormatInt(house.ID, 10)
	otherID := strconv.FormatInt(other.ID, 10)
	reportPath := "/reports?month=" + monthKey + "&house=" + houseID
	confirmPath := func(id string) string { return "/reports/" + id + "/" + monthKey + "/confirm" }
	fingerprintRE := regexp.MustCompile(`name="fingerprint" value="([0-9a-f]{64})"`)
	fingerprint := func(u domain.User) string {
		t.Helper()
		code, body := get(u, reportPath)
		if code != http.StatusOK {
			t.Fatalf("report status %d", code)
		}
		m := fingerprintRE.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("no confirm form in %s", body)
		}
		return m[1]
	}
	// dashboardRow returns the dashboard table row of a house.
	dashboardRow := func(u domain.User, houseName string) string {
		t.Helper()
		code, body := get(u, "/dashboard")
		if code != http.StatusOK {
			t.Fatalf("dashboard status %d", code)
		}
		for _, row := range strings.Split(body, "<tr>") {
			if strings.Contains(row, "<td>"+houseName+"</td>") {
				return row
			}
		}
		return ""
	}

	t.Run("RHC admin cannot confirm", func(t *testing.T) {
		code, body := get(rhc, reportPath)
		if code != http.StatusOK || !strings.Contains(body, "Not confirmed.") || strings.Contains(body, "Confirm data entry complete") {
			t.Fatalf("status %d body %s", code, body)
		}
		code, _, _ = do(rhc, http.MethodPost, confirmPath(houseID), url.Values{"fingerprint": {fingerprint(manager)}})
		if code != http.StatusForbidden {
			t.Fatalf("confirm status %d", code)
		}
	})

	t.Run("manager cannot confirm another house", func(t *testing.T) {
		code, _, _ := do(manager, http.MethodPost, confirmPath(otherID), url.Values{"fingerprint": {fingerprint(manager)}})
		if code != http.StatusForbidden {
			t.Fatalf("status %d", code)
		}
	})

	t.Run("month in progress cannot be confirmed", func(t *testing.T) {
		current := now.Format("2006-01")
		code, body := get(manager, "/reports?month="+current+"&house="+houseID)
		if code != http.StatusOK || !strings.Contains(body, "Data entry can be confirmed complete after the month ends.") || strings.Contains(body, `name="fingerprint"`) {
			t.Fatalf("status %d body %s", code, body)
		}
		code, loc, _ := do(manager, http.MethodPost, "/reports/"+houseID+"/"+current+"/confirm", url.Values{"fingerprint": {fingerprint(manager)}})
		if code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
			t.Fatalf("status %d location %q", code, loc)
		}
	})

	t.Run("stale fingerprint is rejected", func(t *testing.T) {
		code, loc, _ := do(manager, http.MethodPost, confirmPath(houseID), url.Values{"fingerprint": {strings.Repeat("0", 64)}})
		if code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
			t.Fatalf("status %d location %q", code, loc)
		}
	})

	t.Run("manager confirms own house", func(t *testing.T) {
		fp := fingerprint(manager)
		code, loc, _ := do(manager, http.MethodPost, confirmPath(houseID), url.Values{"fingerprint": {fp}})
		if code != http.StatusSeeOther || !strings.Contains(loc, "ok=confirmed") {
			t.Fatalf("status %d location %q", code, loc)
		}
		code, body := get(manager, reportPath)
		if code != http.StatusOK || !strings.Contains(body, "Confirmed complete by Alex Agent on ") || strings.Contains(body, "Confirm data entry complete") {
			t.Fatalf("status %d body %s", code, body)
		}
		var meta string
		if err := db.QueryRowContext(ctx, `SELECT meta::text FROM audit_events WHERE action = 'report.confirm' AND subject_id = $1`, houseID).Scan(&meta); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(meta, monthKey) || !strings.Contains(meta, fp) {
			t.Fatalf("audit meta %s", meta)
		}
	})

	t.Run("dashboard shows statuses", func(t *testing.T) {
		row := dashboardRow(rhc, "Pilot Safe House")
		for _, want := range []string{"TOR", ">Confirmed<", lastMonth.Format("January 2006"), "Alex Agent", reportPath[len("/reports?"):] + `"`, "/reports/" + houseID + "/history"} {
			want = strings.ReplaceAll(want, "&", "&amp;")
			if !strings.Contains(row, want) {
				t.Fatalf("missing %q in %s", want, row)
			}
		}
		row = dashboardRow(rhc, "Other House")
		if !strings.Contains(row, "OTH") || !strings.Contains(row, "None") || !(strings.Contains(row, ">Due<") || strings.Contains(row, ">Overdue<")) {
			t.Fatalf("other house row %s", row)
		}
		if dashboardRow(rhlAdmin, "Other House") != "" || dashboardRow(rhlAdmin, "Pilot Safe House") == "" {
			t.Fatal("RHL admin dashboard should list only houses of their RHL")
		}
		if code, _ := get(manager, "/dashboard"); code != http.StatusForbidden {
			t.Fatalf("manager dashboard status %d", code)
		}
	})

	t.Run("editing an occupant after confirmation flips to changed", func(t *testing.T) {
		code, _, _ := do(manager, http.MethodPost, "/occupants/"+strconv.FormatInt(resident.ID, 10)+"/demographics", url.Values{
			"country_of_origin": {"KE"}, "gender": {"F"}, "birth_year": {""},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("edit status %d", code)
		}
		if row := dashboardRow(rhc, "Pilot Safe House"); !strings.Contains(row, "Changed after confirmation") || strings.Contains(row, lastMonth.Format("January 2006")+"</td>") {
			t.Fatalf("row %s", row)
		}
		code, body := get(manager, reportPath)
		if code != http.StatusOK || !strings.Contains(body, "Data changed after confirmation on ") || !strings.Contains(body, "please confirm again") {
			t.Fatalf("status %d body %s", code, body)
		}
		code, body = get(manager, "/reports/"+houseID+"/history")
		if code != http.StatusOK || !strings.Contains(body, "Changed after confirmation") || strings.Count(body, `<a href="/reports?month=`) != 12 {
			t.Fatalf("history status %d body %s", code, body)
		}
		if code, _ := get(manager, "/reports/"+otherID+"/history"); code != http.StatusForbidden {
			t.Fatalf("other history status %d", code)
		}
	})

	t.Run("RHL admin confirms again", func(t *testing.T) {
		code, loc, _ := do(rhlAdmin, http.MethodPost, confirmPath(houseID), url.Values{"fingerprint": {fingerprint(rhlAdmin)}})
		if code != http.StatusSeeOther || !strings.Contains(loc, "ok=confirmed") {
			t.Fatalf("status %d location %q", code, loc)
		}
		if row := dashboardRow(rhlAdmin, "Pilot Safe House"); !strings.Contains(row, ">Confirmed<") || !strings.Contains(row, "Robin RHL") {
			t.Fatalf("row %s", row)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM month_confirmations WHERE safe_house_id = $1`, house.ID).Scan(&n); err != nil || n != 2 {
			t.Fatalf("confirmations %d (%v)", n, err)
		}
	})
}
