package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
	"github.com/magiconair/beacon/internal/version"
	"github.com/magiconair/beacon/internal/wauser"
	"github.com/magiconair/beacon/web"
)

const (
	sessionCookie     = "beacon_session"
	challengeCookie   = "beacon_wa_challenge"
	inviteCookie      = "beacon_invite"
	inviteFlashCookie = "beacon_invite_flash"
	inviteTTL         = 7 * 24 * time.Hour
	sessionTTL        = 14 * 24 * time.Hour
	challengeTTL      = 5 * time.Minute
)

type Config struct {
	BaseURL             string
	SecureCookies       bool
	WebAuthnRPID        string
	WebAuthnRPName      string
	WebAuthnRPOrigins   []string
	BootstrapAdminEmail string
	BootstrapReissue    bool
	MaxReceiptBytes     int64
	ArrivalFutureDays   int // max days arrival may be after today (UTC); default 1
}

type Server struct {
	cfg        Config
	log        *slog.Logger
	store      *store.Store
	webauthn   *webauthn.WebAuthn
	templates  *template.Template
	static     http.Handler
	assetQuery string // e.g. "v=a1b2c3d4e5f6" for cache busting
}

func New(log *slog.Logger, db *sql.DB, cfg Config) (*Server, error) {
	if cfg.WebAuthnRPID == "" {
		cfg.WebAuthnRPID = "localhost"
	}
	if cfg.WebAuthnRPName == "" {
		cfg.WebAuthnRPName = "Beacon"
	}
	if len(cfg.WebAuthnRPOrigins) == 0 {
		cfg.WebAuthnRPOrigins = []string{"http://localhost:8080"}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = cfg.WebAuthnRPOrigins[0]
	}
	if cfg.MaxReceiptBytes <= 0 {
		cfg.MaxReceiptBytes = 5 << 20 // 5 MiB
	}
	if cfg.ArrivalFutureDays < 0 {
		cfg.ArrivalFutureDays = 0
	}

	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: cfg.WebAuthnRPName,
		RPID:          cfg.WebAuthnRPID,
		RPOrigins:     cfg.WebAuthnRPOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn: %w", err)
	}

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}
	assetHash, err := hashFS(staticFS)
	if err != nil {
		return nil, fmt.Errorf("static hash: %w", err)
	}
	assetQuery := "v=" + assetHash

	tmpl := template.New("").Funcs(template.FuncMap{
		"static": func(path string) string {
			path = strings.TrimPrefix(path, "/")
			path = strings.TrimPrefix(path, "static/")
			return "/static/" + path + "?" + assetQuery
		},
		"appVersion": version.Line,
		"navCurrent": navCurrent,
	})
	if _, err := tmpl.ParseFS(web.Templates, "templates/*.html"); err != nil {
		return nil, fmt.Errorf("templates: %w", err)
	}

	return &Server{
		cfg:        cfg,
		log:        log,
		store:      store.New(db),
		webauthn:   wa,
		templates:  tmpl,
		static:     http.FileServer(http.FS(staticFS)),
		assetQuery: assetQuery,
	}, nil
}

