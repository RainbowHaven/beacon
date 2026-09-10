package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/magiconair/beacon/internal/domain"
	"github.com/magiconair/beacon/internal/store"
)

var allowedReceiptTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// Supported expense currencies (ISO 4217). House default must be one of these.
var expenseCurrencies = []string{"CAD", "USD", "EUR", "GBP", "UGX", "KES", "TZS", "RWF"}

func normalizeCurrency(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	for _, c := range expenseCurrencies {
		if c == s {
			return s, true
		}
	}
	return "", false
}

func (s *Server) handleExpenses(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	ids := houseIDs(houses)
	list, err := s.store.ListExpensesByHouses(r.Context(), ids, 200)
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
	s.render(w, "expenses.html", map[string]any{
		"Title": "Expenses",
		"User":  &u,
		"Rows":  rows,
		"Flash": r.URL.Query().Get("ok"),
		"Error": r.URL.Query().Get("error"),
	})
}

func (s *Server) handleExpenseNew(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil || len(houses) == 0 {
		http.Error(w, "no safe house in scope", http.StatusForbidden)
		return
	}
	s.render(w, "expense_new.html", map[string]any{
		"Title":      "Log expense",
		"User":       &u,
		"Houses":     houses,
		"Currencies": expenseCurrencies,
		"Today":      formatUSDate(time.Now().UTC()),
		"Error":      r.URL.Query().Get("error"),
		"MaxMB":      s.cfg.MaxReceiptBytes / (1024 * 1024),
	})
}

func (s *Server) handleExpenseCreate(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	maxMem := s.cfg.MaxReceiptBytes + (1 << 20)
	if err := r.ParseMultipartForm(maxMem); err != nil {
		http.Redirect(w, r, "/expenses/new?error=bad+form+or+file+too+large", http.StatusSeeOther)
		return
	}
	houseID, err := parseID(r.FormValue("safe_house_id"))
	if err != nil {
		http.Redirect(w, r, "/expenses/new?error=invalid+house", http.StatusSeeOther)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), houseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	cents, err := parseMoneyToCents(r.FormValue("amount"))
	if err != nil || cents < 0 {
		http.Redirect(w, r, "/expenses/new?error=invalid+amount", http.StatusSeeOther)
		return
	}
	spent, err := parseUSDate(r.FormValue("spent_on"))
	if err != nil {
		http.Redirect(w, r, "/expenses/new?error=invalid+date+(use+MM/DD/YYYY)", http.StatusSeeOther)
		return
	}
	currency, ok := normalizeCurrency(r.FormValue("currency"))
	if !ok {
		if house.DefaultCurrency != "" {
			currency, ok = normalizeCurrency(house.DefaultCurrency)
		}
		if !ok {
			http.Redirect(w, r, "/expenses/new?error=invalid+currency", http.StatusSeeOther)
			return
		}
	}

	in := store.CreateExpenseInput{
		SafeHouseID: houseID,
		AmountCents: cents,
		Currency:    currency,
		Note:        r.FormValue("note"),
		SpentOn:     spent,
		CreatedBy:   &u.ID,
	}

	file, header, err := r.FormFile("receipt")
	switch {
	case err == nil:
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxReceiptBytes+1))
		if err != nil {
			http.Redirect(w, r, "/expenses/new?error=could+not+read+receipt", http.StatusSeeOther)
			return
		}
		if int64(len(data)) > s.cfg.MaxReceiptBytes {
			http.Redirect(w, r, "/expenses/new?error=receipt+too+large", http.StatusSeeOther)
			return
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
		ext, ok := allowedReceiptTypes[ct]
		if !ok {
			http.Redirect(w, r, "/expenses/new?error=receipt+must+be+jpeg+png+webp+or+pdf", http.StatusSeeOther)
			return
		}
		key, err := newReceiptKey(houseID, ext)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if err := s.blobs.Put(r.Context(), key, bytes.NewReader(data), int64(len(data))); err != nil {
			s.log.Error("receipt put", "err", err)
			http.Redirect(w, r, "/expenses/new?error=could+not+store+receipt", http.StatusSeeOther)
			return
		}
		n := len(data)
		in.ReceiptKey = &key
		in.ReceiptContentType = &ct
		in.ReceiptBytes = &n
	case err == http.ErrMissingFile:
		// optional receipt
	default:
		http.Redirect(w, r, "/expenses/new?error=could+not+read+receipt", http.StatusSeeOther)
		return
	}

	e, err := s.store.CreateExpense(r.Context(), in)
	if err != nil {
		if in.ReceiptKey != nil {
			_ = s.blobs.Delete(r.Context(), *in.ReceiptKey)
		}
		s.log.Error("create expense", "err", err)
		http.Redirect(w, r, "/expenses/new?error=could+not+save", http.StatusSeeOther)
		return
	}
	_ = s.store.Audit(r.Context(), &u.ID, "expense.create", "expense", idString(e.ID), map[string]any{
		"safe_house_id": houseID,
		"amount_cents":  cents,
		"has_receipt":   e.HasReceipt(),
	})
	http.Redirect(w, r, "/expenses?ok=logged", http.StatusSeeOther)
}

