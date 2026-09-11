-- Phase 5: org management — active flags for RHLs and safe houses.

ALTER TABLE rhls
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE safe_houses
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
