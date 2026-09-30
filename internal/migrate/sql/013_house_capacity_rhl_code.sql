-- Document 50 reporting: approved sleeping places per safe house and a short
-- code per RHL (report header and incident IDs).

ALTER TABLE safe_houses
    ADD COLUMN IF NOT EXISTS approved_sleeping_places INT;

ALTER TABLE safe_houses
    DROP CONSTRAINT IF EXISTS safe_houses_approved_sleeping_places_chk;

ALTER TABLE safe_houses
    ADD CONSTRAINT safe_houses_approved_sleeping_places_chk CHECK (approved_sleeping_places > 0);

ALTER TABLE rhls
    ADD COLUMN IF NOT EXISTS code TEXT;

ALTER TABLE rhls
    DROP CONSTRAINT IF EXISTS rhls_code_chk;

ALTER TABLE rhls
    ADD CONSTRAINT rhls_code_chk CHECK (code ~ '^[A-Z0-9-]{2,12}$');

CREATE UNIQUE INDEX IF NOT EXISTS rhls_code_key ON rhls (code) WHERE code IS NOT NULL;
