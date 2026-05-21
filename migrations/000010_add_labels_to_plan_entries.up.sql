ALTER TABLE plan_entries
    ADD COLUMN labels TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX idx_plan_entries_labels ON plan_entries USING GIN (labels);
