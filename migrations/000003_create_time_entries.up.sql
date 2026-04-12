CREATE TYPE entry_status AS ENUM ('active', 'superseded', 'deleted');
CREATE TYPE created_by_type AS ENUM ('user', 'admin');

CREATE TABLE time_entries (
    id                 BIGSERIAL PRIMARY KEY,
    github_user_id     BIGINT NOT NULL,
    github_username    TEXT NOT NULL,
    repository         TEXT NOT NULL,
    issue_number       INT NOT NULL,
    duration_minutes   INT NOT NULL,
    description        TEXT,
    client_id          BIGINT REFERENCES clients(id),
    source_comment_id  BIGINT NOT NULL,
    source_comment_url TEXT NOT NULL,
    status             entry_status NOT NULL DEFAULT 'active',
    superseded_by      BIGINT REFERENCES time_entries(id),
    created_by         created_by_type NOT NULL DEFAULT 'user',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_time_entries_user_issue ON time_entries (github_user_id, repository, issue_number);
CREATE INDEX idx_time_entries_status ON time_entries (status);
CREATE INDEX idx_time_entries_client ON time_entries (client_id);
CREATE INDEX idx_time_entries_created_at ON time_entries (created_at);
