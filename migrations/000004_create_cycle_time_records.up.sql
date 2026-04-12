CREATE TABLE cycle_time_records (
    id           BIGSERIAL PRIMARY KEY,
    repository   TEXT NOT NULL,
    issue_number INT NOT NULL,
    start_at     TIMESTAMPTZ,
    end_at       TIMESTAMPTZ,
    start_source TEXT,
    end_source   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repository, issue_number)
);
