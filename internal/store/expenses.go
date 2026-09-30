package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
)

var (
	ErrNoReceiptReason = errors.New("explanation required when no receipt is attached")
	ErrExpenseReviewed = errors.New("expense already reviewed")
	ErrExpenseCategory = errors.New("invalid expense category")
	ErrReviewStatus    = errors.New("invalid review status")
)

type CreateExpenseInput struct {
	SafeHouseID        int64
	AmountCents        int64
	Currency           string
	Merchant           string
	Category           string
	Note               string
	NoReceiptReason    string
	SpentOn            time.Time
	ReceiptData        []byte
	ReceiptContentType *string
	ReceiptBytes       *int
	CreatedBy          *int64
}

// UpdateExpenseInput replaces the editable fields of an expense.
// A nil ReceiptData keeps the current receipt.
type UpdateExpenseInput struct {
	AmountCents        int64
	Currency           string
	Merchant           string
	Category           string
	Note               string
	NoReceiptReason    string
	SpentOn            time.Time
	ReceiptData        []byte
	ReceiptContentType *string
}

func normalizeExpenseCurrency(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if c == "" {
		return "CAD"
	}
	return c
}

func normalizeExpenseCategory(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return domain.ExpenseCategoryOther
	}
	return c
}

// expenseWriteErr maps a rejected category key to ErrExpenseCategory.
func expenseWriteErr(err error) error {
	if isForeignKeyViolation(err, "expenses_category_fkey") {
		return ErrExpenseCategory
	}
	return err
}

// noReceiptReason returns the stored explanation: cleared when a receipt exists, required otherwise.
func noReceiptReason(hasReceipt bool, reason string) (string, error) {
	if hasReceipt {
		return "", nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", ErrNoReceiptReason
	}
	return reason, nil
}

