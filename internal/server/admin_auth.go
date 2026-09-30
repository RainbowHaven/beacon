package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

func (s *Server) handleAdminUpdateEmail(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if id == actor.ID {
		http.Redirect(w, r, "/admin/users?error=cannot+modify+yourself", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		adminEditRedirect(w, r, id, "error", "Could not read the form")
		return
	}
	old, err := s.store.UpdateUserEmail(r.Context(), id, r.FormValue("email"))
	switch {
	case errors.Is(err, store.ErrEmailInvalid):
		adminEditRedirect(w, r, id, "error", "Enter a valid email address")
		return
	case errors.Is(err, store.ErrEmailTaken):
		adminEditRedirect(w, r, id, "error", "Another user already has that email")
		return
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	updated, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if old != updated.Email {
		_ = s.store.Audit(r.Context(), &actor.ID, "admin.user.email", "user", idString(id), map[string]any{
			"old_email": old, "new_email": updated.Email,
		})
	}
	adminEditRedirect(w, r, id, "flash", "Email updated")
}

func (s *Server) handleAdminDeletePasskey(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if id == actor.ID {
		http.Redirect(w, r, "/admin/users?error=cannot+modify+yourself", http.StatusSeeOther)
		return
	}
	credID, err := parsePasskeyKey(r.PathValue("cred"))
	if err != nil {
		http.Error(w, "bad passkey", http.StatusBadRequest)
		return
	}
	label, err := s.store.AdminDeletePasskey(r.Context(), id, credID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.passkey.remove", "user", idString(id), map[string]any{
		"passkey": shortKey(r.PathValue("cred")), "label": label,
	})
	adminEditRedirect(w, r, id, "flash", "Passkey removed and the user was signed out")
}

// RecoverAdmin replaces all passkeys of an RHC admin with a fresh invite and
// prints the invite URL. It is the operator path when no other RHC admin can
// sign in to issue a New invite.
func (s *Server) RecoverAdmin(ctx context.Context, email string) error {
	u, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return fmt.Errorf("find %q: %w", email, err)
	}
	if u.Role != domain.RoleRHCAdmin {
		return fmt.Errorf("%s is %s, not rhc_admin", u.Email, u.Role)
	}
	if err := s.store.DeleteCredentialsForUser(ctx, u.ID); err != nil {
		return err
	}
	token, err := s.store.ReissueInvite(ctx, u.ID, inviteTTL)
	if err != nil {
		return err
	}
	_ = s.store.Audit(ctx, nil, "recovery.admin.reinvite", "user", idString(u.ID), map[string]any{"email": u.Email})
	s.logInvite(u.Email, token, "recovery invite for RHC admin")
	return nil
}

func adminEditRedirect(w http.ResponseWriter, r *http.Request, id int64, key, msg string) {
	http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?"+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}
