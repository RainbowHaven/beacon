package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/magiconair/beacon/internal/domain"
)

func (s *Store) GetRHL(ctx context.Context, id int64) (domain.RHL, error) {
	var r domain.RHL
	err := s.db.QueryRowContext(ctx, `SELECT id, name, active FROM rhls WHERE id = $1`, id).
		Scan(&r.ID, &r.Name, &r.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RHL{}, ErrNotFound
	}
	return r, err
}

func (s *Store) CreateRHL(ctx context.Context, name string, active bool) (domain.RHL, error) {
	var r domain.RHL
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO rhls (name, active) VALUES ($1, $2)
		RETURNING id, name, active`, name, active).Scan(&r.ID, &r.Name, &r.Active)
	return r, err
}

func (s *Store) UpdateRHL(ctx context.Context, id int64, name string, active bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE rhls SET name = $2, active = $3 WHERE id = $1`, id, name, active)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSafeHouse(ctx context.Context, rhlID int64, name, currency string, active bool) (domain.SafeHouse, error) {
	var h domain.SafeHouse
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO safe_houses (rhl_id, name, default_currency, active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, rhl_id, name, default_currency, active`,
		rhlID, name, currency, active).
		Scan(&h.ID, &h.RHLID, &h.Name, &h.DefaultCurrency, &h.Active)
	return h, err
}

func (s *Store) UpdateSafeHouse(ctx context.Context, id, rhlID int64, name, currency string, active bool) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE safe_houses
		SET rhl_id = $2, name = $3, default_currency = $4, active = $5
		WHERE id = $1`, id, rhlID, name, currency, active)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type UpdateUserInput struct {
	DisplayName string
	Role        domain.Role
	RHLID       *int64
	SafeHouseID *int64
}

// UpdateUser changes role/scope without re-invite. Always revokes sessions so
// elevation/demotion and scope shrink take effect immediately.
func (s *Store) UpdateUser(ctx context.Context, id int64, in UpdateUserInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET display_name = $2, role = $3, rhl_id = $4, safe_house_id = $5, updated_at = now()
		WHERE id = $1`, id, in.DisplayName, in.Role, in.RHLID, in.SafeHouseID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	return tx.Commit()
}
