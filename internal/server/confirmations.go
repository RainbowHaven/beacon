package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/report"
	"github.com/RainbowHaven/beacon/internal/store"
)

const historyMonths = 12

// reportConfirmation is the data-entry status shown on the monthly report.
type reportConfirmation struct {
	Status      report.Status
	Fingerprint string // empty while the month is in progress
	CanConfirm  bool
}

type dashboardRow struct {
	RHL   domain.RHL
	House domain.SafeHouse
	// LatestConfirmed is the newest month whose confirmation still matches its data.
	LatestConfirmed *domain.MonthConfirmation
	Status          report.Status
}

// canConfirmMonth: Agents confirm their own house and RHL users the houses of
// their RHL. RHC admins only view.
func (s *Server) canConfirmMonth(u domain.User, house domain.SafeHouse) bool {
	switch u.Role {
	case domain.RoleSafeHouseManager, domain.RoleRHLAdmin:
		return s.canAccessHouse(u, house)
	}
	return false
}

// monthStatus computes the status of one house-month. latest is the current
// confirmation or nil.
func (s *Server) monthStatus(ctx context.Context, houseID int64, month, now time.Time, latest *domain.MonthConfirmation) (report.Status, string, error) {
	if !report.Ended(month, now) {
		return report.ComputeStatus(month, now, latest, ""), "", nil
	}
	fp, err := s.store.MonthFingerprint(ctx, houseID, month)
	if err != nil {
		return report.Status{}, "", err
	}
	return report.ComputeStatus(month, now, latest, fp), fp, nil
}

func (s *Server) reportConfirmationFor(ctx context.Context, houseID int64, month, now time.Time) (reportConfirmation, error) {
	var latest *domain.MonthConfirmation
	c, err := s.store.LatestMonthConfirmation(ctx, houseID, month)
	switch {
	case err == nil:
		latest = &c
	case !errors.Is(err, store.ErrNotFound):
		return reportConfirmation{}, err
	}
	st, fp, err := s.monthStatus(ctx, houseID, month, now, latest)
	if err != nil {
		return reportConfirmation{}, err
	}
	return reportConfirmation{Status: st, Fingerprint: fp}, nil
}

