package webhook

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DeliveryStore struct {
	pool *pgxpool.Pool
}

func NewDeliveryStore(pool *pgxpool.Pool) *DeliveryStore {
	return &DeliveryStore{pool: pool}
}

// Claim atomically records the delivery as being processed. Implementation
// uses INSERT ... ON CONFLICT DO NOTHING so that two concurrent receivers
// (e.g. a GitHub webhook retry that arrives while the first is still in
// flight) cannot both observe the delivery as unprocessed. Postgres
// serializes the two inserts on the unique index for delivery_id; the loser
// returns RowsAffected() == 0 and the caller skips processing.
func (s *DeliveryStore) Claim(ctx context.Context, deliveryID, eventType string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (delivery_id, event_type) VALUES ($1, $2) ON CONFLICT (delivery_id) DO NOTHING`,
		deliveryID, eventType,
	)
	if err != nil {
		return false, fmt.Errorf("claiming delivery: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
