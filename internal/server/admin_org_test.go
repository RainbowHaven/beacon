package server_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/server"
	"github.com/magiconair/beacon/internal/store"
	"golang.org/x/crypto/nacl/box"
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
		IdentityPublicKey: testIdentityPublicKey(t),
		IdentityKeyID:     "test-key-1",
		ReceiptDir:        t.TempDir(),
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

	pub := cfg.IdentityPublicKey
	ctRaw, err := box.SealAnonymous(nil, []byte(`{"legal_name":"X","refugee_id":"1"}`), &pub, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"safe_house_id":       {strconv.FormatInt(houseA.ID, 10)},
		"arrived_at":          {time.Now().UTC().Format("2006-01-02")},
		"nickname":            {"OnlyA"},
		"key_id":              {"test-key-1"},
		"identity_ciphertext": {base64.StdEncoding.EncodeToString(ctRaw)},
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
