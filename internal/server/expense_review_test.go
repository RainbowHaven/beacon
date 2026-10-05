package server_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/server"
	"github.com/RainbowHaven/beacon/internal/store"
)

type expenseTestEnv struct {
	t     *testing.T
	st    *store.Store
	ts    *httptest.Server
	house domain.SafeHouse
}

func newExpenseTestEnv(t *testing.T) *expenseTestEnv {
	t.Helper()
	db := testDB(t)
	resetSchema(t, db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := server.New(logger, db, server.Config{
		BaseURL:           "http://localhost",
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "Beacon",
		WebAuthnRPOrigins: []string{"http://localhost"},
		MaxReceiptBytes:   1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := srv.Store()
	_, house, err := st.EnsureDemoTenancy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &expenseTestEnv{t: t, st: st, ts: ts, house: house}
}

func (env *expenseTestEnv) client(email string, role domain.Role, rhlID, houseID *int64) *http.Client {
	t := env.t
	t.Helper()
	ctx := context.Background()
	u, _, err := env.st.CreateUserWithInvite(ctx, store.CreateUserInput{
		Email: email, DisplayName: email, Role: role, RHLID: rhlID, SafeHouseID: houseID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.st.ActivateUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	session, err := env.st.CreateSession(ctx, u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	base, _ := url.Parse(env.ts.URL)
	jar.SetCookies(base, []*http.Cookie{{Name: "beacon_session", Value: session, Path: "/"}})
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func (env *expenseTestEnv) postMultipart(c *http.Client, path string, fields map[string]string, receipt []byte) (int, string) {
	t := env.t
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if receipt != nil {
		part, err := w.CreateFormFile("receipt", "receipt.pdf")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(receipt)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, env.ts.URL+path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return env.do(c, req)
}

func (env *expenseTestEnv) postForm(c *http.Client, path string, fields url.Values) (int, string) {
	req, _ := http.NewRequest(http.MethodPost, env.ts.URL+path, strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return env.do(c, req)
}

func (env *expenseTestEnv) get(c *http.Client, path string) (int, string) {
	req, _ := http.NewRequest(http.MethodGet, env.ts.URL+path, nil)
	return env.do(c, req)
}

func (env *expenseTestEnv) do(c *http.Client, req *http.Request) (int, string) {
	t := env.t
	t.Helper()
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (env *expenseTestEnv) expenseAudit(id int64, action string) []domain.AuditEvent {
	env.t.Helper()
	events, err := env.st.ListAuditEventsForSubject(context.Background(), "expense", strconv.FormatInt(id, 10), 0)
	if err != nil {
		env.t.Fatal(err)
	}
	var out []domain.AuditEvent
	for _, e := range events {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func TestExpenseReviewAndCorrectionFlow(t *testing.T) {
	env := newExpenseTestEnv(t)
	ctx := context.Background()
	house := env.house
	manager := env.client("mgr@example.com", domain.RoleSafeHouseManager, &house.RHLID, &house.ID)
	bookkeeper := env.client("books@example.com", domain.RoleRHLAdmin, &house.RHLID, nil)
	otherRHL, err := env.st.CreateRHL(ctx, "Other RHL", true)
	if err != nil {
		t.Fatal(err)
	}
	outsider := env.client("outsider@example.com", domain.RoleRHLAdmin, &otherRHL.ID, nil)

	fields := map[string]string{
		"safe_house_id": strconv.FormatInt(house.ID, 10),
		"currency":      "USD",
		"amount":        "20.00",
		"spent_on":      "09/15/2026",
		"category":      "maintenance_repairs",
		"merchant":      "Hardware store",
		"note":          "door lock",
	}

	// Without a receipt an explanation is required.
	code, body := env.postMultipart(manager, "/expenses", fields, nil)
	if code != http.StatusOK || !strings.Contains(body, "Explain why there is no receipt.") {
		t.Fatalf("missing explanation: status=%d", code)
	}
	if list, _ := env.st.ListExpensesByHouses(ctx, []int64{house.ID}, 10); len(list) != 0 {
		t.Fatalf("expense saved without explanation: %+v", list)
	}

	fields["no_receipt_reason"] = "market stall, no receipt given"
	if code, _ := env.postMultipart(manager, "/expenses", fields, nil); code != http.StatusSeeOther {
		t.Fatalf("create status %d", code)
	}
	list, err := env.st.ListExpensesByHouses(ctx, []int64{house.ID}, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	e := list[0]
	if e.ReviewStatus != domain.ExpenseSubmitted || e.NoReceiptReason != "market stall, no receipt given" || e.Category != "maintenance_repairs" {
		t.Fatalf("expense=%+v", e)
	}
	idPath := "/expenses/" + strconv.FormatInt(e.ID, 10)

	// Managers cannot set review status; out-of-scope reviewers cannot see the expense.
	if code, _ := env.postForm(manager, idPath+"/review", url.Values{"review_status": {"reviewed"}}); code != http.StatusForbidden {
		t.Fatalf("manager review status %d", code)
	}
	if code, _ := env.postForm(outsider, idPath+"/review", url.Values{"review_status": {"reviewed"}}); code != http.StatusForbidden {
		t.Fatalf("outsider review status %d", code)
	}
	if code, _ := env.get(outsider, idPath+"/edit"); code != http.StatusForbidden {
		t.Fatalf("outsider edit page status %d", code)
	}

	// Needs correction requires a note.
	code, body = env.postForm(bookkeeper, idPath+"/review", url.Values{"review_status": {"needs_correction"}})
	if code != http.StatusOK || !strings.Contains(body, "Say what needs to be corrected.") {
		t.Fatalf("needs_correction without note: status=%d", code)
	}
	if code, _ := env.postForm(bookkeeper, idPath+"/review", url.Values{
		"review_status": {"needs_correction"}, "review_note": {"amount does not match"},
	}); code != http.StatusSeeOther {
		t.Fatalf("review status %d", code)
	}
	e, _ = env.st.GetExpense(ctx, e.ID)
	if e.ReviewStatus != domain.ExpenseNeedsCorrection || e.ReviewedBy == nil || e.ReviewedAt == nil || e.ReviewNote != "amount does not match" {
		t.Fatalf("after review=%+v", e)
	}

	// Correcting the expense sends it back to submitted and records old and new values.
	fields["amount"] = "25.00"
	fields["merchant"] = "City hardware"
	receipt := []byte("%PDF-1.4\n%fake receipt\n")
	if code, _ := env.postMultipart(manager, idPath, fields, receipt); code != http.StatusSeeOther {
		t.Fatalf("update status %d", code)
	}
	e, _ = env.st.GetExpense(ctx, e.ID)
	if e.AmountCents != 2500 || e.Merchant != "City hardware" || !e.HasReceipt() || e.NoReceiptReason != "" ||
		e.ReviewStatus != domain.ExpenseSubmitted || e.ReviewedBy != nil || e.ReviewedAt != nil {
		t.Fatalf("after update=%+v", e)
	}
	updates := env.expenseAudit(e.ID, "expense.update")
	if len(updates) != 1 {
		t.Fatalf("updates=%+v", updates)
	}
	changes, _ := updates[0].Meta["changes"].(map[string]any)
	amount, _ := changes["amount_cents"].(map[string]any)
	status, _ := changes["review_status"].(map[string]any)
	if amount["from"] != float64(2000) || amount["to"] != float64(2500) ||
		status["from"] != "needs_correction" || status["to"] != "submitted" ||
		updates[0].Meta["receipt"] != "added" {
		t.Fatalf("update meta=%v", updates[0].Meta)
	}
	if _, ok := changes["receipt_data"]; ok {
		t.Fatal("receipt bytes must not be audited")
	}

	// Saving without changes does not write another audit event.
	if code, _ := env.postMultipart(manager, idPath, fields, nil); code != http.StatusSeeOther {
		t.Fatalf("no-op update status %d", code)
	}
	if n := len(env.expenseAudit(e.ID, "expense.update")); n != 1 {
		t.Fatalf("no-op update wrote audit: %d events", n)
	}

	code, body = env.get(manager, idPath+"/edit")
	if code != http.StatusOK || !strings.Contains(body, "Amount: 20.00 → 25.00") || !strings.Contains(body, "Receipt added") ||
		!strings.Contains(body, "Review: Needs correction") {
		t.Fatalf("edit page status=%d body=%s", code, body)
	}

	// Once reviewed, the expense cannot be deleted, and an edit resets the review.
	if code, _ := env.postForm(bookkeeper, idPath+"/review", url.Values{"review_status": {"reviewed"}}); code != http.StatusSeeOther {
		t.Fatalf("review status %d", code)
	}
	code, body = env.get(bookkeeper, "/expenses?status=reviewed")
	if code != http.StatusOK || !strings.Contains(body, "City hardware") || !strings.Contains(body, "Maintenance and repairs") {
		t.Fatalf("filter reviewed status=%d", code)
	}
	if _, body := env.get(bookkeeper, "/expenses?status=submitted"); strings.Contains(body, "City hardware") {
		t.Fatal("submitted filter shows reviewed expense")
	}
	if code, _ := env.postForm(manager, idPath+"/delete", nil); code != http.StatusSeeOther {
		t.Fatalf("delete status %d", code)
	}
	if _, err := env.st.GetExpense(ctx, e.ID); err != nil {
		t.Fatalf("reviewed expense deleted: %v", err)
	}
	fields["note"] = "door lock and keys"
	if code, _ := env.postMultipart(bookkeeper, idPath, fields, nil); code != http.StatusSeeOther {
		t.Fatalf("reviewer update status %d", code)
	}
	e, _ = env.st.GetExpense(ctx, e.ID)
	if e.ReviewStatus != domain.ExpenseSubmitted {
		t.Fatalf("edit after review status=%s", e.ReviewStatus)
	}

	// Reports keep counting the expense.
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	totals, err := env.st.ExpenseTotalsByHouses(ctx, []int64{house.ID}, start, start.AddDate(0, 1, -1))
	if err != nil || len(totals) != 1 || totals[0].AmountCents != 2500 || totals[0].ReceiptCount != 1 {
		t.Fatalf("totals=%+v err=%v", totals, err)
	}

	if code, _ := env.postForm(manager, idPath+"/delete", nil); code != http.StatusSeeOther {
		t.Fatalf("delete status %d", code)
	}
	if _, err := env.st.GetExpense(ctx, e.ID); err != store.ErrNotFound {
		t.Fatalf("submitted expense not deleted: %v", err)
	}
}
