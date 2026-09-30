package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

var (
	ErrIncidentIDTaken   = errors.New("incident identifier taken")
	ErrIncidentIDInvalid = errors.New("incident identifier invalid")
)

// SafeguardingInput holds the editable fields of a safeguarding concern.
// OccurredOn must be nil when OccurredPrecision is unknown, and ClosedOn must
// be set exactly when Status is not open.
type SafeguardingInput struct {
	IncidentID        string
	OccurredOn        *time.Time
	OccurredPrecision domain.DatePrecision
	ReportedOn        time.Time
	Status            domain.SafeguardingStatus
	ClosedOn          *time.Time
}

const safeguardingCols = `id, safe_house_id, incident_id, occurred_on, occurred_precision, reported_on,
	status, closed_on, created_by, updated_by, created_at, updated_at`

func scanSafeguardingConcern(row interface{ Scan(dest ...any) error }) (domain.SafeguardingConcern, error) {
	var c domain.SafeguardingConcern
	var occurred, closed sql.NullTime
	var createdBy, updatedBy sql.NullInt64
	err := row.Scan(&c.ID, &c.SafeHouseID, &c.IncidentID, &occurred, &c.OccurredPrecision, &c.ReportedOn,
		&c.Status, &closed, &createdBy, &updatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return domain.SafeguardingConcern{}, err
	}
	c.ReportedOn = c.ReportedOn.UTC().Truncate(24 * time.Hour)
	if occurred.Valid {
		d := occurred.Time.UTC().Truncate(24 * time.Hour)
		c.OccurredOn = &d
	}
	if closed.Valid {
		d := closed.Time.UTC().Truncate(24 * time.Hour)
		c.ClosedOn = &d
	}
	if createdBy.Valid {
		v := createdBy.Int64
		c.CreatedBy = &v
	}
	if updatedBy.Valid {
		v := updatedBy.Int64
		c.UpdatedBy = &v
	}
	return c, nil
}

func dateArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Truncate(24 * time.Hour)
}

func (in SafeguardingInput) args() ([]any, error) {
	id, ok := domain.NormalizeIncidentID(in.IncidentID)
	if !ok {
		return nil, ErrIncidentIDInvalid
	}
	return []any{
		id, dateArg(in.OccurredOn), string(in.OccurredPrecision),
		in.ReportedOn.UTC().Truncate(24 * time.Hour), string(in.Status), dateArg(in.ClosedOn),
	}, nil
}

func (s *Store) CreateSafeguardingConcern(ctx context.Context, houseID int64, in SafeguardingInput, createdBy *int64) (domain.SafeguardingConcern, error) {
	args, err := in.args()
	if err != nil {
		return domain.SafeguardingConcern{}, err
	}
	args = append([]any{houseID}, args...)
	args = append(args, createdBy)
	c, err := scanSafeguardingConcern(s.db.QueryRowContext(ctx, `
		INSERT INTO safeguarding_concerns (
			safe_house_id, incident_id, occurred_on, occurred_precision, reported_on,
			status, closed_on, created_by, updated_by
		) VALUES ($1, $2, $3::date, $4, $5::date, $6, $7::date, $8, $8)
		RETURNING `+safeguardingCols, args...))
	if isUniqueViolation(err) {
		return domain.SafeguardingConcern{}, ErrIncidentIDTaken
	}
	return c, err
}

