package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	var houses []domain.SafeHouse
	var err error
	switch u.Role {
	case domain.RoleRHCAdmin:
		houses, err = s.store.ListSafeHouses(r.Context())
	case domain.RoleRHLAdmin:
		if u.RHLID == nil {
			return nil, nil
		}
		houses, err = s.store.ListSafeHousesByRHL(r.Context(), *u.RHLID)
	case domain.RoleSafeHouseManager:
		if u.SafeHouseID == nil {
			return nil, nil
		}
		h, err := s.store.GetSafeHouse(r.Context(), *u.SafeHouseID)
		if err != nil {
			return nil, err
		}
		houses = []domain.SafeHouse{h}
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]domain.SafeHouse, 0, len(houses))
	for _, h := range houses {
		if h.Active {
			out = append(out, h)
		}
	}
	return out, nil
}

func (s *Server) canAccessHouse(u domain.User, house domain.SafeHouse) bool {
	switch u.Role {
	case domain.RoleRHCAdmin:
		return true
	case domain.RoleRHLAdmin:
		return u.RHLID != nil && *u.RHLID == house.RHLID && house.Active
	case domain.RoleSafeHouseManager:
		return u.SafeHouseID != nil && *u.SafeHouseID == house.ID && house.Active
	default:
		return false
	}
}

func houseIDs(houses []domain.SafeHouse) []int64 {
	ids := make([]int64, len(houses))
	for i, h := range houses {
		ids[i] = h.ID
	}
	return ids
}

