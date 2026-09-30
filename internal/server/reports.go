package server

import (
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/report"
)

// houseReport is the monthly report for one safe house. Each report section
// has its own field, template block in report_sections.html and entry in
// reportCSVSections (or reportCSVTables for row-per-record sections).
type houseReport struct {
	RHL          domain.RHL
	House        domain.SafeHouse
	Monthly      report.Monthly
	Expenses     reportExpenses
	Safeguarding reportSafeguarding
	Operations   []domain.OperationalIssue
	GeneratedAt  time.Time
}

// reportSafeguarding lists every concern open at some point in the month;
// Reported counts only those reported in the month.
type reportSafeguarding struct {
	Reported int
	Concerns []domain.SafeguardingConcern
}

type currencyTotal struct {
	Currency     string
	Amount       string
	ExpenseCount int
	ReceiptCount int
}

type reportExpenses struct {
	Currencies   []currencyTotal
	ExpenseCount int
	ReceiptCount int
}

type overviewRow struct {
	RHL                  domain.RHL
	House                domain.SafeHouse
	Monthly              report.Monthly
	SafeguardingReported int
	OpenProblems         int
}

type overviewTotals struct {
	ResidentsServed      int
	BedNights            int
	Admissions           int
	Departures           int
	SafeguardingReported int
	OpenProblems         int
}

var errFutureMonth = errors.New("future month")

// parseReportMonth parses YYYY-MM; empty means the current month.
func parseReportMonth(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = now.UTC().Format("2006-01")
	}
	m, err := report.MonthStart(raw)
	if err != nil {
		return time.Time{}, err
	}
	if m.After(now.UTC()) {
		return time.Time{}, errFutureMonth
	}
	return m, nil
}

func monthEnd(month time.Time) time.Time { return month.AddDate(0, 1, -1) }

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	q := r.URL.Query()
	month, err := parseReportMonth(q.Get("month"), now)
	if err != nil {
		msg := "invalid+month"
		if errors.Is(err, errFutureMonth) {
			msg = "choose+the+current+month+or+an+earlier+one"
		}
		http.Redirect(w, r, "/reports?error="+msg, http.StatusSeeOther)
		return
	}

	var selected *domain.SafeHouse
	if raw := strings.TrimSpace(q.Get("house")); raw != "" {
		id, err := parseID(raw)
		if err != nil {
			http.Error(w, "bad house", http.StatusBadRequest)
			return
		}
		h, err := s.store.GetSafeHouse(r.Context(), id)
		if err != nil || !s.canAccessHouse(u, h) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		selected = &h
	} else if len(houses) == 1 {
		selected = &houses[0]
	}

	data := map[string]any{
		"Title":           "Monthly report",
		"User":            &u,
		"BodyClass":       "report-page",
		"Month":           month.Format("2006-01"),
		"MaxMonth":        now.Format("2006-01"),
		"Houses":          houses,
		"SelectedHouseID": int64(0),
		"Error":           q.Get("error"),
	}
	if selected != nil {
		rep, err := s.buildHouseReport(r.Context(), *selected, month, now)
		if err != nil {
			s.log.Error("build report", "err", err)
			http.Error(w, "failed to build report", http.StatusInternalServerError)
			return
		}
		data["SelectedHouseID"] = selected.ID
		data["Report"] = rep
	} else if len(houses) > 0 {
		rows, totals, err := s.buildOverview(r.Context(), houses, month, now)
		if err != nil {
			s.log.Error("build report overview", "err", err)
			http.Error(w, "failed to build report", http.StatusInternalServerError)
			return
		}
		data["Overview"] = rows
		data["OverviewTotals"] = totals
		data["OverviewMonth"] = rows[0].Monthly
		data["GeneratedAt"] = now
	}
	s.render(w, r, "reports.html", data)
}

