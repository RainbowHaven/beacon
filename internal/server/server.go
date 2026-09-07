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
	"github.com/google/uuid"
	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
	"github.com/magiconair/beacon/internal/wauser"
	"github.com/magiconair/beacon/web"
)

const (
	sessionCookie   = "beacon_session"
	challengeCookie = "beacon_wa_challenge"
	inviteCookie    = "beacon_invite"
	inviteTTL       = 7 * 24 * time.Hour
	sessionTTL      = 14 * 24 * time.Hour
	challengeTTL    = 5 * time.Minute
)

type Config struct {
	BaseURL             string
	SecureCookies       bool
	WebAuthnRPID        string
	WebAuthnRPName      string
	WebAuthnRPOrigins   []string
	BootstrapAdminEmail string
	BootstrapReissue    bool
	IdentityPublicKey   [32]byte
	IdentityKeyID       string
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
	if strings.TrimSpace(cfg.IdentityKeyID) == "" {
		return nil, errors.New("IdentityKeyID is required")
	}
	if cfg.IdentityPublicKey == ([32]byte{}) {
		return nil, errors.New("IdentityPublicKey is required")
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

func (s *Server) Store() *store.Store { return s.store }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
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
	mux.Handle("POST /admin/users", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminCreateUser)))
	mux.Handle("POST /admin/users/{id}/lock", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminLockUser)))
	mux.Handle("POST /admin/users/{id}/reinvite", s.requireRole(domain.RoleRHCAdmin, http.HandlerFunc(s.handleAdminReinvite)))

	mux.Handle("GET /occupants", s.requireLogin(http.HandlerFunc(s.handleOccupants)))
	mux.Handle("GET /occupants/new", s.requireLogin(http.HandlerFunc(s.handleOccupantNew)))
	mux.Handle("POST /occupants/handoff", s.requireLogin(http.HandlerFunc(s.handleOccupantHandoff)))
	mux.Handle("POST /occupants", s.requireLogin(http.HandlerFunc(s.handleOccupantCreate)))
	mux.Handle("POST /occupants/{id}/depart", s.requireLogin(http.HandlerFunc(s.handleOccupantDepart)))

	return mux
}

func (s *Server) Bootstrap(ctx context.Context) error {
	if _, _, err := s.store.EnsureDemoTenancy(ctx); err != nil {
		return fmt.Errorf("tenancy seed: %w", err)
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
		_ = s.store.Audit(ctx, nil, "bootstrap.admin", "user", u.ID.String(), map[string]any{"email": u.Email})
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
	_ = s.store.Audit(ctx, nil, "bootstrap.reinvite", "user", u.ID.String(), map[string]any{"email": u.Email})
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

func (s *Server) setChallengeCookie(w http.ResponseWriter, id uuid.UUID) {
	http.SetCookie(w, &http.Cookie{
		Name:     challengeCookie,
		Value:    id.String(),
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

func (s *Server) challengeID(r *http.Request) (uuid.UUID, error) {
	c, err := r.Cookie(challengeCookie)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(c.Value)
}

func (s *Server) loadWAUser(ctx context.Context, u domain.User) (wauser.User, error) {
	creds, err := s.store.ListCredentials(ctx, u.ID)
	if err != nil {
		return wauser.User{}, err
	}
	return wauser.FromDomain(u, creds), nil
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
