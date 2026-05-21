// Smoke-seed: insert one bearer token and one plan against a running
// Billbird database, printing the plaintext token to stdout. Intended
// for local smoke runs only — never wire this into production startup.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/smokeseed
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mwesterweel/billbird/internal/apitoken"
	"github.com/mwesterweel/billbird/internal/planentry"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}
	userID := int64(66728126)
	if v := os.Getenv("SMOKE_USER_ID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			log.Fatalf("bad SMOKE_USER_ID: %v", err)
		}
		userID = n
	}
	username := os.Getenv("SMOKE_USERNAME")
	if username == "" {
		username = "MWest2020"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	tok, err := apitoken.NewStore(pool).Generate(
		context.Background(), userID, username, "smoke",
	)
	if err != nil {
		log.Fatalf("generate token: %v", err)
	}
	// Print the token immediately so it lands in stdout even when the
	// optional plan-creation step below fails (e.g. the issue already
	// has an active plan and the partial-unique-index trips).
	fmt.Printf("TOKEN=%s\n", tok.Plaintext)

	plan, err := planentry.NewStore(pool).Create(
		context.Background(),
		&planentry.Entry{
			GitHubUserID:     userID,
			GitHubUsername:   username,
			Repository:       "MWest2020/Billbird",
			IssueNumber:      1,
			DurationMinutes:  480,
			Description:      "Smoke plan",
			SourceCommentID:  1,
			SourceCommentURL: "https://example/comment/1",
			CreatedBy:        "user",
		},
	)
	if err != nil {
		log.Printf("warn: create plan (non-fatal, token still issued): %v", err)
		return
	}

	fmt.Printf("PLAN_ID=%d\n", plan.ID)
}
