package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/RainbowHaven/beacon/internal/domain"
)

// ValidationError carries a plain-English message that is safe to show in a form.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

const (
	maxOperationalIssueDescription = 2000
	maxOperationalIssueText        = 4000
)

// OperationalIssueFields are the editable fields of an operational issue.
type OperationalIssueFields struct {
	IdentifiedOn time.Time
	Category     string
	Description  string
	Effect       string
	ActionTaken  string
	RHLRequest   string
	Status       string
	ClosedOn     *time.Time
	ClosureNotes string
}

// NormalizeOperationalIssueFields trims text, truncates dates to UTC days and
// checks the closure rules. An open issue has no closure date or notes.
func NormalizeOperationalIssueFields(f OperationalIssueFields) (OperationalIssueFields, error) {
	f.Category = strings.TrimSpace(f.Category)
	f.Description = strings.TrimSpace(f.Description)
	f.Effect = strings.TrimSpace(f.Effect)
	f.ActionTaken = strings.TrimSpace(f.ActionTaken)
	f.RHLRequest = strings.TrimSpace(f.RHLRequest)
	f.Status = strings.TrimSpace(f.Status)
	f.ClosureNotes = strings.TrimSpace(f.ClosureNotes)
	if f.Status == "" {
		f.Status = domain.OperationalIssueOpen
	}

	if f.IdentifiedOn.IsZero() {
		return f, ValidationError("Date identified is required.")
	}
	f.IdentifiedOn = f.IdentifiedOn.UTC().Truncate(24 * time.Hour)
	if !domain.ValidOperationalIssueCategory(f.Category) {
		return f, ValidationError("Choose a category.")
	}
	if f.Description == "" {
		return f, ValidationError("Brief description is required.")
	}
	if utf8.RuneCountInString(f.Description) > maxOperationalIssueDescription {
		return f, ValidationError(fmt.Sprintf("Brief description must be at most %d characters.", maxOperationalIssueDescription))
	}
	for _, t := range []string{f.Effect, f.ActionTaken, f.RHLRequest, f.ClosureNotes} {
		if utf8.RuneCountInString(t) > maxOperationalIssueText {
			return f, ValidationError(fmt.Sprintf("Text fields must be at most %d characters.", maxOperationalIssueText))
		}
	}
	if !domain.ValidOperationalIssueStatus(f.Status) {
		return f, ValidationError("Invalid status.")
	}

	if f.Status == domain.OperationalIssueOpen {
		f.ClosedOn = nil
		f.ClosureNotes = ""
		return f, nil
	}
	if f.ClosedOn == nil || f.ClosedOn.IsZero() {
		return f, ValidationError("Resolution or closure date is required when the status is resolved or closed.")
	}
	d := f.ClosedOn.UTC().Truncate(24 * time.Hour)
	f.ClosedOn = &d
	if d.Before(f.IdentifiedOn) {
		return f, ValidationError("Resolution or closure date cannot be before the date identified.")
	}
	if f.ClosureNotes == "" {
		return f, ValidationError("Resolution or closure notes are required when the status is resolved or closed.")
	}
	return f, nil
}

func (s *Store) CreateOperationalIssue(ctx context.Context, safeHouseID int64, f OperationalIssueFields, by *int64) (domain.OperationalIssue, error) {
	f, err := NormalizeOperationalIssueFields(f)
	if err != nil {
		return domain.OperationalIssue{}, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO operational_issues (
			safe_house_id, identified_on, category, description, effect, action_taken,
			rhl_request, status, closed_on, closure_notes, created_by, updated_by
		) VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9::date, $10, $11, $11)
		RETURNING id`,
		safeHouseID, f.IdentifiedOn, f.Category, f.Description, f.Effect, f.ActionTaken,
		f.RHLRequest, f.Status, f.ClosedOn, f.ClosureNotes, by,
	).Scan(&id)
	if err != nil {
		return domain.OperationalIssue{}, err
	}
	return s.GetOperationalIssue(ctx, id)
}

// UpdateOperationalIssue replaces the editable fields. The safe house cannot change.
func (s *Store) UpdateOperationalIssue(ctx context.Context, id int64, f OperationalIssueFields, by *int64) (domain.OperationalIssue, error) {
	f, err := NormalizeOperationalIssueFields(f)
	if err != nil {
		return domain.OperationalIssue{}, err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE operational_issues SET
			identified_on = $2::date, category = $3, description = $4, effect = $5,
			action_taken = $6, rhl_request = $7, status = $8, closed_on = $9::date,
			closure_notes = $10, updated_by = $11, updated_at = now()
		WHERE id = $1`,
		id, f.IdentifiedOn, f.Category, f.Description, f.Effect,
		f.ActionTaken, f.RHLRequest, f.Status, f.ClosedOn,
		f.ClosureNotes, by,
	)
	if err != nil {
		return domain.OperationalIssue{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.OperationalIssue{}, ErrNotFound
	}
	return s.GetOperationalIssue(ctx, id)
}