func hashFS(fsys fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		_, _ = h.Write([]byte(path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(b)
		_, _ = h.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func navCurrent(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

func withNavPath(r *http.Request, data any) any {
	if r == nil || r.URL == nil {
		return data
	}
	path := r.URL.Path
	switch d := data.(type) {
	case map[string]any:
		if d == nil {
			d = map[string]any{}
		}
		d["Path"] = path
		return d
	case occupantFormView:
		d.Path = path
		return d
	default:
		return data
	}
}

func cacheFingerprinted(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Store() *store.Store { return s.store }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFingerprinted(s.static)))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /logout", s.handleLogout)

	mux.HandleFunc("GET /invite/{token}", s.handleInvitePage)
	mux.HandleFunc("POST /webauthn/register/begin", s.handleRegisterBegin)
	mux.HandleFunc("POST /webauthn/register/finish", s.handleRegisterFinish)
	mux.HandleFunc("POST /webauthn/login/begin", s.handleLoginBegin)
	mux.HandleFunc("POST /webauthn/login/finish", s.handleLoginFinish)

	mux.Handle("GET /admin/users", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminUsers)))
	mux.Handle("GET /admin/users/invite", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminInvitePage)))
	mux.Handle("POST /admin/users/invite", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminCreateUser)))
	mux.Handle("GET /admin/users/{id}/edit", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminEditUser)))
	mux.Handle("POST /admin/users/{id}", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminUpdateUser)))
	mux.Handle("POST /admin/users/{id}/lock", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminLockUser)))
	mux.Handle("POST /admin/users/{id}/reinvite", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminReinvite)))
	mux.Handle("GET /admin/houses", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminHouses)))
	mux.Handle("POST /admin/rhls", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminCreateRHL)))
	mux.Handle("POST /admin/rhls/{id}", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminUpdateRHL)))
	mux.Handle("POST /admin/safe-houses", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminCreateSafeHouse)))
	mux.Handle("POST /admin/safe-houses/{id}", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminUpdateSafeHouse)))
	mux.Handle("GET /admin/audit", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAudit)))

	mux.Handle("GET /occupants", s.requireLogin(http.HandlerFunc(s.handleOccupants)))
	mux.Handle("GET /occupants/new", s.requireLogin(http.HandlerFunc(s.handleOccupantNew)))
	mux.Handle("POST /occupants", s.requireLogin(http.HandlerFunc(s.handleOccupantCreate)))
	mux.Handle("GET /occupants/{id}/edit", s.requireLogin(http.HandlerFunc(s.handleOccupantEdit)))
	mux.Handle("POST /occupants/{id}/rename", s.requireLogin(http.HandlerFunc(s.handleOccupantRename)))
	mux.Handle("POST /occupants/{id}/demographics", s.requireLogin(http.HandlerFunc(s.handleOccupantDemographics)))
	mux.Handle("POST /occupants/{id}/depart", s.requireLogin(http.HandlerFunc(s.handleOccupantDepart)))

	mux.Handle("GET /expenses", s.requireLogin(http.HandlerFunc(s.handleExpenses)))
	mux.Handle("GET /expenses/new", s.requireLogin(http.HandlerFunc(s.handleExpenseNew)))
	mux.Handle("POST /expenses", s.requireLogin(http.HandlerFunc(s.handleExpenseCreate)))
	mux.Handle("GET /expenses/{id}/receipt", s.requireLogin(http.HandlerFunc(s.handleExpenseReceipt)))
	mux.Handle("POST /expenses/{id}/delete", s.requireLogin(http.HandlerFunc(s.handleExpenseDelete)))

	mux.Handle("GET /operations", s.requireLogin(http.HandlerFunc(s.handleOperationalIssues)))
	mux.Handle("GET /operations/new", s.requireLogin(http.HandlerFunc(s.handleOperationalIssueNew)))
	mux.Handle("POST /operations", s.requireLogin(http.HandlerFunc(s.handleOperationalIssueCreate)))
	mux.Handle("GET /operations/{id}/edit", s.requireLogin(http.HandlerFunc(s.handleOperationalIssueEdit)))
	mux.Handle("POST /operations/{id}", s.requireLogin(http.HandlerFunc(s.handleOperationalIssueUpdate)))

	mux.Handle("GET /reports", s.requireLogin(http.HandlerFunc(s.handleReports)))
	mux.Handle("GET /reports/{houseID}/{file}", s.requireLogin(http.HandlerFunc(s.handleReportCSV)))

	mux.Handle("GET /safeguarding", s.requireLogin(http.HandlerFunc(s.handleSafeguarding)))
	mux.Handle("GET /safeguarding/new", s.requireLogin(http.HandlerFunc(s.handleSafeguardingNew)))
	mux.Handle("GET /safeguarding/document-37", s.requireLogin(http.HandlerFunc(s.handleDocument37)))
	mux.Handle("POST /safeguarding", s.requireLogin(http.HandlerFunc(s.handleSafeguardingCreate)))
	mux.Handle("GET /safeguarding/{id}/edit", s.requireLogin(http.HandlerFunc(s.handleSafeguardingEdit)))
	mux.Handle("POST /safeguarding/{id}", s.requireLogin(http.HandlerFunc(s.handleSafeguardingUpdate)))

	return mux
}

