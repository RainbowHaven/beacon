-- Phase 6: store receipt binaries in Postgres (BYTEA), drop filesystem keys.

ALTER TABLE expenses ADD COLUMN IF NOT EXISTS receipt_data BYTEA;

-- Local/dev rows may have metadata without file bytes; clear orphaned metadata.
UPDATE expenses
SET receipt_key = NULL, receipt_content_type = NULL, receipt_bytes = NULL
WHERE receipt_data IS NULL AND receipt_key IS NOT NULL;

ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_receipt_meta_chk;

ALTER TABLE expenses DROP COLUMN IF EXISTS receipt_key;

ALTER TABLE expenses ADD CONSTRAINT expenses_receipt_meta_chk CHECK (
    (receipt_data IS NULL AND receipt_content_type IS NULL AND receipt_bytes IS NULL)
    OR (
        receipt_data IS NOT NULL
        AND receipt_content_type IS NOT NULL
        AND receipt_bytes IS NOT NULL
        AND receipt_bytes > 0
        AND octet_length(receipt_data) = receipt_bytes
    )
);
