package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

var allowedReceiptTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// Supported expense currencies (ISO 4217). House default must be one of these.
var expenseCurrencies = []string{"CAD", "USD", "EUR", "GBP", "UGX", "KES", "TZS", "RWF"}

const (
	maxMerchantLen   = 120
	maxNoteLen       = 240
	maxReasonLen     = 500
	maxReviewNoteLen = 500
)

func normalizeCurrency(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	for _, c := range expenseCurrencies {
		if c == s {
			return s, true
		}
	}
	return "", false
}

// canReviewExpenses reports whether the role may set review status; house scope is checked separately.
func canReviewExpenses(u domain.User) bool {
	return u.Role == domain.RoleRHLAdmin || u.Role == domain.RoleRHCAdmin
}

// expenseFormValues holds the raw form input so it can be shown again after an error.
type expenseFormValues struct {
	SafeHouseID     int64
	Amount          string
	Currency        string
	SpentOn         string
	Merchant        string
	Category        string
	Note            string
	NoReceiptReason string
}

type expenseFields struct {
	AmountCents     int64
	Currency        string
	SpentOn         time.Time
	Merchant        string
	Category        string
	Note            string
	NoReceiptReason string
}

func expenseFormFromRequest(r *http.Request) expenseFormValues {
	houseID, _ := parseID(r.FormValue("safe_house_id"))
	return expenseFormValues{
		SafeHouseID:     houseID,
		Amount:          strings.TrimSpace(r.FormValue("amount")),
		Currency:        strings.TrimSpace(r.FormValue("currency")),
		SpentOn:         strings.TrimSpace(r.FormValue("spent_on")),
		Merchant:        strings.TrimSpace(r.FormValue("merchant")),
		Category:        strings.TrimSpace(r.FormValue("category")),
		Note:            strings.TrimSpace(r.FormValue("note")),
		NoReceiptReason: strings.TrimSpace(r.FormValue("no_receipt_reason")),
	}
}

func expenseFormFromExpense(e domain.Expense) expenseFormValues {
	return expenseFormValues{
		SafeHouseID:     e.SafeHouseID,
		Amount:          formatCents(e.AmountCents),
		Currency:        e.Currency,
		SpentOn:         formatUSDate(e.SpentOn),
		Merchant:        e.Merchant,
		Category:        e.Category,
		Note:            e.Note,
		NoReceiptReason: e.NoReceiptReason,
	}
}

const expenseCategoryMsg = "Pick a category."

// parse validates the form. An unknown currency falls back to the house default.
func (v expenseFormValues) parse(defaultCurrency string, categories domain.Categories) (expenseFields, string) {
	var f expenseFields
	cents, err := parseMoneyToCents(v.Amount)
	if err != nil || cents < 0 {
		return f, "Enter a valid amount, for example 12.50."
	}
	spent, err := parseUSDate(v.SpentOn)
	if err != nil {
		return f, "Enter the date as MM/DD/YYYY."
	}
	currency, ok := normalizeCurrency(v.Currency)
	if !ok {
		currency, ok = normalizeCurrency(defaultCurrency)
	}
	if !ok {
		return f, "Pick a currency."
	}
	if !categories.IsOffered(v.Category) {
		return f, expenseCategoryMsg
	}
	switch {
	case utf8.RuneCountInString(v.Merchant) > maxMerchantLen:
		return f, fmt.Sprintf("Merchant must be at most %d characters.", maxMerchantLen)
	case utf8.RuneCountInString(v.Note) > maxNoteLen:
		return f, fmt.Sprintf("Description must be at most %d characters.", maxNoteLen)
	case utf8.RuneCountInString(v.NoReceiptReason) > maxReasonLen:
		return f, fmt.Sprintf("Explanation must be at most %d characters.", maxReasonLen)
	}
	return expenseFields{
		AmountCents:     cents,
		Currency:        currency,
		SpentOn:         spent,
		Merchant:        v.Merchant,
		Category:        v.Category,
		Note:            v.Note,
		NoReceiptReason: v.NoReceiptReason,
	}, ""
}