// confirmationsByHouse maps house id to month key to the latest confirmation.
func (s *Server) confirmationsByHouse(ctx context.Context, ids []int64) (map[int64]map[string]domain.MonthConfirmation, error) {
	list, err := s.store.LatestMonthConfirmations(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := map[int64]map[string]domain.MonthConfirmation{}
	for _, c := range list {
		if out[c.SafeHouseID] == nil {
			out[c.SafeHouseID] = map[string]domain.MonthConfirmation{}
		}
		out[c.SafeHouseID][c.Month.Format("2006-01")] = c
	}
	return out, nil
}

func reportURL(houseID int64, month time.Time) string {
	return "/reports?month=" + month.Format("2006-01") + "&house=" + idString(houseID)
}

func (s *Server) handleMonthConfirm(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("houseID"))
	if err != nil {
		http.Error(w, "bad house", http.StatusBadRequest)
		return
	}
	month, err := report.MonthStart(r.PathValue("month"))
	if err != nil {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), id)
	if err != nil || !s.canConfirmMonth(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	back := reportURL(id, month)
	fail := func(msg string) {
		http.Redirect(w, r, back+"&error="+url.QueryEscape(msg), http.StatusSeeOther)
	}
	now := time.Now().UTC()
	if !report.Ended(month, now) {
		fail("Only months that have ended can be confirmed.")
		return
	}
	fp, err := s.store.MonthFingerprint(r.Context(), id, month)
	if err != nil {
		s.log.Error("month fingerprint", "err", err)
		http.Error(w, "failed to confirm", http.StatusInternalServerError)
		return
	}
	if r.FormValue("fingerprint") != fp {
		fail("The data changed while you were viewing the report. Check it and confirm again.")
		return
	}
	if _, err := s.store.ConfirmMonth(r.Context(), store.MonthConfirmationInput{
		SafeHouseID: id, Month: month, Fingerprint: fp, ConfirmedBy: u.ID,
	}); err != nil {
		s.log.Error("confirm month", "err", err)
		http.Error(w, "failed to confirm", http.StatusInternalServerError)
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "report.confirm", "safe_house", idString(id), map[string]any{
		"safe_house_id": id,
		"month":         month.Format("2006-01"),
		"fingerprint":   fp,
	})
	http.Redirect(w, r, back+"&ok=confirmed", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if u.Role != domain.RoleRHCAdmin && u.Role != domain.RoleRHLAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	rows, err := s.buildDashboard(r.Context(), houses, now)
	if err != nil {
		s.log.Error("build dashboard", "err", err)
		http.Error(w, "failed to build dashboard", http.StatusInternalServerError)
		return
	}
	last := report.LastEndedMonth(now)
	s.render(w, r, "dashboard.html", map[string]any{
		"Title":   "Dashboard",
		"User":    &u,
		"Rows":    rows,
		"Month":   last.Format("January 2006"),
		"DueDate": report.Deadline(last).AddDate(0, 0, -1),
	})
}

func (s *Server) buildDashboard(ctx context.Context, houses []domain.SafeHouse, now time.Time) ([]dashboardRow, error) {
	rhls, err := s.store.ListRHLs(ctx)
	if err != nil {
		return nil, err
	}
	rhlByID := map[int64]domain.RHL{}
	for _, r := range rhls {
		rhlByID[r.ID] = r
	}
	confs, err := s.confirmationsByHouse(ctx, houseIDs(houses))
	if err != nil {
		return nil, err
	}
	last := report.LastEndedMonth(now)
	rows := make([]dashboardRow, 0, len(houses))
	for _, h := range houses {
		var latest *domain.MonthConfirmation
		if c, ok := confs[h.ID][last.Format("2006-01")]; ok {
			latest = &c
		}
		st, _, err := s.monthStatus(ctx, h.ID, last, now, latest)
		if err != nil {
			return nil, err
		}
		row := dashboardRow{RHL: rhlByID[h.RHLID], House: h, Status: st}
		row.LatestConfirmed, err = s.latestValidConfirmation(ctx, h.ID, confs[h.ID], st)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.RHL.Name != b.RHL.Name {
			return a.RHL.Name < b.RHL.Name
		}
		return a.House.Name < b.House.Name
	})
	return rows, nil
}

// latestValidConfirmation walks the house's confirmations newest month first
// and returns the first whose fingerprint still matches the month's data.
// current is the already computed status of the most recently ended month.
func (s *Server) latestValidConfirmation(ctx context.Context, houseID int64, byMonth map[string]domain.MonthConfirmation, current report.Status) (*domain.MonthConfirmation, error) {
	list := make([]domain.MonthConfirmation, 0, len(byMonth))
	for _, c := range byMonth {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Month.After(list[j].Month) })
	for i := range list {
		c := list[i]
		if c.Month.Equal(current.Month) {
			if current.State == report.StateConfirmed {
				return &c, nil
			}
			continue
		}
		fp, err := s.store.MonthFingerprint(ctx, houseID, c.Month)
		if err != nil {
			return nil, err
		}
		if fp == c.Fingerprint {
			return &c, nil
		}
	}
	return nil, nil
}

func (s *Server) handleReportHistory(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("houseID"))
	if err != nil {
		http.Error(w, "bad house", http.StatusBadRequest)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), id)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	rhl, err := s.store.GetRHL(r.Context(), house.RHLID)
	if err != nil {
		http.Error(w, "failed to load RHL", http.StatusInternalServerError)
		return
	}
	confs, err := s.confirmationsByHouse(r.Context(), []int64{id})
	if err != nil {
		http.Error(w, "failed to load confirmations", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	months := report.EndedMonths(now, historyMonths)
	statuses := make([]report.Status, 0, len(months))
	for _, m := range months {
		var latest *domain.MonthConfirmation
		if c, ok := confs[id][m.Format("2006-01")]; ok {
			latest = &c
		}
		st, _, err := s.monthStatus(r.Context(), id, m, now, latest)
		if err != nil {
			s.log.Error("report history", "err", err)
			http.Error(w, "failed to build history", http.StatusInternalServerError)
			return
		}
		statuses = append(statuses, st)
	}
	s.render(w, r, "report_history.html", map[string]any{
		"Title":    "Reporting history",
		"User":     &u,
		"RHL":      rhl,
		"House":    house,
		"Statuses": statuses,
	})
}
