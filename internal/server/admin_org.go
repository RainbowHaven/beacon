package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
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
		rhlName[rhl.ID] = rhl.Label()
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

const maxSleepingPlaces = 10000

// parseSleepingPlaces reads an optional positive whole number; blank means not set.
func parseSleepingPlaces(s string) (*int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > maxSleepingPlaces {
		return nil, false
	}
	return &n, true
}

// rhlCodeErrorRedirect maps store code errors to a friendly message; ok is false for other errors.
func rhlCodeErrorRedirect(err error) (string, bool) {
	switch {
	case errors.Is(err, store.ErrRHLCodeTaken):
		return "/admin/houses?error=RHL+code+already+in+use", true
	case errors.Is(err, store.ErrRHLCodeInvalid):
		return invalidRHLCodeRedirect, true
	}
	return "", false
}

const invalidRHLCodeRedirect = "/admin/houses?error=RHL+code+must+be+2-12+letters,+digits+or+hyphens"

const invalidSleepingPlacesRedirect = "/admin/houses?error=sleeping+places+must+be+a+whole+number+above+zero"

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
	code, ok := domain.NormalizeRHLCode(r.FormValue("code"))
	if !ok {
		http.Redirect(w, r, invalidRHLCodeRedirect, http.StatusSeeOther)
		return
	}
	active := formActive(r)
	rhl, err := s.store.CreateRHLWithCode(r.Context(), name, code, active)
	if err != nil {
		if to, ok := rhlCodeErrorRedirect(err); ok {
			http.Redirect(w, r, to, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/houses?error=create+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.rhl.create", "rhl", idString(rhl.ID), map[string]any{
		"name": rhl.Name, "code": rhl.Code, "active": rhl.Active,
	})
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
	code, ok := domain.NormalizeRHLCode(r.FormValue("code"))
	if !ok {
		http.Redirect(w, r, invalidRHLCodeRedirect, http.StatusSeeOther)
		return
	}
	active := formActive(r)
	if err := s.store.UpdateRHL(r.Context(), id, name, code, active); err != nil {
		if to, ok := rhlCodeErrorRedirect(err); ok {
			http.Redirect(w, r, to, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/houses?error=update+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.rhl.update", "rhl", idString(id), map[string]any{
		"name": name, "code": code, "active": active,
	})
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
	places, ok := parseSleepingPlaces(r.FormValue("approved_sleeping_places"))
	if !ok {
		http.Redirect(w, r, invalidSleepingPlacesRedirect, http.StatusSeeOther)
		return
	}
	active := formActive(r)
	h, err := s.store.CreateSafeHouseWithCapacity(r.Context(), rhlID, name, currency, places, active)
	if err != nil {
		http.Redirect(w, r, "/admin/houses?error=create+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.house.create", "safe_house", idString(h.ID), map[string]any{
		"name": h.Name, "rhl_id": h.RHLID, "currency": h.DefaultCurrency,
		"approved_sleeping_places": h.ApprovedSleepingPlaces, "active": h.Active,
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
	places, ok := parseSleepingPlaces(r.FormValue("approved_sleeping_places"))
	if !ok {
		http.Redirect(w, r, invalidSleepingPlacesRedirect, http.StatusSeeOther)
		return
	}
	active := formActive(r)
	if err := s.store.UpdateSafeHouse(r.Context(), id, rhlID, name, currency, places, active); err != nil {
		http.Redirect(w, r, "/admin/houses?error=update+failed", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &actor.ID, "admin.house.update", "safe_house", idString(id), map[string]any{
		"name": name, "rhl_id": rhlID, "currency": currency, "approved_sleeping_places": places, "active": active,
	})
	http.Redirect(w, r, "/admin/houses?flash=house+updated", http.StatusSeeOther)
}
