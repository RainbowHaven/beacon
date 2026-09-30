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

func TestExpenseCreateAndMonthlyTotals(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := server.Config{
		BaseURL:           "http://localhost",
		SecureCookies:     false,
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "Beacon",
		WebAuthnRPOrigins: []string{"http://localhost"},
		MaxReceiptBytes:   1 << 20,
	}
	srv, err := server.New(logger, db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	st := srv.Store()
	_, house, err := st.EnsureDemoTenancy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	manager, _, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "exp@example.com", DisplayName: "Mgr", Role: domain.RoleSafeHouseManager,
		SafeHouseID: &house.ID, RHLID: &house.RHLID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateUser(context.Background(), manager.ID); err != nil {
		t.Fatal(err)
	}
	session, err := st.CreateSession(context.Background(), manager.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	u, _ := url.Parse(ts.URL)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: "beacon_session", Value: session, Path: "/"}})

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("safe_house_id", strconv.FormatInt(house.ID, 10))
	_ = w.WriteField("currency", "USD")
	_ = w.WriteField("amount", "12.50")
	_ = w.WriteField("spent_on", time.Now().UTC().Format("01/02/2006"))
	_ = w.WriteField("note", "groceries")
	_ = w.WriteField("category", "food")
	_ = w.WriteField("merchant", "Corner market")
	part, err := w.CreateFormFile("receipt", "receipt.png")
	if err != nil {
		t.Fatal(err)
	}
	// Minimal PNG header bytes — DetectContentType recognizes PNG.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f,
		0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59,
		0xe7, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/expenses", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status %d", res.StatusCode)
	}

	list, err := st.ListExpensesByHouses(context.Background(), []int64{house.ID}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AmountCents != 1250 || list[0].Currency != "USD" || !list[0].HasReceipt() ||
		list[0].Category != domain.CategoryFood || list[0].Merchant != "Corner market" || list[0].ReviewStatus != domain.ExpenseSubmitted {
		t.Fatalf("list=%+v", list)
	}

	rec, err := client.Get(ts.URL + "/expenses/" + strconv.FormatInt(list[0].ID, 10) + "/receipt")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Body.Close()
	if rec.StatusCode != 200 {
		t.Fatalf("receipt status %d", rec.StatusCode)
	}

	month := time.Now().UTC().Format("2006-01")
	start, _ := time.Parse("2006-01", month)
	end := start.AddDate(0, 1, -1)
	totals, err := st.ExpenseTotalsByHouses(context.Background(), []int64{house.ID}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 || totals[0].AmountCents != 1250 || totals[0].ReceiptCount != 1 || totals[0].Currency != "USD" {
		t.Fatalf("totals=%+v", totals)
	}

	rep, err := client.Get(ts.URL + "/reports?month=" + month)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	b, _ := io.ReadAll(rep.Body)
	if rep.StatusCode != 200 || !strings.Contains(string(b), "12.50") || !strings.Contains(string(b), "USD") {
		t.Fatalf("report status=%d body=%s", rep.StatusCode, string(b))
	}
}
