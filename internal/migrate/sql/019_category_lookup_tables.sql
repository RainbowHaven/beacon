-- Expense and operational-issue categories are reference data kept in the
-- database. Forms offer active categories; retired ones (active = false) keep
-- their label for existing records. See OPERATOR.md for changing them.

CREATE TABLE expense_categories (
    key TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    sort_order INT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    CONSTRAINT expense_categories_key_chk CHECK (key ~ '^[a-z][a-z0-9]*(_[a-z0-9]+)*$'),
    CONSTRAINT expense_categories_label_chk CHECK (char_length(trim(label)) > 0)
);

INSERT INTO expense_categories (key, label, sort_order) VALUES
    ('food', 'Food', 10),
    ('household_supplies', 'Household supplies', 20),
    ('utilities', 'Utilities', 30),
    ('rent', 'Rent', 40),
    ('maintenance_repairs', 'Maintenance and repairs', 50),
    ('transport', 'Transport', 60),
    ('communication', 'Communication', 70),
    ('other', 'Other', 80);

CREATE TABLE operational_issue_categories (
    key TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    sort_order INT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    CONSTRAINT operational_issue_categories_key_chk CHECK (key ~ '^[a-z][a-z0-9]*(_[a-z0-9]+)*$'),
    CONSTRAINT operational_issue_categories_label_chk CHECK (char_length(trim(label)) > 0)
);

INSERT INTO operational_issue_categories (key, label, sort_order) VALUES
    ('building_maintenance', 'Building or maintenance problem', 10),
    ('utilities', 'Utilities', 20),
    ('safety_security', 'Safety or security issue that is not a safeguarding concern', 30),
    ('food_supplies', 'Food or supplies', 40),
    ('staffing_agent', 'Staffing or Agent change', 50),
    ('capacity_occupancy', 'Capacity or occupancy change', 60),
    ('service_availability', 'Service availability', 70),
    ('other_change', 'Other significant operational change', 80);

ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_category_chk;
ALTER TABLE expenses ADD CONSTRAINT expenses_category_fkey
    FOREIGN KEY (category) REFERENCES expense_categories (key)
    ON UPDATE CASCADE ON DELETE RESTRICT;

ALTER TABLE operational_issues DROP CONSTRAINT IF EXISTS operational_issues_category_chk;
ALTER TABLE operational_issues ADD CONSTRAINT operational_issues_category_fkey
    FOREIGN KEY (category) REFERENCES operational_issue_categories (key)
    ON UPDATE CASCADE ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS expenses_category_idx ON expenses (category);
CREATE INDEX IF NOT EXISTS operational_issues_category_idx ON operational_issues (category);
