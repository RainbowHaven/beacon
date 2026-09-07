package server

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
)

func (s *Server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.loadSessionUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, s.withUser(r, u))
	})
}

func (s *Server) housesForUser(r *http.Request, u domain.User) ([]domain.SafeHouse, error) {
	switch u.Role {
	case domain.RoleRHCAdmin:
		return s.store.ListSafeHouses(r.Context())
	case domain.RoleRHLAdmin:
		if u.RHLID == nil {
			return nil, nil
		}
		return s.store.ListSafeHousesByRHL(r.Context(), *u.RHLID)
	case domain.RoleSafeHouseManager:
		if u.SafeHouseID == nil {
			return nil, nil
		}
		h, err := s.store.GetSafeHouse(r.Context(), *u.SafeHouseID)
		if err != nil {
			return nil, err
		}
		return []domain.SafeHouse{h}, nil
	default:
		return nil, nil
	}
}

func (s *Server) canAccessHouse(u domain.User, house domain.SafeHouse) bool {
	switch u.Role {
	case domain.RoleRHCAdmin:
		return true
	case domain.RoleRHLAdmin:
		return u.RHLID != nil && *u.RHLID == house.RHLID
	case domain.RoleSafeHouseManager:
		return u.SafeHouseID != nil && *u.SafeHouseID == house.ID
	default:
		return false
	}
}

func houseIDs(houses []domain.SafeHouse) []uuid.UUID {
	ids := make([]uuid.UUID, len(houses))
	for i, h := range houses {
		ids[i] = h.ID
	}
	return ids
}

func (s *Server) handleOccupants(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	ids := houseIDs(houses)
	currentOnly := r.URL.Query().Get("all") != "1"
	occupants, err := s.store.ListOccupantsByHouses(r.Context(), ids, currentOnly)
	if err != nil {
		http.Error(w, "failed to list occupants", http.StatusInternalServerError)
		return
	}
	counts, err := s.store.HeadcountByHouses(r.Context(), ids)
	if err != nil {
		http.Error(w, "failed to load headcount", http.StatusInternalServerError)
		return
	}
	houseName := map[uuid.UUID]string{}
	for _, h := range houses {
		houseName[h.ID] = h.Name
	}
	type row struct {
		domain.Occupant
		HouseName string
		Current   bool
	}
	rows := make([]row, 0, len(occupants))
	today := time.Now().UTC()
	for _, o := range occupants {
		rows = append(rows, row{
			Occupant:  o,
			HouseName: houseName[o.SafeHouseID],
			Current:   o.Current(today),
		})
	}
	s.render(w, "occupants.html", map[string]any{
		"Title":       "Occupants",
		"User":        &u,
		"Rows":        rows,
		"Headcount":   counts,
		"CurrentOnly": currentOnly,
		"Flash":       r.URL.Query().Get("ok"),
		"Error":       r.URL.Query().Get("error"),
	})
}

func (s *Server) handleOccupantNew(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	s.render(w, "occupant_new.html", map[string]any{
		"Title":   "Add occupant",
		"User":    &u,
		"Houses":  houses,
		"Today":   time.Now().UTC().Format("2006-01-02"),
		"Error":   r.URL.Query().Get("error"),
	})
}

func (s *Server) handleOccupantHandoff(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	houseID, err := uuid.Parse(strings.TrimSpace(r.FormValue("safe_house_id")))
	if err != nil {
		http.Redirect(w, r, "/occupants/new?error=invalid+house", http.StatusSeeOther)
		return
	}
	arrived := strings.TrimSpace(r.FormValue("arrived_at"))
	if arrived == "" {
		http.Redirect(w, r, "/occupants/new?error=arrival+date+required", http.StatusSeeOther)
		return
	}
	if _, err := time.Parse("2006-01-02", arrived); err != nil {
		http.Redirect(w, r, "/occupants/new?error=invalid+arrival+date", http.StatusSeeOther)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), houseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.render(w, "occupant_handoff.html", map[string]any{
		"Title":              "Resident handoff",
		"User":               &u,
		"House":              house,
		"ArrivedAt":          arrived,
		"IdentityPublicKey":  base64.StdEncoding.EncodeToString(s.cfg.IdentityPublicKey[:]),
		"IdentityKeyID":      s.cfg.IdentityKeyID,
		"Handoff":            true,
	})
}

func (s *Server) handleOccupantCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	houseID, err := uuid.Parse(strings.TrimSpace(r.FormValue("safe_house_id")))
	if err != nil {
		http.Redirect(w, r, "/occupants/new?error=invalid+house", http.StatusSeeOther)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), houseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	arrived, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("arrived_at")))
	if err != nil {
		http.Redirect(w, r, "/occupants/new?error=invalid+arrival+date", http.StatusSeeOther)
		return
	}
	keyID := strings.TrimSpace(r.FormValue("key_id"))
	if keyID != s.cfg.IdentityKeyID {
		http.Redirect(w, r, "/occupants?error=unexpected+key_id", http.StatusSeeOther)
		return
	}
	ctB64 := strings.TrimSpace(r.FormValue("identity_ciphertext"))
	ct, err := base64.StdEncoding.DecodeString(ctB64)
	if err != nil || len(ct) < 48 {
		http.Redirect(w, r, "/occupants?error=missing+sealed+identity", http.StatusSeeOther)
		return
	}
	// Reject accidental plaintext identity fields if a buggy client posts them.
	if strings.TrimSpace(r.FormValue("legal_name")) != "" || strings.TrimSpace(r.FormValue("refugee_id")) != "" {
		http.Redirect(w, r, "/occupants?error=identity+must+be+sealed+client-side", http.StatusSeeOther)
		return
	}

	uid := u.ID
	o, err := s.store.CreateOccupant(r.Context(), store.CreateOccupantInput{
		SafeHouseID:        houseID,
		Nickname:           r.FormValue("nickname"),
		ArrivedAt:          arrived,
		IdentityCiphertext: ct,
		KeyID:              keyID,
		CreatedBy:          &uid,
	})
	if err != nil {
		s.log.Error("create occupant", "err", err)
		http.Redirect(w, r, "/occupants?error=could+not+save+occupant", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &uid, "occupant.create", "occupant", o.ID.String(), map[string]any{
		"safe_house_id": houseID.String(),
		"key_id":        keyID,
	})
	http.Redirect(w, r, "/occupants?ok=added", http.StatusSeeOther)
}

func (s *Server) handleOccupantDepart(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	o, err := s.store.GetOccupant(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), o.SafeHouseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	day := time.Now().UTC()
	if v := strings.TrimSpace(r.FormValue("departed_at")); v != "" {
		parsed, err := time.Parse("2006-01-02", v)
		if err != nil {
			http.Redirect(w, r, "/occupants?error=invalid+departure+date", http.StatusSeeOther)
			return
		}
		day = parsed
	}
	if err := s.store.MarkOccupantDeparted(r.Context(), id, day); err != nil {
		http.Redirect(w, r, "/occupants?error=could+not+mark+departed", http.StatusSeeOther)
		return
	}
	uid := u.ID
	_ = s.store.Audit(r.Context(), &uid, "occupant.depart", "occupant", id.String(), map[string]any{
		"departed_at": day.Format("2006-01-02"),
	})
	http.Redirect(w, r, "/occupants?ok=departed", http.StatusSeeOther)
}
