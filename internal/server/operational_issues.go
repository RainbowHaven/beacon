package server

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

const operationalIssueStatusAll = "all"

var operationalIssueStatuses = []string{
	domain.OperationalIssueOpen,
	domain.OperationalIssueResolved,
	domain.OperationalIssueClosed,
}

type labeledOption struct {
	Key   string
	Label string
}

type statusFilterLink struct {
	Label   string
	Href    string
	Current bool
}

// operationalIssueForm holds raw form values so a rejected submit re-renders as typed.
type operationalIssueForm struct {
	SafeHouseID  int64
	IdentifiedOn string
	Category     string
	Description  string
	Effect       string
	ActionTaken  string
	RHLRequest   string
	Status       string
	ClosedOn     string
	ClosureNotes string
}

func operationalIssueFormFromRequest(r *http.Request) operationalIssueForm {
	return operationalIssueForm{
		IdentifiedOn: strings.TrimSpace(r.FormValue("identified_on")),
		Category:     strings.TrimSpace(r.FormValue("category")),
		Description:  r.FormValue("description"),
		Effect:       r.FormValue("effect"),
		ActionTaken:  r.FormValue("action_taken"),
		RHLRequest:   r.FormValue("rhl_request"),
		Status:       strings.TrimSpace(r.FormValue("status")),
		ClosedOn:     strings.TrimSpace(r.FormValue("closed_on")),
		ClosureNotes: r.FormValue("closure_notes"),
	}
}

func operationalIssueFormFromIssue(o domain.OperationalIssue) operationalIssueForm {
	f := operationalIssueForm{
		SafeHouseID:  o.SafeHouseID,
		IdentifiedOn: o.IdentifiedOn.Format("2006-01-02"),
		Category:     o.Category,
		Description:  o.Description,
		Effect:       o.Effect,
		ActionTaken:  o.ActionTaken,
		RHLRequest:   o.RHLRequest,
		Status:       o.Status,
		ClosureNotes: o.ClosureNotes,
	}
	if o.ClosedOn != nil {
		f.ClosedOn = o.ClosedOn.Format("2006-01-02")
	}
	return f
}

// fields parses dates and applies the store rules. today bounds both dates.
func (f operationalIssueForm) fields(today time.Time) (store.OperationalIssueFields, error) {
	latest := today.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	out := store.OperationalIssueFields{
		Category:     f.Category,
		Description:  f.Description,
		Effect:       f.Effect,
		ActionTaken:  f.ActionTaken,
		RHLRequest:   f.RHLRequest,
		Status:       f.Status,
		ClosureNotes: f.ClosureNotes,
	}
	if f.IdentifiedOn == "" {
		return out, store.ValidationError("Date identified is required.")
	}
	identified, err := time.Parse("2006-01-02", f.IdentifiedOn)
	if err != nil {
		return out, store.ValidationError("Invalid date identified.")
	}
	if identified.After(latest) {
		return out, store.ValidationError("Date identified cannot be in the future.")
	}
	out.IdentifiedOn = identified
	if f.ClosedOn != "" && f.Status != domain.OperationalIssueOpen {
		closed, err := time.Parse("2006-01-02", f.ClosedOn)
		if err != nil {
			return out, store.ValidationError("Invalid resolution or closure date.")
		}
		if closed.After(latest) {
			return out, store.ValidationError("Resolution or closure date cannot be in the future.")
		}
		out.ClosedOn = &closed
	}
	return store.NormalizeOperationalIssueFields(out)
}