func parseID(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func idString(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (s *Server) maxArrivedDate() time.Time {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	return today.AddDate(0, 0, s.cfg.ArrivalFutureDays)
}

func (s *Server) arrivalTooFarAhead(arrived time.Time) bool {
	day := arrived.UTC().Truncate(24 * time.Hour)
	return day.After(s.maxArrivedDate())
}

type occupantFormView struct {
	Title           string
	User            *domain.User
	Houses          []domain.SafeHouse
	House           domain.SafeHouse
	Occupant        domain.Occupant
	SelectedHouseID int64
	Nickname        string
	ArrivedAt       string
	MaxArrived      string
	Error           string
	Suggestion      string
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
	houseName := map[int64]string{}
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
	today := time.Now().UTC().Format("2006-01-02")
	s.render(w, "occupant_new.html", occupantFormView{
		Title:           "Add occupant",
		User:            &u,
		Houses:          houses,
		SelectedHouseID: houses[0].ID,
		Nickname:        "",
		ArrivedAt:       today,
		MaxArrived:      s.maxArrivedDate().Format("2006-01-02"),
	})
}

func (s *Server) renderOccupantNew(w http.ResponseWriter, u domain.User, houses []domain.SafeHouse, houseID int64, nickname, arrivedAt, errMsg, suggestion string) {
	if arrivedAt == "" {
		arrivedAt = time.Now().UTC().Format("2006-01-02")
	}
	if houseID == 0 && len(houses) > 0 {
		houseID = houses[0].ID
	}
	s.render(w, "occupant_new.html", occupantFormView{
		Title:           "Add occupant",
		User:            &u,
		Houses:          houses,
		SelectedHouseID: houseID,
		Nickname:        nickname,
		ArrivedAt:       arrivedAt,
		MaxArrived:      s.maxArrivedDate().Format("2006-01-02"),
		Error:           errMsg,
		Suggestion:      suggestion,
	})
}

func (s *Server) handleOccupantCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	nick := strings.TrimSpace(r.FormValue("nickname"))
	arrivedRaw := strings.TrimSpace(r.FormValue("arrived_at"))
	houseID, err := parseID(r.FormValue("safe_house_id"))
	if err != nil {
		s.renderOccupantNew(w, u, houses, 0, nick, arrivedRaw, "Invalid safe house.", "")
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), houseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	arrived, err := time.Parse("2006-01-02", arrivedRaw)
	if err != nil {
		s.renderOccupantNew(w, u, houses, houseID, nick, arrivedRaw, "Invalid arrival date.", "")
		return
	}
	if s.arrivalTooFarAhead(arrived) {
		msg := "Arrival date cannot be in the future."
		if s.cfg.ArrivalFutureDays > 0 {
			msg = "Arrival date cannot be more than " + strconv.Itoa(s.cfg.ArrivalFutureDays) + " day(s) in the future."
		}
		s.renderOccupantNew(w, u, houses, houseID, nick, arrivedRaw, msg, "")
		return
	}
	if nick == "" {
		s.renderOccupantNew(w, u, houses, houseID, nick, arrivedRaw, "Nickname is required.", "")
		return
	}

	uid := u.ID
	o, err := s.store.CreateOccupant(r.Context(), store.CreateOccupantInput{
		SafeHouseID: houseID,
		Nickname:    nick,
		ArrivedAt:   arrived,
		CreatedBy:   &uid,
	})
	if err != nil {
		if errors.Is(err, store.ErrNicknameInvalid) {
			s.renderOccupantNew(w, u, houses, houseID, nick, arrivedRaw, "Nickname must start with a letter.", "")
			return
		}
		if errors.Is(err, store.ErrNicknameTaken) {
			suggestion := ""
			if sug, sugErr := s.store.SuggestNickname(r.Context(), houseID, nick, 0); sugErr == nil {
				suggestion = sug
			}
			s.renderOccupantNew(w, u, houses, houseID, nick, arrivedRaw, "already taken.", suggestion)
			return
		}
		s.log.Error("create occupant", "err", err)
		s.renderOccupantNew(w, u, houses, houseID, nick, arrivedRaw, "Could not save occupant.", "")
		return
	}
	_ = s.store.Audit(r.Context(), &uid, "occupant.create", "occupant", idString(o.ID), map[string]any{
		"safe_house_id": houseID,
	})
	http.Redirect(w, r, "/occupants?ok=added", http.StatusSeeOther)
}

func (s *Server) handleOccupantEdit(w http.ResponseWriter, r *http.Request) {
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
	s.render(w, "occupant_edit.html", occupantFormView{
		Title:    "Rename occupant",
		User:     &u,
		Occupant: o,
		House:    house,
		Nickname: o.Nickname,
	})
}

func (s *Server) renderOccupantEdit(w http.ResponseWriter, u domain.User, o domain.Occupant, house domain.SafeHouse, nickname, errMsg, suggestion string) {
	s.render(w, "occupant_edit.html", occupantFormView{
		Title:      "Rename occupant",
		User:       &u,
		Occupant:   o,
		House:      house,
		Nickname:   nickname,
		Error:      errMsg,
		Suggestion: suggestion,
	})
}

func (s *Server) handleOccupantRename(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
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
	nick := strings.TrimSpace(r.FormValue("nickname"))
	if nick == "" {
		s.renderOccupantEdit(w, u, o, house, nick, "Nickname is required.", "")
		return
	}
	updated, err := s.store.RenameOccupant(r.Context(), id, nick)
	if err != nil {
		if errors.Is(err, store.ErrNicknameInvalid) {
			s.renderOccupantEdit(w, u, o, house, nick, "Nickname must start with a letter.", "")
			return
		}
		if errors.Is(err, store.ErrNicknameTaken) {
			suggestion := ""
			if sug, sugErr := s.store.SuggestNickname(r.Context(), o.SafeHouseID, nick, id); sugErr == nil {
				suggestion = sug
			}
			s.renderOccupantEdit(w, u, o, house, nick, "already taken.", suggestion)
			return
		}
		s.renderOccupantEdit(w, u, o, house, nick, "Could not rename.", "")
		return
	}
	uid := u.ID
	_ = s.store.Audit(r.Context(), &uid, "occupant.rename", "occupant", idString(id), map[string]any{
		"nickname": updated.Nickname,
	})
	http.Redirect(w, r, "/occupants?ok=renamed", http.StatusSeeOther)
}

func (s *Server) handleOccupantDepart(w http.ResponseWriter, r *http.Request) {
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
	_ = s.store.Audit(r.Context(), &uid, "occupant.depart", "occupant", idString(id), map[string]any{
		"departed_at": day.Format("2006-01-02"),
	})
	http.Redirect(w, r, "/occupants?ok=departed", http.StatusSeeOther)
}