const noReceiptReasonMsg = "Explain why there is no receipt."

// readReceipt returns nil data when no file was uploaded; errMsg is shown to the user.
func (s *Server) readReceipt(r *http.Request) (data []byte, contentType string, errMsg string) {
	file, header, err := r.FormFile("receipt")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, "", ""
	}
	if err != nil {
		return nil, "", "Could not read the receipt."
	}
	defer file.Close()
	data, err = io.ReadAll(io.LimitReader(file, s.cfg.MaxReceiptBytes+1))
	if err != nil {
		return nil, "", "Could not read the receipt."
	}
	if int64(len(data)) > s.cfg.MaxReceiptBytes {
		return nil, "", "The receipt is too large."
	}
	if len(data) == 0 {
		return nil, "", ""
	}
	ct := header.Header.Get("Content-Type")
	if sniffed := http.DetectContentType(data); sniffed != "application/octet-stream" {
		ct = sniffed
	}
	if ct == "" || ct == "application/octet-stream" {
		name := strings.ToLower(header.Filename)
		switch {
		case strings.HasSuffix(name, ".pdf"):
			ct = "application/pdf"
		case strings.HasSuffix(name, ".png"):
			ct = "image/png"
		case strings.HasSuffix(name, ".webp"):
			ct = "image/webp"
		case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
			ct = "image/jpeg"
		}
	}
	if _, ok := allowedReceiptTypes[ct]; !ok {
		return nil, "", "The receipt must be a JPEG, PNG, WebP, or PDF file."
	}
	return data, ct, ""
}

// loadExpenseInScope writes an error response and returns false when the expense is missing or out of scope.
func (s *Server) loadExpenseInScope(w http.ResponseWriter, r *http.Request, u domain.User) (domain.Expense, domain.SafeHouse, bool) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return domain.Expense{}, domain.SafeHouse{}, false
	}
	e, err := s.store.GetExpense(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return domain.Expense{}, domain.SafeHouse{}, false
	}
	house, err := s.store.GetSafeHouse(r.Context(), e.SafeHouseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return domain.Expense{}, domain.SafeHouse{}, false
	}
	return e, house, true
}

func (s *Server) handleExpenses(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	status, _ := domain.ParseExpenseReviewStatus(r.URL.Query().Get("status"))
	list, err := s.store.ListExpenses(r.Context(), store.ExpenseFilter{
		HouseIDs:     houseIDs(houses),
		ReviewStatus: status,
		Limit:        200,
	})
	if err != nil {
		http.Error(w, "failed to list expenses", http.StatusInternalServerError)
		return
	}
	houseName := map[int64]string{}
	for _, h := range houses {
		houseName[h.ID] = h.Name
	}
	type row struct {
		domain.Expense
		HouseName string
		Amount    string
		SpentOnUS string
	}
	rows := make([]row, 0, len(list))
	for _, e := range list {
		rows = append(rows, row{
			Expense:   e,
			HouseName: houseName[e.SafeHouseID],
			Amount:    formatCents(e.AmountCents),
			SpentOnUS: formatUSDate(e.SpentOn),
		})
	}
	s.render(w, r, "expenses.html", map[string]any{
		"Title":     "Expenses",
		"User":      &u,
		"Rows":      rows,
		"Status":    string(status),
		"Statuses":  domain.ExpenseReviewStatuses(),
		"CanReview": canReviewExpenses(u),
		"Flash":     r.URL.Query().Get("ok"),
		"Error":     r.URL.Query().Get("error"),
	})
}

