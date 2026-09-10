-- Phase 3b: per–safe-house default currency for expense entry.

ALTER TABLE safe_houses
    ADD COLUMN default_currency TEXT NOT NULL DEFAULT 'CAD';

ALTER TABLE safe_houses
    ADD CONSTRAINT safe_houses_default_currency_chk
    CHECK (char_length(default_currency) = 3);
