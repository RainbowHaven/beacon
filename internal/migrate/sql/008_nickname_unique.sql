-- Permanent nickname uniqueness per safe house (case-insensitive).
-- Nicknames are stored normalized (trim + collapsed spaces) by the app.

UPDATE occupants
SET nickname = btrim(regexp_replace(nickname, '\s+', ' ', 'g'))
WHERE nickname IS DISTINCT FROM btrim(regexp_replace(nickname, '\s+', ' ', 'g'));

-- If duplicates remain after normalize, append id so the unique index can be created.
WITH dups AS (
    SELECT id,
           nickname || '-' || id::text AS new_nick,
           row_number() OVER (
               PARTITION BY safe_house_id, lower(nickname)
               ORDER BY id
           ) AS rn
    FROM occupants
)
UPDATE occupants o
SET nickname = d.new_nick
FROM dups d
WHERE o.id = d.id AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS occupants_house_nickname_lower_uidx
    ON occupants (safe_house_id, lower(nickname));
