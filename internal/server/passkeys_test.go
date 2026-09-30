package server_test

import (
	"context"
	"encoding/base64"
	"encoding/hex"
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
	auth   virtualwebauthn.Authenticator
	cred   virtualwebauthn.Credential
	header http.Header
}

func newDevice() *passkeyDevice {
	return &passkeyDevice{
		auth: virtualwebauthn.NewAuthenticator(),
		cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2),
	}
}

// newBrowserDevice is a device whose browser sends ua and the Client Hints in
// hints (name → value) and whose authenticator reports aaguid.
func newBrowserDevice(t *testing.T, aaguid, ua string, hints map[string]string) *passkeyDevice {
	t.Helper()
	d := newDevice()
	b, err := hex.DecodeString(strings.ReplaceAll(aaguid, "-", ""))
	if err != nil || len(b) != 16 {
		t.Fatalf("bad aaguid %q", aaguid)
	}
	copy(d.auth.Aaguid[:], b)
	d.header = http.Header{"User-Agent": {ua}}
	for k, v := range hints {
		d.header.Set(k, v)
	}
	return d
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
	registerWithHeader(t, c, e.ts.URL, token, e.rp, &dev.auth, &dev.cred, dev.header)
	return u, c, nf
}

func addPasskeyHTTP(t *testing.T, client *http.Client, base string, rp virtualwebauthn.RelyingParty, dev *passkeyDevice, mustExclude ...virtualwebauthn.Credential) {
	t.Helper()
	if status, body := tryAddPasskey(t, client, base, rp, dev, mustExclude...); status != 200 {
		t.Fatalf("add finish %d: %s", status, body)
	}
	dev.auth.AddCredential(dev.cred)
}

