-- Passkey management: user-chosen labels and last sign-in time per credential.

ALTER TABLE webauthn_credentials
    ADD COLUMN label TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_used_at TIMESTAMPTZ;