func (s *Server) Bootstrap(ctx context.Context) error {
	if _, _, err := s.store.EnsureDemoTenancy(ctx); err != nil {
		return fmt.Errorf("tenancy seed: %w", err)
	}
	if err := s.store.SyncNicknameKeys(ctx); err != nil {
		return fmt.Errorf("nickname keys: %w", err)
	}

	email := strings.TrimSpace(s.cfg.BootstrapAdminEmail)
	if email == "" {
		s.log.Info("bootstrap skipped: set BOOTSTRAP_ADMIN_EMAIL to create or refresh the first RHC invite")
		return nil
	}

	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}

	if n == 0 {
		u, token, err := s.store.CreateUserWithInvite(ctx, store.CreateUserInput{
			Email:       email,
			DisplayName: "RHC Admin",
			Role:        domain.RoleRHCAdmin,
		}, inviteTTL)
		if err != nil {
			return err
		}
		_ = s.store.Audit(ctx, nil, "bootstrap.admin", "user", idString(u.ID), map[string]any{"email": u.Email})
		s.logInvite(u.Email, token, "bootstrap RHC admin created")
		return nil
	}

	u, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.log.Info("bootstrap skipped: users already exist but BOOTSTRAP_ADMIN_EMAIL was not found",
				"email", email, "user_count", n)
			return nil
		}
		return err
	}

	creds, err := s.store.ListCredentials(ctx, u.ID)
	if err != nil {
		return err
	}
	if len(creds) > 0 && u.Status == domain.UserActive {
		s.log.Info("bootstrap skipped: admin already enrolled a passkey", "email", u.Email)
		return nil
	}

	if !s.cfg.BootstrapReissue {
		s.log.Info("bootstrap skipped: pending admin already exists (set BOOTSTRAP_REISSUE=true to mint a new invite URL)",
			"email", u.Email)
		return nil
	}

	// Explicit reissue: mint a fresh invite (raw token cannot be recovered from DB).
	if err := s.store.DeleteCredentialsForUser(ctx, u.ID); err != nil {
		return err
	}
	token, err := s.store.ReissueInvite(ctx, u.ID, inviteTTL)
	if err != nil {
		return err
	}
	_ = s.store.Audit(ctx, nil, "bootstrap.reinvite", "user", idString(u.ID), map[string]any{"email": u.Email})
	s.logInvite(u.Email, token, "bootstrap refreshed invite for pending admin")
	return nil
}

func (s *Server) logInvite(email, token, msg string) {
	url := strings.TrimRight(s.cfg.BaseURL, "/") + "/invite/" + token
	s.log.Info(msg, "email", email, "invite_url", url)
	// Also print a plain line so it is obvious in local terminals.
	fmt.Fprintf(os.Stderr, "\n=== Beacon invite ===\n%s\nOpen this on http://localhost:8080 (WebAuthn RP ID).\n\n", url)
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) currentUser(r *http.Request) (domain.User, bool) {
	u, ok := r.Context().Value(userKey).(domain.User)
	return u, ok
}

func (s *Server) withUser(r *http.Request, u domain.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userKey, u))
}

func (s *Server) loadSessionUser(r *http.Request) (domain.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return domain.User{}, false
	}
	u, err := s.store.UserBySession(r.Context(), c.Value)
	if err != nil {
		return domain.User{}, false
	}
	if !u.IsActive() {
		return domain.User{}, false
	}
	return u, true
}

func (s *Server) requireRole(role domain.Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.loadSessionUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if u.Role != role {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, s.withUser(r, u))
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   -1,
	})
}

func (s *Server) setChallengeCookie(w http.ResponseWriter, id int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     challengeCookie,
		Value:    idString(id),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   int(challengeTTL.Seconds()),
	})
}

func (s *Server) setInviteCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     inviteCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   int((2 * time.Hour).Seconds()),
	})
}

// One-time display of a freshly minted invite token (avoids putting tokens in redirect query strings).
func (s *Server) setInviteFlashCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     inviteFlashCookie,
		Value:    token,
		Path:     "/admin/users",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   300,
	})
}

func (s *Server) takeInviteFlashCookie(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(inviteFlashCookie)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     inviteFlashCookie,
		Value:    "",
		Path:     "/admin/users",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   -1,
	})
	return strings.TrimSpace(c.Value)
}

func (s *Server) challengeID(r *http.Request) (int64, error) {
	c, err := r.Cookie(challengeCookie)
	if err != nil {
		return 0, err
	}
	return parseID(c.Value)
}

func (s *Server) loadWAUser(ctx context.Context, u domain.User) (wauser.User, error) {
	creds, err := s.store.ListCredentials(ctx, u.ID)
	if err != nil {
		return wauser.User{}, err
	}
	return wauser.FromDomain(u, creds), nil
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data = withNavPath(r, data)
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template", "name", name, "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.DB().PingContext(ctx); err != nil {
		http.Error(w, "database unavailable\n", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ready\n"))
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SplitCSV is exported for main.
func SplitCSV(s string) []string { return splitCSV(s) }
