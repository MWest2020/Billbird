CREATE TABLE sessions (
    id              BIGSERIAL PRIMARY KEY,
    github_user_id  BIGINT NOT NULL,
    github_username TEXT NOT NULL,
    org_member      BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sessions_user ON sessions (github_user_id);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);
