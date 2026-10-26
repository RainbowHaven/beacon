-- Active expense categories, in the order forms offer them.
-- Existing keys stay put: a key change is copied onto every expense and marks
-- confirmed months as changed. Retired rows keep their labels so historical
-- expenses still show the old wording. Operational-issue categories are unchanged.

UPDATE expense_categories SET label = 'Rent/Lease', sort_order = 10 WHERE key = 'rent';
UPDATE expense_categories SET sort_order = 20 WHERE key = 'utilities';
UPDATE expense_categories SET label = 'Food/Groceries', sort_order = 30 WHERE key = 'food';
UPDATE expense_categories SET sort_order = 40 WHERE key = 'transport';

INSERT INTO expense_categories (key, label, sort_order) VALUES
    ('household_maintenance', 'Household/Maintenance', 50),
    ('fees', 'Fees', 60);

UPDATE expense_categories SET sort_order = 70 WHERE key = 'other';

UPDATE expense_categories SET active = false, sort_order = 110 WHERE key = 'household_supplies';
UPDATE expense_categories SET active = false, sort_order = 120 WHERE key = 'maintenance_repairs';
UPDATE expense_categories SET active = false, sort_order = 130 WHERE key = 'communication';