func (s *Server) handleReportCSV(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("houseID"))
	if err != nil {
		http.Error(w, "bad house", http.StatusBadRequest)
		return
	}
	key, ok := strings.CutSuffix(r.PathValue("file"), ".csv")
	if !ok || key == "" {
		http.NotFound(w, r)
		return
	}
	now := time.Now().UTC()
	month, err := parseReportMonth(key, now)
	if err != nil {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), id)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	rep, err := s.buildHouseReport(r.Context(), house, month, now)
	if err != nil {
		s.log.Error("build report", "err", err)
		http.Error(w, "failed to build report", http.StatusInternalServerError)
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "report.export", "safe_house", idString(id), map[string]any{
		"month": key,
	})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="beacon-report-house-`+idString(id)+"-"+key+`.csv"`)
	if err := writeReportCSV(w, rep); err != nil {
		s.log.Error("write report csv", "err", err)
	}
}

func (s *Server) buildHouseReport(ctx context.Context, house domain.SafeHouse, month, now time.Time) (houseReport, error) {
	rhl, err := s.store.GetRHL(ctx, house.RHLID)
	if err != nil {
		return houseReport{}, err
	}
	occupants, err := s.store.ListOccupantsInPeriod(ctx, []int64{house.ID}, month, monthEnd(month))
	if err != nil {
		return houseReport{}, err
	}
	totals, err := s.store.ExpenseTotalsByHouses(ctx, []int64{house.ID}, month, monthEnd(month))
	if err != nil {
		return houseReport{}, err
	}
	concerns, err := s.store.SafeguardingConcernsForMonth(ctx, []int64{house.ID}, month, monthEnd(month))
	if err != nil {
		return houseReport{}, err
	}
	reported, err := s.store.CountSafeguardingConcernsReported(ctx, []int64{house.ID}, month, monthEnd(month))
	if err != nil {
		return houseReport{}, err
	}
	issues, err := s.store.OperationalIssuesActiveInRange(ctx, []int64{house.ID}, month, monthEnd(month))
	if err != nil {
		return houseReport{}, err
	}
	return houseReport{
		RHL:          rhl,
		House:        house,
		Monthly:      report.Build(month, now, house.ApprovedSleepingPlaces, occupants),
		Expenses:     summarizeExpenses(totals),
		Safeguarding: reportSafeguarding{Reported: reported, Concerns: concerns},
		Operations:   issues,
		GeneratedAt:  now,
	}, nil
}

// openAtMonthEnd reports whether an issue active in the month was still open
// on its last day. For the current month that is the same as open now.
func openAtMonthEnd(o domain.OperationalIssue, month time.Time) bool {
	return o.ClosedOn == nil || o.ClosedOn.After(monthEnd(month))
}

func (s *Server) buildOverview(ctx context.Context, houses []domain.SafeHouse, month, now time.Time) ([]overviewRow, overviewTotals, error) {
	rhls, err := s.store.ListRHLs(ctx)
	if err != nil {
		return nil, overviewTotals{}, err
	}
	rhlByID := map[int64]domain.RHL{}
	for _, r := range rhls {
		rhlByID[r.ID] = r
	}
	occupants, err := s.store.ListOccupantsInPeriod(ctx, houseIDs(houses), month, monthEnd(month))
	if err != nil {
		return nil, overviewTotals{}, err
	}
	byHouse := map[int64][]domain.Occupant{}
	for _, o := range occupants {
		byHouse[o.SafeHouseID] = append(byHouse[o.SafeHouseID], o)
	}
	concerns, err := s.store.SafeguardingConcernsForMonth(ctx, houseIDs(houses), month, monthEnd(month))
	if err != nil {
		return nil, overviewTotals{}, err
	}
	reported := map[int64]int{}
	for _, c := range concerns {
		if !c.ReportedOn.Before(month) {
			reported[c.SafeHouseID]++
		}
	}
	issues, err := s.store.OperationalIssuesActiveInRange(ctx, houseIDs(houses), month, monthEnd(month))
	if err != nil {
		return nil, overviewTotals{}, err
	}
	openProblems := map[int64]int{}
	for _, o := range issues {
		if openAtMonthEnd(o, month) {
			openProblems[o.SafeHouseID]++
		}
	}
	rows := make([]overviewRow, 0, len(houses))
	var totals overviewTotals
	for _, h := range houses {
		m := report.Build(month, now, h.ApprovedSleepingPlaces, byHouse[h.ID])
		rows = append(rows, overviewRow{
			RHL: rhlByID[h.RHLID], House: h, Monthly: m,
			SafeguardingReported: reported[h.ID],
			OpenProblems:         openProblems[h.ID],
		})
		totals.ResidentsServed += m.ResidentsServed
		totals.BedNights += m.BedNights
		totals.Admissions += m.Admissions
		totals.Departures += m.Departures
		totals.SafeguardingReported += reported[h.ID]
		totals.OpenProblems += openProblems[h.ID]
	}
	return rows, totals, nil
}

func summarizeExpenses(totals []domain.ExpenseTotals) reportExpenses {
	var out reportExpenses
	cents := map[string]int64{}
	byCurrency := map[string]*currencyTotal{}
	var order []string
	for _, t := range totals {
		ct, ok := byCurrency[t.Currency]
		if !ok {
			ct = &currencyTotal{Currency: t.Currency}
			byCurrency[t.Currency] = ct
			order = append(order, t.Currency)
		}
		ct.ExpenseCount += t.ExpenseCount
		ct.ReceiptCount += t.ReceiptCount
		cents[t.Currency] += t.AmountCents
		out.ExpenseCount += t.ExpenseCount
		out.ReceiptCount += t.ReceiptCount
	}
	for _, c := range order {
		ct := byCurrency[c]
		ct.Amount = formatCents(cents[c])
		out.Currencies = append(out.Currencies, *ct)
	}
	return out
}

// reportCSVSections lists the summary rows of the CSV export in order.
var reportCSVSections = []func(houseReport) [][]string{
	csvHeaderRows,
	csvSummaryRows,
	csvDemographicsRows,
	csvExpenseRows,
	csvSafeguardingSummaryRows,
}

// reportCSVTables lists the tables that follow the summary, each a header row
// and one row per record, separated by an empty line.
var reportCSVTables = []func(houseReport) [][]string{
	csvResidentRows,
	csvSafeguardingRows,
	csvOperationalRows,
}

func writeReportCSV(w http.ResponseWriter, rep houseReport) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Field", "Value"})
	for _, section := range reportCSVSections {
		for _, row := range section(rep) {
			_ = cw.Write(row)
		}
	}
	for _, table := range reportCSVTables {
		_ = cw.Write(nil)
		for _, row := range table(rep) {
			_ = cw.Write(row)
		}
	}
	cw.Flush()
	return cw.Error()
}

func csvResidentRows(rep houseReport) [][]string {
	rows := [][]string{{"Nickname", "Arrival", "Departure", "Bed-nights in month", "Country of origin", "Gender", "Birth year"}}
	for _, res := range rep.Monthly.Residents {
		birth := ""
		if res.BirthYear != nil {
			birth = strconv.Itoa(*res.BirthYear)
		}
		rows = append(rows, []string{
			res.Nickname,
			res.ArrivedAt.Format("2006-01-02"),
			formatDay(res.DepartedAt),
			strconv.Itoa(res.BedNights),
			res.Country,
			res.Gender,
			birth,
		})
	}
	return rows
}

func csvSafeguardingSummaryRows(rep houseReport) [][]string {
	return [][]string{{"Safeguarding concerns reported", strconv.Itoa(rep.Safeguarding.Reported)}}
}

func csvSafeguardingRows(rep houseReport) [][]string {
	rows := [][]string{{"Incident identifier", "Date reported", "Status", "Resolved or closed"}}
	for _, c := range rep.Safeguarding.Concerns {
		rows = append(rows, []string{c.IncidentID, c.ReportedOn.Format("2006-01-02"), c.Status.Label(), formatDay(c.ClosedOn)})
	}
	return rows
}

func csvOperationalRows(rep houseReport) [][]string {
	rows := [][]string{{"Date identified", "Category", "Brief description", "Effect on the safe house", "Status", "Resolved or closed", "Action taken or required", "Action requested from the RHL"}}
	for _, o := range rep.Operations {
		rows = append(rows, []string{
			o.IdentifiedOn.Format("2006-01-02"),
			o.CategoryLabel(),
			o.Description,
			o.Effect,
			o.StatusLabel(),
			formatDay(o.ClosedOn),
			o.ActionTaken,
			o.RHLRequest,
		})
	}
	return rows
}

func csvHeaderRows(rep houseReport) [][]string {
	m := rep.Monthly
	status := "Complete"
	if m.InProgress {
		status = "Month in progress"
		if m.NightsCounted > 0 {
			status += " (nights up to " + m.LastCountedDay().Format("2006-01-02") + ")"
		}
	}
	places := "not set"
	if rep.House.ApprovedSleepingPlaces != nil {
		places = strconv.Itoa(*rep.House.ApprovedSleepingPlaces)
	}
	return [][]string{
		{"RHL code", rep.RHL.Code},
		{"RHL", rep.RHL.Name},
		{"Safe house", rep.House.Name},
		{"Reporting month", m.MonthKey()},
		{"Status", status},
		{"Approved sleeping places", places},
		{"Generated", rep.GeneratedAt.Format("2006-01-02 15:04") + " UTC"},
	}
}

func csvSummaryRows(rep houseReport) [][]string {
	m := rep.Monthly
	occupancy := m.OccupancyText()
	if occupancy == "" {
		occupancy = "not available"
	}
	return [][]string{
		{"Residents served", strconv.Itoa(m.ResidentsServed)},
		{"Bed-nights", strconv.Itoa(m.BedNights)},
		{"Admissions", strconv.Itoa(m.Admissions)},
		{"Departures", strconv.Itoa(m.Departures)},
		{"Days in month", strconv.Itoa(m.DaysInMonth)},
		{"Average occupancy", occupancy},
	}
}

func csvDemographicsRows(rep houseReport) [][]string {
	var rows [][]string
	for _, c := range rep.Monthly.ByGender {
		rows = append(rows, []string{"Gender: " + c.Label, strconv.Itoa(c.N)})
	}
	for _, c := range rep.Monthly.ByCountry {
		rows = append(rows, []string{"Country of origin: " + c.Label, strconv.Itoa(c.N)})
	}
	return rows
}

func csvExpenseRows(rep houseReport) [][]string {
	rows := [][]string{
		{"Expenses", strconv.Itoa(rep.Expenses.ExpenseCount)},
		{"Expenses with receipt", strconv.Itoa(rep.Expenses.ReceiptCount)},
	}
	for _, c := range rep.Expenses.Currencies {
		rows = append(rows, []string{"Expense total " + c.Currency, c.Amount})
	}
	return rows
}
