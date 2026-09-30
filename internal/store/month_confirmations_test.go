package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
)

func TestMonthFingerprint(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	rhl, err := st.CreateRHLWithCode(ctx, "Toronto", "TOR", true)
	if err != nil {
		t.Fatal(err)
	}
	house, err := st.CreateSafeHouse(ctx, rhl.ID, "House A", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateSafeHouse(ctx, rhl.ID, "House B", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}
	feb := day("2026-02-01")

	occupant := func(houseID int64, nick, arrived string) domain.Occupant {
		t.Helper()
		o, err := st.CreateOccupant(ctx, store.CreateOccupantInput{SafeHouseID: houseID, Nickname: nick, ArrivedAt: day(arrived)})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	expense := func(houseID int64, spent string, cents int64) domain.Expense {
		t.Helper()
		e, err := st.CreateExpense(ctx, store.CreateExpenseInput{SafeHouseID: houseID, AmountCents: cents, Currency: "CAD", SpentOn: day(spent)})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	kestrel := occupant(house.ID, "Kestrel", "2026-01-20")
	march := occupant(house.ID, "Plover", "2026-03-05")
	elsewhere := occupant(other.ID, "Heron", "2026-02-10")
	febExpense := expense(house.ID, "2026-02-14", 1250)
	expense(house.ID, "2026-02-20", 900)
	concern, err := st.CreateSafeguardingConcern(ctx, house.ID, openConcern("Incident-TOR-2026-1", "2026-02-03"), nil)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := st.CreateOperationalIssue(ctx, house.ID, store.OperationalIssueFields{
		IdentifiedOn: day("2026-02-02"), Category: "utilities", Description: "Boiler failure",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	fingerprint := func() string {
		t.Helper()
		fp, err := st.MonthFingerprint(ctx, house.ID, feb)
		if err != nil {
			t.Fatal(err)
		}
		return fp
	}
	base := fingerprint()
	if len(base) != 64 || fingerprint() != base {
		t.Fatalf("fingerprint %q is not stable", base)
	}
	same := func(what string) {
		t.Helper()
		if fp := fingerprint(); fp != base {
			t.Fatalf("%s changed the February fingerprint", what)
		}
	}
	changed := func(what string) {
		t.Helper()
		fp := fingerprint()
		if fp == base {
			t.Fatalf("%s did not change the February fingerprint", what)
		}
		base = fp
	}

	demo := store.UpdateOccupantDemographicsInput{CountryOfOrigin: "UG", Gender: "M"}
	if _, err := st.UpdateOccupantDemographics(ctx, march.ID, demo); err != nil {
		t.Fatal(err)
	}
	same("editing a resident of another month")
	if _, err := st.UpdateOccupantDemographics(ctx, elsewhere.ID, demo); err != nil {
		t.Fatal(err)
	}
	same("editing a resident of another house")
	expense(house.ID, "2026-03-01", 500)
	expense(other.ID, "2026-02-14", 500)
	same("expenses in another month or house")
	if err := st.MarkOccupantDeparted(ctx, kestrel.ID, day("2026-04-02")); err != nil {
		t.Fatal(err)
	}
	same("a departure after the month")
	closedLater := openConcern(concern.IncidentID, "2026-02-03")
	closedLater.Status, closedLater.ClosedOn = domain.SafeguardingClosed, dayPtr("2026-04-10")
	if _, err := st.UpdateSafeguardingConcern(ctx, concern.ID, closedLater, nil); err != nil {
		t.Fatal(err)
	}
	same("closing a concern after the month")

	if _, err := st.UpdateOccupantDemographics(ctx, kestrel.ID, demo); err != nil {
		t.Fatal(err)
	}
	changed("editing a resident in the month")
	if err := st.DeleteExpense(ctx, febExpense.ID); err != nil {
		t.Fatal(err)
	}
	changed("deleting an expense in the month")
	closedInMonth := closedLater
	closedInMonth.ClosedOn = dayPtr("2026-02-20")
	if _, err := st.UpdateSafeguardingConcern(ctx, concern.ID, closedInMonth, nil); err != nil {
		t.Fatal(err)
	}
	changed("closing a concern in the month")
	if _, err := st.UpdateOperationalIssue(ctx, issue.ID, store.OperationalIssueFields{
		IdentifiedOn: day("2026-02-02"), Category: "utilities", Description: "Boiler failure", ActionTaken: "Called a plumber",
	}, nil); err != nil {
		t.Fatal(err)
	}
	changed("editing an operational issue active in the month")
	places := 4
	if err := st.UpdateSafeHouse(ctx, house.ID, rhl.ID, house.Name, house.DefaultCurrency, &places, true); err != nil {
		t.Fatal(err)
	}
	changed("changing the approved sleeping places")
}

func TestConfirmMonthKeepsHistory(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	_, house, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{
		Email: "mgr@example.com", DisplayName: "Alex", Role: domain.RoleSafeHouseManager, RHLID: &house.RHLID, SafeHouseID: &house.ID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	fp := func(c byte) string {
		b := make([]byte, 64)
		for i := range b {
			b[i] = c
		}
		return string(b)
	}
	for _, in := range []store.MonthConfirmationInput{
		{SafeHouseID: house.ID, Month: day("2026-01-15"), Fingerprint: fp('a'), ConfirmedBy: u.ID},
		{SafeHouseID: house.ID, Month: day("2026-02-01"), Fingerprint: fp('b'), ConfirmedBy: u.ID},
		{SafeHouseID: house.ID, Month: day("2026-02-01"), Fingerprint: fp('c'), ConfirmedBy: u.ID},
	} {
		if _, err := st.ConfirmMonth(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := st.DB().QueryRowContext(ctx, `SELECT count(*) FROM month_confirmations`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("history rows %d (%v)", n, err)
	}
	latest, err := st.LatestMonthConfirmations(ctx, []int64{house.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 {
		t.Fatalf("latest %+v", latest)
	}
	if latest[0].Month.Format("2006-01-02") != "2026-02-01" || latest[0].Fingerprint != fp('c') || latest[0].ConfirmedByName != "Alex" {
		t.Fatalf("latest February %+v", latest[0])
	}
	if latest[1].Month.Format("2006-01-02") != "2026-01-01" || latest[1].Fingerprint != fp('a') {
		t.Fatalf("latest January %+v", latest[1])
	}
}
