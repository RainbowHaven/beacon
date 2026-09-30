package store

import (
	"context"
	"errors"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// ExpenseCategories returns every expense category, active or not, in display order.
func (s *Store) ExpenseCategories(ctx context.Context) (domain.Categories, error) {
	return s.listCategories(ctx, `SELECT key, label, sort_order, active FROM expense_categories ORDER BY sort_order, key`)
}

// OperationalIssueCategories returns every operational issue category, active or not, in display order.
func (s *Store) OperationalIssueCategories(ctx context.Context) (domain.Categories, error) {
	return s.listCategories(ctx, `SELECT key, label, sort_order, active FROM operational_issue_categories ORDER BY sort_order, key`)
}

func (s *Store) listCategories(ctx context.Context, q string) (domain.Categories, error) {
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out domain.Categories
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.Key, &c.Label, &c.SortOrder, &c.Active); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func isForeignKeyViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == constraint
}
