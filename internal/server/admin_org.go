package server

import (
	"net/http"
	"strings"

	"github.com/magiconair/beacon/internal/domain"
)

func (s *Server) handleAdminHouses(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	rhls, err := s.store.ListRHLs(r.Context())
	if err != nil {
		http.Error(w, "failed to load RHLs", http.StatusInternalServerError)
		return
	}
	houses, err := s.store.ListSafeHouses(r.Context())
	if err != nil {
		http.Error(w, "failed to load safe houses", http.StatusInternalServerError)
		return
	}
	rhlName := map[int64]string{}
	for _, rhl := range rhls {
		rhlName[rhl.ID] = rhl.Name
	}
	type houseRow struct {
		domain.SafeHouse
		RHLName string
	}
	rows := make([]houseRow, 0, len(houses))
	for _, h := range houses {
		rows = append(rows, houseRow{SafeHouse: h, RHLName: rhlName[h.RHLID]})
	}
	s.render(w, r, "admin_houses.html", map[string]any{
		"Title":      "Houses",
		"User":       &actor,
		"RHLs":       rhls,
		"Houses":     rows,
		"Currencies": expenseCurrencies,
		"Flash":      r.URL.Query().Get("flash"),
		"Error":      r.URL.Query().Get("error"),
	})
}

func formActive(r *http.Request) bool {
	return r.FormValue("active") != "false"
}

func (s *Server) handleAdminCreateRHL(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/houses?error=bad+form", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/admin/houses?error=name+required", http.StatusSeeOther)
		return
	}
	active := formActive(r)
	rhl, err := s.store.CreateRHL(r.Context(), name, active)
	if err != nil {
		http.Redirect(w, r, "/admin/houses?error=create+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.rhl.create", "rhl", idString(rhl.ID), map[string]any{"name": rhl.Name})
	http.Redirect(w, r, "/admin/houses?flash=rhl+created", http.StatusSeeOther)
}

func (s *Server) handleAdminUpdateRHL(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/houses?error=bad+form", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/admin/houses?error=name+required", http.StatusSeeOther)
		return
	}
	active := formActive(r)
	if err := s.store.UpdateRHL(r.Context(), id, name, active); err != nil {
		http.Redirect(w, r, "/admin/houses?error=update+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.rhl.update", "rhl", idString(id), map[string]any{"name": name, "active": active})
	http.Redirect(w, r, "/admin/houses?flash=rhl+updated", http.StatusSeeOther)
}

func (s *Server) handleAdminCreateSafeHouse(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/houses?error=bad+form", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	rhlID, err := parseID(r.FormValue("rhl_id"))
	if err != nil || name == "" {
		http.Redirect(w, r, "/admin/houses?error=rhl+and+name+required", http.StatusSeeOther)
		return
	}
	currency, ok := normalizeCurrency(r.FormValue("default_currency"))
	if !ok {
		currency = "CAD"
	}
	active := formActive(r)
	h, err := s.store.CreateSafeHouse(r.Context(), rhlID, name, currency, active)
	if err != nil {
		http.Redirect(w, r, "/admin/houses?error=create+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.house.create", "safe_house", idString(h.ID), map[string]any{
		"name": h.Name, "rhl_id": h.RHLID, "currency": h.DefaultCurrency,
	})
	http.Redirect(w, r, "/admin/houses?flash=house+created", http.StatusSeeOther)
}

func (s *Server) handleAdminUpdateSafeHouse(w http.ResponseWriter, r *http.Request) {
	actor, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/houses?error=bad+form", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	rhlID, err := parseID(r.FormValue("rhl_id"))
	if err != nil || name == "" {
		http.Redirect(w, r, "/admin/houses?error=rhl+and+name+required", http.StatusSeeOther)
		return
	}
	currency, ok := normalizeCurrency(r.FormValue("default_currency"))
	if !ok {
		http.Redirect(w, r, "/admin/houses?error=invalid+currency", http.StatusSeeOther)
		return
	}
	active := formActive(r)
	if err := s.store.UpdateSafeHouse(r.Context(), id, rhlID, name, currency, active); err != nil {
		http.Redirect(w, r, "/admin/houses?error=update+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.house.update", "safe_house", idString(id), map[string]any{
		"name": name, "rhl_id": rhlID, "currency": currency, "active": active,
	})
	http.Redirect(w, r, "/admin/houses?flash=house+updated", http.StatusSeeOther)
}
