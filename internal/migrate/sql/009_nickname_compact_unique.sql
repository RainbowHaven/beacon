-- Uniqueness ignores whitespace as well as case ("tom" == "T O M").
DROP INDEX IF EXISTS occupants_house_nickname_lower_uidx;

WITH dups AS (
    SELECT id,
           nickname || '-' || id::text AS new_nick,
           row_number() OVER (
               PARTITION BY safe_house_id, lower(regexp_replace(nickname, '\s+', '', 'g'))
               ORDER BY id
           ) AS rn
    FROM occupants
)
UPDATE occupants o
SET nickname = d.new_nick
FROM dups d
WHERE o.id = d.id AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS occupants_house_nickname_compact_uidx
    ON occupants (safe_house_id, (lower(regexp_replace(nickname, '\s+', '', 'g'))));
