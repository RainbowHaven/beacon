package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/magiconair/beacon/internal/domain"
)

var (
	ErrRHLCodeTaken   = errors.New("rhl code taken")
	ErrRHLCodeInvalid = errors.New("rhl code invalid")
)

const rhlCols = `id, name, COALESCE(code, ''), active`

func scanRHL(row interface{ Scan(dest ...any) error }) (domain.RHL, error) {
	var r domain.RHL
	err := row.Scan(&r.ID, &r.Name, &r.Code, &r.Active)
	return r, err
}

const safeHouseCols = `id, rhl_id, name, default_currency, approved_sleeping_places, active`

func scanSafeHouse(row interface{ Scan(dest ...any) error }) (domain.SafeHouse, error) {
	var h domain.SafeHouse
	var places sql.NullInt32
	if err := row.Scan(&h.ID, &h.RHLID, &h.Name, &h.DefaultCurrency, &places, &h.Active); err != nil {
		return domain.SafeHouse{}, err
	}
	if places.Valid {
		n := int(places.Int32)
		h.ApprovedSleepingPlaces = &n
	}
	return h, nil
}

// rhlCodeArg normalizes code and returns nil for "no code" so the column is NULL.
func rhlCodeArg(code string) (any, error) {
	c, ok := domain.NormalizeRHLCode(code)
	if !ok {
		return nil, ErrRHLCodeInvalid
	}
	if c == "" {
		return nil, nil
	}
	return c, nil
}

func (s *Store) GetRHL(ctx context.Context, id int64) (domain.RHL, error) {
	r, err := scanRHL(s.db.QueryRowContext(ctx, `SELECT `+rhlCols+` FROM rhls WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RHL{}, ErrNotFound
	}
	return r, err
}

func (s *Store) CreateRHL(ctx context.Context, name string, active bool) (domain.RHL, error) {
	return s.CreateRHLWithCode(ctx, name, "", active)
}

// CreateRHLWithCode stores an RHL with an optional code ("" for none).
func (s *Store) CreateRHLWithCode(ctx context.Context, name, code string, active bool) (domain.RHL, error) {
	codeArg, err := rhlCodeArg(code)
	if err != nil {
		return domain.RHL{}, err
	}
	r, err := scanRHL(s.db.QueryRowContext(ctx, `
		INSERT INTO rhls (name, code, active) VALUES ($1, $2, $3)
		RETURNING `+rhlCols, name, codeArg, active))
	if isUniqueViolation(err) {
		return domain.RHL{}, ErrRHLCodeTaken
	}
	return r, err
}

// UpdateRHL sets name, code ("" clears it) and active.
func (s *Store) UpdateRHL(ctx context.Context, id int64, name, code string, active bool) error {
	codeArg, err := rhlCodeArg(code)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE rhls SET name = $2, code = $3, active = $4 WHERE id = $1`, id, name, codeArg, active)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrRHLCodeTaken
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSafeHouse(ctx context.Context, rhlID int64, name, currency string, active bool) (domain.SafeHouse, error) {
	return s.CreateSafeHouseWithCapacity(ctx, rhlID, name, currency, nil, active)
}

// CreateSafeHouseWithCapacity stores a safe house with optional approved
// sleeping places (nil for not set).
func (s *Store) CreateSafeHouseWithCapacity(ctx context.Context, rhlID int64, name, currency string, sleepingPlaces *int, active bool) (domain.SafeHouse, error) {
	return scanSafeHouse(s.db.QueryRowContext(ctx, `
		INSERT INTO safe_houses (rhl_id, name, default_currency, approved_sleeping_places, active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+safeHouseCols,
		rhlID, name, currency, sleepingPlaces, active))
}

// UpdateSafeHouse sets all editable fields; nil sleepingPlaces clears the value.
func (s *Store) UpdateSafeHouse(ctx context.Context, id, rhlID int64, name, currency string, sleepingPlaces *int, active bool) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE safe_houses
		SET rhl_id = $2, name = $3, default_currency = $4, approved_sleeping_places = $5, active = $6
		WHERE id = $1`, id, rhlID, name, currency, sleepingPlaces, active)
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
