package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func dayPtr(s string) *time.Time {
	t := day(s)
	return &t
}

func TestNormalizeOperationalIssueFields(t *testing.T) {
	base := store.OperationalIssueFields{
		IdentifiedOn: day("2026-03-10"),
		Category:     "utilities",
		Description:  "  Water off  ",
	}

	got, err := store.NormalizeOperationalIssueFields(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.OperationalIssueOpen || got.Description != "Water off" {
		t.Fatalf("defaults=%+v", got)
	}

	reopened := base
	reopened.Status = "open"
	reopened.ClosedOn = dayPtr("2026-03-12")
	reopened.ClosureNotes = "fixed"
	got, err = store.NormalizeOperationalIssueFields(reopened)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClosedOn != nil || got.ClosureNotes != "" {
		t.Fatalf("open should clear closure: %+v", got)
	}

	resolved := base
	resolved.Status = "resolved"
	resolved.ClosedOn = dayPtr("2026-03-10")
	resolved.ClosureNotes = "Plumber came"
	if _, err := store.NormalizeOperationalIssueFields(resolved); err != nil {
		t.Fatalf("same-day resolution: %v", err)
	}

	cases := []struct {
		name string
		mod  func(f *store.OperationalIssueFields)
		want string
	}{
		{"no date", func(f *store.OperationalIssueFields) { f.IdentifiedOn = time.Time{} }, "Date identified"},
		{"bad category", func(f *store.OperationalIssueFields) { f.Category = "residents" }, "category"},
		{"no description", func(f *store.OperationalIssueFields) { f.Description = "   " }, "description"},
		{"long description", func(f *store.OperationalIssueFields) { f.Description = strings.Repeat("a", 2001) }, "at most"},
		{"bad status", func(f *store.OperationalIssueFields) { f.Status = "pending" }, "status"},
		{"resolved without date", func(f *store.OperationalIssueFields) {
			f.Status = "resolved"
			f.ClosureNotes = "done"
		}, "date is required"},
		{"closed without notes", func(f *store.OperationalIssueFields) {
			f.Status = "closed"
			f.ClosedOn = dayPtr("2026-03-11")
		}, "notes are required"},
		{"closed before identified", func(f *store.OperationalIssueFields) {
			f.Status = "closed"
			f.ClosedOn = dayPtr("2026-03-09")
			f.ClosureNotes = "done"
		}, "before the date identified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := base
			tc.mod(&f)
			_, err := store.NormalizeOperationalIssueFields(f)
			var ve store.ValidationError
			if !errors.As(err, &ve) || !strings.Contains(ve.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}

func TestOperationalIssuesCRUDAndActiveInRange(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	rhl, house, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateSafeHouse(ctx, rhl.ID, "Other House", "CAD", true)
	if err != nil {
		t.Fatal(err)
	}

	create := func(houseID int64, identified string, status string, closed string) domain.OperationalIssue {
		t.Helper()
		f := store.OperationalIssueFields{
			IdentifiedOn: day(identified),
			Category:     "building_maintenance",
			Description:  "Issue identified " + identified,
			Status:       status,
		}
		if closed != "" {
			f.ClosedOn = dayPtr(closed)
			f.ClosureNotes = "done"
		}
		o, err := st.CreateOperationalIssue(ctx, houseID, f, nil)
		if err != nil {
			t.Fatalf("create %s: %v", identified, err)
		}
		return o
	}

	create(house.ID, "2026-01-05", "resolved", "2026-02-28") // resolved before the month
	resolvedOnStart := create(house.ID, "2026-02-10", "resolved", "2026-03-01")
	resolvedMid := create(house.ID, "2026-02-20", "closed", "2026-03-15")
	openOld := create(house.ID, "2025-12-01", "open", "")
	identifiedMid := create(house.ID, "2026-03-20", "open", "")
	identifiedOnEnd := create(house.ID, "2026-03-31", "open", "")
	identifiedAfter := create(house.ID, "2026-04-01", "open", "")
	otherHouse := create(other.ID, "2026-03-02", "open", "")

	got, err := st.OperationalIssuesActiveInRange(ctx, []int64{house.ID}, day("2026-03-01"), day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int64{openOld.ID, resolvedOnStart.ID, resolvedMid.ID, identifiedMid.ID, identifiedOnEnd.ID}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d issues %+v want %v", len(got), got, wantIDs)
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("index %d id=%d want %d (got %+v)", i, got[i].ID, id, got)
		}
	}
	if got[0].SafeHouseName != house.Name {
		t.Fatalf("house name=%q", got[0].SafeHouseName)
	}

	both, err := st.OperationalIssuesActiveInRange(ctx, []int64{house.ID, other.ID}, day("2026-03-01"), day("2026-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 6 || both[0].SafeHouseID != other.ID || both[0].ID != otherHouse.ID {
		t.Fatalf("multi-house order: %+v", both)
	}

	if none, err := st.OperationalIssuesActiveInRange(ctx, nil, day("2026-03-01"), day("2026-03-31")); err != nil || none != nil {
		t.Fatalf("no houses: %v %v", none, err)
	}

	open, err := st.ListOperationalIssuesByHouses(ctx, []int64{house.ID}, "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 4 || open[0].ID != identifiedAfter.ID {
		t.Fatalf("open list: %+v", open)
	}
	all, err := st.ListOperationalIssuesByHouses(ctx, []int64{house.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 7 {
		t.Fatalf("all list: %d", len(all))
	}

	updated, err := st.UpdateOperationalIssue(ctx, identifiedMid.ID, store.OperationalIssueFields{
		IdentifiedOn: day("2026-03-20"),
		Category:     "utilities",
		Description:  "Power cut",
		Effect:       "No lights",
		Status:       "resolved",
		ClosedOn:     dayPtr("2026-03-22"),
		ClosureNotes: "Restored",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "resolved" || updated.ClosedOn == nil || !updated.ClosedOn.Equal(day("2026-03-22")) || updated.Effect != "No lights" {
		t.Fatalf("updated=%+v", updated)
	}

	reopened, err := st.UpdateOperationalIssue(ctx, identifiedMid.ID, store.OperationalIssueFields{
		IdentifiedOn: day("2026-03-20"),
		Category:     "utilities",
		Description:  "Power cut",
		Status:       "open",
		ClosedOn:     dayPtr("2026-03-22"),
		ClosureNotes: "Restored",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "open" || reopened.ClosedOn != nil || reopened.ClosureNotes != "" {
		t.Fatalf("reopened=%+v", reopened)
	}

	if _, err := st.UpdateOperationalIssue(ctx, 999999, store.OperationalIssueFields{
		IdentifiedOn: day("2026-03-20"), Category: "utilities", Description: "x",
	}, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing update err=%v", err)
	}
	if _, err := st.GetOperationalIssue(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing get err=%v", err)
	}

	if _, err := st.DB().ExecContext(ctx, `
		INSERT INTO operational_issues (safe_house_id, identified_on, category, description, status)
		VALUES ($1, '2026-03-01', 'utilities', 'x', 'closed')`, house.ID); err == nil {
		t.Fatal("database accepted closed issue without closure date")
	}
}
