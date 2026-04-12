package webhook

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeliveryStore struct {
	pool *pgxpool.Pool
}

func NewDeliveryStore(pool *pgxpool.Pool) *DeliveryStore {
	return &DeliveryStore{pool: pool}
}

// IsProcessed checks if a webhook delivery has already been processed.
func (s *DeliveryStore) IsProcessed(ctx context.Context, deliveryID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM webhook_deliveries WHERE delivery_id = $1)`,
		deliveryID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking delivery: %w", err)
	}
	return exists, nil
}

// MarkProcessed records a webhook delivery as processed.
func (s *DeliveryStore) MarkProcessed(ctx context.Context, deliveryID, eventType string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (delivery_id, event_type) VALUES ($1, $2) ON CONFLICT (delivery_id) DO NOTHING`,
		deliveryID, eventType,
	)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("marking delivery processed: %w", err)
	}
	return nil
}
