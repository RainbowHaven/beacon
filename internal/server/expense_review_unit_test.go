package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

func TestExpenseChanges(t *testing.T) {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	before := domain.Expense{
		AmountCents: 1250, Currency: "USD", SpentOn: day, Merchant: "Market",
		Category: domain.CategoryFood, Note: "veg", ReviewStatus: domain.ExpenseReviewed,
	}
	after := before
	after.AmountCents = 1300
	after.Category = domain.CategoryHouseholdSupplies
	after.SpentOn = day.AddDate(0, 0, 1)
	after.ReviewStatus = domain.ExpenseSubmitted

	got := expenseChanges(before, after)
	want := map[string][2]any{
		"amount_cents":  {int64(1250), int64(1300)},
		"category":      {"food", "household_supplies"},
		"spent_on":      {"2026-09-01", "2026-09-02"},
		"review_status": {"reviewed", "submitted"},
	}
	if len(got) != len(want) {
		t.Fatalf("changes=%v", got)
	}
	for k, w := range want {
		c, ok := got[k]
		if !ok || c["from"] != w[0] || c["to"] != w[1] {
			t.Fatalf("%s: got %v want %v", k, c, w)
		}
	}
	if len(expenseChanges(before, before)) != 0 {
		t.Fatal("expected no changes")
	}
}

func TestExpenseHistoryFromAuditMeta(t *testing.T) {
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	before := domain.Expense{AmountCents: 2000, Currency: "USD", SpentOn: day, Category: domain.CategoryOther, ReviewStatus: domain.ExpenseNeedsCorrection}
	after := before
	after.AmountCents = 2500
	after.Merchant = "Hardware store"
	after.ReviewStatus = domain.ExpenseSubmitted

	// Round-trip through JSON the way audit_events.meta is stored.
	raw, err := json.Marshal(map[string]any{"changes": expenseChanges(before, after), "receipt": "replaced"})
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	rows := expenseHistory([]domain.AuditEvent{
		{Action: "expense.create", ActorEmail: "mgr@example.com"},
		{Action: "expense.update", ActorName: "Agent", Meta: meta},
		{Action: "expense.review", Meta: map[string]any{"to": "needs_correction", "note": "wrong total"}},
	})
	if len(rows) != 3 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].Summary != "Logged" || rows[0].Actor != "mgr@example.com" {
		t.Fatalf("create row=%+v", rows[0])
	}
	edit := strings.Join(rows[1].Lines, "\n")
	for _, s := range []string{
		"Amount: 20.00 → 25.00",
		"Merchant: — → Hardware store",
		"Review status: Needs correction → Submitted",
		"Receipt replaced",
	} {
		if !strings.Contains(edit, s) {
			t.Fatalf("edit lines missing %q:\n%s", s, edit)
		}
	}
	if rows[1].Actor != "Agent" {
		t.Fatalf("actor=%q", rows[1].Actor)
	}
	if rows[2].Summary != "Review: Needs correction" || len(rows[2].Lines) != 1 || rows[2].Lines[0] != "Note: wrong total" || rows[2].Actor != "System" {
		t.Fatalf("review row=%+v", rows[2])
	}
}

func TestExpenseFormParse(t *testing.T) {
	ok := expenseFormValues{Amount: "12.5", Currency: "", SpentOn: "09/30/2026", Category: "rent", Merchant: "Landlord"}
	f, msg := ok.parse("KES")
	if msg != "" || f.AmountCents != 1250 || f.Currency != "KES" || f.Category != domain.CategoryRent {
		t.Fatalf("f=%+v msg=%q", f, msg)
	}
	bad := []expenseFormValues{
		{Amount: "x", SpentOn: "09/30/2026", Category: "rent"},
		{Amount: "1", SpentOn: "30/09/2026", Category: "rent"},
		{Amount: "1", SpentOn: "09/30/2026", Category: ""},
		{Amount: "1", SpentOn: "09/30/2026", Category: "groceries"},
		{Amount: "1", SpentOn: "09/30/2026", Category: "food", Merchant: strings.Repeat("m", maxMerchantLen+1)},
	}
	for _, v := range bad {
		if _, msg := v.parse("USD"); msg == "" {
			t.Fatalf("expected error for %+v", v)
		}
	}
}
