-- Permanent reservation of folded nickname keys per safe house.
-- Keys are never freed on rename or depart (occupants stay decoupled from history).

CREATE TABLE IF NOT EXISTS safehouse_nickname_keys (
    safe_house_id BIGINT NOT NULL REFERENCES safe_houses (id) ON DELETE CASCADE,
    nickname_key  TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (safe_house_id, nickname_key),
    CONSTRAINT safehouse_nickname_keys_key_chk CHECK (char_length(nickname_key) > 0)
);

INSERT INTO safehouse_nickname_keys (safe_house_id, nickname_key)
SELECT DISTINCT safe_house_id, nickname_key
FROM occupants
WHERE nickname_key <> ''
ON CONFLICT DO NOTHING;
