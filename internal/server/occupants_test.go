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
	"strings"
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
		ArrivalFutureDays: 1,
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

	today := time.Now().UTC().Format("2006-01-02")
	form := url.Values{
		"safe_house_id": {strconv.FormatInt(house.ID, 10)},
		"arrived_at":    {today},
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

	mustFormError := func(t *testing.T, nick string, wantSubstrings ...string) {
		t.Helper()
		f := url.Values{
			"safe_house_id": {strconv.FormatInt(house.ID, 10)},
			"arrived_at":    {today},
			"nickname":      {nick},
		}
		res, err := client.PostForm(ts.URL+"/occupants", f)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("nick %q status %d want 200 form re-render", nick, res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		html := string(body)
		for _, want := range wantSubstrings {
			if !strings.Contains(html, want) {
				t.Fatalf("nick %q missing %q in body:\n%s", nick, want, html)
			}
		}
		list, err := st.ListOccupantsByHouses(context.Background(), []int64{house.ID}, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Fatalf("nick %q should not insert, list=%+v", nick, list)
		}
	}

	mustFormError(t, "  sparrow  ", "already taken.", `value="sparrow-2"`, `class="nick-suggest"`)
	mustFormError(t, "S p a r r o w", "already taken.", `value="sparrow-2"`)
	mustFormError(t, "Sp\u00e4rr\u00f6w", "already taken.", `value="sparrow-2"`)
	mustFormError(t, "Sp\u0430rrow", "already taken.", `value="sparrow-2"`)
	mustFormError(t, "S-p-a-r-r-o-w", "already taken.", `value="sparrow-2"`)
	mustFormError(t, "Sparrow!", "already taken.", `value="sparrow-2"`)
	mustFormError(t, "1bad", "Nickname must start with a letter.")
	mustFormError(t, "!tom", "Nickname must start with a letter.")

	// Arrival too far ahead (beyond ArrivalFutureDays=1).
	far := time.Now().UTC().AddDate(0, 0, 5).Format("2006-01-02")
	farForm := url.Values{
		"safe_house_id": {strconv.FormatInt(house.ID, 10)},
		"arrived_at":    {far},
		"nickname":      {"Later"},
	}
	resFar, err := client.PostForm(ts.URL+"/occupants", farForm)
	if err != nil {
		t.Fatal(err)
	}
	defer resFar.Body.Close()
	farBody, _ := io.ReadAll(resFar.Body)
	if resFar.StatusCode != http.StatusOK || !strings.Contains(string(farBody), "future") {
		t.Fatalf("future arrival status=%d body=%s", resFar.StatusCode, farBody)
	}

	// Rename in place.
	rename := url.Values{"nickname": {"Robin"}}
	res3, err := client.PostForm(ts.URL+"/occupants/"+strconv.FormatInt(list[0].ID, 10)+"/rename", rename)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusSeeOther {
		t.Fatalf("rename status %d", res3.StatusCode)
	}
	got, err := st.GetOccupant(context.Background(), list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nickname != "Robin" {
		t.Fatalf("nickname=%q", got.Nickname)
	}

	// Home page should expose always-visible profile logout control.
	res4, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res4.Body.Close()
	body, _ := io.ReadAll(res4.Body)
	html := string(body)
	if !strings.Contains(html, `class="profile-menu"`) || !strings.Contains(html, `action="/logout"`) {
		t.Fatalf("home missing profile logout: %s", html[:min(500, len(html))])
	}
}
