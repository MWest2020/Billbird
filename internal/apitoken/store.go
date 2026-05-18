package apitoken

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// TokenPrefix is the literal prefix every Billbird API token starts with.
// It identifies tokens at a glance and is part of the public string the
// user copies. The first 8 characters AFTER the prefix are stored in the
// database (as the row's `prefix` column) for the listing UI.
const TokenPrefix = "bb_"

// BcryptCost is the work factor used to hash tokens. 12 is the project's
// floor; raise rather than lower.
const BcryptCost = 12

// LastUsedThrottle is the minimum gap between two writes of a single
// token's last_used_at column. Prevents write amplification under sustained
// MCP calls.
const LastUsedThrottle = 60 * time.Second

// ErrTokenNotFound is returned when a token cannot be resolved by its
// prefix or ID.
var ErrTokenNotFound = errors.New("token not found")

// Token represents a single API token row. The plaintext is never stored,
// so it is only ever populated immediately after Generate.
type Token struct {
	ID             int64
	GitHubUserID   int64
	GitHubUsername string
	Label          string
	Prefix         string
	CreatedAt      time.Time
	LastUsedAt     *time.Time
	Revoked        bool
	RevokedAt      *time.Time
	RevokedBy      *string
	// Plaintext is only set on the create-call response and is never
	// persisted.
	Plaintext string
}

type Store struct {
	pool *pgxpool.Pool

	mu             sync.Mutex
	lastUsedMemo   map[int64]time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:         pool,
		lastUsedMemo: make(map[int64]time.Time),
	}
}

// Generate creates a new token for a user. The returned Token carries the
// plaintext — the caller MUST show it to the user immediately and never
// store it. After this call returns the plaintext is unrecoverable.
func (s *Store) Generate(ctx context.Context, userID int64, username, label string) (*Token, error) {
	if label == "" {
		return nil, errors.New("label is required")
	}

	plaintext, prefix, err := generateTokenString()
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing token: %w", err)
	}

	t := &Token{
		GitHubUserID:   userID,
		GitHubUsername: username,
		Label:          label,
		Prefix:         prefix,
		Plaintext:      plaintext,
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO api_tokens (github_user_id, github_username, label, prefix, hash)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		userID, username, label, prefix, string(hash),
	)
	if err := row.Scan(&t.ID, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting token: %w", err)
	}
	return t, nil
}

// Verify resolves a presented plaintext token. Returns (token, nil) when the
// token is valid and not revoked. Returns (nil, nil) for any unverifiable
// or revoked input — the caller treats both cases as 401 without
// distinguishing between them, to avoid token-existence side channels.
// Returns (nil, err) only for genuine database errors.
func (s *Store) Verify(ctx context.Context, plaintext string) (*Token, error) {
	prefix, ok := parsePrefix(plaintext)
	if !ok {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, github_user_id, github_username, label, prefix, hash,
			created_at, last_used_at, revoked, revoked_at, revoked_by
		FROM api_tokens
		WHERE prefix = $1 AND revoked = false`,
		prefix)
	if err != nil {
		return nil, fmt.Errorf("looking up token: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Token
		var hash string
		if err := rows.Scan(
			&t.ID, &t.GitHubUserID, &t.GitHubUsername, &t.Label, &t.Prefix, &hash,
			&t.CreatedAt, &t.LastUsedAt, &t.Revoked, &t.RevokedAt, &t.RevokedBy,
		); err != nil {
			return nil, fmt.Errorf("scanning token row: %w", err)
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil {
			s.recordLastUsed(ctx, t.ID)
			return &t, nil
		}
	}
	return nil, nil
}

// GetByID fetches a token row by ID. Does not include the hash.
func (s *Store) GetByID(ctx context.Context, id int64) (*Token, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, github_user_id, github_username, label, prefix,
			created_at, last_used_at, revoked, revoked_at, revoked_by
		FROM api_tokens WHERE id = $1`, id)
	var t Token
	err := row.Scan(
		&t.ID, &t.GitHubUserID, &t.GitHubUsername, &t.Label, &t.Prefix,
		&t.CreatedAt, &t.LastUsedAt, &t.Revoked, &t.RevokedAt, &t.RevokedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting token: %w", err)
	}
	return &t, nil
}

// ListForUser returns every token owned by a specific user.
func (s *Store) ListForUser(ctx context.Context, userID int64) ([]Token, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, github_user_id, github_username, label, prefix,
			created_at, last_used_at, revoked, revoked_at, revoked_by
		FROM api_tokens
		WHERE github_user_id = $1
		ORDER BY created_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(
			&t.ID, &t.GitHubUserID, &t.GitHubUsername, &t.Label, &t.Prefix,
			&t.CreatedAt, &t.LastUsedAt, &t.Revoked, &t.RevokedAt, &t.RevokedBy,
		); err != nil {
			return nil, fmt.Errorf("scanning token row: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// Revoke flips a token's revoked flag. revokedBy SHOULD be the GitHub
// username of the caller. Idempotent — revoking an already-revoked token
// is a no-op and returns nil.
func (s *Store) Revoke(ctx context.Context, id int64, revokedBy string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE api_tokens
		SET revoked = true, revoked_at = now(), revoked_by = $1
		WHERE id = $2 AND revoked = false`,
		revokedBy, id,
	)
	if err != nil {
		return fmt.Errorf("revoking token %d: %w", id, err)
	}
	return nil
}

func (s *Store) recordLastUsed(ctx context.Context, id int64) {
	now := time.Now()
	s.mu.Lock()
	last, seen := s.lastUsedMemo[id]
	if seen && now.Sub(last) < LastUsedThrottle {
		s.mu.Unlock()
		return
	}
	s.lastUsedMemo[id] = now
	s.mu.Unlock()

	// Best-effort; a missed update is harmless.
	_, _ = s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE id = $1`, id)
}

// --- token-string helpers ---

// generateTokenString returns a freshly generated token like
// "bb_<32 random bytes base64-url no padding>" plus the 8-character prefix
// used for display.
func generateTokenString() (plaintext string, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	plaintext = TokenPrefix + encoded
	prefix = encoded[:8]
	return plaintext, prefix, nil
}

// parsePrefix extracts the display-prefix portion of a presented token.
// Returns "", false when the input is not a Billbird token string.
func parsePrefix(plaintext string) (string, bool) {
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		return "", false
	}
	body := strings.TrimPrefix(plaintext, TokenPrefix)
	if len(body) < 8 {
		return "", false
	}
	return body[:8], true
}
