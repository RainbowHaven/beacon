package server_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

func TestOccupantCreateScopedAndSealed(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pub := testIdentityPublicKey(t)
	cfg := server.Config{
		BaseURL:           "http://localhost",
		SecureCookies:     false,
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "Beacon",
		WebAuthnRPOrigins: []string{"http://localhost"},
		IdentityPublicKey: pub,
		IdentityKeyID:     "test-key-1",
		ReceiptDir:        t.TempDir(),
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

	privB64 := "kkaOEMtAGbjNKyXqJqzDTP9Un3oUWnyjkGqP9ZuXVbw="
	privRaw, _ := base64.StdEncoding.DecodeString(privB64)
	var priv [32]byte
	copy(priv[:], privRaw)
	msg := []byte(`{"legal_name":"Secret Name","refugee_id":"UN-99"}`)
	ct, err := box.SealAnonymous(nil, msg, &pub, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	opened, ok := box.OpenAnonymous(nil, ct, &pub, &priv)
	if !ok || string(opened) != string(msg) {
		t.Fatal("fixture seal broken")
	}

	form := url.Values{
		"safe_house_id":       {strconv.FormatInt(house.ID, 10)},
		"arrived_at":          {time.Now().UTC().Format("2006-01-02")},
		"nickname":            {"Sparrow"},
		"key_id":              {"test-key-1"},
		"identity_ciphertext": {base64.StdEncoding.EncodeToString(ct)},
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
	if strings.Contains(string(list[0].IdentityCiphertext), "Secret") {
		t.Fatal("ciphertext must not contain plaintext name")
	}

	counts, err := st.HeadcountByHouses(context.Background(), []int64{house.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 1 || counts[0].Current != 1 {
		t.Fatalf("headcount=%+v", counts)
	}

	form.Set("legal_name", "leak")
	form.Set("identity_ciphertext", base64.StdEncoding.EncodeToString(ct))
	res2, err := client.PostForm(ts.URL+"/occupants", form)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	loc := res2.Header.Get("Location")
	if !strings.Contains(loc, "sealed") {
		t.Fatalf("expected reject plaintext, loc=%s", loc)
	}
}