func (s *Server) renderExpenseNew(w http.ResponseWriter, r *http.Request, u domain.User, houses []domain.SafeHouse, categories domain.Categories, form expenseFormValues, errMsg string) {
	if form.SafeHouseID == 0 && len(houses) > 0 {
		form.SafeHouseID = houses[0].ID
	}
	if form.Currency == "" {
		for _, h := range houses {
			if h.ID == form.SafeHouseID {
				form.Currency = h.DefaultCurrency
			}
		}
	}
	if form.SpentOn == "" {
		form.SpentOn = formatUSDate(time.Now().UTC())
	}
	s.render(w, r, "expense_new.html", map[string]any{
		"Title":      "Log expense",
		"User":       &u,
		"Houses":     houses,
		"Currencies": expenseCurrencies,
		"Categories": categories.Offered(),
		"Form":       form,
		"Error":      errMsg,
		"MaxMB":      s.cfg.MaxReceiptBytes / (1024 * 1024),
	})
}

func (s *Server) handleExpenseNew(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	categories, err := s.store.ExpenseCategories(r.Context())
	if err != nil {
		http.Error(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	s.renderExpenseNew(w, r, u, houses, categories, expenseFormValues{}, r.URL.Query().Get("error"))
}

func (s *Server) handleExpenseCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	categories, err := s.store.ExpenseCategories(r.Context())
	if err != nil {
		http.Error(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	maxMem := s.cfg.MaxReceiptBytes + (1 << 20)
	if err := r.ParseMultipartForm(maxMem); err != nil {
		s.renderExpenseNew(w, r, u, houses, categories, expenseFormValues{}, "The form could not be read or the file is too large.")
		return
	}
	form := expenseFormFromRequest(r)
	house, err := s.store.GetSafeHouse(r.Context(), form.SafeHouseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fields, msg := form.parse(house.DefaultCurrency, categories)
	if msg != "" {
		s.renderExpenseNew(w, r, u, houses, categories, form, msg)
		return
	}
	data, ct, msg := s.readReceipt(r)
	if msg != "" {
		s.renderExpenseNew(w, r, u, houses, categories, form, msg)
		return
	}
	if data == nil && fields.NoReceiptReason == "" {
		s.renderExpenseNew(w, r, u, houses, categories, form, noReceiptReasonMsg)
		return
	}

	in := store.CreateExpenseInput{
		SafeHouseID:     house.ID,
		AmountCents:     fields.AmountCents,
		Currency:        fields.Currency,
		Merchant:        fields.Merchant,
		Category:        fields.Category,
		Note:            fields.Note,
		NoReceiptReason: fields.NoReceiptReason,
		SpentOn:         fields.SpentOn,
		CreatedBy:       &u.ID,
	}
	if data != nil {
		in.ReceiptData = data
		in.ReceiptContentType = &ct
	}
	e, err := s.store.CreateExpense(r.Context(), in)
	if errors.Is(err, store.ErrExpenseCategory) {
		s.renderExpenseNew(w, r, u, houses, categories, form, expenseCategoryMsg)
		return
	}
	if err != nil {
		s.log.Error("create expense", "err", err)
		s.renderExpenseNew(w, r, u, houses, categories, form, "Could not save the expense.")
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "expense.create", "expense", idString(e.ID), map[string]any{
		"safe_house_id": house.ID,
		"amount_cents":  e.AmountCents,
		"category":      e.Category,
		"has_receipt":   e.HasReceipt(),
	})
	http.Redirect(w, r, "/expenses?ok=logged", http.StatusSeeOther)
}

type expenseEditView struct {
	Expense     domain.Expense
	House       domain.SafeHouse
	Categories  domain.Categories
	Form        expenseFormValues
	Error       string
	ReviewError string
	ReviewNote  string
}

func (s *Server) renderExpenseEdit(w http.ResponseWriter, r *http.Request, u domain.User, v expenseEditView) {
	events, err := s.store.ListAuditEventsForSubject(r.Context(), "expense", idString(v.Expense.ID), 0)
	if err != nil {
		s.log.Error("expense history", "err", err)
	}
	reviewedAt := ""
	if v.Expense.ReviewedAt != nil {
		reviewedAt = formatUSDate(*v.Expense.ReviewedAt)
	}
	if v.ReviewNote == "" && v.ReviewError == "" {
		v.ReviewNote = v.Expense.ReviewNote
	}
	s.render(w, r, "expense_edit.html", map[string]any{
		"Title":      "Expense",
		"User":       &u,
		"Expense":    v.Expense,
		"House":      v.House,
		"Amount":     formatCents(v.Expense.AmountCents),
		"ReviewedAt": reviewedAt,
		"Form":       v.Form,
		"Currencies": expenseCurrencies,
		"Categories": v.Categories.Offered(),
		"Statuses":   domain.ExpenseReviewStatuses(),
		"CanReview":  canReviewExpenses(u),
		"ReviewNote": v.ReviewNote,
		"History":    expenseHistory(events, v.Categories),
		"MaxMB":      s.cfg.MaxReceiptBytes / (1024 * 1024),
		"Flash":      r.URL.Query().Get("ok"),
		"Error":      v.Error,
		"ReviewErr":  v.ReviewError,
	})
}

func (s *Server) handleExpenseEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	e, house, ok := s.loadExpenseInScope(w, r, u)
	if !ok {
		return
	}
	categories, err := s.store.ExpenseCategories(r.Context())
	if err != nil {
		http.Error(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	s.renderExpenseEdit(w, r, u, expenseEditView{
		Expense:    e,
		House:      house,
		Categories: categories,
		Form:       expenseFormFromExpense(e),
		Error:      r.URL.Query().Get("error"),
	})
}

func (s *Server) handleExpenseUpdate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	e, house, ok := s.loadExpenseInScope(w, r, u)
	if !ok {
		return
	}
	categories, err := s.store.ExpenseCategories(r.Context())
	if err != nil {
		http.Error(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	view := expenseEditView{Expense: e, House: house, Categories: categories, Form: expenseFormFromExpense(e)}
	maxMem := s.cfg.MaxReceiptBytes + (1 << 20)
	if err := r.ParseMultipartForm(maxMem); err != nil {
		view.Error = "The form could not be read or the file is too large."
		s.renderExpenseEdit(w, r, u, view)
		return
	}
	view.Form = expenseFormFromRequest(r)
	view.Form.SafeHouseID = e.SafeHouseID
	fields, msg := view.Form.parse(house.DefaultCurrency, categories)
	if msg != "" {
		view.Error = msg
		s.renderExpenseEdit(w, r, u, view)
		return
	}
	data, ct, msg := s.readReceipt(r)
	if msg != "" {
		view.Error = msg
		s.renderExpenseEdit(w, r, u, view)
		return
	}
	in := store.UpdateExpenseInput{
		AmountCents:     fields.AmountCents,
		Currency:        fields.Currency,
		Merchant:        fields.Merchant,
		Category:        fields.Category,
		Note:            fields.Note,
		NoReceiptReason: fields.NoReceiptReason,
		SpentOn:         fields.SpentOn,
	}
	if data != nil {
		in.ReceiptData = data
		in.ReceiptContentType = &ct
	}
	before, after, err := s.store.UpdateExpense(r.Context(), e.ID, in)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNoReceiptReason):
			view.Error = noReceiptReasonMsg
		case errors.Is(err, store.ErrExpenseCategory):
			view.Error = expenseCategoryMsg
		default:
			s.log.Error("update expense", "err", err)
			view.Error = "Could not save the expense."
		}
		s.renderExpenseEdit(w, r, u, view)
		return
	}
	changes := expenseChanges(before, after)
	receipt := ""
	if data != nil {
		receipt = "added"
		if before.HasReceipt() {
			receipt = "replaced"
		}
	}
	if len(changes) == 0 && receipt == "" {
		http.Redirect(w, r, "/expenses/"+idString(e.ID)+"/edit?ok=no+changes", http.StatusSeeOther)
		return
	}
	meta := map[string]any{
		"safe_house_id": e.SafeHouseID,
		"changes":       changes,
	}
	if receipt != "" {
		meta["receipt"] = receipt
	}
	_ = s.store.Audit(r.Context(), &u.ID, "expense.update", "expense", idString(e.ID), meta)
	http.Redirect(w, r, "/expenses/"+idString(e.ID)+"/edit?ok=saved", http.StatusSeeOther)
}

func (s *Server) handleExpenseReview(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if !canReviewExpenses(u) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	e, house, ok := s.loadExpenseInScope(w, r, u)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	categories, err := s.store.ExpenseCategories(r.Context())
	if err != nil {
		http.Error(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	view := expenseEditView{Expense: e, House: house, Categories: categories, Form: expenseFormFromExpense(e)}
	note := strings.TrimSpace(r.FormValue("review_note"))
	view.ReviewNote = note
	status, ok := domain.ParseExpenseReviewStatus(r.FormValue("review_status"))
	switch {
	case !ok:
		view.ReviewError = "Pick a review status."
	case status == domain.ExpenseNeedsCorrection && note == "":
		view.ReviewError = "Say what needs to be corrected."
	case utf8.RuneCountInString(note) > maxReviewNoteLen:
		view.ReviewError = fmt.Sprintf("Review note must be at most %d characters.", maxReviewNoteLen)
	}
	if view.ReviewError != "" {
		s.renderExpenseEdit(w, r, u, view)
		return
	}
	before, after, err := s.store.SetExpenseReview(r.Context(), e.ID, status, u.ID, note)
	if err != nil {
		s.log.Error("review expense", "err", err)
		view.ReviewError = "Could not save the review."
		s.renderExpenseEdit(w, r, u, view)
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "expense.review", "expense", idString(e.ID), map[string]any{
		"safe_house_id": e.SafeHouseID,
		"from":          string(before.ReviewStatus),
		"to":            string(after.ReviewStatus),
		"note":          after.ReviewNote,
	})
	http.Redirect(w, r, "/expenses/"+idString(e.ID)+"/edit?ok=review+saved", http.StatusSeeOther)
}

// expenseChanges lists changed editable fields as {"field": {"from": old, "to": new}}.
// Receipt bytes are never included.
func expenseChanges(before, after domain.Expense) map[string]map[string]any {
	out := map[string]map[string]any{}
	add := func(field string, from, to any) {
		if from != to {
			out[field] = map[string]any{"from": from, "to": to}
		}
	}
	add("amount_cents", before.AmountCents, after.AmountCents)
	add("currency", before.Currency, after.Currency)
	add("spent_on", before.SpentOn.Format("2006-01-02"), after.SpentOn.Format("2006-01-02"))
	add("merchant", before.Merchant, after.Merchant)
	add("category", before.Category, after.Category)
	add("note", before.Note, after.Note)
	add("no_receipt_reason", before.NoReceiptReason, after.NoReceiptReason)
	add("review_status", string(before.ReviewStatus), string(after.ReviewStatus))
	return out
}

var expenseFieldLabels = []struct{ Key, Label string }{
	{"amount_cents", "Amount"},
	{"currency", "Currency"},
	{"spent_on", "Date"},
	{"merchant", "Merchant"},
	{"category", "Category"},
	{"note", "Description"},
	{"no_receipt_reason", "No-receipt explanation"},
	{"review_status", "Review status"},
}

func formatExpenseValue(field string, v any, categories domain.Categories) string {
	switch field {
	case "amount_cents":
		switch n := v.(type) {
		case float64:
			return formatCents(int64(n))
		case int64:
			return formatCents(n)
		}
	case "spent_on":
		if s, ok := v.(string); ok {
			if t, err := time.Parse("2006-01-02", s); err == nil {
				return formatUSDate(t)
			}
		}
	case "category":
		if s, ok := v.(string); ok {
			return categories.Label(s)
		}
	case "review_status":
		if s, ok := v.(string); ok {
			return domain.ExpenseReviewStatus(s).Label()
		}
	}
	s := fmt.Sprint(v)
	if v == nil || s == "" {
		return "—"
	}
	return s
}

type expenseHistoryRow struct {
	When    string
	Actor   string
	Summary string
	Lines   []string
}

func expenseHistory(events []domain.AuditEvent, categories domain.Categories) []expenseHistoryRow {
	rows := make([]expenseHistoryRow, 0, len(events))
	for _, ev := range events {
		row := expenseHistoryRow{
			When:  ev.CreatedAt.UTC().Format("01/02/2006 15:04 UTC"),
			Actor: ev.ActorName,
		}
		if row.Actor == "" {
			row.Actor = ev.ActorEmail
		}
		if row.Actor == "" {
			row.Actor = "System"
		}
		switch ev.Action {
		case "expense.create":
			row.Summary = "Logged"
		case "expense.update":
			row.Summary = "Edited"
			changes, _ := ev.Meta["changes"].(map[string]any)
			for _, f := range expenseFieldLabels {
				c, ok := changes[f.Key].(map[string]any)
				if !ok {
					continue
				}
				row.Lines = append(row.Lines, fmt.Sprintf("%s: %s → %s",
					f.Label, formatExpenseValue(f.Key, c["from"], categories), formatExpenseValue(f.Key, c["to"], categories)))
			}
			switch ev.Meta["receipt"] {
			case "replaced":
				row.Lines = append(row.Lines, "Receipt replaced")
			case "added":
				row.Lines = append(row.Lines, "Receipt added")
			}
		case "expense.review":
			to, _ := ev.Meta["to"].(string)
			row.Summary = "Review: " + domain.ExpenseReviewStatus(to).Label()
			if note, _ := ev.Meta["note"].(string); note != "" {
				row.Lines = append(row.Lines, "Note: "+note)
			}
		default:
			row.Summary = ev.Action
		}
		rows = append(rows, row)
	}
	return rows
}

func wantsReceiptHTML(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "image", "iframe", "embed", "object":
		return false
	case "document":
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func receiptBackURL(r *http.Request, expenseID int64) string {
	fallback := "/expenses/" + idString(expenseID) + "/edit"
	ref := r.Referer()
	if ref == "" {
		return fallback
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host != r.Host {
		return fallback
	}
	if u.Path == r.URL.Path {
		return fallback
	}
	return u.RequestURI()
}

func (s *Server) handleExpenseReceipt(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	e, _, ok := s.loadExpenseInScope(w, r, u)
	if !ok {
		return
	}
	if !e.HasReceipt() {
		http.Error(w, "no receipt", http.StatusNotFound)
		return
	}
	if wantsReceiptHTML(r) {
		isImage := e.ReceiptContentType != nil && strings.HasPrefix(*e.ReceiptContentType, "image/")
		s.render(w, r, "receipt_view.html", map[string]any{
			"Title":     "Receipt",
			"ExpenseID": e.ID,
			"IsImage":   isImage,
			"Back":      receiptBackURL(r, e.ID),
		})
		return
	}
	data, ct, err := s.store.GetExpenseReceipt(r.Context(), e.ID)
	if err != nil {
		http.Error(w, "receipt missing", http.StatusNotFound)
		return
	}
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Disposition", "inline; filename=\"receipt-"+idString(e.ID)+"\"")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

func (s *Server) handleExpenseDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	e, _, ok := s.loadExpenseInScope(w, r, u)
	if !ok {
		return
	}
	if err := s.store.DeleteExpense(r.Context(), e.ID); err != nil {
		msg := "could not delete"
		if errors.Is(err, store.ErrExpenseReviewed) {
			msg = "reviewed expenses cannot be deleted"
		}
		http.Redirect(w, r, "/expenses?error="+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "expense.delete", "expense", idString(e.ID), map[string]any{
		"safe_house_id": e.SafeHouseID,
		"amount_cents":  e.AmountCents,
		"currency":      e.Currency,
		"spent_on":      e.SpentOn.Format("2006-01-02"),
		"review_status": string(e.ReviewStatus),
	})
	http.Redirect(w, r, "/expenses?ok=deleted", http.StatusSeeOther)
}
