package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/server"
	"github.com/magiconair/beacon/internal/store"
)

func TestOccupantCreateScoped(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := server.Config{
		BaseURL:           "http://localhost",
		SecureCookies:     false,
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "Beacon",
		WebAuthnRPOrigins: []string{"http://localhost"},
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
		Email: "mgr@example.com", DisplayName: "Mgr", Role: domain.RoleSafeHouseManager,
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

	form := url.Values{
		"safe_house_id": {strconv.FormatInt(house.ID, 10)},
		"arrived_at":    {time.Now().UTC().Format("2006-01-02")},
		"nickname":      {"Sparrow"},
	}
	res, err := client.PostForm(ts.URL+"/occupants", form)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create status %d", res.StatusCode)
	}

	list, err := st.ListOccupantsByHouses(context.Background(), []int64{house.ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Nickname != "Sparrow" {
		t.Fatalf("list=%+v", list)
	}

	counts, err := st.HeadcountByHouses(context.Background(), []int64{house.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 1 || counts[0].Current != 1 {
		t.Fatalf("headcount=%+v", counts)
	}
}
