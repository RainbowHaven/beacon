package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
	"github.com/magiconair/beacon/web"
)

// canAccessSafeguarding decides who may see and record safeguarding concerns
// for a house. All roles currently have access to their scoped houses.
func (s *Server) canAccessSafeguarding(u domain.User, house domain.SafeHouse) bool {
	return s.canAccessHouse(u, house)
}

func (s *Server) safeguardingHouses(r *http.Request, u domain.User) ([]domain.SafeHouse, error) {
	houses, err := s.housesForUser(r, u)
	if err != nil {
		return nil, err
	}
	out := houses[:0]
	for _, h := range houses {
		if s.canAccessSafeguarding(u, h) {
			out = append(out, h)
		}
	}
	return out, nil
}

// safeguardingMaxDate allows one day past today (UTC) so houses east of UTC
// can enter their local today.
func safeguardingMaxDate() time.Time {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
}

var datePrecisionOptions = []struct {
	Value domain.DatePrecision
	Label string
}{
	{domain.DateExact, "Exact date"},
	{domain.DateApproximate, "Approximate date"},
	{domain.DateUnknown, "Unknown"},
}

var safeguardingFlash = map[string]string{
	"added":     "Safeguarding concern recorded.",
	"updated":   "Safeguarding concern updated.",
	"unchanged": "No changes to save.",
}