func (s *Store) CreateExpense(ctx context.Context, in CreateExpenseInput) (domain.Expense, error) {
	if in.AmountCents < 0 {
		return domain.Expense{}, errors.New("amount must be non-negative")
	}
	cur := normalizeExpenseCurrency(in.Currency)
	spent := in.SpentOn.UTC().Truncate(24 * time.Hour)
	cat := normalizeExpenseCategory(in.Category)

	hasReceipt := len(in.ReceiptData) > 0
	if hasReceipt {
		if in.ReceiptContentType == nil || strings.TrimSpace(*in.ReceiptContentType) == "" {
			return domain.Expense{}, errors.New("receipt content type required")
		}
		n := len(in.ReceiptData)
		in.ReceiptBytes = &n
	} else {
		in.ReceiptData = nil
		in.ReceiptContentType = nil
		in.ReceiptBytes = nil
	}
	reason, err := noReceiptReason(hasReceipt, in.NoReceiptReason)
	if err != nil {
		return domain.Expense{}, err
	}

	var id int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO expenses (
			safe_house_id, amount_cents, currency, merchant, category, note, no_receipt_reason, spent_on,
			receipt_data, receipt_content_type, receipt_bytes, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::date,$9,$10,$11,$12)
		RETURNING id`,
		in.SafeHouseID, in.AmountCents, cur, strings.TrimSpace(in.Merchant), cat,
		strings.TrimSpace(in.Note), reason, spent,
		in.ReceiptData, in.ReceiptContentType, in.ReceiptBytes, in.CreatedBy,
	).Scan(&id)
	if err != nil {
		return domain.Expense{}, expenseWriteErr(err)
	}
	return s.GetExpense(ctx, id)
}

func scanExpense(row interface{ Scan(dest ...any) error }) (domain.Expense, error) {
	var e domain.Expense
	var receiptCT sql.NullString
	var receiptBytes sql.NullInt64
	var createdBy, reviewedBy sql.NullInt64
	var reviewedAt sql.NullTime
	var status string
	err := row.Scan(
		&e.ID, &e.SafeHouseID, &e.AmountCents, &e.Currency, &e.Merchant, &e.Category, &e.CategoryLabel, &e.Note, &e.NoReceiptReason, &e.SpentOn,
		&receiptCT, &receiptBytes, &status, &reviewedBy, &reviewedAt, &e.ReviewNote,
		&createdBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return domain.Expense{}, err
	}
	e.ReviewStatus = domain.ExpenseReviewStatus(status)
	e.SpentOn = e.SpentOn.UTC().Truncate(24 * time.Hour)
	if receiptCT.Valid {
		v := receiptCT.String
		e.ReceiptContentType = &v
	}
	if receiptBytes.Valid {
		v := int(receiptBytes.Int64)
		e.ReceiptBytes = &v
	}
	if createdBy.Valid {
		v := createdBy.Int64
		e.CreatedBy = &v
	}
	if reviewedBy.Valid {
		v := reviewedBy.Int64
		e.ReviewedBy = &v
	}
	if reviewedAt.Valid {
		v := reviewedAt.Time
		e.ReviewedAt = &v
	}
	return e, nil
}

// expenseCols omits receipt_data so list/get stay lightweight.
const expenseCols = `id, safe_house_id, amount_cents, currency, merchant, category,
	COALESCE((SELECT c.label FROM expense_categories c WHERE c.key = expenses.category), category), note, no_receipt_reason, spent_on,
	receipt_content_type, receipt_bytes, review_status, reviewed_by, reviewed_at, review_note,
	created_by, created_at, updated_at`

func (s *Store) GetExpense(ctx context.Context, id int64) (domain.Expense, error) {
	e, err := scanExpense(s.db.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Expense{}, ErrNotFound
	}
	return e, err
}

// UpdateExpense applies an edit and returns the expense before and after.
// Any review decision is reset so the bookkeeper sees the corrected values;
// an edit that changes nothing leaves the row (and its review) untouched.
func (s *Store) UpdateExpense(ctx context.Context, id int64, in UpdateExpenseInput) (before, after domain.Expense, err error) {
	if in.AmountCents < 0 {
		return before, after, errors.New("amount must be non-negative")
	}
	cat := normalizeExpenseCategory(in.Category)
	replaceReceipt := len(in.ReceiptData) > 0
	if replaceReceipt && (in.ReceiptContentType == nil || strings.TrimSpace(*in.ReceiptContentType) == "") {
		return before, after, errors.New("receipt content type required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return before, after, err
	}
	defer func() { _ = tx.Rollback() }()

	before, err = scanExpense(tx.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return before, after, ErrNotFound
	}
	if err != nil {
		return before, after, err
	}
	reason, err := noReceiptReason(replaceReceipt || before.HasReceipt(), in.NoReceiptReason)
	if err != nil {
		return before, after, err
	}

	next := before
	next.AmountCents = in.AmountCents
	next.Currency = normalizeExpenseCurrency(in.Currency)
	next.Merchant = strings.TrimSpace(in.Merchant)
	next.Category = cat
	next.Note = strings.TrimSpace(in.Note)
	next.NoReceiptReason = reason
	next.SpentOn = in.SpentOn.UTC().Truncate(24 * time.Hour)
	if !replaceReceipt && sameEditableFields(before, next) {
		return before, before, nil
	}

	args := []any{
		id, next.AmountCents, next.Currency, next.Merchant, next.Category,
		next.Note, next.NoReceiptReason, next.SpentOn,
	}
	receiptSet := ""
	if replaceReceipt {
		args = append(args, in.ReceiptData, *in.ReceiptContentType, len(in.ReceiptData))
		receiptSet = `, receipt_data = $9, receipt_content_type = $10, receipt_bytes = $11`
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE expenses SET
			amount_cents = $2, currency = $3, merchant = $4, category = $5, note = $6,
			no_receipt_reason = $7, spent_on = $8::date`+receiptSet+`,
			review_status = 'submitted', reviewed_by = NULL, reviewed_at = NULL,
			updated_at = now()
		WHERE id = $1`, args...); err != nil {
		return before, after, expenseWriteErr(err)
	}
	after, err = scanExpense(tx.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = $1`, id))
	if err != nil {
		return before, after, err
	}
	if err := tx.Commit(); err != nil {
		return before, after, err
	}
	return before, after, nil
}

func sameEditableFields(a, b domain.Expense) bool {
	return a.AmountCents == b.AmountCents &&
		a.Currency == b.Currency &&
		a.Merchant == b.Merchant &&
		a.Category == b.Category &&
		a.Note == b.Note &&
		a.NoReceiptReason == b.NoReceiptReason &&
		a.SpentOn.Equal(b.SpentOn)
}

// SetExpenseReview records a bookkeeper decision and returns the expense before and after.
func (s *Store) SetExpenseReview(ctx context.Context, id int64, status domain.ExpenseReviewStatus, reviewer int64, note string) (before, after domain.Expense, err error) {
	if _, ok := domain.ParseExpenseReviewStatus(string(status)); !ok {
		return before, after, ErrReviewStatus
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return before, after, err
	}
	defer func() { _ = tx.Rollback() }()

	before, err = scanExpense(tx.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return before, after, ErrNotFound
	}
	if err != nil {
		return before, after, err
	}
	var reviewedBy any
	if status != domain.ExpenseSubmitted {
		reviewedBy = reviewer
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE expenses SET
			review_status = $2,
			reviewed_by = $3,
			reviewed_at = CASE WHEN $3::bigint IS NULL THEN NULL ELSE now() END,
			review_note = $4,
			updated_at = now()
		WHERE id = $1`, id, string(status), reviewedBy, strings.TrimSpace(note)); err != nil {
		return before, after, err
	}
	after, err = scanExpense(tx.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = $1`, id))
	if err != nil {
		return before, after, err
	}
	if err := tx.Commit(); err != nil {
		return before, after, err
	}
	return before, after, nil
}

// GetExpenseReceipt returns receipt bytes and content type for an expense.
func (s *Store) GetExpenseReceipt(ctx context.Context, id int64) (data []byte, contentType string, err error) {
	var ct sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT receipt_data, receipt_content_type
		FROM expenses WHERE id = $1`, id).Scan(&data, &ct)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", ErrNotFound
	}
	if ct.Valid {
		contentType = ct.String
	}
	return data, contentType, nil
}