func (s *Server) handleExpenseReceipt(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	e, err := s.store.GetExpense(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), e.SafeHouseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !e.HasReceipt() {
		http.Error(w, "no receipt", http.StatusNotFound)
		return
	}
	rc, err := s.blobs.Open(r.Context(), *e.ReceiptKey)
	if err != nil {
		http.Error(w, "receipt missing", http.StatusNotFound)
		return
	}
	defer rc.Close()
	if e.ReceiptContentType != nil {
		w.Header().Set("Content-Type", *e.ReceiptContentType)
	}
	w.Header().Set("Content-Disposition", "inline; filename=\"receipt-"+idString(e.ID)+"\"")
	_, _ = io.Copy(w, rc)
}

func (s *Server) handleExpenseDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	e, err := s.store.GetExpense(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	house, err := s.store.GetSafeHouse(r.Context(), e.SafeHouseID)
	if err != nil || !s.canAccessHouse(u, house) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.store.DeleteExpense(r.Context(), id); err != nil {
		http.Redirect(w, r, "/expenses?error=could+not+delete", http.StatusSeeOther)
		return
	}
	if e.HasReceipt() {
		_ = s.blobs.Delete(r.Context(), *e.ReceiptKey)
	}
	_ = s.store.Audit(r.Context(), &u.ID, "expense.delete", "expense", idString(id), nil)
	http.Redirect(w, r, "/expenses?ok=deleted", http.StatusSeeOther)
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	houses, err := s.housesForUser(r, u)
	if err != nil {
		http.Error(w, "failed to load houses", http.StatusInternalServerError)
		return
	}
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}
	start, err := time.Parse("2006-01", month)
	if err != nil {
		http.Redirect(w, r, "/reports?error=invalid+month", http.StatusSeeOther)
		return
	}
	end := start.AddDate(0, 1, -1)

	ids := houseIDs(houses)
	headcount, err := s.store.HeadcountByHouses(r.Context(), ids)
	if err != nil {
		http.Error(w, "failed to load headcount", http.StatusInternalServerError)
		return
	}
	totals, err := s.store.ExpenseTotalsByHouses(r.Context(), ids, start, end)
	if err != nil {
		http.Error(w, "failed to load expense totals", http.StatusInternalServerError)
		return
	}
	type totalRow struct {
		domain.ExpenseTotals
		Amount string
	}
	type currencyTotal struct {
		Currency     string
		Amount       string
		ExpenseCount int
		ReceiptCount int
	}
	rows := make([]totalRow, 0, len(totals))
	curCents := map[string]int64{}
	curExpenses := map[string]int{}
	curReceipts := map[string]int{}
	var sumExpenses, sumReceipts int
	for _, t := range totals {
		rows = append(rows, totalRow{ExpenseTotals: t, Amount: formatCents(t.AmountCents)})
		sumExpenses += t.ExpenseCount
		sumReceipts += t.ReceiptCount
		curCents[t.Currency] += t.AmountCents
		curExpenses[t.Currency] += t.ExpenseCount
		curReceipts[t.Currency] += t.ReceiptCount
	}
	currencyTotals := make([]currencyTotal, 0, len(curCents))
	seen := map[string]bool{}
	for _, c := range expenseCurrencies {
		if _, ok := curCents[c]; !ok {
			continue
		}
		currencyTotals = append(currencyTotals, currencyTotal{
			Currency:     c,
			Amount:       formatCents(curCents[c]),
			ExpenseCount: curExpenses[c],
			ReceiptCount: curReceipts[c],
		})
		seen[c] = true
	}
	for c, cents := range curCents {
		if seen[c] {
			continue
		}
		currencyTotals = append(currencyTotals, currencyTotal{
			Currency:     c,
			Amount:       formatCents(cents),
			ExpenseCount: curExpenses[c],
			ReceiptCount: curReceipts[c],
		})
	}
	s.render(w, "reports.html", map[string]any{
		"Title":          "Monthly report",
		"User":           &u,
		"BodyClass":      "report-page",
		"Month":          month,
		"MonthLabel":     start.Format("January 2006"),
		"PrintedOn":      formatUSDate(time.Now().UTC()),
		"Headcount":      headcount,
		"Totals":         rows,
		"CurrencyTotals": currencyTotals,
		"SumExpenses":    sumExpenses,
		"SumReceipts":    sumReceipts,
		"Error":          r.URL.Query().Get("error"),
	})
}

func newReceiptKey(houseID int64, ext string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	day := time.Now().UTC().Format("2006/01/02")
	return path.Join("receipts", fmt.Sprintf("%d", houseID), day, hex.EncodeToString(b[:])+ext), nil
}
