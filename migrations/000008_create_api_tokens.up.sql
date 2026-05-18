CREATE TABLE api_tokens (
    id                BIGSERIAL PRIMARY KEY,
    github_user_id    BIGINT NOT NULL,
    github_username   TEXT NOT NULL,
    label             TEXT NOT NULL,
    prefix            TEXT NOT NULL,
    hash              TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at      TIMESTAMPTZ,
    revoked           BOOLEAN NOT NULL DEFAULT false,
    revoked_at        TIMESTAMPTZ,
    revoked_by        TEXT
);

CREATE INDEX idx_api_tokens_user ON api_tokens (github_user_id);
CREATE INDEX idx_api_tokens_revoked ON api_tokens (revoked);
CREATE INDEX idx_api_tokens_prefix ON api_tokens (prefix);
