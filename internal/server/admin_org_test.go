package server_test

import (
	"context"
	"fmt"
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

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/server"
	"github.com/RainbowHaven/beacon/internal/store"
)

func TestOrgHousesAndUserScope(t *testing.T) {
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
	rhl, houseA, err := st.EnsureDemoTenancy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	houseB, err := st.CreateSafeHouse(context.Background(), rhl.ID, "House B", "USD", true)
	if err != nil {
		t.Fatal(err)
	}

	admin, _, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "rhc@example.com", DisplayName: "RHC", Role: domain.RoleRHCAdmin,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateUser(context.Background(), admin.ID); err != nil {
		t.Fatal(err)
	}
	adminSession, err := st.CreateSession(context.Background(), admin.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	mgrA, _, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "a@example.com", Role: domain.RoleSafeHouseManager,
		SafeHouseID: &houseA.ID, RHLID: &rhl.ID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.ActivateUser(context.Background(), mgrA.ID)
	sessA, err := st.CreateSession(context.Background(), mgrA.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	mgrB, _, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "b@example.com", Role: domain.RoleSafeHouseManager,
		SafeHouseID: &houseB.ID, RHLID: &rhl.ID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.ActivateUser(context.Background(), mgrB.ID)
	sessB, err := st.CreateSession(context.Background(), mgrB.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	clientWith := func(session string) *http.Client {
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		u, _ := url.Parse(ts.URL)
		c.Jar.SetCookies(u, []*http.Cookie{{Name: "beacon_session", Value: session, Path: "/"}})
		return c
	}

	adminClient := clientWith(adminSession)
	housesPage, err := adminClient.Get(ts.URL + "/admin/houses")
	if err != nil {
		t.Fatal(err)
	}
	defer housesPage.Body.Close()
	if housesPage.StatusCode != 200 {
		t.Fatalf("houses page %d", housesPage.StatusCode)
	}
	body, _ := io.ReadAll(housesPage.Body)
	if !strings.Contains(string(body), "House B") {
		t.Fatal("expected House B on admin houses page")
	}

	form := url.Values{
		"safe_house_id": {strconv.FormatInt(houseA.ID, 10)},
		"arrived_at":    {time.Now().UTC().Format("2006-01-02")},
		"nickname":      {"OnlyA"},
	}
	res, err := clientWith(sessA).PostForm(ts.URL+"/occupants", form)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("create occupant status %d", res.StatusCode)
	}

	listB, err := clientWith(sessB).Get(ts.URL + "/occupants")
	if err != nil {
		t.Fatal(err)
	}
	defer listB.Body.Close()
	bBody, _ := io.ReadAll(listB.Body)
	if listB.StatusCode != 200 {
		t.Fatalf("manager B occupants status %d", listB.StatusCode)
	}
	if strings.Contains(string(bBody), "OnlyA") {
		t.Fatal("manager B must not see house A nickname")
	}

	if err := st.UpdateUser(context.Background(), mgrB.ID, store.UpdateUserInput{
		DisplayName: "Mgr B",
		Role:        domain.RoleSafeHouseManager,
		RHLID:       &rhl.ID,
		SafeHouseID: &houseA.ID,
	}); err != nil {
		t.Fatal(err)
	}
	probe, err := clientWith(sessB).Get(ts.URL + "/occupants")
	if err != nil {
		t.Fatal(err)
	}
	probe.Body.Close()
	if probe.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect after session revoke; status=%d", probe.StatusCode)
	}

	selfEdit, err := adminClient.Get(ts.URL + "/admin/users/" + fmt.Sprintf("%d", admin.ID) + "/edit")
	if err != nil {
		t.Fatal(err)
	}
	selfEdit.Body.Close()
	if selfEdit.StatusCode != http.StatusSeeOther {
		t.Fatalf("self edit get status=%d", selfEdit.StatusCode)
	}
}

func TestAdminRHLCodeAndSleepingPlaces(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := server.New(logger, db, server.Config{
		BaseURL:           "http://localhost",
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "Beacon",
		WebAuthnRPOrigins: []string{"http://localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	st := srv.Store()
	ctx := context.Background()
	admin, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{
		Email: "rhc@example.com", DisplayName: "RHC", Role: domain.RoleRHCAdmin,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateUser(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	session, err := st.CreateSession(ctx, admin.ID, time.Hour)
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
	jar.SetCookies(u, []*http.Cookie{{Name: "beacon_session", Value: session, Path: "/"}})

	post := func(path string, form url.Values) string {
		t.Helper()
		res, err := client.PostForm(ts.URL+path, form)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("POST %s status %d", path, res.StatusCode)
		}
		return res.Header.Get("Location")
	}

	if loc := post("/admin/rhls", url.Values{"name": {"Toronto"}, "code": {" tor "}}); !strings.Contains(loc, "flash=") {
		t.Fatalf("create rhl: %s", loc)
	}
	if loc := post("/admin/rhls", url.Values{"name": {"Toronto 2"}, "code": {"TOR"}}); !strings.Contains(loc, "already+in+use") {
		t.Fatalf("duplicate code: %s", loc)
	}
	if loc := post("/admin/rhls", url.Values{"name": {"Bad"}, "code": {"T"}}); !strings.Contains(loc, "error=RHL+code+must") {
		t.Fatalf("invalid code: %s", loc)
	}
	rhls, err := st.ListRHLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rhls) != 1 || rhls[0].Code != "TOR" {
		t.Fatalf("rhls: %+v", rhls)
	}
	rhlID := strconv.FormatInt(rhls[0].ID, 10)

	other, err := st.CreateRHLWithCode(ctx, "Vancouver", "VAN", true)
	if err != nil {
		t.Fatal(err)
	}
	if loc := post("/admin/rhls/"+strconv.FormatInt(other.ID, 10), url.Values{"name": {"Vancouver"}, "code": {"tor"}}); !strings.Contains(loc, "already+in+use") {
		t.Fatalf("duplicate code on update: %s", loc)
	}

	houseForm := url.Values{"name": {"House A"}, "rhl_id": {rhlID}, "default_currency": {"CAD"}, "approved_sleeping_places": {"0"}}
	if loc := post("/admin/safe-houses", houseForm); !strings.Contains(loc, "error=sleeping+places") {
		t.Fatalf("zero places: %s", loc)
	}
	houseForm.Set("approved_sleeping_places", "6")
	if loc := post("/admin/safe-houses", houseForm); !strings.Contains(loc, "flash=") {
		t.Fatalf("create house: %s", loc)
	}
	houses, err := st.ListSafeHouses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(houses) != 1 || houses[0].ApprovedSleepingPlaces == nil || *houses[0].ApprovedSleepingPlaces != 6 {
		t.Fatalf("houses: %+v", houses)
	}

	res, err := client.Get(ts.URL + "/admin/houses")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{"Toronto (TOR)", `name="code" value="TOR"`, `name="approved_sleeping_places" value="6"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("admin houses page missing %q", want)
		}
	}

	houseForm.Set("approved_sleeping_places", "")
	if loc := post("/admin/safe-houses/"+strconv.FormatInt(houses[0].ID, 10), houseForm); !strings.Contains(loc, "flash=") {
		t.Fatalf("update house: %s", loc)
	}
	h, err := st.GetSafeHouse(ctx, houses[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if h.ApprovedSleepingPlaces != nil {
		t.Fatalf("sleeping places not cleared: %d", *h.ApprovedSleepingPlaces)
	}

	events, err := st.ListAuditEvents(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]any{}
	for _, e := range events {
		if _, ok := seen[e.Action]; !ok {
			seen[e.Action] = e.Meta
		}
	}
	if m := seen["admin.rhl.create"]; m == nil || m["code"] != "TOR" {
		t.Fatalf("rhl create audit meta: %v", m)
	}
	if m := seen["admin.house.create"]; m == nil || m["approved_sleeping_places"] != float64(6) {
		t.Fatalf("house create audit meta: %v", m)
	}
	if m, ok := seen["admin.house.update"]; !ok || m["approved_sleeping_places"] != nil {
		t.Fatalf("house update audit meta: %v", m)
	}
}
