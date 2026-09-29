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

	// Duplicate nickname (case/spacing) must fail with suggestion.
	dup := url.Values{
		"safe_house_id": {strconv.FormatInt(house.ID, 10)},
		"arrived_at":    {time.Now().UTC().Format("2006-01-02")},
		"nickname":      {"  sparrow  "},
	}
	res2, err := client.PostForm(ts.URL+"/occupants", dup)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusSeeOther {
		t.Fatalf("dup status %d", res2.StatusCode)
	}
	loc := res2.Header.Get("Location")
	if !strings.Contains(loc, "error=") || !strings.Contains(strings.ToLower(loc), "taken") {
		t.Fatalf("expected nickname-taken redirect, got %q", loc)
	}

	// Spaced letters must also collide ("tom" vs "T O M").
	spaced := url.Values{
		"safe_house_id": {strconv.FormatInt(house.ID, 10)},
		"arrived_at":    {time.Now().UTC().Format("2006-01-02")},
		"nickname":      {"S p a r r o w"},
	}
	resSpaced, err := client.PostForm(ts.URL+"/occupants", spaced)
	if err != nil {
		t.Fatal(err)
	}
	defer resSpaced.Body.Close()
	if resSpaced.StatusCode != http.StatusSeeOther {
		t.Fatalf("spaced status %d", resSpaced.StatusCode)
	}
	locSpaced := resSpaced.Header.Get("Location")
	if !strings.Contains(locSpaced, "error=") || !strings.Contains(strings.ToLower(locSpaced), "taken") {
		t.Fatalf("expected spaced nickname-taken redirect, got %q", locSpaced)
	}

	// Accents and Cyrillic look-alikes collide with the base spelling.
	for _, nick := range []string{"Sp\u00e4rr\u00f6w", "Sp\u0430rrow"} { // U+00E4, U+00F6; Cyrillic a U+0430
		form := url.Values{
			"safe_house_id": {strconv.FormatInt(house.ID, 10)},
			"arrived_at":    {time.Now().UTC().Format("2006-01-02")},
			"nickname":      {nick},
		}
		resFold, err := client.PostForm(ts.URL+"/occupants", form)
		if err != nil {
			t.Fatal(err)
		}
		locFold := resFold.Header.Get("Location")
		resFold.Body.Close()
		if resFold.StatusCode != http.StatusSeeOther || !strings.Contains(strings.ToLower(locFold), "taken") {
			t.Fatalf("expected fold collision for %q, status=%d loc=%q", nick, resFold.StatusCode, locFold)
		}
	}

	list, err = st.ListOccupantsByHouses(context.Background(), []int64{house.ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("dup should not insert, list=%+v", list)
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
