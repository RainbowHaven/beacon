package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

type MonthConfirmationInput struct {
	SafeHouseID int64
	Month       time.Time
	Fingerprint string
	ConfirmedBy int64
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

const monthConfirmationCols = `mc.id, mc.safe_house_id, mc.month, mc.fingerprint, mc.confirmed_by,
	COALESCE(NULLIF(u.display_name, ''), u.email, ''), mc.confirmed_at`

const monthConfirmationFrom = ` FROM month_confirmations mc LEFT JOIN users u ON u.id = mc.confirmed_by`

func scanMonthConfirmation(row interface{ Scan(dest ...any) error }) (domain.MonthConfirmation, error) {
	var c domain.MonthConfirmation
	var by sql.NullInt64
	if err := row.Scan(&c.ID, &c.SafeHouseID, &c.Month, &c.Fingerprint, &by, &c.ConfirmedByName, &c.ConfirmedAt); err != nil {
		return domain.MonthConfirmation{}, err
	}
	c.Month = monthStart(c.Month)
	c.ConfirmedAt = c.ConfirmedAt.UTC()
	if by.Valid {
		v := by.Int64
		c.ConfirmedBy = &v
	}
	return c, nil
}

// ConfirmMonth records a confirmation. Earlier confirmations of the same
// house and month are kept.
func (s *Store) ConfirmMonth(ctx context.Context, in MonthConfirmationInput) (domain.MonthConfirmation, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO month_confirmations (safe_house_id, month, fingerprint, confirmed_by)
		VALUES ($1, $2::date, $3, $4)
		RETURNING id`,
		in.SafeHouseID, monthStart(in.Month), in.Fingerprint, in.ConfirmedBy,
	).Scan(&id)
	if err != nil {
		return domain.MonthConfirmation{}, err
	}
	return scanMonthConfirmation(s.db.QueryRowContext(ctx,
		`SELECT `+monthConfirmationCols+monthConfirmationFrom+` WHERE mc.id = $1`, id))
}

// LatestMonthConfirmation returns the current confirmation of a house and
// month, or ErrNotFound.
func (s *Store) LatestMonthConfirmation(ctx context.Context, houseID int64, month time.Time) (domain.MonthConfirmation, error) {
	c, err := scanMonthConfirmation(s.db.QueryRowContext(ctx, `SELECT `+monthConfirmationCols+monthConfirmationFrom+`
		WHERE mc.safe_house_id = $1 AND mc.month = $2::date
		ORDER BY mc.confirmed_at DESC, mc.id DESC LIMIT 1`, houseID, monthStart(month)))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MonthConfirmation{}, ErrNotFound
	}
	return c, err
}

// LatestMonthConfirmations returns the latest confirmation of every house and
// month that has one, ordered by house and then newest month first.
func (s *Store) LatestMonthConfirmations(ctx context.Context, houseIDs []int64) ([]domain.MonthConfirmation, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	in, args := int64InClause(1, houseIDs)
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (mc.safe_house_id, mc.month) `+monthConfirmationCols+monthConfirmationFrom+`
		WHERE mc.safe_house_id IN (`+in+`)
		ORDER BY mc.safe_house_id, mc.month DESC, mc.confirmed_at DESC, mc.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MonthConfirmation
	for rows.Next() {
		c, err := scanMonthConfirmation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MonthFingerprint is a SHA-256 over the data behind the monthly report of a
// safe house: residents in the month, approved sleeping places, safeguarding
// concerns and operational issues shown for the month, and expenses in the
// month. Only the fields written here are included, so new columns do not
// change it. Departure, status and closure are taken as of the last day of
// the month, so a later departure or closure does not change an earlier month.
func (s *Store) MonthFingerprint(ctx context.Context, houseID int64, month time.Time) (string, error) {
	start := monthStart(month)
	end := start.AddDate(0, 1, -1)
	ids := []int64{houseID}
	house, err := s.GetSafeHouse(ctx, houseID)
	if err != nil {
		return "", err
	}
	occupants, err := s.ListOccupantsInPeriod(ctx, ids, start, end)
	if err != nil {
		return "", err
	}
	concerns, err := s.SafeguardingConcernsForMonth(ctx, ids, start, end)
	if err != nil {
		return "", err
	}
	issues, err := s.OperationalIssuesActiveInRange(ctx, ids, start, end)
	if err != nil {
		return "", err
	}
	expenses, err := s.fingerprintExpenses(ctx, houseID, start, end)
	if err != nil {
		return "", err
	}

	const dayFmt = "2006-01-02"
	date := func(t time.Time) string { return t.UTC().Format(dayFmt) }
	// byMonthEnd hides dates after the month.
	byMonthEnd := func(t *time.Time) any {
		if t == nil || t.After(end) {
			return nil
		}
		return date(*t)
	}

	var records [][]any
	add := func(rec ...any) { records = append(records, rec) }
	add("beacon-month-v1", houseID, date(start))
	add("house", house.ApprovedSleepingPlaces)

	sort.Slice(occupants, func(i, j int) bool { return occupants[i].ID < occupants[j].ID })
	for _, o := range occupants {
		add("occupant", o.ID, o.Nickname, date(o.ArrivedAt), byMonthEnd(o.DepartedAt),
			o.CountryOfOrigin, o.Gender, o.BirthYear)
	}

	sort.Slice(concerns, func(i, j int) bool { return concerns[i].ID < concerns[j].ID })
	for _, c := range concerns {
		status := string(domain.SafeguardingOpen)
		if byMonthEnd(c.ClosedOn) != nil {
			status = string(c.Status)
		}
		var occurred any
		if c.OccurredOn != nil {
			occurred = date(*c.OccurredOn)
		}
		add("safeguarding", c.ID, c.IncidentID, occurred, string(c.OccurredPrecision),
			date(c.ReportedOn), status, byMonthEnd(c.ClosedOn))
	}

	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	for _, o := range issues {
		status, notes := domain.OperationalIssueOpen, ""
		if byMonthEnd(o.ClosedOn) != nil {
			status, notes = o.Status, o.ClosureNotes
		}
		add("operational", o.ID, date(o.IdentifiedOn), o.Category, o.Description, o.Effect,
			o.ActionTaken, o.RHLRequest, status, byMonthEnd(o.ClosedOn), notes)
	}

	for _, e := range expenses {
		add("expense", e.id, date(e.spentOn), e.amountCents, e.currency, e.merchant, e.category,
			e.note, e.noReceiptReason, e.hasReceipt, e.receiptBytes)
	}

	h := sha256.New()
	enc := json.NewEncoder(h)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			return "", fmt.Errorf("fingerprint: %w", err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fingerprintExpense holds what the house enters. Review status and notes are
// the bookkeeper's and do not reopen a confirmed month.
type fingerprintExpense struct {
	id              int64
	spentOn         time.Time
	amountCents     int64
	currency        string
	merchant        string
	category        string
	note            string
	noReceiptReason string
	hasReceipt      bool
	receiptBytes    int64
}

func (s *Store) fingerprintExpenses(ctx context.Context, houseID int64, from, to time.Time) ([]fingerprintExpense, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, spent_on, amount_cents, currency, merchant, category, note, no_receipt_reason,
			receipt_data IS NOT NULL, COALESCE(octet_length(receipt_data), 0)
		FROM expenses
		WHERE safe_house_id = $1 AND spent_on >= $2::date AND spent_on <= $3::date
		ORDER BY id`, houseID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fingerprintExpense
	for rows.Next() {
		var e fingerprintExpense
		if err := rows.Scan(&e.id, &e.spentOn, &e.amountCents, &e.currency, &e.merchant, &e.category,
			&e.note, &e.noReceiptReason, &e.hasReceipt, &e.receiptBytes); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
