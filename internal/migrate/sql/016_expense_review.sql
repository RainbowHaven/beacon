-- Expense entry refinement: merchant, category, no-receipt explanation, bookkeeper review.

ALTER TABLE expenses
    ADD COLUMN IF NOT EXISTS merchant TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other',
    ADD COLUMN IF NOT EXISTS no_receipt_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS review_status TEXT NOT NULL DEFAULT 'submitted',
    ADD COLUMN IF NOT EXISTS reviewed_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS review_note TEXT NOT NULL DEFAULT '';

ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_category_chk;
ALTER TABLE expenses ADD CONSTRAINT expenses_category_chk CHECK (
    category IN (
        'food', 'household_supplies', 'utilities', 'rent',
        'maintenance_repairs', 'transport', 'communication', 'other'
    )
);

ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_review_status_chk;
ALTER TABLE expenses ADD CONSTRAINT expenses_review_status_chk CHECK (
    review_status IN ('submitted', 'reviewed', 'needs_correction')
);

-- Rows logged before this migration may lack an explanation; the app requires one
-- for new and edited expenses without a receipt.

CREATE INDEX IF NOT EXISTS expenses_review_status_idx ON expenses (review_status);

CREATE INDEX IF NOT EXISTS audit_events_subject_idx ON audit_events (subject_type, subject_id);