// UpdateSafeguardingConcern replaces the editable fields. The safe house of a
// concern never changes.
func (s *Store) UpdateSafeguardingConcern(ctx context.Context, id int64, in SafeguardingInput, updatedBy *int64) (domain.SafeguardingConcern, error) {
	args, err := in.args()
	if err != nil {
		return domain.SafeguardingConcern{}, err
	}
	args = append([]any{id}, args...)
	args = append(args, updatedBy)
	c, err := scanSafeguardingConcern(s.db.QueryRowContext(ctx, `
		UPDATE safeguarding_concerns
		SET incident_id = $2, occurred_on = $3::date, occurred_precision = $4, reported_on = $5::date,
			status = $6, closed_on = $7::date, updated_by = $8, updated_at = now()
		WHERE id = $1
		RETURNING `+safeguardingCols, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SafeguardingConcern{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return domain.SafeguardingConcern{}, ErrIncidentIDTaken
	}
	return c, err
}

func (s *Store) GetSafeguardingConcern(ctx context.Context, id int64) (domain.SafeguardingConcern, error) {
	c, err := scanSafeguardingConcern(s.db.QueryRowContext(ctx, `SELECT `+safeguardingCols+` FROM safeguarding_concerns WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SafeguardingConcern{}, ErrNotFound
	}
	return c, err
}

func (s *Store) querySafeguarding(ctx context.Context, q string, args ...any) ([]domain.SafeguardingConcern, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SafeguardingConcern
	for rows.Next() {
		c, err := scanSafeguardingConcern(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListSafeguardingConcerns returns concerns for the houses, newest report
// first. An empty status list means all statuses.
func (s *Store) ListSafeguardingConcerns(ctx context.Context, houseIDs []int64, statuses []domain.SafeguardingStatus) ([]domain.SafeguardingConcern, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	in, args := int64InClause(1, houseIDs)
	q := `SELECT ` + safeguardingCols + ` FROM safeguarding_concerns WHERE safe_house_id IN (` + in + `)`
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		for i, st := range statuses {
			args = append(args, string(st))
			ph[i] = fmt.Sprintf("$%d", len(args))
		}
		q += ` AND status IN (` + strings.Join(ph, ",") + `)`
	}
	q += ` ORDER BY reported_on DESC, id DESC`
	return s.querySafeguarding(ctx, q, args...)
}

// SafeguardingConcernsForMonth returns concerns that were reported on or
// before monthEnd and not closed or resolved before monthStart, so unresolved
// concerns appear in every month until they are closed. Ordered by reported
// date, then id.
func (s *Store) SafeguardingConcernsForMonth(ctx context.Context, houseIDs []int64, monthStart, monthEnd time.Time) ([]domain.SafeguardingConcern, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	in, args := int64InClause(1, houseIDs)
	args = append(args, monthStart.UTC().Truncate(24*time.Hour), monthEnd.UTC().Truncate(24*time.Hour))
	n := len(houseIDs)
	q := fmt.Sprintf(`SELECT %s FROM safeguarding_concerns
		WHERE safe_house_id IN (%s)
			AND reported_on <= $%d::date
			AND (closed_on IS NULL OR closed_on >= $%d::date)
		ORDER BY reported_on, id`, safeguardingCols, in, n+2, n+1)
	return s.querySafeguarding(ctx, q, args...)
}

// CountSafeguardingConcernsReported counts concerns whose reported date is in
// [from, to] inclusive.
func (s *Store) CountSafeguardingConcernsReported(ctx context.Context, houseIDs []int64, from, to time.Time) (int, error) {
	if len(houseIDs) == 0 {
		return 0, nil
	}
	in, args := int64InClause(1, houseIDs)
	args = append(args, from.UTC().Truncate(24*time.Hour), to.UTC().Truncate(24*time.Hour))
	n := len(houseIDs)
	var count int
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*)::int FROM safeguarding_concerns
		WHERE safe_house_id IN (%s) AND reported_on >= $%d::date AND reported_on <= $%d::date`,
		in, n+1, n+2), args...).Scan(&count)
	return count, err
}

// NextIncidentSequence returns one more than the highest sequence number used
// in identifiers of the form Incident-<rhlCode>-<year>-<n>, or 1 if none.
func (s *Store) NextIncidentSequence(ctx context.Context, rhlCode string, year int) (int, error) {
	pattern := fmt.Sprintf(`^Incident-%s-%d-([0-9]{1,9})$`, regexp.QuoteMeta(rhlCode), year)
	var maxSeq int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX((regexp_match(incident_id, $1, 'i'))[1]::int), 0)
		FROM safeguarding_concerns WHERE incident_id ~* $1`, pattern).Scan(&maxSeq)
	if err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
}
