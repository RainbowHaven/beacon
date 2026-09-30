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

type CreateOccupantInput struct {
	SafeHouseID     int64
	Nickname        string
	ArrivedAt       time.Time
	CountryOfOrigin string
	Gender          string
	BirthYear       *int
	CreatedBy       *int64
}

type UpdateOccupantDemographicsInput struct {
	CountryOfOrigin string
	Gender          string
	BirthYear       *int
}

func (s *Store) CreateOccupant(ctx context.Context, in CreateOccupantInput) (domain.Occupant, error) {
	nick, key, err := PrepareNickname(in.Nickname)
	if err != nil {
		return domain.Occupant{}, err
	}
	arrived := in.ArrivedAt.UTC().Truncate(24 * time.Hour)
	if in.CountryOfOrigin == "" {
		in.CountryOfOrigin = "NR"
	}
	if in.Gender == "" {
		in.Gender = "NR"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Occupant{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := reserveNicknameKeyTx(ctx, tx, in.SafeHouseID, key); err != nil {
		return domain.Occupant{}, err
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO occupants (
			safe_house_id, nickname, nickname_key, arrived_at,
			country_of_origin, gender, birth_year, created_by
		) VALUES ($1, $2, $3, $4::date, $5, $6, $7, $8)
		RETURNING id`,
		in.SafeHouseID, nick, key, arrived,
		in.CountryOfOrigin, in.Gender, in.BirthYear, in.CreatedBy).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Occupant{}, ErrNicknameTaken
		}
		return domain.Occupant{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Occupant{}, err
	}
	return s.GetOccupant(ctx, id)
}

func (s *Store) RenameOccupant(ctx context.Context, id int64, nickname string) (domain.Occupant, error) {
	nick, key, err := PrepareNickname(nickname)
	if err != nil {
		return domain.Occupant{}, err
	}

	var houseID int64
	var oldKey string
	err = s.db.QueryRowContext(ctx, `
		SELECT safe_house_id, nickname_key FROM occupants WHERE id = $1`, id).
		Scan(&houseID, &oldKey)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Occupant{}, ErrNotFound
	}
	if err != nil {
		return domain.Occupant{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Occupant{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if key != oldKey {
		if err := reserveNicknameKeyTx(ctx, tx, houseID, key); err != nil {
			return domain.Occupant{}, err
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE occupants
		SET nickname = $2, nickname_key = $3, updated_at = now()
		WHERE id = $1`, id, nick, key)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Occupant{}, ErrNicknameTaken
		}
		return domain.Occupant{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.Occupant{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return domain.Occupant{}, err
	}
	return s.GetOccupant(ctx, id)
}

// SuggestNickname returns a free kebab-case alternative (stem, stem-2, stem-3, …)
// that is not permanently reserved for the house.
func (s *Store) SuggestNickname(ctx context.Context, houseID int64, desired string, _ int64) (string, error) {
	stem := NicknameSuggestionStem(desired)
	for n := 1; n < 1002; n++ {
		candidate := KebabSuggestion(stem, n)
		taken, err := s.nicknameKeyReserved(ctx, houseID, NicknameKey(candidate))
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", ErrNicknameTaken
}

func (s *Store) nicknameKeyReserved(ctx context.Context, houseID int64, key string) (bool, error) {
	if key == "" {
		return true, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM safehouse_nickname_keys
			WHERE safe_house_id = $1 AND nickname_key = $2
		)`, houseID, key).Scan(&exists)
	return exists, err
}

func reserveNicknameKeyTx(ctx context.Context, tx *sql.Tx, houseID int64, key string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO safehouse_nickname_keys (safe_house_id, nickname_key)
		VALUES ($1, $2)`, houseID, key)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrNicknameTaken
		}
		return err
	}
	return nil
}

// SyncNicknameKeys recomputes occupant nickname_key values and ensures every
// current key is permanently reserved for its house (idempotent).
func (s *Store) SyncNicknameKeys(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, safe_house_id, nickname, COALESCE(nickname_key, '')
		FROM occupants
		ORDER BY safe_house_id, id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		id, houseID int64
		nick, key   string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.houseID, &r.nick, &r.key); err != nil {
			return err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	used := map[int64]map[string]int64{} // house → key → id
	for _, r := range list {
		want := NicknameKey(r.nick)
		if want == "" {
			want = fmt.Sprintf("occupant-%d", r.id)
		}
		if used[r.houseID] == nil {
			used[r.houseID] = map[string]int64{}
		}
		if other, ok := used[r.houseID][want]; ok && other != r.id {
			want = fmt.Sprintf("%s-%d", want, r.id)
		}
		used[r.houseID][want] = r.id
		if want != r.key {
			if _, err := s.db.ExecContext(ctx, `
				UPDATE occupants SET nickname_key = $2, updated_at = now() WHERE id = $1`, r.id, want); err != nil {
				return err
			}
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO safehouse_nickname_keys (safe_house_id, nickname_key)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, r.houseID, want); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateOccupantDemographics(ctx context.Context, id int64, in UpdateOccupantDemographicsInput) (domain.Occupant, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE occupants
		SET country_of_origin = $2, gender = $3, birth_year = $4, updated_at = now()
		WHERE id = $1`, id, in.CountryOfOrigin, in.Gender, in.BirthYear)
	if err != nil {
		return domain.Occupant{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.Occupant{}, ErrNotFound
	}
	return s.GetOccupant(ctx, id)
}

func scanOccupant(row interface{ Scan(dest ...any) error }) (domain.Occupant, error) {
	var o domain.Occupant
	var departed sql.NullTime
	var createdBy sql.NullInt64
	var birthYear sql.NullInt64
	err := row.Scan(
		&o.ID, &o.SafeHouseID, &o.Nickname, &o.ArrivedAt, &departed,
		&o.CountryOfOrigin, &o.Gender, &birthYear,
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
	if birthYear.Valid {
		y := int(birthYear.Int64)
		o.BirthYear = &y
	}
	if createdBy.Valid {
		id := createdBy.Int64
		o.CreatedBy = &id
	}
	return o, nil
}

const occupantCols = `id, safe_house_id, nickname, arrived_at, departed_at, country_of_origin, gender, birth_year, created_by, created_at, updated_at`

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

// ListOccupantsInPeriod returns occupants whose stay touches [from, to]
// inclusive, including those who arrived or departed on either end.
func (s *Store) ListOccupantsInPeriod(ctx context.Context, houseIDs []int64, from, to time.Time) ([]domain.Occupant, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	from = from.UTC().Truncate(24 * time.Hour)
	to = to.UTC().Truncate(24 * time.Hour)
	in, args := int64InClause(1, houseIDs)
	args = append(args, from, to)
	q := fmt.Sprintf(`SELECT %s FROM occupants
		WHERE safe_house_id IN (%s)
			AND arrived_at <= $%d::date
			AND (departed_at IS NULL OR departed_at >= $%d::date)
		ORDER BY arrived_at, nickname`,
		occupantCols, in, len(houseIDs)+2, len(houseIDs)+1)
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
	h, err := scanSafeHouse(s.db.QueryRowContext(ctx, `SELECT `+safeHouseCols+` FROM safe_houses WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SafeHouse{}, ErrNotFound
	}
	return h, err
}

func (s *Store) ListSafeHousesByRHL(ctx context.Context, rhlID int64) ([]domain.SafeHouse, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+safeHouseCols+` FROM safe_houses WHERE rhl_id = $1 ORDER BY name`, rhlID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SafeHouse
	for rows.Next() {
		h, err := scanSafeHouse(rows)
		if err != nil {
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
