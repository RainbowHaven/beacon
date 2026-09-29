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

type CreateOccupantInput struct {
	SafeHouseID int64
	Nickname    string
	ArrivedAt   time.Time
	CreatedBy   *int64
}

func (s *Store) CreateOccupant(ctx context.Context, in CreateOccupantInput) (domain.Occupant, error) {
	nick := strings.TrimSpace(in.Nickname)
	if nick == "" {
		return domain.Occupant{}, errors.New("nickname required")
	}
	arrived := in.ArrivedAt.UTC().Truncate(24 * time.Hour)
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO occupants (
			safe_house_id, nickname, arrived_at, created_by
		) VALUES ($1, $2, $3::date, $4)
		RETURNING id`,
		in.SafeHouseID, nick, arrived, in.CreatedBy).Scan(&id)
	if err != nil {
		return domain.Occupant{}, err
	}
	return s.GetOccupant(ctx, id)
}

func scanOccupant(row interface{ Scan(dest ...any) error }) (domain.Occupant, error) {
	var o domain.Occupant
	var departed sql.NullTime
	var createdBy sql.NullInt64
	err := row.Scan(
		&o.ID, &o.SafeHouseID, &o.Nickname, &o.ArrivedAt, &departed,
		&createdBy, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return domain.Occupant{}, err
	}
	o.ArrivedAt = o.ArrivedAt.UTC().Truncate(24 * time.Hour)
	if departed.Valid {
		d := departed.Time.UTC().Truncate(24 * time.Hour)
		o.DepartedAt = &d
	}
	if createdBy.Valid {
		id := createdBy.Int64
		o.CreatedBy = &id
	}
	return o, nil
}

const occupantCols = `id, safe_house_id, nickname, arrived_at, departed_at, created_by, created_at, updated_at`

func (s *Store) GetOccupant(ctx context.Context, id int64) (domain.Occupant, error) {
	o, err := scanOccupant(s.db.QueryRowContext(ctx, `SELECT `+occupantCols+` FROM occupants WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Occupant{}, ErrNotFound
	}
	return o, err
}

func (s *Store) ListOccupantsByHouses(ctx context.Context, houseIDs []int64, currentOnly bool) ([]domain.Occupant, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	in, args := int64InClause(1, houseIDs)
	q := `SELECT ` + occupantCols + ` FROM occupants WHERE safe_house_id IN (` + in + `)`
	if currentOnly {
		q += ` AND (departed_at IS NULL OR departed_at > CURRENT_DATE)`
	}
	q += ` ORDER BY arrived_at DESC, nickname`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Occupant
	for rows.Next() {
		o, err := scanOccupant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) MarkOccupantDeparted(ctx context.Context, id int64, departedAt time.Time) error {
	day := departedAt.UTC().Truncate(24 * time.Hour)
	res, err := s.db.ExecContext(ctx, `
		UPDATE occupants
		SET departed_at = $2::date, updated_at = now()
		WHERE id = $1 AND departed_at IS NULL AND $2::date >= arrived_at`,
		id, day)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) HeadcountByHouses(ctx context.Context, houseIDs []int64) ([]domain.HeadcountRow, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	in, args := int64InClause(1, houseIDs)
	rows, err := s.db.QueryContext(ctx, `
		SELECT sh.id, sh.name,
			COALESCE(SUM(CASE WHEN o.id IS NOT NULL AND (o.departed_at IS NULL OR o.departed_at > CURRENT_DATE) THEN 1 ELSE 0 END), 0)::int
		FROM safe_houses sh
		LEFT JOIN occupants o ON o.safe_house_id = sh.id
		WHERE sh.id IN (`+in+`)
		GROUP BY sh.id, sh.name
		ORDER BY sh.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.HeadcountRow
	for rows.Next() {
		var r domain.HeadcountRow
		if err := rows.Scan(&r.SafeHouseID, &r.SafeHouseName, &r.Current); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetSafeHouse(ctx context.Context, id int64) (domain.SafeHouse, error) {
	var h domain.SafeHouse
	err := s.db.QueryRowContext(ctx, `SELECT id, rhl_id, name, default_currency, active FROM safe_houses WHERE id = $1`, id).
		Scan(&h.ID, &h.RHLID, &h.Name, &h.DefaultCurrency, &h.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SafeHouse{}, ErrNotFound
	}
	return h, err
}

func (s *Store) ListSafeHousesByRHL(ctx context.Context, rhlID int64) ([]domain.SafeHouse, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, rhl_id, name, default_currency, active FROM safe_houses WHERE rhl_id = $1 ORDER BY name`, rhlID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SafeHouse
	for rows.Next() {
		var h domain.SafeHouse
		if err := rows.Scan(&h.ID, &h.RHLID, &h.Name, &h.DefaultCurrency, &h.Active); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func int64InClause(start int, ids []int64) (string, []any) {
	parts := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("$%d", start+i)
		args[i] = id
	}
	return strings.Join(parts, ","), args
}
