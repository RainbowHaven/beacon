package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
)

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	actor, _ := s.currentUser(r)
	s.render(w, "admin_users.html", map[string]any{
		"Title":     "Users",
		"User":      &actor,
		"Users":     users,
		"Flash":     r.URL.Query().Get("flash"),
		"InviteURL": r.URL.Query().Get("invite"),
		"Error":     r.URL.Query().Get("error"),
	})
}

func (s *Server) handleAdminInvitePage(w http.ResponseWriter, r *http.Request) {
	rhls, _ := s.store.ListRHLs(r.Context())
	houses, _ := s.store.ListSafeHouses(r.Context())
	actor, _ := s.currentUser(r)
	s.render(w, "admin_invite.html", map[string]any{
		"Title":      "Invite user",
		"User":       &actor,
		"RHLs":       rhls,
		"SafeHouses": houses,
		"Error":      r.URL.Query().Get("error"),
	})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/users/invite?error=bad+form", http.StatusSeeOther)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	role := domain.Role(r.FormValue("role"))
	display := strings.TrimSpace(r.FormValue("display_name"))
	in := store.CreateUserInput{Email: email, DisplayName: display, Role: role}

	switch role {
	case domain.RoleRHCAdmin:
		// no scope
	case domain.RoleRHLAdmin:
		id, err := parseID(r.FormValue("rhl_id"))
		if err != nil {
			http.Redirect(w, r, "/admin/users/invite?error=rhl+required", http.StatusSeeOther)
			return
		}
		in.RHLID = &id
	case domain.RoleSafeHouseManager:
		id, err := parseID(r.FormValue("safe_house_id"))
		if err != nil {
			http.Redirect(w, r, "/admin/users/invite?error=safe+house+required", http.StatusSeeOther)
			return
		}
		in.SafeHouseID = &id
		houseList, _ := s.store.ListSafeHouses(r.Context())
		for _, h := range houseList {
			if h.ID == id {
				rid := h.RHLID
				in.RHLID = &rid
				break
			}
		}
	default:
		http.Redirect(w, r, "/admin/users/invite?error=invalid+role", http.StatusSeeOther)
		return
	}

	u, token, err := s.store.CreateUserWithInvite(r.Context(), in, inviteTTL)
	if err != nil {
		msg := "create+failed"
		if err == store.ErrEmailTaken {
			msg = "email+taken"
		}
		http.Redirect(w, r, "/admin/users/invite?error="+msg, http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.invite", "user", idString(u.ID), map[string]any{"email": u.Email, "role": u.Role})
	inviteURL := s.cfg.BaseURL + "/invite/" + token
	http.Redirect(w, r, "/admin/users?flash=created&invite="+url.QueryEscape(inviteURL), http.StatusSeeOther)
}

func (s *Server) handleAdminLockUser(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.LockUser(r.Context(), id); err != nil {
		http.Error(w, "lock failed", http.StatusInternalServerError)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.lock", "user", idString(id), nil)
	http.Redirect(w, r, "/admin/users?flash=locked", http.StatusSeeOther)
}

func (s *Server) handleAdminReinvite(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.DeleteCredentialsForUser(r.Context(), id); err != nil {
		http.Error(w, "reinvite failed", http.StatusInternalServerError)
		return
	}
	token, err := s.store.ReissueInvite(r.Context(), id, inviteTTL)
	if err != nil {
		http.Error(w, "reinvite failed", http.StatusInternalServerError)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.reinvite", "user", idString(id), nil)
	inviteURL := s.cfg.BaseURL + "/invite/" + token
	http.Redirect(w, r, "/admin/users?flash=reinvited&invite="+url.QueryEscape(inviteURL), http.StatusSeeOther)
}