// operationalIssueChanges lists changed field names only, never their text.
func operationalIssueChanges(old domain.OperationalIssue, f store.OperationalIssueFields) []string {
	var changed []string
	if !old.IdentifiedOn.Equal(f.IdentifiedOn) {
		changed = append(changed, "identified_on")
	}
	if old.Category != f.Category {
		changed = append(changed, "category")
	}
	if old.Description != f.Description {
		changed = append(changed, "description")
	}
	if old.Effect != f.Effect {
		changed = append(changed, "effect")
	}
	if old.ActionTaken != f.ActionTaken {
		changed = append(changed, "action_taken")
	}
	if old.RHLRequest != f.RHLRequest {
		changed = append(changed, "rhl_request")
	}
	if old.Status != f.Status {
		changed = append(changed, "status")
	}
	switch {
	case old.ClosedOn == nil && f.ClosedOn == nil:
	case old.ClosedOn == nil || f.ClosedOn == nil || !old.ClosedOn.Equal(*f.ClosedOn):
		changed = append(changed, "closed_on")
	}
	if old.ClosureNotes != f.ClosureNotes {
		changed = append(changed, "closure_notes")
	}
	return changed
}

func operationalIssueCategoryOptions() []labeledOption {
	out := make([]labeledOption, len(domain.OperationalIssueCategories))
	for i, c := range domain.OperationalIssueCategories {
		out[i] = labeledOption{Key: c.Key, Label: c.Label}
	}
	return out
}

func operationalIssueStatusOptions() []labeledOption {
	out := make([]labeledOption, len(operationalIssueStatuses))
	for i, s := range operationalIssueStatuses {
		out[i] = labeledOption{Key: s, Label: domain.OperationalIssueStatusLabel(s)}
	}
	return out
}

func operationalIssueFilterLinks(status string, houseID int64) []statusFilterLink {
	keys := append(slices.Clone(operationalIssueStatuses), operationalIssueStatusAll)
	out := make([]statusFilterLink, 0, len(keys))
	for _, k := range keys {
		q := url.Values{}
		if k != domain.OperationalIssueOpen {
			q.Set("status", k)
		}
		if houseID != 0 {
			q.Set("house", idString(houseID))
		}
		href := "/operations"
		if len(q) > 0 {
			href += "?" + q.Encode()
		}
		label := "All"
		if k != operationalIssueStatusAll {
			label = domain.OperationalIssueStatusLabel(k)
		}
		out = append(out, statusFilterLink{Label: label, Href: href, Current: k == status})
	}
	return out
}