func tryAddPasskey(t *testing.T, client *http.Client, base string, rp virtualwebauthn.RelyingParty, dev *passkeyDevice, mustExclude ...virtualwebauthn.Credential) (int, string) {
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
	req, _ := http.NewRequest(http.MethodPost, base+"/account/passkeys/finish", strings.NewReader(resp))
	for k, v := range dev.header {
		req.Header[k] = v
	}
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

const (
	aaguidApplePasswords = "fbfc3007-154e-4ecc-8c0b-6e020557d7bd"
	aaguidWindowsHello   = "08987058-cadc-4b81-b6e1-30de50dcbe96"
	aaguidGooglePM       = "ea9b8d66-4d01-1d21-3ce4-b6b48cb575d4"

	uaIPhone        = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Mobile/15E148 Safari/604.1"
	uaWindowsChrome = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	uaAndroidChrome = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36"

	labelIPhone  = "iPhone · iOS 18 · Apple Passwords"
	labelWindows = "Windows PC · Windows 11 · Windows Hello"
	labelPixel   = "Pixel 8 · Android 15 · Google Password Manager"
)

func newIPhone(t *testing.T) *passkeyDevice {
	return newBrowserDevice(t, aaguidApplePasswords, uaIPhone, nil)
}

func newWindowsPC(t *testing.T) *passkeyDevice {
	return newBrowserDevice(t, aaguidWindowsHello, uaWindowsChrome, map[string]string{
		"Sec-CH-UA-Platform": `"Windows"`, "Sec-CH-UA-Platform-Version": `"15.0.0"`, "Sec-CH-UA-Mobile": "?0",
	})
}

func newPixel(t *testing.T) *passkeyDevice {
	return newBrowserDevice(t, aaguidGooglePM, uaAndroidChrome, map[string]string{
		"Sec-CH-UA-Platform": `"Android"`, "Sec-CH-UA-Platform-Version": `"15.0.0"`,
		"Sec-CH-UA-Model": `"Pixel 8"`, "Sec-CH-UA-Mobile": "?1",
	})
}

func pageBody(t *testing.T, client *http.Client, target string) (*http.Response, string) {
	t.Helper()
	res, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(b)
}

func TestAddSecondPasskeyAndKeepLast(t *testing.T) {
	e := newPasskeyEnv(t)
	ctx := context.Background()
	phone := newIPhone(t)
	u, client, noFollow := e.enroll(t, store.CreateUserInput{
		Email: "mgr@example.com", Role: domain.RoleSafeHouseManager, SafeHouseID: &e.house.ID, RHLID: &e.house.RHLID,
	}, phone)
	if ev := lastAudit(t, e.st, "auth.register"); ev.Meta["label"] != labelIPhone {
		t.Fatalf("register audit meta %v", ev.Meta)
	}

	laptop := newWindowsPC(t)
	addPasskeyHTTP(t, client, e.ts.URL, e.rp, laptop, phone.cred)
	if ev := lastAudit(t, e.st, "auth.passkey.add"); ev.Meta["label"] != labelWindows {
		t.Fatalf("add audit meta %v", ev.Meta)
	}

	if status, body := tryAddPasskey(t, client, e.ts.URL, e.rp, laptop); status != http.StatusBadRequest {
		t.Fatalf("re-adding the same credential: %d %s", status, body)
	}

	passkeys, err := e.st.ListPasskeys(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(passkeys) != 2 || passkeys[0].Label != labelIPhone || passkeys[1].Label != labelWindows || passkeys[1].LastUsedAt != nil {
		t.Fatalf("passkeys after add: %+v", passkeys)
	}

	// Sign in on a fresh client with the laptop passkey only; its Go
	// User-Agent must not change the stored label.
	laptopClient, laptopNoFollow := newClient()
	login(t, laptopClient, e.ts.URL, "mgr@example.com", e.rp, laptop.auth, laptop.cred)
	passkeys, _ = e.st.ListPasskeys(ctx, u.ID)
	if passkeys[1].LastUsedAt == nil || passkeys[0].LastUsedAt != nil {
		t.Fatalf("last_used_at after laptop login: %+v", passkeys)
	}
	if passkeys[0].Label != labelIPhone || passkeys[1].Label != labelWindows {
		t.Fatalf("labels changed by sign-in: %+v", passkeys)
	}

	page, body := pageBody(t, laptopClient, e.ts.URL+"/account")
	if !strings.Contains(body, labelIPhone) || !strings.Contains(body, labelWindows) {
		t.Fatalf("account page missing passkeys: %s", body)
	}
	if strings.Contains(body, `name="label"`) || strings.Contains(body, "/rename") {
		t.Fatalf("account page still offers renaming: %s", body)
	}
	if got := page.Header.Get("Accept-CH"); !strings.Contains(got, "Sec-CH-UA-Platform-Version") || !strings.Contains(got, "Sec-CH-UA-Model") {
		t.Fatalf("account Accept-CH %q", got)
	}

	phoneKey := passkeys[0].Key()
	res := postForm(t, laptopNoFollow, e.ts.URL+"/account/passkeys/"+phoneKey+"/rename", url.Values{"label": {"Old phone"}})
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("rename status %d, want 404 or 405", res.StatusCode)
	}
	if passkeys, _ = e.st.ListPasskeys(ctx, u.ID); passkeys[0].Label != labelIPhone {
		t.Fatalf("label after rename attempt: %+v", passkeys)
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
	if ev := lastAudit(t, e.st, "auth.passkey.remove"); ev.Meta["label"] != labelIPhone {
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
	res = postForm(t, otherNoFollow, e.ts.URL+"/account/passkeys/"+laptopKey+"/delete", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-user delete status %d", res.StatusCode)
	}
}

func TestInvitePageRequestsClientHints(t *testing.T) {
	e := newPasskeyEnv(t)
	_, token, err := e.st.CreateUserWithInvite(context.Background(), store.CreateUserInput{Email: "rhc@example.com", Role: domain.RoleRHCAdmin}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := pageBody(t, http.DefaultClient, e.ts.URL+"/invite/"+token)
	if res.StatusCode != 200 {
		t.Fatalf("invite page status %d", res.StatusCode)
	}
	got := res.Header.Get("Accept-CH")
	for _, h := range []string{"Sec-CH-UA-Platform", "Sec-CH-UA-Platform-Version", "Sec-CH-UA-Model", "Sec-CH-UA-Mobile"} {
		if !strings.Contains(got, h) {
			t.Fatalf("invite Accept-CH %q missing %s", got, h)
		}
	}
}

func TestLegacyPasskeyLabelFromAAGUID(t *testing.T) {
	e := newPasskeyEnv(t)
	ctx := context.Background()
	u, _, _ := e.enroll(t, store.CreateUserInput{Email: "rhc@example.com", Role: domain.RoleRHCAdmin}, newIPhone(t))
	if _, err := e.st.DB().ExecContext(ctx, `UPDATE webauthn_credentials SET label = '' WHERE user_id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if passkeys, _ := e.st.ListPasskeys(ctx, u.ID); len(passkeys) != 1 || passkeys[0].Label != "Apple Passwords" {
		t.Fatalf("legacy label: %+v", passkeys)
	}
	if _, err := e.st.DB().ExecContext(ctx, `UPDATE webauthn_credentials SET aaguid = $2 WHERE user_id = $1`, u.ID, make([]byte, 16)); err != nil {
		t.Fatal(err)
	}
	if passkeys, _ := e.st.ListPasskeys(ctx, u.ID); passkeys[0].Label != "Passkey" {
		t.Fatalf("legacy label without AAGUID: %+v", passkeys)
	}
}

func TestAdminEmailChangeAndPasskeyRemoval(t *testing.T) {
	e := newPasskeyEnv(t)
	ctx := context.Background()
	_, _, adminNF := e.enroll(t, store.CreateUserInput{Email: "rhc@example.com", Role: domain.RoleRHCAdmin}, newDevice())
	phone := newPixel(t)
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
		!strings.Contains(string(body), labelPixel) ||
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
