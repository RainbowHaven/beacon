package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

type CreateExpenseInput struct {
	SafeHouseID        int64
	AmountCents        int64
	Currency           string
	Note               string
	SpentOn            time.Time
	ReceiptKey         *string
	ReceiptContentType *string
	ReceiptBytes       *int
	CreatedBy          *int64
}

func (s *Store) CreateExpense(ctx context.Context, in CreateExpenseInput) (domain.Expense, error) {
	if in.AmountCents < 0 {
		return domain.Expense{}, errors.New("amount must be non-negative")
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	if cur == "" {
		cur = "CAD"
	}
	spent := in.SpentOn.UTC().Truncate(24 * time.Hour)
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO expenses (
			safe_house_id, amount_cents, currency, note, spent_on,
			receipt_key, receipt_content_type, receipt_bytes, created_by
		) VALUES ($1,$2,$3,$4,$5::date,$6,$7,$8,$9)
		RETURNING id`,
		in.SafeHouseID, in.AmountCents, cur, strings.TrimSpace(in.Note), spent,
		in.ReceiptKey, in.ReceiptContentType, in.ReceiptBytes, in.CreatedBy,
	).Scan(&id)
	if err != nil {
		return domain.Expense{}, err
	}
	return s.GetExpense(ctx, id)
}

func scanExpense(row interface{ Scan(dest ...any) error }) (domain.Expense, error) {
	var e domain.Expense
	var receiptKey, receiptCT sql.NullString
	var receiptBytes sql.NullInt64
	var createdBy sql.NullInt64
	err := row.Scan(
		&e.ID, &e.SafeHouseID, &e.AmountCents, &e.Currency, &e.Note, &e.SpentOn,
		&receiptKey, &receiptCT, &receiptBytes, &createdBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return domain.Expense{}, err
	}
	e.SpentOn = e.SpentOn.UTC().Truncate(24 * time.Hour)
	if receiptKey.Valid {
		v := receiptKey.String
		e.ReceiptKey = &v
	}
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
	return e, nil
}

const expenseCols = `id, safe_house_id, amount_cents, currency, note, spent_on, receipt_key, receipt_content_type, receipt_bytes, created_by, created_at, updated_at`

func (s *Store) GetExpense(ctx context.Context, id int64) (domain.Expense, error) {
	e, err := scanExpense(s.db.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Expense{}, ErrNotFound
	}
	return e, err
}

func (s *Store) ListExpensesByHouses(ctx context.Context, houseIDs []int64, limit int) ([]domain.Expense, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	in, args := int64InClause(1, houseIDs)
	args = append(args, limit)
	q := fmt.Sprintf(`SELECT %s FROM expenses WHERE safe_house_id IN (%s) ORDER BY spent_on DESC, id DESC LIMIT $%d`,
		expenseCols, in, len(args))
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

func (s *Store) DeleteExpense(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM expenses WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ExpenseTotalsByHouses aggregates expenses in [from, to] inclusive by house.
func (s *Store) ExpenseTotalsByHouses(ctx context.Context, houseIDs []int64, from, to time.Time) ([]domain.ExpenseTotals, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	from = from.UTC().Truncate(24 * time.Hour)
	to = to.UTC().Truncate(24 * time.Hour)
	in, args := int64InClause(1, houseIDs)
	args = append(args, from, to)
	q := `
		SELECT sh.id, sh.name,
			COALESCE(COUNT(e.id), 0)::int,
			COALESCE(COUNT(e.receipt_key), 0)::int,
			COALESCE(SUM(e.amount_cents), 0)::bigint,
			COALESCE(MAX(e.currency), 'CAD')
		FROM safe_houses sh
		LEFT JOIN expenses e ON e.safe_house_id = sh.id
			AND e.spent_on >= $` + fmt.Sprint(len(houseIDs)+1) + `::date
			AND e.spent_on <= $` + fmt.Sprint(len(houseIDs)+2) + `::date
		WHERE sh.id IN (` + in + `)
		GROUP BY sh.id, sh.name
		ORDER BY sh.name`
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
