-- Phase 2: occupants with sealed identity vault fields.

CREATE TABLE occupants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    safe_house_id UUID NOT NULL REFERENCES safe_houses (id) ON DELETE CASCADE,
    nickname TEXT NOT NULL,
    arrived_at DATE NOT NULL,
    departed_at DATE,
    identity_ciphertext BYTEA NOT NULL,
    key_id TEXT NOT NULL,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT occupants_nickname_chk CHECK (char_length(trim(nickname)) > 0),
    CONSTRAINT occupants_depart_chk CHECK (departed_at IS NULL OR departed_at >= arrived_at),
    CONSTRAINT occupants_ciphertext_chk CHECK (octet_length(identity_ciphertext) >= 48)
);

CREATE INDEX occupants_safe_house_id_idx ON occupants (safe_house_id);
CREATE INDEX occupants_current_idx ON occupants (safe_house_id) WHERE departed_at IS NULL;