type ExpenseFilter struct {
	HouseIDs     []int64
	ReviewStatus domain.ExpenseReviewStatus // empty = any
	Limit        int
}

func (s *Store) ListExpensesByHouses(ctx context.Context, houseIDs []int64, limit int) ([]domain.Expense, error) {
	return s.ListExpenses(ctx, ExpenseFilter{HouseIDs: houseIDs, Limit: limit})
}

func (s *Store) ListExpenses(ctx context.Context, f ExpenseFilter) ([]domain.Expense, error) {
	if len(f.HouseIDs) == 0 {
		return nil, nil
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	in, args := int64InClause(1, f.HouseIDs)
	where := `safe_house_id IN (` + in + `)`
	if f.ReviewStatus != "" {
		args = append(args, string(f.ReviewStatus))
		where += fmt.Sprintf(` AND review_status = $%d`, len(args))
	}
	args = append(args, limit)
	q := fmt.Sprintf(`SELECT %s FROM expenses WHERE %s ORDER BY spent_on DESC, id DESC LIMIT $%d`,
		expenseCols, where, len(args))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Expense
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteExpense removes an expense unless a reviewer has accepted it.
func (s *Store) DeleteExpense(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM expenses WHERE id = $1 AND review_status <> 'reviewed'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM expenses WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrExpenseReviewed
	}
	return ErrNotFound
}

// ExpenseTotalsByHouses aggregates expenses in [from, to] inclusive by house and currency.
func (s *Store) ExpenseTotalsByHouses(ctx context.Context, houseIDs []int64, from, to time.Time) ([]domain.ExpenseTotals, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	from = from.UTC().Truncate(24 * time.Hour)
	to = to.UTC().Truncate(24 * time.Hour)
	in, args := int64InClause(1, houseIDs)
	args = append(args, from, to)
	fromPH := fmt.Sprint(len(houseIDs) + 1)
	toPH := fmt.Sprint(len(houseIDs) + 2)
	q := `
		SELECT sh.id, sh.name,
			COUNT(e.id)::int,
			COUNT(e.receipt_data)::int,
			SUM(e.amount_cents)::bigint,
			e.currency
		FROM expenses e
		JOIN safe_houses sh ON sh.id = e.safe_house_id
		WHERE sh.id IN (` + in + `)
			AND e.spent_on >= $` + fromPH + `::date
			AND e.spent_on <= $` + toPH + `::date
		GROUP BY sh.id, sh.name, e.currency
		ORDER BY sh.name, e.currency`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExpenseTotals
	for rows.Next() {
		var t domain.ExpenseTotals
		if err := rows.Scan(&t.SafeHouseID, &t.SafeHouseName, &t.ExpenseCount, &t.ReceiptCount, &t.AmountCents, &t.Currency); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
