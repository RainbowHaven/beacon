package server

import (
	"net/http"
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
	passkeyCounts, err := s.store.CountPasskeysByUser(r.Context())
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	actor, _ := s.currentUser(r)
	token := s.takeInviteFlashCookie(w, r)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("invite_token"))
	}
	inviteURL := ""
	invitePath := ""
	if token != "" {
		invitePath = "/invite/" + token
		inviteURL = strings.TrimRight(s.cfg.BaseURL, "/") + invitePath
	}
	s.render(w, r, "admin_users.html", map[string]any{
		"Title":         "Users",
		"User":          &actor,
		"Users":         users,
		"PasskeyCounts": passkeyCounts,
		"Flash":         r.URL.Query().Get("flash"),
		"InviteURL":     inviteURL,
		"InvitePath":    invitePath,
		"Error":         r.URL.Query().Get("error"),
	})
}

func (s *Server) handleAdminInvitePage(w http.ResponseWriter, r *http.Request) {
	rhls, _ := s.store.ListRHLs(r.Context())
	houses, _ := s.store.ListSafeHouses(r.Context())
	actor, _ := s.currentUser(r)
	s.render(w, r, "admin_invite.html", map[string]any{
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
	s.setInviteFlashCookie(w, token)
	http.Redirect(w, r, "/admin/users?flash=created", http.StatusSeeOther)
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
	s.setInviteFlashCookie(w, token)
	http.Redirect(w, r, "/admin/users?flash=reinvited", http.StatusSeeOther)
}

func (s *Server) handleAdminEditUser(w http.ResponseWriter, r *http.Request) {
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
	target, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	passkeys, err := s.store.ListPasskeys(r.Context(), id)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	rhls, _ := s.store.ListRHLs(r.Context())
	houses, _ := s.store.ListSafeHouses(r.Context())
	var selectedRHL, selectedHouse int64
	if target.RHLID != nil {
		selectedRHL = *target.RHLID
	}
	if target.SafeHouseID != nil {
		selectedHouse = *target.SafeHouseID
	}
	s.render(w, r, "admin_user_edit.html", map[string]any{
		"Title":         "Edit user",
		"User":          &actor,
		"Target":        target,
		"SelectedRHL":   selectedRHL,
		"SelectedHouse": selectedHouse,
		"RHLs":          rhls,
		"SafeHouses":    houses,
		"Passkeys":      passkeys,
		"Flash":         r.URL.Query().Get("flash"),
		"Error":         r.URL.Query().Get("error"),
	})
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?error=bad+form", http.StatusSeeOther)
		return
	}
	role := domain.Role(r.FormValue("role"))
	in := store.UpdateUserInput{
		DisplayName: strings.TrimSpace(r.FormValue("display_name")),
		Role:        role,
	}
	switch role {
	case domain.RoleRHCAdmin:
		// clear scope
	case domain.RoleRHLAdmin:
		rid, err := parseID(r.FormValue("rhl_id"))
		if err != nil {
			http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?error=rhl+required", http.StatusSeeOther)
			return
		}
		in.RHLID = &rid
	case domain.RoleSafeHouseManager:
		hid, err := parseID(r.FormValue("safe_house_id"))
		if err != nil {
			http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?error=safe+house+required", http.StatusSeeOther)
			return
		}
		in.SafeHouseID = &hid
		house, err := s.store.GetSafeHouse(r.Context(), hid)
		if err != nil {
			http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?error=safe+house+required", http.StatusSeeOther)
			return
		}
		rid := house.RHLID
		in.RHLID = &rid
	default:
		http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?error=invalid+role", http.StatusSeeOther)
		return
	}
	if err := s.store.UpdateUser(r.Context(), id, in); err != nil {
		http.Redirect(w, r, "/admin/users/"+idString(id)+"/edit?error=update+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.user.update", "user", idString(id), map[string]any{
		"role": in.Role, "rhl_id": in.RHLID, "safe_house_id": in.SafeHouseID,
	})
	http.Redirect(w, r, "/admin/users?flash=user+updated", http.StatusSeeOther)
}
