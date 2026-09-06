package store

import (
	"context"
	"encoding/json"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

func (s *Store) ListCredentials(ctx context.Context, userID uuid.UUID) ([]webauthn.Credential, error) {
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

func (s *Store) SaveCredential(ctx context.Context, userID uuid.UUID, c *webauthn.Credential) error {
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

func (s *Store) DeleteCredentialsForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE user_id = $1`, userID)
	return err
}
