package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

func openConcern(id, reported string) store.SafeguardingInput {
	return store.SafeguardingInput{
		IncidentID:        id,
		OccurredPrecision: domain.DateUnknown,
		ReportedOn:        day(reported),
		Status:            domain.SafeguardingOpen,
	}
}

func TestSafeguardingCreateUpdate(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	rhl, err := st.CreateRHLWithCode(ctx, "Toronto", "TOR", true)
	if err != nil {
		t.Fatal(err)
	}
	h, err := st.CreateSafeHouse(ctx, rhl.ID, "House A", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}

	in := store.SafeguardingInput{
		IncidentID:        " Incident-TOR-2026-1 ",
		OccurredOn:        dayPtr("2026-03-01"),
		OccurredPrecision: domain.DateApproximate,
		ReportedOn:        day("2026-03-02"),
		Status:            domain.SafeguardingOpen,
	}
	c, err := st.CreateSafeguardingConcern(ctx, h.ID, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.IncidentID != "Incident-TOR-2026-1" || c.OccurredPrecision != domain.DateApproximate ||
		c.OccurredOn == nil || !c.OccurredOn.Equal(day("2026-03-01")) || !c.ReportedOn.Equal(day("2026-03-02")) ||
		c.Status != domain.SafeguardingOpen || c.ClosedOn != nil {
		t.Fatalf("created %+v", c)
	}

	if _, err := st.CreateSafeguardingConcern(ctx, h.ID, openConcern("incident-tor-2026-1", "2026-03-05"), nil); !errors.Is(err, store.ErrIncidentIDTaken) {
		t.Fatalf("duplicate (case-insensitive): err = %v", err)
	}
	if _, err := st.CreateSafeguardingConcern(ctx, h.ID, openConcern("a\nb", "2026-03-05"), nil); !errors.Is(err, store.ErrIncidentIDInvalid) {
		t.Fatalf("newline: err = %v", err)
	}

	bad := openConcern("Incident-TOR-2026-9", "2026-03-05")
	bad.Status = domain.SafeguardingResolved
	if _, err := st.CreateSafeguardingConcern(ctx, h.ID, bad, nil); err == nil {
		t.Fatal("resolved without closed date must fail")
	}
	bad.ClosedOn = dayPtr("2026-03-04")
	if _, err := st.CreateSafeguardingConcern(ctx, h.ID, bad, nil); err == nil {
		t.Fatal("closed before reported must fail")
	}
	bad = openConcern("Incident-TOR-2026-9", "2026-03-05")
	bad.OccurredPrecision = domain.DateExact
	if _, err := st.CreateSafeguardingConcern(ctx, h.ID, bad, nil); err == nil {
		t.Fatal("exact precision without a date must fail")
	}

	up := in
	up.Status = domain.SafeguardingClosed
	up.ClosedOn = dayPtr("2026-04-10")
	up.OccurredOn = nil
	up.OccurredPrecision = domain.DateUnknown
	got, err := st.UpdateSafeguardingConcern(ctx, c.ID, up, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.SafeguardingClosed || got.ClosedOn == nil || !got.ClosedOn.Equal(day("2026-04-10")) ||
		got.OccurredOn != nil || got.SafeHouseID != h.ID {
		t.Fatalf("updated %+v", got)
	}
	if _, err := st.UpdateSafeguardingConcern(ctx, c.ID+1000, up, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: err = %v", err)
	}

	open, err := st.ListSafeguardingConcerns(ctx, []int64{h.ID}, []domain.SafeguardingStatus{domain.SafeguardingOpen})
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("open list %+v", open)
	}
	all, err := st.ListSafeguardingConcerns(ctx, []int64{h.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != c.ID {
		t.Fatalf("all list %+v", all)
	}
}

func TestNextIncidentSequence(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	rhl, err := st.CreateRHLWithCode(ctx, "Toronto", "TOR", true)
	if err != nil {
		t.Fatal(err)
	}
	h, err := st.CreateSafeHouse(ctx, rhl.ID, "House A", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}

	if n, err := st.NextIncidentSequence(ctx, "TOR", 2026); err != nil || n != 1 {
		t.Fatalf("empty: n=%d err=%v", n, err)
	}
	for _, id := range []string{
		"Incident-TOR-2026-1",
		"incident-tor-2026-12",
		"Incident-TOR-2025-40",
		"Incident-TORX-2026-99",
		"Incident-TOR-2026-7-a",
		"Custom 2026",
	} {
		if _, err := st.CreateSafeguardingConcern(ctx, h.ID, openConcern(id, "2026-01-10"), nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if n, err := st.NextIncidentSequence(ctx, "TOR", 2026); err != nil || n != 13 {
		t.Fatalf("2026: n=%d err=%v", n, err)
	}
	if n, err := st.NextIncidentSequence(ctx, "TOR", 2025); err != nil || n != 41 {
		t.Fatalf("2025: n=%d err=%v", n, err)
	}
}

func TestSafeguardingMonthQueries(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	rhl, err := st.CreateRHL(ctx, "Toronto", true)
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.CreateSafeHouse(ctx, rhl.ID, "House A", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateSafeHouse(ctx, rhl.ID, "House B", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}

	mk := func(house int64, id, reported string, status domain.SafeguardingStatus, closed string) {
		t.Helper()
		in := openConcern(id, reported)
		in.Status = status
		if closed != "" {
			in.ClosedOn = dayPtr(closed)
		}
		if _, err := st.CreateSafeguardingConcern(ctx, house, in, nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	mk(a.ID, "open-since-jan", "2026-01-15", domain.SafeguardingOpen, "")
	mk(a.ID, "resolved-feb", "2026-01-20", domain.SafeguardingResolved, "2026-02-10")
	mk(a.ID, "closed-mar-first", "2026-02-20", domain.SafeguardingClosed, "2026-03-01")
	mk(a.ID, "reported-mar", "2026-03-31", domain.SafeguardingOpen, "")
	mk(a.ID, "reported-and-resolved-mar", "2026-03-03", domain.SafeguardingResolved, "2026-03-04")
	mk(a.ID, "reported-apr", "2026-04-01", domain.SafeguardingOpen, "")
	mk(b.ID, "other-house-mar", "2026-03-10", domain.SafeguardingOpen, "")

	ids := func(cs []domain.SafeguardingConcern) []string {
		out := make([]string, len(cs))
		for i, c := range cs {
			out[i] = c.IncidentID
		}
		return out
	}

	march, err := st.SafeguardingConcernsForMonth(ctx, []int64{a.ID}, day("2026-03-01"), day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"open-since-jan", "closed-mar-first", "reported-and-resolved-mar", "reported-mar"}
	if got := ids(march); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("march = %v, want %v", got, want)
	}

	feb, err := st.SafeguardingConcernsForMonth(ctx, []int64{a.ID}, day("2026-02-01"), day("2026-02-28"))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(feb); len(got) != 3 || got[0] != "open-since-jan" || got[1] != "resolved-feb" || got[2] != "closed-mar-first" {
		t.Fatalf("feb = %v", got)
	}

	both, err := st.SafeguardingConcernsForMonth(ctx, []int64{a.ID, b.ID}, day("2026-03-01"), day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 5 {
		t.Fatalf("both houses = %v", ids(both))
	}

	n, err := st.CountSafeguardingConcernsReported(ctx, []int64{a.ID}, day("2026-03-01"), day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("reported in march = %d, want 2", n)
	}
	n, err = st.CountSafeguardingConcernsReported(ctx, []int64{a.ID, b.ID}, day("2026-03-01"), day("2026-03-31"))
	if err != nil || n != 3 {
		t.Fatalf("reported in march, both houses = %d, %v", n, err)
	}
	if n, err := st.CountSafeguardingConcernsReported(ctx, nil, day("2026-03-01"), day("2026-03-31")); err != nil || n != 0 {
		t.Fatalf("no houses = %d, %v", n, err)
	}
}
