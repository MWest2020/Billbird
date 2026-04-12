CREATE TABLE webhook_deliveries (
    id           BIGSERIAL PRIMARY KEY,
    delivery_id  TEXT NOT NULL UNIQUE,
    event_type   TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
