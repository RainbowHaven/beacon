package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/magiconair/beacon/internal/domain"
)

func (s *Store) ListAuditEvents(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.actor_user_id, COALESCE(u.email, ''), a.action,
			COALESCE(a.subject_type, ''), COALESCE(a.subject_id, ''), a.meta, a.created_at
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_user_id
		ORDER BY a.id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		var actor sql.NullInt64
		var meta []byte
		if err := rows.Scan(&e.ID, &actor, &e.ActorEmail, &e.Action, &e.SubjectType, &e.SubjectID, &meta, &e.CreatedAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			id := actor.Int64
			e.ActorUserID = &id
		}
		e.Meta = map[string]any{}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &e.Meta)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListAuditEventsForSubject returns events for one subject, oldest first.
func (s *Store) ListAuditEventsForSubject(ctx context.Context, subjectType, subjectID string, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.actor_user_id, COALESCE(u.email, ''), COALESCE(u.display_name, ''), a.action,
			COALESCE(a.subject_type, ''), COALESCE(a.subject_id, ''), a.meta, a.created_at
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.subject_type = $1 AND a.subject_id = $2
		ORDER BY a.id
		LIMIT $3`, subjectType, subjectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		var actor sql.NullInt64
		var meta []byte
		if err := rows.Scan(&e.ID, &actor, &e.ActorEmail, &e.ActorName, &e.Action, &e.SubjectType, &e.SubjectID, &meta, &e.CreatedAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			id := actor.Int64
			e.ActorUserID = &id
		}
		e.Meta = map[string]any{}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &e.Meta)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
