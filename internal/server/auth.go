package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	u, ok := s.loadSessionUser(r)
	var userPtr *domain.User
	var counts []domain.HeadcountRow
	if ok {
		userPtr = &u
		if houses, err := s.housesForUser(r, u); err == nil {
			counts, _ = s.store.HeadcountByHouses(r.Context(), houseIDs(houses))
		}
	}
	s.render(w, "home.html", map[string]any{
		"Title":     "Home",
		"User":      userPtr,
		"LoggedIn":  ok,
		"Headcount": counts,
	})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.loadSessionUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", map[string]any{
		"Title": "Log in",
		"Error": r.URL.Query().Get("error"),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.RevokeSession(r.Context(), c.Value)
	}
	s.clearCookie(w, sessionCookie)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleInvitePage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	u, err := s.store.LookupInvite(r.Context(), token)
	if err != nil {
		s.log.Warn("invite page lookup failed", "err", err, "token_len", len(token), "host", r.Host)
		reason := "missing, expired, or already used"
		switch {
		case errors.Is(err, store.ErrUserLocked):
			reason = "the account is locked"
		case errors.Is(err, store.ErrInviteInvalid):
			reason = "missing, expired, already used, or from a different database"
		case token == "":
			reason = "the link has no token"
		}
		s.render(w, "invite_invalid.html", map[string]any{
			"Title":  "Invite",
			"Reason": reason,
		})
		return
	}
	s.setInviteCookie(w, token)
	s.render(w, "invite.html", map[string]any{
		"Title": "Invite",
		"Email": u.Email,
		"Token": token,
		"Role":  u.Role,
	})
}

func (s *Server) inviteToken(r *http.Request) string {
	if c, err := r.Cookie(inviteCookie); err == nil {
		if t := strings.TrimSpace(c.Value); t != "" {
			return t
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func (s *Server) handleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	token := s.inviteToken(r)
	if token == "" {
		// Begin may send { "token": "..." } in the JSON body (not the WebAuthn payload).
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			token = strings.TrimSpace(body.Token)
		}
	}
	if token == "" {
		http.Error(w, "missing invite token", http.StatusBadRequest)
		return
	}
	u, err := s.store.LookupInvite(r.Context(), token)
	if err != nil {
		s.log.Warn("register begin invite lookup failed", "err", err)
		http.Error(w, "invalid invite", http.StatusBadRequest)
		return
	}
	waUser, err := s.loadWAUser(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	creation, session, err := s.webauthn.BeginRegistration(waUser,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
		webauthn.WithExclusions(webauthn.Credentials(waUser.WebAuthnCredentials()).CredentialDescriptors()),
	)
	if err != nil {
		s.log.Error("register begin", "err", err)
		http.Error(w, "could not start registration", http.StatusInternalServerError)
		return
	}
	uid := u.ID
	chalID, err := s.store.SaveWebAuthnChallenge(r.Context(), &uid, "register", session, challengeTTL)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.setInviteCookie(w, token)
	s.setChallengeCookie(w, chalID)
	writeJSON(w, creation)
}

func (s *Server) handleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	// Never read r.Body here for the invite token — FinishRegistration needs the WebAuthn JSON body intact.
	token := s.inviteToken(r)
	if token == "" {
		http.Error(w, "missing invite token", http.StatusBadRequest)
		return
	}
	u, err := s.store.LookupInvite(r.Context(), token)
	if err != nil {
		s.log.Warn("register finish invite lookup failed", "err", err)
		http.Error(w, "invalid invite", http.StatusBadRequest)
		return
	}
	chalID, err := s.challengeID(r)
	if err != nil {
		http.Error(w, "missing challenge", http.StatusBadRequest)
		return
	}
	var session webauthn.SessionData
	if _, err := s.store.TakeWebAuthnChallenge(r.Context(), chalID, "register", &session); err != nil {
		http.Error(w, "challenge expired", http.StatusBadRequest)
		return
	}
	waUser, err := s.loadWAUser(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	cred, err := s.webauthn.FinishRegistration(waUser, session, r)
	if err != nil {
		s.log.Error("register finish", "err", err)
		http.Error(w, "registration failed", http.StatusBadRequest)
		return
	}
	if err := s.store.SaveCredential(r.Context(), u.ID, cred); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := s.store.MarkInviteUsed(r.Context(), token); err != nil {
		http.Error(w, "invite invalid", http.StatusBadRequest)
		return
	}
	if err := s.store.ActivateUser(r.Context(), u.ID); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	raw, err := s.store.CreateSession(r.Context(), u.ID, sessionTTL)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.clearCookie(w, challengeCookie)
	s.clearCookie(w, inviteCookie)
	s.setSessionCookie(w, raw)
	_ = s.store.Audit(r.Context(), &u.ID, "auth.register", "user", idString(u.ID), nil)
	writeJSON(w, map[string]string{"status": "ok", "redirect": "/"})
}

func (s *Server) handleLoginBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Email) == "" {
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}
	u, err := s.store.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		http.Error(w, "unknown user", http.StatusBadRequest)
		return
	}
	if u.Status == domain.UserLocked {
		http.Error(w, "account locked", http.StatusForbidden)
		return
	}
	waUser, err := s.loadWAUser(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if len(waUser.Credentials) == 0 {
		http.Error(w, "no passkey enrolled", http.StatusBadRequest)
		return
	}
	assertion, session, err := s.webauthn.BeginLogin(waUser)
	if err != nil {
		s.log.Error("login begin", "err", err)
		http.Error(w, "could not start login", http.StatusInternalServerError)
		return
	}
	uid := u.ID
	chalID, err := s.store.SaveWebAuthnChallenge(r.Context(), &uid, "login", session, challengeTTL)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.setChallengeCookie(w, chalID)
	writeJSON(w, assertion)
}

func (s *Server) handleLoginFinish(w http.ResponseWriter, r *http.Request) {
	chalID, err := s.challengeID(r)
	if err != nil {
		http.Error(w, "missing challenge", http.StatusBadRequest)
		return
	}
	var session webauthn.SessionData
	userID, err := s.store.TakeWebAuthnChallenge(r.Context(), chalID, "login", &session)
	if err != nil || userID == nil {
		http.Error(w, "challenge expired", http.StatusBadRequest)
		return
	}
	u, err := s.store.GetUser(r.Context(), *userID)
	if err != nil {
		http.Error(w, "unknown user", http.StatusBadRequest)
		return
	}
	if u.Status == domain.UserLocked {
		http.Error(w, "account locked", http.StatusForbidden)
		return
	}
	waUser, err := s.loadWAUser(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	cred, err := s.webauthn.FinishLogin(waUser, session, r)
	if err != nil {
		s.log.Error("login finish", "err", err)
		http.Error(w, "login failed", http.StatusBadRequest)
		return
	}
	if err := s.store.SaveCredential(r.Context(), u.ID, cred); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	raw, err := s.store.CreateSession(r.Context(), u.ID, sessionTTL)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.clearCookie(w, challengeCookie)
	s.setSessionCookie(w, raw)
	_ = s.store.Audit(r.Context(), &u.ID, "auth.login", "user", idString(u.ID), nil)
	writeJSON(w, map[string]string{"status": "ok", "redirect": "/"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
