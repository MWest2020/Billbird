CREATE TABLE label_mappings (
    id            BIGSERIAL PRIMARY KEY,
    label_pattern TEXT NOT NULL,
    client_id     BIGINT NOT NULL REFERENCES clients(id),
    repository    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_label_mappings_label ON label_mappings (label_pattern);
CREATE INDEX idx_label_mappings_repo ON label_mappings (repository);
