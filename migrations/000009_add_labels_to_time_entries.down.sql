DROP INDEX IF EXISTS idx_time_entries_labels;
ALTER TABLE time_entries DROP COLUMN IF EXISTS labels;
