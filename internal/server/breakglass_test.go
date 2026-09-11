package server_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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

func TestBreakGlassSealedIdentityRHCOnly(t *testing.T) {
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

	admin, _, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "rhc@example.com", DisplayName: "RHC", Role: domain.RoleRHCAdmin,
	}, time.Hour)
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
	_ = st.ActivateUser(context.Background(), admin.ID)
	_ = st.ActivateUser(context.Background(), manager.ID)
	adminSession, _ := st.CreateSession(context.Background(), admin.ID, time.Hour)
	mgrSession, _ := st.CreateSession(context.Background(), manager.ID, time.Hour)

	msg := []byte(`{"legal_name":"Secret","refugee_id":"UN-1"}`)
	ct, err := box.SealAnonymous(nil, msg, &pub, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	o, err := st.CreateOccupant(context.Background(), store.CreateOccupantInput{
		SafeHouseID:        house.ID,
		Nickname:           "Sparrow",
		ArrivedAt:          time.Now().UTC(),
		IdentityCiphertext: ct,
		KeyID:              "test-key-1",
		CreatedBy:          &manager.ID,
	})
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

	path := ts.URL + "/admin/occupants/" + strconv.FormatInt(o.ID, 10) + "/sealed-identity"

	mgrRes, err := clientWith(mgrSession).Get(path)
	if err != nil {
		t.Fatal(err)
	}
	defer mgrRes.Body.Close()
	if mgrRes.StatusCode != http.StatusForbidden {
		t.Fatalf("manager got %d, want 403", mgrRes.StatusCode)
	}

	adminRes, err := clientWith(adminSession).Get(path)
	if err != nil {
		t.Fatal(err)
	}
	defer adminRes.Body.Close()
	if adminRes.StatusCode != 200 {
		t.Fatalf("admin got %d", adminRes.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(adminRes.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	ctB64, _ := payload["identity_ciphertext"].(string)
	raw, err := base64.StdEncoding.DecodeString(ctB64)
	if err != nil || len(raw) < 48 {
		t.Fatalf("bad ciphertext in response")
	}
	if strings.Contains(string(raw), "Secret") {
		t.Fatal("response must stay sealed")
	}

	events, err := st.ListAuditEvents(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Action == "identity.break_glass" && e.SubjectID == strconv.FormatInt(o.ID, 10) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected identity.break_glass audit event")
	}

	page, err := clientWith(adminSession).Get(ts.URL + "/admin/break-glass")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	body, _ := io.ReadAll(page.Body)
	if page.StatusCode != 200 || !strings.Contains(string(body), "Sparrow") {
		t.Fatalf("break-glass page status=%d", page.StatusCode)
	}
}
