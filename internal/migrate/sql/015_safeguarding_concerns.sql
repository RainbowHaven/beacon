-- Safeguarding concerns: Document 37 incident identifier and basic tracking
-- only. No narrative or free-text columns. Rows are never deleted.

CREATE TABLE IF NOT EXISTS safeguarding_concerns (
    id BIGSERIAL PRIMARY KEY,
    safe_house_id BIGINT NOT NULL REFERENCES safe_houses(id),
    incident_id TEXT NOT NULL,
    occurred_on DATE,
    occurred_precision TEXT NOT NULL DEFAULT 'unknown',
    reported_on DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    closed_on DATE,
    created_by BIGINT REFERENCES users(id),
    updated_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT safeguarding_incident_id_chk
        CHECK (length(incident_id) BETWEEN 1 AND 40 AND incident_id !~ '[\r\n]' AND incident_id = btrim(incident_id)),
    CONSTRAINT safeguarding_occurred_precision_chk
        CHECK (occurred_precision IN ('exact', 'approximate', 'unknown')),
    CONSTRAINT safeguarding_occurred_on_chk
        CHECK ((occurred_precision = 'unknown') = (occurred_on IS NULL)),
    CONSTRAINT safeguarding_status_chk
        CHECK (status IN ('open', 'resolved', 'closed')),
    CONSTRAINT safeguarding_closed_on_chk
        CHECK ((status = 'open') = (closed_on IS NULL) AND (closed_on IS NULL OR closed_on >= reported_on))
);

CREATE UNIQUE INDEX IF NOT EXISTS safeguarding_concerns_incident_id_key
    ON safeguarding_concerns (lower(incident_id));

CREATE INDEX IF NOT EXISTS safeguarding_concerns_house_idx
    ON safeguarding_concerns (safe_house_id, status, reported_on);
