package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

func TestCategoryTablesSeeded(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		load func(context.Context) (domain.Categories, error)
		want [][2]string
	}{
		{"expense", st.ExpenseCategories, [][2]string{
			{"food", "Food"},
			{"household_supplies", "Household supplies"},
			{"utilities", "Utilities"},
			{"rent", "Rent"},
			{"maintenance_repairs", "Maintenance and repairs"},
			{"transport", "Transport"},
			{"communication", "Communication"},
			{"other", "Other"},
		}},
		{"operational issue", st.OperationalIssueCategories, [][2]string{
			{"building_maintenance", "Building or maintenance problem"},
			{"utilities", "Utilities"},
			{"safety_security", "Safety or security issue that is not a safeguarding concern"},
			{"food_supplies", "Food or supplies"},
			{"staffing_agent", "Staffing or Agent change"},
			{"capacity_occupancy", "Capacity or occupancy change"},
			{"service_availability", "Service availability"},
			{"other_change", "Other significant operational change"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cats, err := tc.load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]string
			for _, c := range cats {
				got = append(got, [2]string{c.Key, c.Label})
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("categories=%v\nwant %v", got, tc.want)
			}
			if len(cats.Offered()) != len(cats) {
				t.Fatalf("seeded categories not all offered: %+v", cats)
			}
		})
	}
}

func TestCategoryForeignKeys(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	_, house, err := st.EnsureDemoTenancy(ctx)
	if err != nil {
		t.Fatal(err)
	}

	in := store.CreateExpenseInput{
		SafeHouseID: house.ID, AmountCents: 500, Currency: "CAD", Category: "groceries",
		NoReceiptReason: "market", SpentOn: day("2026-03-01"),
	}
	if _, err := st.CreateExpense(ctx, in); !errors.Is(err, store.ErrExpenseCategory) {
		t.Fatalf("create with unknown category: %v", err)
	}
	in.Category = ""
	e, err := st.CreateExpense(ctx, in)
	if err != nil || e.Category != domain.ExpenseCategoryOther || e.CategoryLabel != "Other" {
		t.Fatalf("default category: %+v err=%v", e, err)
	}
	_, _, err = st.UpdateExpense(ctx, e.ID, store.UpdateExpenseInput{
		AmountCents: 500, Currency: "CAD", Category: "groceries", NoReceiptReason: "market", SpentOn: e.SpentOn,
	})
	if !errors.Is(err, store.ErrExpenseCategory) {
		t.Fatalf("update with unknown category: %v", err)
	}

	issue := store.OperationalIssueFields{IdentifiedOn: day("2026-03-01"), Category: "residents", Description: "x"}
	if _, err := st.CreateOperationalIssue(ctx, house.ID, issue, nil); !errors.Is(err, store.ErrOperationalIssueCategory) {
		t.Fatalf("create issue with unknown category: %v", err)
	}
	issue.Category = "utilities"
	o, err := st.CreateOperationalIssue(ctx, house.ID, issue, nil)
	if err != nil || o.CategoryLabel != "Utilities" {
		t.Fatalf("issue=%+v err=%v", o, err)
	}
	issue.Category = "residents"
	if _, err := st.UpdateOperationalIssue(ctx, o.ID, issue, nil); !errors.Is(err, store.ErrOperationalIssueCategory) {
		t.Fatalf("update issue with unknown category: %v", err)
	}

	db := st.DB()
	if _, err := db.ExecContext(ctx, `DELETE FROM expense_categories WHERE key = 'other'`); err == nil {
		t.Fatal("deleted a category that is in use")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO expense_categories (key, label, sort_order) VALUES ('Bad-Key', 'Bad', 90)`); err == nil {
		t.Fatal("accepted a key that is not snake case")
	}

	// Labels are read at display time; key renames cascade to existing rows.
	if _, err := db.ExecContext(ctx, `UPDATE expense_categories SET label = 'Other costs' WHERE key = 'other'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE operational_issue_categories SET key = 'utilities_outage' WHERE key = 'utilities'`); err != nil {
		t.Fatal(err)
	}
	if e, err = st.GetExpense(ctx, e.ID); err != nil || e.CategoryLabel != "Other costs" {
		t.Fatalf("relabelled expense=%+v err=%v", e, err)
	}
	if o, err = st.GetOperationalIssue(ctx, o.ID); err != nil || o.Category != "utilities_outage" || o.CategoryLabel != "Utilities" {
		t.Fatalf("rekeyed issue=%+v err=%v", o, err)
	}
}
