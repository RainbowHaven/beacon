package server_test

import (
	"context"
	"encoding/csv"
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
		NoReceiptReason: "market stall",
	}); err != nil {
		t.Fatal(err)
	}
	dayPtr := func(s string) *time.Time {
		d := day(s)
		return &d
	}
	addConcern := func(houseID int64, id, reported, status, closed string) {
		t.Helper()
		in := store.SafeguardingInput{
			IncidentID: id, OccurredPrecision: domain.DateUnknown,
			ReportedOn: day(reported), Status: domain.SafeguardingStatus(status),
		}
		if closed != "" {
			in.ClosedOn = dayPtr(closed)
		}
		if _, err := st.CreateSafeguardingConcern(ctx, houseID, in, nil); err != nil {
			t.Fatal(err)
		}
	}
	addConcern(house.ID, "Incident-TOR-2024-1", "2024-01-15", "open", "")
	addConcern(house.ID, "Incident-TOR-2024-2", "2024-02-10", "closed", "2024-02-20")
	addConcern(house.ID, "Incident-TOR-2024-3", "2024-01-05", "resolved", "2024-01-25")
	addConcern(other.ID, "Incident-OTH-2024-1", "2024-02-03", "open", "")
	addIssue := func(houseID int64, identified, category, desc, status, closed string) {
		t.Helper()
		f := store.OperationalIssueFields{
			IdentifiedOn: day(identified), Category: category, Description: desc,
			Effect: "Effect of " + desc, ActionTaken: "Fixing " + desc, RHLRequest: "Help with " + desc,
			Status: status,
		}
		if closed != "" {
			f.ClosedOn = dayPtr(closed)
			f.ClosureNotes = "Done."
		}
		if _, err := st.CreateOperationalIssue(ctx, houseID, f, nil); err != nil {
			t.Fatal(err)
		}
	}
	addIssue(house.ID, "2024-01-03", "building_maintenance", "Leaking roof", "resolved", "2024-01-20")
	addIssue(house.ID, "2024-01-28", "utilities", "Boiler failure", "resolved", "2024-02-12")
	addIssue(house.ID, "2024-02-15", "utilities", "Water outage", "closed", "2024-03-04")
	addIssue(house.ID, "2024-02-20", "staffing_agent", "New agent", "open", "")
	addIssue(house.ID, "2024-03-10", "food_supplies", "Food shortage", "open", "")

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
			"<dt>Concerns reported this month</dt><dd>1</dd>",
			"Incident-TOR-2024-1", "Incident-TOR-2024-2",
			"Details are held in Document 37 by the RHL Safeguarding Contact.",
			"Boiler failure", "Effect of Boiler failure", "Fixing Boiler failure", "Help with Boiler failure",
			`<span class="report-status">Resolved 2024-02-12</span>`,
			"Water outage", `<span class="report-status">Closed 2024-03-04</span>`,
			"New agent", "Staffing or Agent change",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in %s", want, body)
			}
		}
		for _, notWant := range []string{"Osprey", "Heron", "Month in progress", "Incident-TOR-2024-3", "Incident-OTH-2024-1", "Leaking roof", "Food shortage"} {
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
		if !strings.Contains(body, "Incident-OTH-2024-1") || strings.Contains(body, "Incident-TOR") || strings.Contains(body, "Boiler failure") {
			t.Fatalf("safeguarding or operations of another RHL in %s", body)
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
		cells := regexp.MustCompile(`>\s+<`).ReplaceAllString(body, "><")
		for _, want := range []string{
			"<th>Safeguarding reported</th><th>Open problems</th>",
			"53.4%</td><td>1</td><td>2</td>",
			`not available</span></td><td>1</td><td>0</td>`,
			"<td></td><td><strong>2</strong></td><td><strong>2</strong></td>",
		} {
			if !strings.Contains(cells, want) {
				t.Fatalf("missing %q in %s", want, cells)
			}
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
		tables := map[string][][]string{}
		table := ""
		for _, rec := range records {
			switch {
			case rec[0] == "Nickname" || rec[0] == "Incident identifier" || rec[0] == "Date identified":
				table = rec[0]
			case table != "":
				tables[table] = append(tables[table], rec)
			case len(rec) == 2:
				fields[rec[0]] = rec[1]
			}
		}
		residents := tables["Nickname"]
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

			"Safeguarding concerns reported": "1",
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
		for name, want := range map[string][][]string{
			"Incident identifier": {
				{"Incident-TOR-2024-1", "2024-01-15", "Open", ""},
				{"Incident-TOR-2024-2", "2024-02-10", "Closed", "2024-02-20"},
			},
			"Date identified": {
				{"2024-01-28", "Utilities", "Boiler failure", "Effect of Boiler failure", "Resolved", "2024-02-12", "Fixing Boiler failure", "Help with Boiler failure"},
				{"2024-02-15", "Utilities", "Water outage", "Effect of Water outage", "Closed", "2024-03-04", "Fixing Water outage", "Help with Water outage"},
				{"2024-02-20", "Staffing or Agent change", "New agent", "Effect of New agent", "Open", "", "Fixing New agent", "Help with New agent"},
			},
		} {
			got := tables[name]
			if len(got) != len(want) {
				t.Fatalf("%s table %v, want %v", name, got, want)
			}
			for i := range want {
				if strings.Join(got[i], "|") != strings.Join(want[i], "|") {
					t.Fatalf("%s row %d = %v, want %v", name, i, got[i], want[i])
				}
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
