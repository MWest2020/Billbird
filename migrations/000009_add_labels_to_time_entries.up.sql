ALTER TABLE time_entries
    ADD COLUMN labels TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX idx_time_entries_labels ON time_entries USING GIN (labels);
