package store

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
)

var ErrEmailInvalid = errors.New("email invalid")

// NormalizeEmail trims and lower-cases a bare address ("a@b.org", not "Name <a@b.org>").
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", ErrEmailInvalid
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return "", ErrEmailInvalid
	}
	at := strings.LastIndex(email, "@")
	if at < 1 || !strings.Contains(email[at+1:], ".") {
		return "", ErrEmailInvalid
	}
	return email, nil
}

// UpdateUserEmail changes the sign-in email and returns the previous one.
// Passkeys stay valid: WebAuthn identifies the user by ID, not by email.
func (s *Store) UpdateUserEmail(ctx context.Context, id int64, raw string) (string, error) {
	email, err := NormalizeEmail(raw)
	if err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var old string
	err = tx.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var taken bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $2 AND id <> $1)`, id, email).Scan(&taken); err != nil {
		return "", err
	}
	if taken {
		return "", ErrEmailTaken
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET email = $2, updated_at = now() WHERE id = $1`, id, email); err != nil {
		if isUniqueViolation(err) {
			return "", ErrEmailTaken
		}
		return "", err
	}
	return old, tx.Commit()
}
