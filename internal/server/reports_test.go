package server_test

import (
	"context"
	"encoding/csv"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/server"
	"github.com/magiconair/beacon/internal/store"
)

func TestMonthlyReport(t *testing.T) {
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
	places := 2
	if err := st.UpdateSafeHouse(ctx, house.ID, rhl.ID, house.Name, house.DefaultCurrency, &places, true); err != nil {
		t.Fatal(err)
	}
	otherRHL, err := st.CreateRHL(ctx, "Other Local", true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateSafeHouse(ctx, otherRHL.ID, "Other House", "USD", true)
	if err != nil {
		t.Fatal(err)
	}

	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	addOccupant := func(houseID int64, nick, arrived, departed, country, gender string, birthYear *int) {
		t.Helper()
		o, err := st.CreateOccupant(ctx, store.CreateOccupantInput{
			SafeHouseID: houseID, Nickname: nick, ArrivedAt: day(arrived),
			CountryOfOrigin: country, Gender: gender, BirthYear: birthYear,
		})
		if err != nil {
			t.Fatal(err)
		}
		if departed != "" {
			if err := st.MarkOccupantDeparted(ctx, o.ID, day(departed)); err != nil {
				t.Fatal(err)
			}
		}
	}
	born := 1999
	addOccupant(house.ID, "Kestrel", "2024-01-20", "2024-03-02", "UG", "M", &born) // 29 nights
	addOccupant(house.ID, "Plover", "2024-02-28", "2024-03-01", "KE", "F", nil)    // 2 nights
	addOccupant(house.ID, "Osprey", "2024-01-10", "2024-02-01", "UG", "M", nil)    // departed on the 1st
	addOccupant(other.ID, "Heron", "2024-02-05", "", "RW", "X", nil)
	if _, err := st.CreateExpense(ctx, store.CreateExpenseInput{
		SafeHouseID: house.ID, AmountCents: 1250, Currency: "USD", SpentOn: day("2024-02-14"),
	}); err != nil {
		t.Fatal(err)
	}

	newUser := func(email string, role domain.Role, rhlID, houseID *int64) domain.User {
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
		return u
	}
	manager := newUser("mgr@example.com", domain.RoleSafeHouseManager, &house.RHLID, &house.ID)
	otherRHLAdmin := newUser("rhl@example.com", domain.RoleRHLAdmin, &otherRHL.ID, nil)
	rhc := newUser("rhc@example.com", domain.RoleRHCAdmin, nil, nil)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	get := func(u domain.User, path string) (int, http.Header, string) {
		t.Helper()
		session, err := st.CreateSession(ctx, u.ID, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "beacon_session", Value: session})
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header, string(b)
	}
	houseID := strconv.FormatInt(house.ID, 10)
	otherID := strconv.FormatInt(other.ID, 10)

	t.Run("manager sees own house", func(t *testing.T) {
		code, _, body := get(manager, "/reports?month=2024-02")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		for _, want := range []string{
			"TOR", "Pilot Safe House", "February 2024",
			"<dt>Residents served</dt><dd>2</dd>",
			"<dt>Bed-nights</dt><dd>31</dd>",
			"<dt>Admissions</dt><dd>1</dd>",
			"<dt>Departures</dt><dd>1</dd>",
			"53.4%",
			"Kestrel", "Plover", "Uganda", "Kenya", "1999",
			"12.50 USD",
			"/reports/" + houseID + "/2024-02.csv",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in %s", want, body)
			}
		}
		for _, notWant := range []string{"Osprey", "Heron", "Month in progress"} {
			if strings.Contains(body, notWant) {
				t.Fatalf("unexpected %q", notWant)
			}
		}
	})

	t.Run("manager cannot open another house", func(t *testing.T) {
		if code, _, _ := get(manager, "/reports?month=2024-02&house="+otherID); code != http.StatusForbidden {
			t.Fatalf("report status %d", code)
		}
		if code, _, _ := get(manager, "/reports/"+otherID+"/2024-02.csv"); code != http.StatusForbidden {
			t.Fatalf("csv status %d", code)
		}
	})

	t.Run("RHL admin cannot open a house of another RHL", func(t *testing.T) {
		if code, _, _ := get(otherRHLAdmin, "/reports/"+houseID+"/2024-02.csv"); code != http.StatusForbidden {
			t.Fatalf("csv status %d", code)
		}
		code, _, body := get(otherRHLAdmin, "/reports?month=2024-02")
		if code != http.StatusOK || !strings.Contains(body, "Other House") || strings.Contains(body, "Kestrel") {
			t.Fatalf("status %d body %s", code, body)
		}
	})

	t.Run("RHC admin sees overview", func(t *testing.T) {
		code, _, body := get(rhc, "/reports?month=2024-02")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		for _, want := range []string{
			"All houses", "Pilot Safe House", "Other House", "53.4%", "not available",
			"/reports?month=2024-02&amp;house=" + houseID,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in %s", want, body)
			}
		}
		if strings.Contains(body, "Kestrel") {
			t.Fatal("overview should not list residents")
		}
	})

	t.Run("CSV export", func(t *testing.T) {
		code, header, body := get(manager, "/reports/"+houseID+"/2024-02.csv")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
			t.Fatalf("content type %q", ct)
		}
		r := csv.NewReader(strings.NewReader(body))
		r.FieldsPerRecord = -1
		records, err := r.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]string{}
		var residents [][]string
		inResidents := false
		for _, rec := range records {
			switch {
			case rec[0] == "Nickname":
				inResidents = true
			case inResidents:
				residents = append(residents, rec)
			case len(rec) == 2:
				fields[rec[0]] = rec[1]
			}
		}
		for k, v := range map[string]string{
			"RHL code":                 "TOR",
			"Safe house":               "Pilot Safe House",
			"Reporting month":          "2024-02",
			"Status":                   "Complete",
			"Approved sleeping places": "2",
			"Residents served":         "2",
			"Bed-nights":               "31",
			"Admissions":               "1",
			"Departures":               "1",
			"Average occupancy":        "53.4%",
			"Gender: M":                "1",
			"Country of origin: Kenya": "1",
			"Expense total USD":        "12.50",
		} {
			if fields[k] != v {
				t.Fatalf("%s = %q, want %q (all: %v)", k, fields[k], v, fields)
			}
		}
		if !strings.HasSuffix(fields["Generated"], " UTC") {
			t.Fatalf("generated %q", fields["Generated"])
		}
		want := [][]string{
			{"Kestrel", "2024-01-20", "2024-03-02", "29", "Uganda", "M", "1999"},
			{"Plover", "2024-02-28", "2024-03-01", "2", "Kenya", "F", ""},
		}
		if len(residents) != len(want) {
			t.Fatalf("residents %v", residents)
		}
		for i := range want {
			if strings.Join(residents[i], ",") != strings.Join(want[i], ",") {
				t.Fatalf("resident %d = %v, want %v", i, residents[i], want[i])
			}
		}
	})

	t.Run("future month is rejected", func(t *testing.T) {
		next := time.Now().UTC().AddDate(0, 2, 0).Format("2006-01")
		code, header, _ := get(manager, "/reports?month="+url.QueryEscape(next))
		if code != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "error=") {
			t.Fatalf("status %d location %q", code, header.Get("Location"))
		}
	})
}
