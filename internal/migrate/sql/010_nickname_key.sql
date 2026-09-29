-- Store a Go-computed uniqueness key (accents + look-alikes folded).
-- Temporary values are compact lower(nickname); SyncNicknameKeys on boot rewrites them.

DROP INDEX IF EXISTS occupants_house_nickname_compact_uidx;

ALTER TABLE occupants
    ADD COLUMN IF NOT EXISTS nickname_key TEXT;

UPDATE occupants
SET nickname_key = lower(regexp_replace(nickname, '\s+', '', 'g'))
WHERE nickname_key IS NULL OR nickname_key = '';

-- Resolve any compact-key collisions before the unique index.
WITH dups AS (
    SELECT id,
           nickname_key || '-' || id::text AS new_key,
           row_number() OVER (
               PARTITION BY safe_house_id, nickname_key
               ORDER BY id
           ) AS rn
    FROM occupants
)
UPDATE occupants o
SET nickname_key = d.new_key
FROM dups d
WHERE o.id = d.id AND d.rn > 1;

ALTER TABLE occupants
    ALTER COLUMN nickname_key SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS occupants_house_nickname_key_uidx
    ON occupants (safe_house_id, nickname_key);
