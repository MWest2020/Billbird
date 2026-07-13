---
status: draft
last_reviewed: 2026-07-13
---

# Webhook idempotency

Billbird is a webhook receiver and every `/log`, `/correct`, `/delete`, `/plan`, `/unplan` command is processed exactly once even though GitHub may deliver the same event multiple times.

## What GitHub guarantees (and doesn't)

GitHub sends a unique `X-GitHub-Delivery` UUID with every webhook. If a delivery's HTTP response arrives later than ~10 seconds, GitHub considers the delivery failed and **retries** — same `X-GitHub-Delivery`, same payload, fresh TCP connection. A delivery can also be replayed manually from the App's "Recent Deliveries" UI. Multiple receivers behind a load balancer compound the problem: two webhook receivers could both be working through the same delivery at the same time.

What GitHub does *not* guarantee:

- That a retry won't arrive while the original is still in flight.
- That two retries won't arrive at the same instant.
- That a manual replay won't collide with an automatic retry.

## Why a naive "have we seen it?" check is not enough

The obvious dedup is: when a delivery arrives, look up its ID in a `webhook_deliveries` table; if present, skip; otherwise, process and insert. The bug is the gap between "look up" and "insert":

```text
T=0   delivery A arrives           SELECT ... FROM webhook_deliveries WHERE delivery_id=A   →  empty
T=1   GitHub retry for delivery A  SELECT ...                                              →  empty
T=2   first goroutine processes A  INSERT into time_entries                                →  entry #1
T=3   second goroutine processes A INSERT into time_entries                                →  entry #2  (duplicate!)
T=4   first goroutine inserts dedupe row                                                    →  row exists
T=5   second goroutine inserts dedupe row                                                   →  conflict (too late)
```

This is a classic time-of-check-to-time-of-use (TOCTOU) race. Two concurrent receivers both see the delivery as unprocessed, both run the command, both create a time entry. The dedupe table is right *eventually*, but the side-effect (the duplicate `/log`) is already in the database.

This is not a theoretical concern. GitHub's retry policy is aggressive: if your dispatch takes more than 10 seconds — three sequential GitHub API calls (label fetch + org membership + post comment) under any upstream latency can hit that bound — a retry is already on the wire while your first dispatch is still running.

## How Billbird fixes this

Postgres serializes writers on a unique index. A single statement that *both* checks-and-claims in one atomic step is race-free:

```sql
INSERT INTO webhook_deliveries (delivery_id, event_type)
VALUES ($1, $2)
ON CONFLICT (delivery_id) DO NOTHING
```

The behaviour:

- If `delivery_id` is new, Postgres inserts the row and returns `RowsAffected() == 1`. The caller "won" the claim and proceeds to process.
- If `delivery_id` already exists, the `ON CONFLICT DO NOTHING` clause makes the statement a no-op and returns `RowsAffected() == 0`. The caller skips processing and returns 200 to GitHub.

Critically: **two concurrent calls cannot both observe `RowsAffected() == 1`**. The unique index on `delivery_id` enforces serialization; whichever transaction commits second sees the conflict.

The implementation lives in `internal/webhook/delivery.go` as `DeliveryStore.Claim()`:

```go
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
```

And the call site (`internal/webhook/handler.go`):

```go
claimed, err := h.deliveries.Claim(r.Context(), deliveryID, eventType)
if !claimed {
    w.WriteHeader(http.StatusOK)  // already processed; skip
    return
}
// proceed with dispatch
```

## What is *not* solved

- **GitHub UI may still show timed-out deliveries.** If a single dispatch exceeds 10 seconds, GitHub records the first attempt as failed and retries. The retry is correctly deduped (no duplicate `/log` entry) but the App's "Recent Deliveries" panel will show the original delivery in red. Cosmetic — the data is correct.
- **Empty `X-GitHub-Delivery` headers are not deduped.** GitHub always sends one, but if a non-GitHub source ever posts to `/webhook`, it can bypass the claim. Signature verification still rejects it earlier, so this is not exploitable in practice.

## Testing

`internal/webhook/handler_test.go` includes `TestHandle_Idempotency_DuplicateDeliveryShortCircuits` which posts the same delivery twice and asserts the second attempt produces zero downstream GitHub API calls. The fake `DeliveryTracker` uses an in-memory map; the real `DeliveryStore` uses Postgres — the contract (single `Claim` method returning `(claimed bool, err error)`) is identical.
