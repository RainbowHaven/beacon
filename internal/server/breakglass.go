package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

func (s *Server) handleBreakGlass(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	occupants, err := s.store.ListOccupantsByHouses(r.Context(), houseIDs(houses), false)
	if err != nil {
		http.Error(w, "failed to list occupants", http.StatusInternalServerError)
		return
	}
	houseName := map[int64]string{}
	for _, h := range houses {
		houseName[h.ID] = h.Name
	}
	type row struct {
		ID        int64
		Nickname  string
		HouseName string
		KeyID     string
		Arrived   string
		Departed  string
		Current   bool
	}
	rows := make([]row, 0, len(occupants))
	today := time.Now().UTC()
	for _, o := range occupants {
		departed := "—"
		if o.DepartedAt != nil {
			departed = formatUSDate(*o.DepartedAt)
		}
		rows = append(rows, row{
			ID:        o.ID,
			Nickname:  o.Nickname,
			HouseName: houseName[o.SafeHouseID],
			KeyID:     o.KeyID,
			Arrived:   formatUSDate(o.ArrivedAt),
			Departed:  departed,
			Current:   o.Current(today),
		})
	}
	s.render(w, "break_glass.html", map[string]any{
		"Title":             "Break-glass decrypt",
		"User":              &u,
		"Rows":              rows,
		"IdentityPublicKey": base64.StdEncoding.EncodeToString(s.cfg.IdentityPublicKey[:]),
		"IdentityKeyID":     s.cfg.IdentityKeyID,
	})
}

// handleSealedIdentity returns ciphertext for RHC browser decrypt. Never accepts a private key.
func (s *Server) handleSealedIdentity(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
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

	_ = s.store.Audit(r.Context(), &u.ID, "identity.break_glass", "occupant", idString(o.ID), map[string]any{
		"key_id":        o.KeyID,
		"safe_house_id": o.SafeHouseID,
		"nickname":      o.Nickname,
	})

	writeJSON(w, map[string]any{
		"occupant_id":          o.ID,
		"nickname":             o.Nickname,
		"key_id":               o.KeyID,
		"identity_ciphertext":  base64.StdEncoding.EncodeToString(o.IdentityCiphertext),
		"identity_public_key":  base64.StdEncoding.EncodeToString(s.cfg.IdentityPublicKey[:]),
	})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	events, err := s.store.ListAuditEvents(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load audit log", http.StatusInternalServerError)
		return
	}
	type row struct {
		domain.AuditEvent
		When string
		Meta string
	}
	rows := make([]row, 0, len(events))
	for _, e := range events {
		meta := ""
		if len(e.Meta) > 0 {
			b, err := json.Marshal(e.Meta)
			if err == nil {
				meta = string(b)
			}
		}
		rows = append(rows, row{
			AuditEvent: e,
			When:       e.CreatedAt.UTC().Format("01/02/2006 15:04:05 UTC"),
			Meta:       meta,
		})
	}
	s.render(w, "audit.html", map[string]any{
		"Title":  "Audit log",
		"User":   &u,
		"Events": rows,
	})
}