const operationalIssueCols = `oi.id, oi.safe_house_id, sh.name, oi.identified_on, oi.category, oi.description,
	oi.effect, oi.action_taken, oi.rhl_request, oi.status, oi.closed_on, oi.closure_notes,
	oi.created_by, oi.updated_by, oi.created_at, oi.updated_at`

const operationalIssueFrom = ` FROM operational_issues oi JOIN safe_houses sh ON sh.id = oi.safe_house_id`

func scanOperationalIssue(row interface{ Scan(dest ...any) error }) (domain.OperationalIssue, error) {
	var o domain.OperationalIssue
	var closedOn sql.NullTime
	var createdBy, updatedBy sql.NullInt64
	err := row.Scan(
		&o.ID, &o.SafeHouseID, &o.SafeHouseName, &o.IdentifiedOn, &o.Category, &o.Description,
		&o.Effect, &o.ActionTaken, &o.RHLRequest, &o.Status, &closedOn, &o.ClosureNotes,
		&createdBy, &updatedBy, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return domain.OperationalIssue{}, err
	}
	o.IdentifiedOn = o.IdentifiedOn.UTC().Truncate(24 * time.Hour)
	if closedOn.Valid {
		d := closedOn.Time.UTC().Truncate(24 * time.Hour)
		o.ClosedOn = &d
	}
	if createdBy.Valid {
		v := createdBy.Int64
		o.CreatedBy = &v
	}
	if updatedBy.Valid {
		v := updatedBy.Int64
		o.UpdatedBy = &v
	}
	return o, nil
}

func (s *Store) GetOperationalIssue(ctx context.Context, id int64) (domain.OperationalIssue, error) {
	o, err := scanOperationalIssue(s.db.QueryRowContext(ctx,
		`SELECT `+operationalIssueCols+operationalIssueFrom+` WHERE oi.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OperationalIssue{}, ErrNotFound
	}
	return o, err
}

// ListOperationalIssuesByHouses lists issues newest first. An empty status lists all.
func (s *Store) ListOperationalIssuesByHouses(ctx context.Context, houseIDs []int64, status string) ([]domain.OperationalIssue, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	in, args := int64InClause(1, houseIDs)
	q := `SELECT ` + operationalIssueCols + operationalIssueFrom + ` WHERE oi.safe_house_id IN (` + in + `)`
	if status != "" {
		args = append(args, status)
		q += fmt.Sprintf(` AND oi.status = $%d`, len(args))
	}
	q += ` ORDER BY oi.identified_on DESC, oi.id DESC`
	return s.queryOperationalIssues(ctx, q, args...)
}

// OperationalIssuesActiveInRange returns issues that were open during any part of
// [start, end] (inclusive dates), ordered by house name, date identified and id.
func (s *Store) OperationalIssuesActiveInRange(ctx context.Context, houseIDs []int64, start, end time.Time) ([]domain.OperationalIssue, error) {
	if len(houseIDs) == 0 {
		return nil, nil
	}
	start = start.UTC().Truncate(24 * time.Hour)
	end = end.UTC().Truncate(24 * time.Hour)
	in, args := int64InClause(1, houseIDs)
	args = append(args, start, end)
	q := fmt.Sprintf(`SELECT %s%s
		WHERE oi.safe_house_id IN (%s)
			AND oi.identified_on <= $%d::date
			AND (oi.closed_on IS NULL OR oi.closed_on >= $%d::date)
		ORDER BY sh.name, oi.identified_on, oi.id`,
		operationalIssueCols, operationalIssueFrom, in, len(args), len(args)-1)
	return s.queryOperationalIssues(ctx, q, args...)
}

func (s *Store) queryOperationalIssues(ctx context.Context, q string, args ...any) ([]domain.OperationalIssue, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OperationalIssue
	for rows.Next() {
		o, err := scanOperationalIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
