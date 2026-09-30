package server_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

func TestRetiredCategories(t *testing.T) {
	env := newExpenseTestEnv(t)
	ctx := context.Background()
	house := env.house
	houseID := strconv.FormatInt(house.ID, 10)
	manager := env.client("cat-mgr@example.com", domain.RoleSafeHouseManager, &house.RHLID, &house.ID)

	// The forms offer exactly the categories in the database.
	expenseCats, err := env.st.ExpenseCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, body := env.get(manager, "/expenses/new")
	for _, c := range expenseCats {
		if !strings.Contains(body, `<option value="`+c.Key+`">`+c.Label+`</option>`) {
			t.Fatalf("expense form lacks %q", c.Key)
		}
	}
	issueCats, err := env.st.OperationalIssueCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, body = env.get(manager, "/operations/new")
	for _, c := range issueCats {
		if !strings.Contains(body, `<option value="`+c.Key+`">`+c.Label+`</option>`) {
			t.Fatalf("operations form lacks %q", c.Key)
		}
	}

	expense := map[string]string{
		"safe_house_id":     houseID,
		"currency":          "CAD",
		"amount":            "8.00",
		"spent_on":          "03/05/2026",
		"category":          "transport",
		"no_receipt_reason": "bus fare",
	}
	for range 2 {
		if code, _ := env.postMultipart(manager, "/expenses", expense, nil); code != http.StatusSeeOther {
			t.Fatalf("create expense status %d", code)
		}
	}
	list, err := env.st.ListExpensesByHouses(ctx, []int64{house.ID}, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	kept, edited := list[0], list[1]
	editedPath := "/expenses/" + strconv.FormatInt(edited.ID, 10)
	expense["category"] = "food"
	if code, _ := env.postMultipart(manager, editedPath, expense, nil); code != http.StatusSeeOther {
		t.Fatalf("edit expense status %d", code)
	}
	if _, err := env.st.CreateOperationalIssue(ctx, house.ID, store.OperationalIssueFields{
		IdentifiedOn: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), Category: "staffing_agent", Description: "Agent on leave",
	}, nil); err != nil {
		t.Fatal(err)
	}

	db := env.st.DB()
	for _, q := range []string{
		`UPDATE expense_categories SET active = false WHERE key = 'transport'`,
		`UPDATE operational_issue_categories SET active = false WHERE key = 'staffing_agent'`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	// Retired categories are no longer offered or accepted.
	if _, body := env.get(manager, "/expenses/new"); strings.Contains(body, `value="transport"`) || !strings.Contains(body, `value="food"`) {
		t.Fatal("expense form still offers transport")
	}
	expense["category"] = "transport"
	code, body := env.postMultipart(manager, "/expenses", expense, nil)
	if code != http.StatusOK || !strings.Contains(body, "Pick a category.") {
		t.Fatalf("create with retired category: status=%d", code)
	}
	if list, _ := env.st.ListExpensesByHouses(ctx, []int64{house.ID}, 10); len(list) != 2 {
		t.Fatalf("expense saved with retired category: %+v", list)
	}
	if _, body := env.get(manager, "/operations/new"); strings.Contains(body, `value="staffing_agent"`) {
		t.Fatal("operations form still offers staffing_agent")
	}
	code, body = env.postForm(manager, "/operations", url.Values{
		"safe_house_id": {houseID}, "identified_on": {"2026-03-03"}, "category": {"staffing_agent"},
		"description": {"Agent change"}, "status": {"open"},
	})
	if code != http.StatusOK || !strings.Contains(body, "Choose a category.") {
		t.Fatalf("create issue with retired category: status=%d", code)
	}

	// Existing records keep their label everywhere.
	if _, body := env.get(manager, "/expenses"); !strings.Contains(body, "<td>Transport</td>") {
		t.Fatal("expense list lacks the retired label")
	}
	_, body = env.get(manager, "/expenses/"+strconv.FormatInt(kept.ID, 10)+"/edit")
	if !strings.Contains(body, "· Transport</p>") || strings.Contains(body, `value="transport"`) ||
		!strings.Contains(body, `<option value="" selected disabled>Choose a category</option>`) {
		t.Fatal("edit page of a retired-category expense")
	}
	if _, body := env.get(manager, editedPath+"/edit"); !strings.Contains(body, "Category: Transport → Food") {
		t.Fatal("expense history lacks the retired label")
	}
	if _, body := env.get(manager, "/operations?status=all"); !strings.Contains(body, "Staffing or Agent change") {
		t.Fatal("operations list lacks the retired label")
	}
	if _, body := env.get(manager, "/reports?month=2026-03"); !strings.Contains(body, "<td>Staffing or Agent change</td>") {
		t.Fatal("report lacks the retired label")
	}
	if _, body := env.get(manager, "/reports/"+houseID+"/2026-03.csv"); !strings.Contains(body, "Staffing or Agent change") {
		t.Fatal("CSV lacks the retired label")
	}
}
