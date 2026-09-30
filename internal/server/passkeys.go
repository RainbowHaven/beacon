package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/passkeylabel"
	"github.com/RainbowHaven/beacon/internal/store"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const addPasskeyChallenge = "add_passkey"

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	passkeys, err := s.store.ListPasskeys(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	requestClientHints(w)
	s.render(w, r, "account.html", map[string]any{
		"Title":    "Account",
		"User":     &u,
		"Passkeys": passkeys,
		"Flash":    r.URL.Query().Get("flash"),
		"Error":    r.URL.Query().Get("error"),
	})
}

func (s *Server) handleHelpPasskeys(w http.ResponseWriter, r *http.Request) {
	var userPtr *domain.User
	if u, ok := s.loadSessionUser(r); ok {
		userPtr = &u
	}
	s.render(w, r, "help_passkeys.html", map[string]any{
		"Title": "Passkey help",
		"User":  userPtr,
	})
}

func (s *Server) handleAddPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
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
		s.log.Error("add passkey begin", "err", err)
		http.Error(w, "could not start adding a passkey", http.StatusInternalServerError)
		return
	}
	uid := u.ID
	chalID, err := s.store.SaveWebAuthnChallenge(r.Context(), &uid, addPasskeyChallenge, session, challengeTTL)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.setChallengeCookie(w, chalID)
	writeJSON(w, creation)
}

func (s *Server) handleAddPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	chalID, err := s.challengeID(r)
	if err != nil {
		http.Error(w, "missing challenge", http.StatusBadRequest)
		return
	}
	var session webauthn.SessionData
	owner, err := s.store.TakeWebAuthnChallenge(r.Context(), chalID, addPasskeyChallenge, &session)
	if err != nil || owner == nil || *owner != u.ID {
		http.Error(w, "challenge expired, please try again", http.StatusBadRequest)
		return
	}
	waUser, err := s.loadWAUser(r.Context(), u)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	cred, err := s.webauthn.FinishRegistration(waUser, session, r)
	if err != nil {
		s.log.Error("add passkey finish", "err", err)
		http.Error(w, "adding the passkey failed", http.StatusBadRequest)
		return
	}
	label := passkeyLabel(r, cred)
	if err := s.store.AddCredential(r.Context(), u.ID, cred, label); err != nil {
		if errors.Is(err, store.ErrCredentialExists) {
			http.Error(w, "this passkey is already registered", http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	s.clearCookie(w, challengeCookie)
	key := base64.RawURLEncoding.EncodeToString(cred.ID)
	_ = s.store.Audit(r.Context(), &u.ID, "auth.passkey.add", "user", idString(u.ID), map[string]any{
		"passkey": shortKey(key), "label": label,
	})
	writeJSON(w, map[string]string{"status": "ok", "redirect": "/account?flash=" + url.QueryEscape("Passkey added")})
}

func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	credID, err := parsePasskeyKey(r.PathValue("cred"))
	if err != nil {
		http.Error(w, "bad passkey", http.StatusBadRequest)
		return
	}
	var keep string
	if c, err := r.Cookie(sessionCookie); err == nil {
		keep = c.Value
	}
	label, err := s.store.DeleteOwnPasskey(r.Context(), u.ID, credID, keep)
	switch {
	case errors.Is(err, store.ErrLastPasskey):
		accountRedirect(w, r, "error", "You cannot remove your only passkey. Add another passkey first.")
		return
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "auth.passkey.remove", "user", idString(u.ID), map[string]any{
		"passkey": shortKey(r.PathValue("cred")), "label": label,
	})
	accountRedirect(w, r, "flash", "Passkey removed. Other devices were signed out.")
}

func accountRedirect(w http.ResponseWriter, r *http.Request, key, msg string) {
	http.Redirect(w, r, "/account?"+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}

// requestClientHints asks the browser to send the hints passkeyLabel needs on
// the WebAuthn requests that follow this page.
func requestClientHints(w http.ResponseWriter) {
	w.Header().Set("Accept-CH", "Sec-CH-UA-Platform, Sec-CH-UA-Platform-Version, Sec-CH-UA-Model, Sec-CH-UA-Mobile")
}

// passkeyLabel names a newly registered passkey. The label is stored once and
// never updated, and the User-Agent itself is not kept.
func passkeyLabel(r *http.Request, cred *webauthn.Credential) string {
	in := passkeylabel.FromHeader(r.Header)
	in.AAGUID = cred.Authenticator.AAGUID
	in.Attachment = string(cred.Authenticator.Attachment)
	for _, t := range cred.Transport {
		in.Transports = append(in.Transports, string(t))
	}
	return passkeylabel.Derive(in)
}

func parsePasskeyKey(key string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(b) == 0 {
		return nil, errors.New("bad passkey id")
	}
	return b, nil
}

// shortKey identifies a passkey in the audit log without storing the full credential ID.
func shortKey(key string) string {
	if len(key) > 8 {
		return key[:8]
	}
	return key
}
