package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/passkeylabel"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var (
	ErrLastPasskey      = errors.New("cannot remove the last passkey")
	ErrCredentialExists = errors.New("passkey already registered")
)

func (s *Store) ListCredentials(ctx context.Context, userID int64) ([]webauthn.Credential, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, public_key, attestation_type, transport,
		       flag_user_present, flag_user_verified, flag_backup_eligible, flag_backup_state,
		       aaguid, sign_count
		FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []webauthn.Credential
	for rows.Next() {
		var c webauthn.Credential
		var transportJSON []byte
		var aaguid []byte
		if err := rows.Scan(
			&c.ID, &c.PublicKey, &c.AttestationType, &transportJSON,
			&c.Flags.UserPresent, &c.Flags.UserVerified, &c.Flags.BackupEligible, &c.Flags.BackupState,
			&aaguid, &c.Authenticator.SignCount,
		); err != nil {
			return nil, err
		}
		var transports []string
		if len(transportJSON) > 0 {
			_ = json.Unmarshal(transportJSON, &transports)
		}
		for _, t := range transports {
			c.Transport = append(c.Transport, protocol.AuthenticatorTransport(t))
		}
		c.Authenticator.AAGUID = aaguid
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SaveCredential(ctx context.Context, userID int64, c *webauthn.Credential) error {
	transports := make([]string, 0, len(c.Transport))
	for _, t := range c.Transport {
		transports = append(transports, string(t))
	}
	transportJSON, err := json.Marshal(transports)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO webauthn_credentials (
			id, user_id, public_key, attestation_type, transport,
			flag_user_present, flag_user_verified, flag_backup_eligible, flag_backup_state,
			aaguid, sign_count
		) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			sign_count = EXCLUDED.sign_count,
			flag_user_present = EXCLUDED.flag_user_present,
			flag_user_verified = EXCLUDED.flag_user_verified,
			flag_backup_eligible = EXCLUDED.flag_backup_eligible,
			flag_backup_state = EXCLUDED.flag_backup_state`,
		c.ID, userID, c.PublicKey, c.AttestationType, string(transportJSON),
		c.Flags.UserPresent, c.Flags.UserVerified, c.Flags.BackupEligible, c.Flags.BackupState,
		c.Authenticator.AAGUID, c.Authenticator.SignCount,
	)
	return err
}

// AddCredential stores a newly registered passkey. Unlike SaveCredential it
// never touches an existing row, so a credential ID cannot be claimed twice.
func (s *Store) AddCredential(ctx context.Context, userID int64, c *webauthn.Credential, label string) error {
	transports := make([]string, 0, len(c.Transport))
	for _, t := range c.Transport {
		transports = append(transports, string(t))
	}
	transportJSON, err := json.Marshal(transports)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO webauthn_credentials (
			id, user_id, public_key, attestation_type, transport,
			flag_user_present, flag_user_verified, flag_backup_eligible, flag_backup_state,
			aaguid, sign_count, label
		) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12)`,
		c.ID, userID, c.PublicKey, c.AttestationType, string(transportJSON),
		c.Flags.UserPresent, c.Flags.UserVerified, c.Flags.BackupEligible, c.Flags.BackupState,
		c.Authenticator.AAGUID, c.Authenticator.SignCount, label,
	)
	if isUniqueViolation(err) {
		return ErrCredentialExists
	}
	return err
}

func (s *Store) MarkCredentialUsed(ctx context.Context, userID int64, credID []byte) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE webauthn_credentials SET last_used_at = now() WHERE user_id = $1 AND id = $2`, userID, credID)
	return err
}

func (s *Store) ListPasskeys(ctx context.Context, userID int64) ([]domain.Passkey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, label, aaguid, transport, created_at, last_used_at
		FROM webauthn_credentials WHERE user_id = $1
		ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Passkey
	for rows.Next() {
		var p domain.Passkey
		var in passkeylabel.Input
		var transportJSON []byte
		var lastUsed sql.NullTime
		if err := rows.Scan(&p.ID, &p.Label, &in.AAGUID, &transportJSON, &p.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if p.Label == "" {
			if len(transportJSON) > 0 {
				_ = json.Unmarshal(transportJSON, &in.Transports)
			}
			p.Label = passkeylabel.Derive(in)
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			p.LastUsedAt = &t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CountPasskeysByUser(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, COUNT(*) FROM webauthn_credentials GROUP BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// DeleteOwnPasskey removes one of the user's passkeys, refusing to remove the
// last one, and signs out every other session of that user.
func (s *Store) DeleteOwnPasskey(ctx context.Context, userID int64, credID []byte, keepSessionToken string) (string, error) {
	return s.deletePasskey(ctx, userID, credID, false, hashToken(keepSessionToken))
}

// AdminDeletePasskey removes any passkey of the user (including the last one)
// and revokes all of that user's sessions.
func (s *Store) AdminDeletePasskey(ctx context.Context, userID int64, credID []byte) (string, error) {
	return s.deletePasskey(ctx, userID, credID, true, nil)
}

func (s *Store) deletePasskey(ctx context.Context, userID int64, credID []byte, allowLast bool, keepSessionHash []byte) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	// Row lock on the user serializes concurrent removals so the count check holds.
	if _, err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		return "", err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return "", err
	}
	var label string
	err = tx.QueryRowContext(ctx, `
		DELETE FROM webauthn_credentials WHERE user_id = $1 AND id = $2 RETURNING label`, userID, credID).Scan(&label)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if n <= 1 && !allowLast {
		return "", ErrLastPasskey
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL AND ($2::bytea IS NULL OR token_hash <> $2)`,
		userID, keepSessionHash); err != nil {
		return "", err
	}
	return label, tx.Commit()
}

func (s *Store) DeleteCredentialsForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE user_id = $1`, userID)
	return err
}
