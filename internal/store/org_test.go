package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magiconair/beacon/internal/store"
)

func TestRHLCodeUniqueAndNormalized(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	a, err := st.CreateRHLWithCode(ctx, "Toronto", " tor ", true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != "TOR" {
		t.Fatalf("code = %q, want TOR", a.Code)
	}
	if _, err := st.CreateRHLWithCode(ctx, "Toronto 2", "TOR", true); !errors.Is(err, store.ErrRHLCodeTaken) {
		t.Fatalf("duplicate create: err = %v, want ErrRHLCodeTaken", err)
	}
	if _, err := st.CreateRHLWithCode(ctx, "Bad", "t o r", true); !errors.Is(err, store.ErrRHLCodeInvalid) {
		t.Fatalf("invalid create: err = %v, want ErrRHLCodeInvalid", err)
	}

	// Several RHLs without a code must not collide.
	b, err := st.CreateRHL(ctx, "Vancouver", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRHL(ctx, "Montreal", true); err != nil {
		t.Fatal(err)
	}
	if b.Code != "" {
		t.Fatalf("code = %q, want empty", b.Code)
	}

	if err := st.UpdateRHL(ctx, b.ID, "Vancouver", "tor", true); !errors.Is(err, store.ErrRHLCodeTaken) {
		t.Fatalf("duplicate update: err = %v, want ErrRHLCodeTaken", err)
	}
	if err := st.UpdateRHL(ctx, b.ID, "Vancouver", "van", false); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRHL(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "VAN" || got.Active {
		t.Fatalf("got %+v", got)
	}

	if err := st.UpdateRHL(ctx, a.ID, "Toronto", "", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetRHL(ctx, a.ID); got.Code != "" {
		t.Fatalf("code not cleared: %q", got.Code)
	}
	if _, err := st.CreateRHLWithCode(ctx, "Toronto 2", "TOR", true); err != nil {
		t.Fatalf("code free after clear: %v", err)
	}
}

func TestSafeHouseApprovedSleepingPlaces(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	rhl, err := st.CreateRHL(ctx, "Toronto", true)
	if err != nil {
		t.Fatal(err)
	}
	six := 6
	h, err := st.CreateSafeHouseWithCapacity(ctx, rhl.ID, "House A", "CAD", &six, true)
	if err != nil {
		t.Fatal(err)
	}
	if h.ApprovedSleepingPlaces == nil || *h.ApprovedSleepingPlaces != 6 {
		t.Fatalf("create: got %v", h.ApprovedSleepingPlaces)
	}

	if err := st.UpdateSafeHouse(ctx, h.ID, rhl.ID, "House A", "CAD", nil, true); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSafeHouse(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ApprovedSleepingPlaces != nil {
		t.Fatalf("not cleared: %d", *got.ApprovedSleepingPlaces)
	}

	eight := 8
	if err := st.UpdateSafeHouse(ctx, h.ID, rhl.ID, "House A", "CAD", &eight, true); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListSafeHousesByRHL(ctx, rhl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ApprovedSleepingPlaces == nil || *list[0].ApprovedSleepingPlaces != 8 {
		t.Fatalf("list: %+v", list)
	}

	zero := 0
	if _, err := st.CreateSafeHouseWithCapacity(ctx, rhl.ID, "House B", "CAD", &zero, true); err == nil {
		t.Fatal("expected check violation for zero sleeping places")
	}
}
