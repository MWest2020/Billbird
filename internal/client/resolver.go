package client

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Mapping struct {
	ClientID  int64
	Label     string
	Repo      *string
}

type Resolver struct {
	pool *pgxpool.Pool
}

func NewResolver(pool *pgxpool.Pool) *Resolver {
	return &Resolver{pool: pool}
}

// ResolveClient finds the client for an issue based on its labels and repository.
// Returns the client ID or nil if no match. Repository-specific mappings take precedence.
func (r *Resolver) ResolveClient(ctx context.Context, labels []string, repo string) (*int64, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT lm.client_id, lm.label_pattern, lm.repository
		FROM label_mappings lm
		JOIN clients c ON c.id = lm.client_id AND c.active = true
		WHERE lm.label_pattern = ANY($1)
		ORDER BY
			CASE WHEN lm.repository IS NOT NULL THEN 0 ELSE 1 END,
			lm.created_at ASC`,
		labels,
	)
	if err != nil {
		return nil, fmt.Errorf("querying label mappings: %w", err)
	}
	defer rows.Close()

	var matches []Mapping
	for rows.Next() {
		var m Mapping
		if err := rows.Scan(&m.ClientID, &m.Label, &m.Repo); err != nil {
			return nil, fmt.Errorf("scanning label mapping: %w", err)
		}
		matches = append(matches, m)
	}

	if len(matches) == 0 {
		return nil, nil
	}

	// Prefer repo-specific match
	for _, m := range matches {
		if m.Repo != nil && *m.Repo == repo {
			if len(matches) > 1 {
				log.Printf("warning: multiple client labels matched for repo %s, using %q (client %d)", repo, m.Label, m.ClientID)
			}
			return &m.ClientID, nil
		}
	}

	// Fall back to first global match
	if len(matches) > 1 {
		log.Printf("warning: multiple client labels matched for repo %s, using %q (client %d)", repo, matches[0].Label, matches[0].ClientID)
	}
	return &matches[0].ClientID, nil
}
