-- Occupant demographics for reporting (requirements §2.1).
-- No legal names or government IDs.

ALTER TABLE occupants
    ADD COLUMN IF NOT EXISTS country_of_origin TEXT NOT NULL DEFAULT 'NR',
    ADD COLUMN IF NOT EXISTS gender TEXT NOT NULL DEFAULT 'NR',
    ADD COLUMN IF NOT EXISTS birth_year INT;

ALTER TABLE occupants
    DROP CONSTRAINT IF EXISTS occupants_gender_chk,
    DROP CONSTRAINT IF EXISTS occupants_country_chk,
    DROP CONSTRAINT IF EXISTS occupants_birth_year_chk;

ALTER TABLE occupants
    ADD CONSTRAINT occupants_gender_chk CHECK (gender IN ('M', 'F', 'X', 'NR')),
    ADD CONSTRAINT occupants_country_chk CHECK (char_length(trim(country_of_origin)) > 0),
    ADD CONSTRAINT occupants_birth_year_chk CHECK (birth_year IS NULL OR (birth_year >= 1900 AND birth_year <= 2100));