func formatDay(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func (s *Server) handleSafeguarding(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.safeguardingHouses(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	filter := q.Get("status")
	var statuses []domain.SafeguardingStatus
	switch st := domain.SafeguardingStatus(filter); {
	case filter == "all":
	case st.Valid():
		statuses = []domain.SafeguardingStatus{st}
	default:
		filter = string(domain.SafeguardingOpen)
		statuses = []domain.SafeguardingStatus{domain.SafeguardingOpen}
	}
	ids := houseIDs(houses)
	var houseFilter int64
	if id, err := parseID(q.Get("house")); err == nil {
		for _, h := range houses {
			if h.ID == id {
				houseFilter = id
				ids = []int64{id}
			}
		}
	}
	list, err := s.store.ListSafeguardingConcerns(r.Context(), ids, statuses)
	if err != nil {
		http.Error(w, "failed to list safeguarding concerns", http.StatusInternalServerError)
		return
	}
	houseName := map[int64]string{}
	for _, h := range houses {
		houseName[h.ID] = h.Name
	}
	type row struct {
		domain.SafeguardingConcern
		HouseName   string
		Occurred    string
		Reported    string
		StatusLabel string
		Closed      string
	}
	rows := make([]row, 0, len(list))
	for _, c := range list {
		occurred := "Unknown"
		switch c.OccurredPrecision {
		case domain.DateExact:
			occurred = formatDay(c.OccurredOn)
		case domain.DateApproximate:
			occurred = "About " + formatDay(c.OccurredOn)
		}
		rows = append(rows, row{
			SafeguardingConcern: c,
			HouseName:           houseName[c.SafeHouseID],
			Occurred:            occurred,
			Reported:            c.ReportedOn.Format("2006-01-02"),
			StatusLabel:         c.Status.Label(),
			Closed:              formatDay(c.ClosedOn),
		})
	}
	type filterLink struct {
		Label, URL string
		Current    bool
	}
	var filters []filterLink
	for _, f := range []struct{ value, label string }{
		{"open", "Open"}, {"resolved", "Resolved"}, {"closed", "Closed"}, {"all", "All"},
	} {
		v := url.Values{"status": {f.value}}
		if houseFilter != 0 {
			v.Set("house", idString(houseFilter))
		}
		filters = append(filters, filterLink{Label: f.label, URL: "/safeguarding?" + v.Encode(), Current: filter == f.value})
	}
	s.render(w, r, "safeguarding.html", map[string]any{
		"Title":           "Safeguarding",
		"User":            &u,
		"Rows":            rows,
		"Houses":          houses,
		"ShowHouseFilter": len(houses) > 1,
		"HouseFilter":     houseFilter,
		"StatusFilter":    filter,
		"Filters":         filters,
		"DocVersion":      web.Document37Version,
		"Flash":           safeguardingFlash[q.Get("ok")],
	})
}

func (s *Server) handleDocument37(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.safeguardingHouses(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	data, err := web.Docs.ReadFile(web.Document37Path)
	if err != nil {
		http.Error(w, "document missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="`+web.Document37Filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(data)
}

type safeguardingForm struct {
	HouseID           int64
	IncidentID        string
	OccurredOn        string
	OccurredPrecision string
	ReportedOn        string
	Status            string
	ClosedOn          string
}

func readSafeguardingForm(v url.Values) safeguardingForm {
	f := safeguardingForm{
		IncidentID:        v.Get("incident_id"),
		OccurredOn:        strings.TrimSpace(v.Get("occurred_on")),
		OccurredPrecision: v.Get("occurred_precision"),
		ReportedOn:        strings.TrimSpace(v.Get("reported_on")),
		Status:            v.Get("status"),
		ClosedOn:          strings.TrimSpace(v.Get("closed_on")),
	}
	f.HouseID, _ = parseID(v.Get("safe_house_id"))
	if !domain.DatePrecision(f.OccurredPrecision).Valid() {
		f.OccurredPrecision = string(domain.DateUnknown)
	}
	if !domain.SafeguardingStatus(f.Status).Valid() {
		f.Status = string(domain.SafeguardingOpen)
	}
	return f
}

// input validates the form and returns a user-facing message on failure.
func (f safeguardingForm) input(maxDay time.Time) (store.SafeguardingInput, string) {
	var in store.SafeguardingInput
	id, ok := domain.NormalizeIncidentID(f.IncidentID)
	if !ok {
		return in, "Enter the incident identifier on one line, up to " + strconv.Itoa(domain.MaxIncidentIDLen) + " characters."
	}
	in.IncidentID = id

	reported, err := time.Parse("2006-01-02", f.ReportedOn)
	if err != nil {
		return in, "Enter the date the concern was reported."
	}
	if reported.After(maxDay) {
		return in, "The date reported cannot be in the future."
	}
	in.ReportedOn = reported

	in.OccurredPrecision = domain.DatePrecision(f.OccurredPrecision)
	if in.OccurredPrecision != domain.DateUnknown {
		occurred, err := time.Parse("2006-01-02", f.OccurredOn)
		if err != nil {
			return in, "Enter the date the concern occurred or started, or choose Unknown."
		}
		if occurred.After(reported) {
			return in, "The date the concern occurred cannot be after the date reported."
		}
		in.OccurredOn = &occurred
	}

	in.Status = domain.SafeguardingStatus(f.Status)
	if in.Status != domain.SafeguardingOpen {
		closed, err := time.Parse("2006-01-02", f.ClosedOn)
		if err != nil {
			return in, "Enter the date the concern was resolved or closed."
		}
		if closed.Before(reported) {
			return in, "The resolved or closed date cannot be before the date reported."
		}
		if closed.After(maxDay) {
			return in, "The resolved or closed date cannot be in the future."
		}
		in.ClosedOn = &closed
	}
	return in, ""
}

// suggestIncidentID returns the next free Document 37 identifier for the
// house's RHL and the year of reported, or "" when the RHL has no code.
func (s *Server) suggestIncidentID(ctx context.Context, house domain.SafeHouse, reported time.Time) string {
	rhl, err := s.store.GetRHL(ctx, house.RHLID)
	if err != nil || rhl.Code == "" {
		return ""
	}
	seq, err := s.store.NextIncidentSequence(ctx, rhl.Code, reported.Year())
	if err != nil {
		s.log.Error("next incident sequence", "err", err)
		return ""
	}
	return domain.IncidentID(rhl.Code, reported.Year(), seq)
}

func houseByID(houses []domain.SafeHouse, id int64) (domain.SafeHouse, bool) {
	for _, h := range houses {
		if h.ID == id {
			return h, true
		}
	}
	return domain.SafeHouse{}, false
}

func (s *Server) renderSafeguardingForm(w http.ResponseWriter, r *http.Request, u domain.User, data map[string]any, f safeguardingForm, errMsg string) {
	data["User"] = &u
	data["Form"] = f
	data["Statuses"] = domain.SafeguardingStatuses
	data["Precisions"] = datePrecisionOptions
	data["MaxDate"] = safeguardingMaxDate().Format("2006-01-02")
	data["MaxIDLen"] = domain.MaxIncidentIDLen
	data["Error"] = errMsg
	s.render(w, r, "safeguarding_form.html", data)
}

func (s *Server) renderSafeguardingNew(w http.ResponseWriter, r *http.Request, u domain.User, houses []domain.SafeHouse, f safeguardingForm, errMsg string) {
	house, _ := houseByID(houses, f.HouseID)
	suggestion := ""
	if reported, err := time.Parse("2006-01-02", f.ReportedOn); err == nil {
		suggestion = s.suggestIncidentID(r.Context(), house, reported)
	}
	if errMsg != "" && suggestion != "" && suggestion != strings.TrimSpace(f.IncidentID) {
		errMsg += " Next free identifier: " + suggestion + "."
	}
	s.renderSafeguardingForm(w, r, u, map[string]any{
		"Title":      "Record safeguarding concern",
		"Action":     "/safeguarding",
		"New":        true,
		"Houses":     houses,
		"Suggestion": suggestion,
	}, f, errMsg)
}

func (s *Server) handleSafeguardingNew(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.safeguardingHouses(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	f := readSafeguardingForm(q)
	if _, ok := houseByID(houses, f.HouseID); !ok {
		f.HouseID = houses[0].ID
	}
	reported, err := time.Parse("2006-01-02", f.ReportedOn)
	if err != nil {
		reported = time.Now().UTC().Truncate(24 * time.Hour)
		f.ReportedOn = reported.Format("2006-01-02")
	}
	if strings.TrimSpace(f.IncidentID) == "" || q.Get("suggest") != "" {
		house, _ := houseByID(houses, f.HouseID)
		f.IncidentID = s.suggestIncidentID(r.Context(), house, reported)
	}
	s.renderSafeguardingNew(w, r, u, houses, f, "")
}

func (s *Server) handleSafeguardingCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.safeguardingHouses(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f := readSafeguardingForm(r.PostForm)
	house, err := s.store.GetSafeHouse(r.Context(), f.HouseID)
	if err != nil || !s.canAccessSafeguarding(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	in, msg := f.input(safeguardingMaxDate())
	if msg != "" {
		s.renderSafeguardingNew(w, r, u, houses, f, msg)
		return
	}
	uid := u.ID
	c, err := s.store.CreateSafeguardingConcern(r.Context(), house.ID, in, &uid)
	if err != nil {
		if errors.Is(err, store.ErrIncidentIDTaken) {
			s.renderSafeguardingNew(w, r, u, houses, f, "This incident identifier is already recorded.")
			return
		}
		s.log.Error("create safeguarding concern", "err", err)
		s.renderSafeguardingNew(w, r, u, houses, f, "Could not save the safeguarding concern.")
		return
	}
	_ = s.store.Audit(r.Context(), &uid, "safeguarding.create", "safeguarding_concern", idString(c.ID), map[string]any{
		"incident_id":   c.IncidentID,
		"safe_house_id": c.SafeHouseID,
		"status":        c.Status,
	})
	http.Redirect(w, r, "/safeguarding?ok=added", http.StatusSeeOther)
}

// loadSafeguardingConcern loads the concern for the {id} path value and checks
// access. It writes the error response and returns false on failure.
func (s *Server) loadSafeguardingConcern(w http.ResponseWriter, r *http.Request, u domain.User) (domain.SafeguardingConcern, domain.SafeHouse, bool) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return domain.SafeguardingConcern{}, domain.SafeHouse{}, false
	}
	c, err := s.store.GetSafeguardingConcern(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return domain.SafeguardingConcern{}, domain.SafeHouse{}, false
	}
	house, err := s.store.GetSafeHouse(r.Context(), c.SafeHouseID)
	if err != nil || !s.canAccessSafeguarding(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return domain.SafeguardingConcern{}, domain.SafeHouse{}, false
	}
	return c, house, true
}

func (s *Server) renderSafeguardingEdit(w http.ResponseWriter, r *http.Request, u domain.User, c domain.SafeguardingConcern, house domain.SafeHouse, f safeguardingForm, errMsg string) {
	s.renderSafeguardingForm(w, r, u, map[string]any{
		"Title":   "Edit safeguarding concern",
		"Action":  "/safeguarding/" + idString(c.ID),
		"House":   house,
		"Concern": c,
	}, f, errMsg)
}

func (s *Server) handleSafeguardingEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	c, house, ok := s.loadSafeguardingConcern(w, r, u)
	if !ok {
		return
	}
	f := safeguardingForm{
		HouseID:           c.SafeHouseID,
		IncidentID:        c.IncidentID,
		OccurredOn:        formatDay(c.OccurredOn),
		OccurredPrecision: string(c.OccurredPrecision),
		ReportedOn:        c.ReportedOn.Format("2006-01-02"),
		Status:            string(c.Status),
		ClosedOn:          formatDay(c.ClosedOn),
	}
	s.renderSafeguardingEdit(w, r, u, c, house, f, "")
}

func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// changedSafeguardingFields lists the column names that differ between the
// stored concern and the new input.
func changedSafeguardingFields(c domain.SafeguardingConcern, in store.SafeguardingInput) []string {
	var changed []string
	if c.IncidentID != in.IncidentID {
		changed = append(changed, "incident_id")
	}
	if !sameDay(c.OccurredOn, in.OccurredOn) {
		changed = append(changed, "occurred_on")
	}
	if c.OccurredPrecision != in.OccurredPrecision {
		changed = append(changed, "occurred_precision")
	}
	if !c.ReportedOn.Equal(in.ReportedOn) {
		changed = append(changed, "reported_on")
	}
	if c.Status != in.Status {
		changed = append(changed, "status")
	}
	if !sameDay(c.ClosedOn, in.ClosedOn) {
		changed = append(changed, "closed_on")
	}
	return changed
}

func (s *Server) handleSafeguardingUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	c, house, ok := s.loadSafeguardingConcern(w, r, u)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f := readSafeguardingForm(r.PostForm)
	f.HouseID = c.SafeHouseID
	in, msg := f.input(safeguardingMaxDate())
	if msg != "" {
		s.renderSafeguardingEdit(w, r, u, c, house, f, msg)
		return
	}
	changed := changedSafeguardingFields(c, in)
	if len(changed) == 0 {
		http.Redirect(w, r, "/safeguarding?ok=unchanged", http.StatusSeeOther)
		return
	}
	uid := u.ID
	updated, err := s.store.UpdateSafeguardingConcern(r.Context(), c.ID, in, &uid)
	if err != nil {
		if errors.Is(err, store.ErrIncidentIDTaken) {
			s.renderSafeguardingEdit(w, r, u, c, house, f, "This incident identifier is already recorded.")
			return
		}
		s.log.Error("update safeguarding concern", "err", err)
		s.renderSafeguardingEdit(w, r, u, c, house, f, "Could not save the safeguarding concern.")
		return
	}
	meta := map[string]any{
		"incident_id": updated.IncidentID,
		"changed":     changed,
	}
	if updated.IncidentID != c.IncidentID {
		meta["previous_incident_id"] = c.IncidentID
	}
	if updated.Status != c.Status {
		meta["status_from"] = c.Status
		meta["status_to"] = updated.Status
	}
	_ = s.store.Audit(r.Context(), &uid, "safeguarding.update", "safeguarding_concern", idString(c.ID), meta)
	http.Redirect(w, r, "/safeguarding?ok=updated", http.StatusSeeOther)
}
