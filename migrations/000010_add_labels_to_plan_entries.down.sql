DROP INDEX IF EXISTS idx_plan_entries_labels;
ALTER TABLE plan_entries DROP COLUMN IF EXISTS labels;
