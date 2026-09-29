package server_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/migrate"
	"github.com/magiconair/beacon/internal/server"
	"github.com/magiconair/beacon/internal/store"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://beacon:beacon@127.0.0.1:5433/beacon?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("postgres not available (%v); start with ./scripts/compose.sh up -d db", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func resetSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		DROP TABLE IF EXISTS expenses, occupants, audit_events, sessions, webauthn_challenges, webauthn_credentials, invites, users, safe_houses, rhls, schema_migrations CASCADE;
		DROP TYPE IF EXISTS user_status, user_role CASCADE;
	`)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := migrate.Up(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestInviteRegisterLoginLock(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := server.Config{
		BaseURL:             "http://localhost",
		SecureCookies:       false,
		WebAuthnRPID:        "localhost",
		WebAuthnRPName:      "Beacon",
		WebAuthnRPOrigins:   []string{"http://localhost"},
		BootstrapAdminEmail: "",
	}
	srv, err := server.New(logger, db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}

	st := srv.Store()
	_, house, err := st.EnsureDemoTenancy(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	admin, adminToken, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "rhc@example.com", DisplayName: "RHC", Role: domain.RoleRHCAdmin,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	manager, managerToken, err := st.CreateUserWithInvite(context.Background(), store.CreateUserInput{
		Email: "manager@example.com", DisplayName: "Mgr", Role: domain.RoleSafeHouseManager, SafeHouseID: &house.ID, RHLID: &house.RHLID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_ = admin
	_ = manager

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	rp := virtualwebauthn.RelyingParty{Name: "Beacon", ID: "localhost", Origin: "http://localhost"}
	authenticator := virtualwebauthn.NewAuthenticator()
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Register admin passkey
	register(t, client, ts.URL, adminToken, rp, &authenticator, &credential)

	// Login admin
	login(t, client, ts.URL, "rhc@example.com", rp, authenticator, credential)

	// Admin page requires session
	res, err := client.Get(ts.URL + "/admin/users")
	if err != nil {
		t.Fatal(err)
	}
	usersBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("admin users status %d", res.StatusCode)
	}
	if !strings.Contains(string(usersBody), "Signed in") {
		t.Fatal("expected own row to show Signed in instead of Lock/New invite")
	}
	invitePage, err := client.Get(ts.URL + "/admin/users/invite")
	if err != nil {
		t.Fatal(err)
	}
	defer invitePage.Body.Close()
	if invitePage.StatusCode != 200 {
		t.Fatalf("admin invite status %d", invitePage.StatusCode)
	}
	noFollow := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	selfLock, err := noFollow.Post(ts.URL+"/admin/users/"+fmt.Sprintf("%d", admin.ID)+"/lock", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	selfLock.Body.Close()
	if selfLock.StatusCode != http.StatusSeeOther {
		t.Fatalf("self-lock status=%d", selfLock.StatusCode)
	}
	if loc := selfLock.Header.Get("Location"); !strings.Contains(loc, "cannot+modify+yourself") && !strings.Contains(loc, "cannot%20modify%20yourself") {
		t.Fatalf("self-lock location=%q", loc)
	}
	adminAfter, err := st.GetUser(context.Background(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adminAfter.Status != domain.UserActive {
		t.Fatalf("self-lock must not change status; got %q", adminAfter.Status)
	}
	selfRe, err := noFollow.Post(ts.URL+"/admin/users/"+fmt.Sprintf("%d", admin.ID)+"/reinvite", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	selfRe.Body.Close()
	if selfRe.StatusCode != http.StatusSeeOther {
		t.Fatalf("self-reinvite status=%d", selfRe.StatusCode)
	}
	adminAfter, err = st.GetUser(context.Background(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adminAfter.Status != domain.UserActive {
		t.Fatalf("self-reinvite must not change status; got %q", adminAfter.Status)
	}
	creds, err := st.ListCredentials(context.Background(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) == 0 {
		t.Fatal("self-reinvite must not delete own credentials")
	}

	// Register manager with fresh client
	jar2, _ := cookiejar.New(nil)
	client2 := &http.Client{Jar: jar2}
	auth2 := virtualwebauthn.NewAuthenticator()
	cred2 := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	register(t, client2, ts.URL, managerToken, rp, &auth2, &cred2)

	// Lock manager, login should fail
	if err := st.LockUser(context.Background(), manager.ID); err != nil {
		t.Fatal(err)
	}
	assertLoginFails(t, ts.URL, "manager@example.com")

	// Re-invite after lock must unlock to pending and yield a usable invite.
	if err := st.DeleteCredentialsForUser(context.Background(), manager.ID); err != nil {
		t.Fatal(err)
	}
	newToken, err := st.ReissueInvite(context.Background(), manager.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.LookupInvite(context.Background(), newToken)
	if err != nil {
		t.Fatalf("reinvite after lock should be valid: %v", err)
	}
	if u.Status != domain.UserPending {
		t.Fatalf("status after reinvite: got %q want pending", u.Status)
	}
	page, err := client2.Get(ts.URL + "/invite/" + newToken)
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	if page.StatusCode != 200 {
		t.Fatalf("invite page status %d", page.StatusCode)
	}
	body, _ := io.ReadAll(page.Body)
	if strings.Contains(string(body), "Invite not valid") {
		t.Fatal("invite page showed invalid after lock+reinvite")
	}
	auth3 := virtualwebauthn.NewAuthenticator()
	cred3 := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	register(t, client2, ts.URL, newToken, rp, &auth3, &cred3)
	login(t, client2, ts.URL, "manager@example.com", rp, auth3, cred3)
}

func register(t *testing.T, client *http.Client, base, token string, rp virtualwebauthn.RelyingParty, authenticator *virtualwebauthn.Authenticator, credential *virtualwebauthn.Credential) {
	t.Helper()
	beginBody, _ := json.Marshal(map[string]string{"token": token})
	res, err := client.Post(base+"/webauthn/register/begin?token="+url.QueryEscape(token), "application/json", bytes.NewReader(beginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("register begin %d: %s", res.StatusCode, b)
	}
	optsJSON, _ := io.ReadAll(res.Body)
	// CredentialCreation JSON nests under publicKey
	var wrapped struct {
		PublicKey json.RawMessage `json:"publicKey"`
	}
	if err := json.Unmarshal(optsJSON, &wrapped); err != nil {
		t.Fatal(err)
	}
	attestationOptions := string(wrapped.PublicKey)
	parsed, err := virtualwebauthn.ParseAttestationOptions(attestationOptions)
	if err != nil {
		t.Fatalf("parse attestation options: %v\n%s", err, attestationOptions)
	}
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, *authenticator, *credential, *parsed)
	req, _ := http.NewRequest(http.MethodPost, base+"/webauthn/register/finish?token="+url.QueryEscape(token), strings.NewReader(attestationResponse))
	req.Header.Set("Content-Type", "application/json")
	res2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("register finish %d: %s", res2.StatusCode, b)
	}
	authenticator.AddCredential(*credential)
}

func login(t *testing.T, client *http.Client, base, email string, rp virtualwebauthn.RelyingParty, authenticator virtualwebauthn.Authenticator, credential virtualwebauthn.Credential) {
	t.Helper()
	beginBody, _ := json.Marshal(map[string]string{"email": email})
	res, err := client.Post(base+"/webauthn/login/begin", "application/json", bytes.NewReader(beginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("login begin %d: %s", res.StatusCode, b)
	}
	optsJSON, _ := io.ReadAll(res.Body)
	var wrapped struct {
		PublicKey json.RawMessage `json:"publicKey"`
	}
	if err := json.Unmarshal(optsJSON, &wrapped); err != nil {
		t.Fatal(err)
	}
	parsed, err := virtualwebauthn.ParseAssertionOptions(string(wrapped.PublicKey))
	if err != nil {
		t.Fatalf("parse assertion options: %v", err)
	}
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, authenticator, credential, *parsed)
	req, _ := http.NewRequest(http.MethodPost, base+"/webauthn/login/finish", strings.NewReader(assertionResponse))
	req.Header.Set("Content-Type", "application/json")
	res2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("login finish %d: %s", res2.StatusCode, b)
	}
}

func assertLoginFails(t *testing.T, base, email string) {
	t.Helper()
	beginBody, _ := json.Marshal(map[string]string{"email": email})
	res, err := http.Post(base+"/webauthn/login/begin", "application/json", bytes.NewReader(beginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		t.Fatal("expected locked login begin to fail")
	}
}