func (s *Server) handleOperationalIssues(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	ids := houseIDs(houses)
	var houseID int64
	if v := strings.TrimSpace(r.URL.Query().Get("house")); v != "" {
		houseID, err = parseID(v)
		if err != nil {
			http.Error(w, "bad house", http.StatusBadRequest)
			return
		}
		if !slices.Contains(ids, houseID) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ids = []int64{houseID}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != operationalIssueStatusAll && !domain.ValidOperationalIssueStatus(status) {
		status = domain.OperationalIssueOpen
	}
	listStatus := status
	if status == operationalIssueStatusAll {
		listStatus = ""
	}
	issues, err := s.store.ListOperationalIssuesByHouses(r.Context(), ids, listStatus)
	if err != nil {
		http.Error(w, "failed to list operational issues", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "operational_issues.html", map[string]any{
		"Title":       "Operations",
		"User":        &u,
		"Issues":      issues,
		"Houses":      houses,
		"HouseID":     houseID,
		"Status":      status,
		"StatusLabel": strings.ToLower(domain.OperationalIssueStatusLabel(status)),
		"Filters":     operationalIssueFilterLinks(status, houseID),
		"Flash":       r.URL.Query().Get("ok"),
		"Error":       r.URL.Query().Get("error"),
	})
}

func (s *Server) handleOperationalIssueNew(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	form := operationalIssueForm{
		IdentifiedOn: time.Now().UTC().Format("2006-01-02"),
		Status:       domain.OperationalIssueOpen,
	}
	if id, err := parseID(r.URL.Query().Get("house")); err == nil && slices.Contains(houseIDs(houses), id) {
		form.SafeHouseID = id
	}
	s.renderOperationalIssueForm(w, r, u, houses, domain.OperationalIssue{}, form, "")
}

func (s *Server) renderOperationalIssueForm(w http.ResponseWriter, r *http.Request, u domain.User, houses []domain.SafeHouse, issue domain.OperationalIssue, form operationalIssueForm, errMsg string) {
	title := "Record operational issue"
	if issue.ID != 0 {
		title = "Edit operational issue"
	}
	s.render(w, r, "operational_issue_form.html", map[string]any{
		"Title":      title,
		"User":       &u,
		"Houses":     houses,
		"Issue":      issue,
		"Form":       form,
		"Categories": operationalIssueCategoryOptions(),
		"Statuses":   operationalIssueStatusOptions(),
		"MaxDate":    time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02"),
		"Error":      errMsg,
	})
}

func (s *Server) handleOperationalIssueCreate(w http.ResponseWriter, r *http.Request) {
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
	form := operationalIssueFormFromRequest(r)
	houseID, err := parseID(r.FormValue("safe_house_id"))
	if err != nil {
		s.renderOperationalIssueForm(w, r, u, houses, domain.OperationalIssue{}, form, "Invalid safe house.")
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), houseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	form.SafeHouseID = houseID
	fields, err := form.fields(time.Now())
	if err != nil {
		s.renderOperationalIssueForm(w, r, u, houses, domain.OperationalIssue{}, form, operationalIssueErrorMessage(err))
		return
	}
	uid := u.ID
	o, err := s.store.CreateOperationalIssue(r.Context(), houseID, fields, &uid)
	if err != nil {
		s.log.Error("create operational issue", "err", err)
		s.renderOperationalIssueForm(w, r, u, houses, domain.OperationalIssue{}, form, "Could not save the operational issue.")
		return
	}
	_ = s.store.Audit(r.Context(), &uid, "operational_issue.create", "operational_issue", idString(o.ID), map[string]any{
		"safe_house_id": houseID,
		"category":      o.Category,
		"status":        o.Status,
	})
	http.Redirect(w, r, "/operations?ok=Operational+issue+recorded.", http.StatusSeeOther)
}

// loadOperationalIssue enforces house scope for routes that take an issue ID.
func (s *Server) loadOperationalIssue(w http.ResponseWriter, r *http.Request, u domain.User) (domain.OperationalIssue, bool) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return domain.OperationalIssue{}, false
	}
	o, err := s.store.GetOperationalIssue(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return domain.OperationalIssue{}, false
	}
	house, err := s.store.GetSafeHouse(r.Context(), o.SafeHouseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return domain.OperationalIssue{}, false
	}
	return o, true
}

func (s *Server) handleOperationalIssueEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	o, ok := s.loadOperationalIssue(w, r, u)
	if !ok {
		return
	}
	s.renderOperationalIssueForm(w, r, u, nil, o, operationalIssueFormFromIssue(o), "")
}

func (s *Server) handleOperationalIssueUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	o, ok := s.loadOperationalIssue(w, r, u)
	if !ok {
		return
	}
	form := operationalIssueFormFromRequest(r)
	form.SafeHouseID = o.SafeHouseID
	fields, err := form.fields(time.Now())
	if err != nil {
		s.renderOperationalIssueForm(w, r, u, nil, o, form, operationalIssueErrorMessage(err))
		return
	}
	changed := operationalIssueChanges(o, fields)
	if len(changed) == 0 {
		http.Redirect(w, r, "/operations?ok=No+changes.", http.StatusSeeOther)
		return
	}
	uid := u.ID
	updated, err := s.store.UpdateOperationalIssue(r.Context(), o.ID, fields, &uid)
	if err != nil {
		s.log.Error("update operational issue", "err", err)
		s.renderOperationalIssueForm(w, r, u, nil, o, form, "Could not save the operational issue.")
		return
	}
	meta := map[string]any{
		"safe_house_id": o.SafeHouseID,
		"changed":       changed,
	}
	if o.Status != updated.Status {
		meta["status_from"] = o.Status
		meta["status_to"] = updated.Status
	}
	_ = s.store.Audit(r.Context(), &uid, "operational_issue.update", "operational_issue", idString(o.ID), meta)
	http.Redirect(w, r, "/operations?ok=Operational+issue+updated.", http.StatusSeeOther)
}

func operationalIssueErrorMessage(err error) string {
	var ve store.ValidationError
	if errors.As(err, &ve) {
		return ve.Error()
	}
	return "Could not save the operational issue."
}
