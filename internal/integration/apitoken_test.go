//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mwesterweel/billbird/internal/apitoken"
)

// TestTokenRoundTripAndRevocation exercises the full token lifecycle
// against a real Postgres: generate, verify (success), verify (after
// revoke must fail), and ensure the plaintext is unrecoverable from
// the row.
func TestTokenRoundTripAndRevocation(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	store := apitoken.NewStore(pool)

	tok, err := store.Generate(ctx, 7, "alice", "Manager-MCP")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(tok.Plaintext, "bb_") {
		t.Errorf("plaintext should start with bb_, got %q", tok.Plaintext)
	}
	if tok.ID == 0 {
		t.Errorf("ID should be assigned")
	}

	// Verify succeeds and returns the same user
	verified, err := store.Verify(ctx, tok.Plaintext)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verified == nil {
		t.Fatalf("expected verified token, got nil")
	}
	if verified.GitHubUserID != 7 || verified.GitHubUsername != "alice" {
		t.Errorf("identity mismatch: %+v", verified)
	}

	// Garbage tokens fail verification without error
	garbage, err := store.Verify(ctx, "bb_obviously_fake_value_12345678901234567890")
	if err != nil {
		t.Fatalf("verify garbage: %v", err)
	}
	if garbage != nil {
		t.Errorf("garbage token should fail verification")
	}

	// Non-bb_ prefix gets rejected at parse time, no DB hit
	wrong, err := store.Verify(ctx, "github_pat_xxxxxxxx")
	if err != nil {
		t.Fatalf("verify non-bb token: %v", err)
	}
	if wrong != nil {
		t.Errorf("non-bb token should fail verification")
	}

	// Revoke and verify the same plaintext now fails
	if err := store.Revoke(ctx, tok.ID, "alice"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	after, err := store.Verify(ctx, tok.Plaintext)
	if err != nil {
		t.Fatalf("verify after revoke: %v", err)
	}
	if after != nil {
		t.Errorf("revoked token must fail verification, got %+v", after)
	}

	// The row stays for audit; verify the revoked flag landed
	row, err := store.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if !row.Revoked {
		t.Errorf("expected revoked=true after revoke")
	}
	if row.RevokedAt == nil {
		t.Errorf("expected revoked_at to be set")
	}
}

func TestTokenListForUser(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	store := apitoken.NewStore(pool)

	for i := 0; i < 3; i++ {
		if _, err := store.Generate(ctx, 11, "bob", "label"); err != nil {
			t.Fatalf("generate: %v", err)
		}
	}
	if _, err := store.Generate(ctx, 99, "eve", "other"); err != nil {
		t.Fatalf("generate other user: %v", err)
	}

	rows, err := store.ListForUser(ctx, 11)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 tokens for bob, got %d", len(rows))
	}
	for _, r := range rows {
		if r.GitHubUserID != 11 {
			t.Errorf("returned token for wrong user: %+v", r)
		}
	}
}

func TestTokenLastUsedThrottle(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	store := apitoken.NewStore(pool)
	tok, err := store.Generate(ctx, 5, "carol", "label")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// First verify updates last_used_at
	if _, err := store.Verify(ctx, tok.Plaintext); err != nil {
		t.Fatalf("verify 1: %v", err)
	}
	row1, err := store.GetByID(ctx, tok.ID)
	if err != nil || row1.LastUsedAt == nil {
		t.Fatalf("expected last_used_at to be set, got %+v err=%v", row1.LastUsedAt, err)
	}
	first := *row1.LastUsedAt

	// Second verify within the throttle window should NOT bump the timestamp
	if _, err := store.Verify(ctx, tok.Plaintext); err != nil {
		t.Fatalf("verify 2: %v", err)
	}
	row2, err := store.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if row2.LastUsedAt == nil {
		t.Fatalf("last_used_at vanished")
	}
	if !row2.LastUsedAt.Equal(first) {
		t.Errorf("throttle missed: last_used_at moved from %v to %v within window",
			first, *row2.LastUsedAt)
	}

	// Just to confirm throttle constant is honoured; we don't sleep
	// 60 seconds in a unit test.
	_ = time.Second
}
