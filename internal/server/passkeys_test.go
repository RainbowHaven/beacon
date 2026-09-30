package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/server"
	"github.com/RainbowHaven/beacon/internal/store"
	"github.com/descope/virtualwebauthn"
)

type passkeyEnv struct {
	ts    *httptest.Server
	st    *store.Store
	rp    virtualwebauthn.RelyingParty
	house domain.SafeHouse
}

func newPasskeyEnv(t *testing.T) passkeyEnv {
	t.Helper()
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
	_, house, err := srv.Store().EnsureDemoTenancy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return passkeyEnv{
		ts:    ts,
		st:    srv.Store(),
		rp:    virtualwebauthn.RelyingParty{Name: "Beacon", ID: "localhost", Origin: "http://localhost"},
		house: house,
	}
}

type passkeyDevice struct {
	auth virtualwebauthn.Authenticator
	cred virtualwebauthn.Credential
}

func newDevice() *passkeyDevice {
	return &passkeyDevice{
		auth: virtualwebauthn.NewAuthenticator(),
		cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2),
	}
}

func newClient() (*http.Client, *http.Client) {
	jar, _ := cookiejar.New(nil)
	follow := &http.Client{Jar: jar}
	noFollow := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return follow, noFollow
}

// enroll creates a user from an invite, registers dev and returns a signed-in client.
func (e passkeyEnv) enroll(t *testing.T, in store.CreateUserInput, dev *passkeyDevice) (domain.User, *http.Client, *http.Client) {
	t.Helper()
	u, token, err := e.st.CreateUserWithInvite(context.Background(), in, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, nf := newClient()
	register(t, c, e.ts.URL, token, e.rp, &dev.auth, &dev.cred)
	return u, c, nf
}

func addPasskeyHTTP(t *testing.T, client *http.Client, base, label string, rp virtualwebauthn.RelyingParty, dev *passkeyDevice, mustExclude ...virtualwebauthn.Credential) {
	t.Helper()
	if status, body := tryAddPasskey(t, client, base, label, rp, dev, mustExclude...); status != 200 {
		t.Fatalf("add finish %d: %s", status, body)
	}
	dev.auth.AddCredential(dev.cred)
}

func tryAddPasskey(t *testing.T, client *http.Client, base, label string, rp virtualwebauthn.RelyingParty, dev *passkeyDevice, mustExclude ...virtualwebauthn.Credential) (int, string) {
	t.Helper()
	res, err := client.Post(base+"/account/passkeys/begin", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	optsJSON, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("add begin %d: %s", res.StatusCode, optsJSON)
	}
	var wrapped struct {
		PublicKey json.RawMessage `json:"publicKey"`
	}
	if err := json.Unmarshal(optsJSON, &wrapped); err != nil {
		t.Fatal(err)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(string(wrapped.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range mustExclude {
		if !c.IsExcludedForAttestation(*parsed) {
			t.Fatal("existing passkey missing from excludeCredentials")
		}
	}
	resp := virtualwebauthn.CreateAttestationResponse(rp, dev.auth, dev.cred, *parsed)
	req, _ := http.NewRequest(http.MethodPost, base+"/account/passkeys/finish?label="+url.QueryEscape(label), strings.NewReader(resp))
	req.Header.Set("Content-Type", "application/json")
	res2, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res2.Body)
	res2.Body.Close()
	return res2.StatusCode, string(b)
}

func signedIn(t *testing.T, noFollow *http.Client, base string) bool {
	t.Helper()
	res, err := noFollow.Get(base + "/account")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func postForm(t *testing.T, noFollow *http.Client, target string, form url.Values) *http.Response {
	t.Helper()
	res, err := noFollow.PostForm(target, form)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func lastAudit(t *testing.T, st *store.Store, action string) domain.AuditEvent {
	t.Helper()
	events, err := st.ListAuditEvents(context.Background(), 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Action == action {
			return e
		}
	}
	t.Fatalf("no audit event %q", action)
	return domain.AuditEvent{}
}

func TestAddSecondPasskeyAndKeepLast(t *testing.T) {
	e := newPasskeyEnv(t)
	ctx := context.Background()
	phone := newDevice()
	u, client, noFollow := e.enroll(t, store.CreateUserInput{
		Email: "mgr@example.com", Role: domain.RoleSafeHouseManager, SafeHouseID: &e.house.ID, RHLID: &e.house.RHLID,
	}, phone)

	laptop := newDevice()
	addPasskeyHTTP(t, client, e.ts.URL, "  Work   laptop ", e.rp, laptop, phone.cred)
	if ev := lastAudit(t, e.st, "auth.passkey.add"); ev.Meta["label"] != "Work laptop" {
		t.Fatalf("add audit meta %v", ev.Meta)
	}

	if status, body := tryAddPasskey(t, client, e.ts.URL, "again", e.rp, laptop); status != http.StatusBadRequest {
		t.Fatalf("re-adding the same credential: %d %s", status, body)
	}

	passkeys, err := e.st.ListPasskeys(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(passkeys) != 2 || passkeys[1].Label != "Work laptop" || passkeys[1].LastUsedAt != nil {
		t.Fatalf("passkeys after add: %+v", passkeys)
	}

	// Sign in on a fresh client with the laptop passkey only.
	laptopClient, laptopNoFollow := newClient()
	login(t, laptopClient, e.ts.URL, "mgr@example.com", e.rp, laptop.auth, laptop.cred)
	passkeys, _ = e.st.ListPasskeys(ctx, u.ID)
	if passkeys[1].LastUsedAt == nil || passkeys[0].LastUsedAt != nil {
		t.Fatalf("last_used_at after laptop login: %+v", passkeys)
	}

	page, err := laptopClient.Get(e.ts.URL + "/account")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if !strings.Contains(string(body), "Work laptop") || !strings.Contains(string(body), "Unnamed passkey") {
		t.Fatalf("account page missing passkeys: %s", body)
	}

	phoneKey := passkeys[0].Key()
	res := postForm(t, laptopNoFollow, e.ts.URL+"/account/passkeys/"+phoneKey+"/rename", url.Values{"label": {"Old phone"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("rename status %d", res.StatusCode)
	}
	if ev := lastAudit(t, e.st, "auth.passkey.rename"); ev.Meta["old_label"] != "" || ev.Meta["label"] != "Old phone" {
		t.Fatalf("rename audit meta %v", ev.Meta)
	}

	// Removing the phone passkey from the laptop signs out the phone session.
	if !signedIn(t, noFollow, e.ts.URL) {
		t.Fatal("phone should still be signed in before removal")
	}
	res = postForm(t, laptopNoFollow, e.ts.URL+"/account/passkeys/"+phoneKey+"/delete", nil)
	if loc := res.Header.Get("Location"); !strings.Contains(loc, "flash=") {
		t.Fatalf("delete location %q", loc)
	}
	if signedIn(t, noFollow, e.ts.URL) {
		t.Fatal("phone session must be revoked after its passkey is removed")
	}
	if !signedIn(t, laptopNoFollow, e.ts.URL) {
		t.Fatal("the removing session must stay signed in")
	}
	if ev := lastAudit(t, e.st, "auth.passkey.remove"); ev.Meta["label"] != "Old phone" {
		t.Fatalf("remove audit meta %v", ev.Meta)
	}

	laptopKey := base64.RawURLEncoding.EncodeToString(laptop.cred.ID)
	res = postForm(t, laptopNoFollow, e.ts.URL+"/account/passkeys/"+laptopKey+"/delete", nil)
	if loc := res.Header.Get("Location"); !strings.Contains(loc, "error=") {
		t.Fatalf("removing the last passkey must be refused; location %q", loc)
	}
	passkeys, _ = e.st.ListPasskeys(ctx, u.ID)
	if len(passkeys) != 1 {
		t.Fatalf("want 1 passkey left, got %d", len(passkeys))
	}

	// Another user cannot touch this passkey.
	other := newDevice()
	_, _, otherNoFollow := e.enroll(t, store.CreateUserInput{
		Email: "other@example.com", Role: domain.RoleSafeHouseManager, SafeHouseID: &e.house.ID, RHLID: &e.house.RHLID,
	}, other)
	res = postForm(t, otherNoFollow, e.ts.URL+"/account/passkeys/"+laptopKey+"/rename", url.Values{"label": {"mine"}})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-user rename status %d", res.StatusCode)
	}
}

func TestAdminEmailChangeAndPasskeyRemoval(t *testing.T) {
	e := newPasskeyEnv(t)
	ctx := context.Background()
	_, _, adminNF := e.enroll(t, store.CreateUserInput{Email: "rhc@example.com", Role: domain.RoleRHCAdmin}, newDevice())
	phone := newDevice()
	mgr, _, mgrNF := e.enroll(t, store.CreateUserInput{
		Email: "mgr@example.com", Role: domain.RoleSafeHouseManager, SafeHouseID: &e.house.ID, RHLID: &e.house.RHLID,
	}, phone)
	_, _, _ = e.enroll(t, store.CreateUserInput{Email: "taken@example.com", Role: domain.RoleRHCAdmin}, newDevice())

	emailURL := fmt.Sprintf("%s/admin/users/%d/email", e.ts.URL, mgr.ID)
	for _, bad := range []string{"not-an-email", "Name <x@example.com>", "taken@example.com", "TAKEN@example.com"} {
		res := postForm(t, adminNF, emailURL, url.Values{"email": {bad}})
		if loc := res.Header.Get("Location"); !strings.Contains(loc, "error=") {
			t.Fatalf("email %q accepted; location %q", bad, loc)
		}
	}
	if res := postForm(t, mgrNF, emailURL, url.Values{"email": {"x@example.com"}}); res.StatusCode != http.StatusForbidden {
		t.Fatalf("manager changing email status %d", res.StatusCode)
	}

	res := postForm(t, adminNF, emailURL, url.Values{"email": {"  New.Mgr@Example.COM "}})
	if loc := res.Header.Get("Location"); !strings.Contains(loc, "flash=") {
		t.Fatalf("email change location %q", loc)
	}
	got, err := e.st.GetUser(ctx, mgr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "new.mgr@example.com" {
		t.Fatalf("email = %q", got.Email)
	}
	ev := lastAudit(t, e.st, "admin.user.email")
	if ev.Meta["old_email"] != "mgr@example.com" || ev.Meta["new_email"] != "new.mgr@example.com" {
		t.Fatalf("email audit meta %v", ev.Meta)
	}

	// Same passkey signs in with the new email.
	c2, c2NF := newClient()
	login(t, c2, e.ts.URL, "new.mgr@example.com", e.rp, phone.auth, phone.cred)

	users, err := adminNF.Get(e.ts.URL + "/admin/users")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(users.Body)
	users.Body.Close()
	if !strings.Contains(string(body), "1 passkey") {
		t.Fatalf("users page missing passkey count: %s", body)
	}

	passkeys, _ := e.st.ListPasskeys(ctx, mgr.ID)
	if len(passkeys) != 1 {
		t.Fatalf("want 1 passkey, got %d", len(passkeys))
	}
	edit, err := adminNF.Get(fmt.Sprintf("%s/admin/users/%d/edit", e.ts.URL, mgr.ID))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(edit.Body)
	edit.Body.Close()
	if edit.StatusCode != 200 || !strings.Contains(string(body), "/passkeys/"+passkeys[0].Key()+"/delete") ||
		!strings.Contains(string(body), `value="new.mgr@example.com"`) {
		t.Fatalf("edit page %d missing passkey or email form: %s", edit.StatusCode, body)
	}
	if !signedIn(t, mgrNF, e.ts.URL) || !signedIn(t, c2NF, e.ts.URL) {
		t.Fatal("manager sessions should be active before removal")
	}
	res = postForm(t, adminNF, fmt.Sprintf("%s/admin/users/%d/passkeys/%s/delete", e.ts.URL, mgr.ID, passkeys[0].Key()), nil)
	if loc := res.Header.Get("Location"); !strings.Contains(loc, "flash=") {
		t.Fatalf("admin remove location %q", loc)
	}
	if signedIn(t, mgrNF, e.ts.URL) || signedIn(t, c2NF, e.ts.URL) {
		t.Fatal("admin passkey removal must revoke the user's sessions")
	}
	if n, _ := e.st.ListPasskeys(ctx, mgr.ID); len(n) != 0 {
		t.Fatalf("passkey not removed: %d left", len(n))
	}
	lastAudit(t, e.st, "admin.passkey.remove")
	assertLoginFails(t, e.ts.URL, "new.mgr@example.com")
}

func TestPasskeyHelpLinkedFromLogin(t *testing.T) {
	e := newPasskeyEnv(t)
	for _, path := range []string{"/help/passkeys", "/login"} {
		res, err := http.Get(e.ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%s status %d", path, res.StatusCode)
		}
		want := `href="/help/passkeys"`
		if path == "/help/passkeys" {
			want = "If your phone is lost"
		}
		if !strings.Contains(string(body), want) {
			t.Fatalf("%s missing %q", path, want)
		}
	}
}

func TestRecoverAdmin(t *testing.T) {
	db := testDB(t)
	resetSchema(t, db)
	srv, err := server.New(slog.New(slog.NewTextHandler(io.Discard, nil)), db, server.Config{
		BaseURL: "http://localhost", WebAuthnRPOrigins: []string{"http://localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st := srv.Store()
	_, house, _ := st.EnsureDemoTenancy(ctx)
	admin, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{Email: "rhc@example.com", Role: domain.RoleRHCAdmin}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateUser(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	mgr, _, err := st.CreateUserWithInvite(ctx, store.CreateUserInput{
		Email: "mgr@example.com", Role: domain.RoleSafeHouseManager, SafeHouseID: &house.ID, RHLID: &house.RHLID,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.RecoverAdmin(ctx, mgr.Email); err == nil {
		t.Fatal("recover-admin must refuse non-admins")
	}
	if err := srv.RecoverAdmin(ctx, "RHC@example.com"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetUser(ctx, admin.ID)
	if got.Status != domain.UserPending {
		t.Fatalf("status after recovery %q", got.Status)
	}
	lastAudit(t, st, "recovery.admin.reinvite")
}

func TestAdminLockRevokesSessions(t *testing.T) {
	e := newPasskeyEnv(t)
	_, _, adminNF := e.enroll(t, store.CreateUserInput{Email: "rhc@example.com", Role: domain.RoleRHCAdmin}, newDevice())
	mgr, _, mgrNF := e.enroll(t, store.CreateUserInput{
		Email: "mgr@example.com", Role: domain.RoleSafeHouseManager, SafeHouseID: &e.house.ID, RHLID: &e.house.RHLID,
	}, newDevice())
	if !signedIn(t, mgrNF, e.ts.URL) {
		t.Fatal("manager should be signed in")
	}
	postForm(t, adminNF, fmt.Sprintf("%s/admin/users/%d/lock", e.ts.URL, mgr.ID), nil)
	if signedIn(t, mgrNF, e.ts.URL) {
		t.Fatal("lock must revoke sessions")
	}
}
