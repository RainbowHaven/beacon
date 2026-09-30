-- Safe houses are small; more than 100 approved sleeping places is a typo.

ALTER TABLE safe_houses
    DROP CONSTRAINT IF EXISTS safe_houses_approved_sleeping_places_chk;

ALTER TABLE safe_houses
    ADD CONSTRAINT safe_houses_approved_sleeping_places_chk
    CHECK (approved_sleeping_places BETWEEN 1 AND 100);
