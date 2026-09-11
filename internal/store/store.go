package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrInviteInvalid = errors.New("invite invalid or expired")
	ErrUserLocked    = errors.New("user locked")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) DB() *sql.DB { return s.db }

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func newToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	raw := fmt.Sprintf("%x", b)
	return raw, hashToken(raw), nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) EnsureDemoTenancy(ctx context.Context) (domain.RHL, domain.SafeHouse, error) {
	var rhl domain.RHL
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM rhls ORDER BY created_at LIMIT 1`).Scan(&rhl.ID, &rhl.Name)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, `
			INSERT INTO rhls (name) VALUES ($1) RETURNING id, name`,
			"Rainbow Haven Local (Pilot)").Scan(&rhl.ID, &rhl.Name)
		if err != nil {
			return domain.RHL{}, domain.SafeHouse{}, err
		}
	} else if err != nil {
		return domain.RHL{}, domain.SafeHouse{}, err
	}

	var house domain.SafeHouse
	err = s.db.QueryRowContext(ctx, `SELECT id, rhl_id, name, default_currency FROM safe_houses WHERE rhl_id = $1 ORDER BY created_at LIMIT 1`, rhl.ID).
		Scan(&house.ID, &house.RHLID, &house.Name, &house.DefaultCurrency)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, `
			INSERT INTO safe_houses (rhl_id, name, default_currency) VALUES ($1, $2, $3)
			RETURNING id, rhl_id, name, default_currency`,
			rhl.ID, "Pilot Safe House", "CAD").Scan(&house.ID, &house.RHLID, &house.Name, &house.DefaultCurrency)
		if err != nil {
			return domain.RHL{}, domain.SafeHouse{}, err
		}
	} else if err != nil {
		return domain.RHL{}, domain.SafeHouse{}, err
	}
	return rhl, house, nil
}

func scanUser(row interface{ Scan(dest ...any) error }) (domain.User, error) {
	var u domain.User
	var rhl, house sql.NullInt64
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.Role, &rhl, &house, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	if rhl.Valid {
		id := rhl.Int64
		u.RHLID = &id
	}
	if house.Valid {
		id := house.Int64
		u.SafeHouseID = &id
	}
	return u, nil
}

const userCols = `id, email, display_name, status, role, rhl_id, safe_house_id, created_at, updated_at`

func (s *Store) GetUser(ctx context.Context, id int64) (domain.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) ListRHLs(ctx context.Context) ([]domain.RHL, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM rhls ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RHL
	for rows.Next() {
		var r domain.RHL
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListSafeHouses(ctx context.Context) ([]domain.SafeHouse, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, rhl_id, name, default_currency FROM safe_houses ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SafeHouse
	for rows.Next() {
		var h domain.SafeHouse
		if err := rows.Scan(&h.ID, &h.RHLID, &h.Name, &h.DefaultCurrency); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

type CreateUserInput struct {
	Email       string
	DisplayName string
	Role        domain.Role
	RHLID       *int64
	SafeHouseID *int64
}

func (s *Store) CreateUserWithInvite(ctx context.Context, in CreateUserInput, inviteTTL time.Duration) (domain.User, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, "", err
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (email, display_name, status, role, rhl_id, safe_house_id)
		VALUES (lower($1), $2, 'pending', $3, $4, $5)
		RETURNING id`,
		in.Email, in.DisplayName, in.Role, in.RHLID, in.SafeHouseID).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, "", ErrEmailTaken
		}
		return domain.User{}, "", err
	}

	raw, hash, err := newToken()
	if err != nil {
		return domain.User{}, "", err
	}
	expires := time.Now().UTC().Add(inviteTTL)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO invites (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		id, hash, expires); err != nil {
		return domain.User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return domain.User{}, "", err
	}
	u, err := s.GetUser(ctx, id)
	return u, raw, err
}

func (s *Store) ReissueInvite(ctx context.Context, userID int64, inviteTTL time.Duration) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(inviteTTL)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE invites SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return "", err
	}
	// Re-invite is the recovery path after lock or lost device: always return to pending.
	if _, err := tx.ExecContext(ctx, `UPDATE users SET status = 'pending', updated_at = now() WHERE id = $1`, userID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invites (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, hash, expires); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Store) LookupInvite(ctx context.Context, rawToken string) (domain.User, error) {
	hash := hashToken(rawToken)
	var userID int64
	var expires time.Time
	var used sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, expires_at, used_at FROM invites WHERE token_hash = $1`, hash).
		Scan(&userID, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ErrInviteInvalid
	}
	if err != nil {
		return domain.User{}, err
	}
	if used.Valid || time.Now().UTC().After(expires) {
		return domain.User{}, ErrInviteInvalid
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if u.Status == domain.UserLocked {
		return domain.User{}, ErrUserLocked
	}
	return u, nil
}

func (s *Store) MarkInviteUsed(ctx context.Context, rawToken string) error {
	hash := hashToken(rawToken)
	res, err := s.db.ExecContext(ctx, `
		UPDATE invites SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`, hash)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInviteInvalid
	}
	return nil
}

func (s *Store) ActivateUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET status = 'active', updated_at = now() WHERE id = $1`, userID)
	return err
}

func (s *Store) LockUser(ctx context.Context, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET status = 'locked', updated_at = now() WHERE id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(ttl)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, hash, expires)
	return raw, err
}

func (s *Store) UserBySession(ctx context.Context, rawToken string) (domain.User, error) {
	hash := hashToken(rawToken)
	var userID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id FROM sessions
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, hash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if u.Status == domain.UserLocked {
		return domain.User{}, ErrUserLocked
	}
	return u, nil
}

func (s *Store) RevokeSession(ctx context.Context, rawToken string) error {
	hash := hashToken(rawToken)
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	return err
}

func (s *Store) SaveWebAuthnChallenge(ctx context.Context, userID *int64, purpose string, data any, ttl time.Duration) (int64, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	expires := time.Now().UTC().Add(ttl)
	var id int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO webauthn_challenges (user_id, purpose, data, expires_at)
		VALUES ($1, $2, $3::jsonb, $4)
		RETURNING id`, userID, purpose, string(b), expires).Scan(&id)
	return id, err
}

func (s *Store) TakeWebAuthnChallenge(ctx context.Context, id int64, purpose string, dest any) (*int64, error) {
	var userID sql.NullInt64
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		DELETE FROM webauthn_challenges
		WHERE id = $1 AND purpose = $2 AND expires_at > now()
		RETURNING user_id, data`, id, purpose).Scan(&userID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return nil, err
	}
	if !userID.Valid {
		return nil, nil
	}
	uid := userID.Int64
	return &uid, nil
}

func (s *Store) Audit(ctx context.Context, actor *int64, action, subjectType, subjectID string, meta map[string]any) error {
	b, err := json.Marshal(meta)
	if err != nil {
		b = []byte(`{}`)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_events (actor_user_id, action, subject_type, subject_id, meta)
		VALUES ($1, $2, $3, $4, $5::jsonb)`, actor, action, subjectType, subjectID, string(b))
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
