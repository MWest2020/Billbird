CREATE TYPE plan_status AS ENUM ('active', 'superseded', 'deleted');

CREATE TABLE plan_entries (
    id                   BIGSERIAL PRIMARY KEY,
    github_user_id       BIGINT NOT NULL,
    github_username      TEXT NOT NULL,
    repository           TEXT NOT NULL,
    issue_number         INT NOT NULL,
    duration_minutes     INT NOT NULL,
    description          TEXT,
    source_comment_id    BIGINT NOT NULL,
    source_comment_url   TEXT NOT NULL,
    closing_comment_id   BIGINT,
    closing_comment_url  TEXT,
    status               plan_status NOT NULL DEFAULT 'active',
    superseded_by        BIGINT REFERENCES plan_entries(id),
    created_by           created_by_type NOT NULL DEFAULT 'user',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_plan_entries_user_issue ON plan_entries (github_user_id, repository, issue_number);
CREATE INDEX idx_plan_entries_status ON plan_entries (status);
CREATE INDEX idx_plan_entries_issue ON plan_entries (repository, issue_number);
CREATE INDEX idx_plan_entries_created_at ON plan_entries (created_at);

-- At most one active plan per (repository, issue_number).
CREATE UNIQUE INDEX uniq_active_plan ON plan_entries (repository, issue_number) WHERE status = 'active';
