//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mwesterweel/billbird/internal/api"
	"github.com/mwesterweel/billbird/internal/apitoken"
	"github.com/mwesterweel/billbird/internal/auth"
	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

// fixedMembership is a deterministic stand-in for MembershipChecker:
// it returns the configured boolean for any username, so tests can
// flip the allow/deny path without spinning up the GitHub App.
type fixedMembership bool

func (f fixedMembership) IsAllowed(string) bool { return bool(f) }

// TestAPIAuthBearer wires the real api.Handler against a real Postgres,
// a real planentry/timeentry store, the real token store, and the real
// bearer-and-cookie middleware (with a fake MembershipPolicy). It then
// hits the endpoints over loopback HTTP to prove the wire-level
// contract.
func TestAPIAuthBearer(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	timeStore := timeentry.NewStore(pool)
	planStore := planentry.NewStore(pool)
	tokenStore := apitoken.NewStore(pool)

	apiHandler := api.NewHandler(pool, timeStore, planStore, tokenStore)

	deps := auth.APIAuthDependencies{
		Cookie:     nil, // cookie path not exercised in this test
		Tokens:     tokenStore,
		Membership: fixedMembership(true),
	}

	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux, deps.RequireAPIAuth)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Without any auth, /api/v1/plans returns 401
	resp, err := http.Get(srv.URL + "/api/v1/plans")
	if err != nil {
		t.Fatalf("GET /plans no auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}

	// 2. Bogus bearer token also 401
	resp = withAuth(t, srv.URL+"/api/v1/plans", "bb_obviously_invalid_value_here_padding_padding")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for bogus token, got %d", resp.StatusCode)
	}

	// 3. Create a real token in the DB, hit the endpoint, expect 200
	tok, err := tokenStore.Generate(ctx, 7, "alice", "integration-test")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	// Seed a plan so the response body is non-empty
	if _, err := planStore.Create(ctx, &planentry.Entry{
		GitHubUserID:     7,
		GitHubUsername:   "alice",
		Repository:       "org/billbird",
		IssueNumber:      1,
		DurationMinutes:  240,
		SourceCommentID:  1,
		SourceCommentURL: "https://example/comment/1",
		CreatedBy:        "user",
	}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	resp = withAuth(t, srv.URL+"/api/v1/plans", tok.Plaintext)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d", resp.StatusCode)
	}
	var plans []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		t.Errorf("decode response: %v", err)
	}
	resp.Body.Close()
	if len(plans) != 1 {
		t.Errorf("expected 1 plan in response, got %d", len(plans))
	}

	// 4. plan-vs-actual endpoint round-trips with the same token
	if _, err := timeStore.Create(ctx, &timeentry.Entry{
		GitHubUserID:     7,
		GitHubUsername:   "alice",
		Repository:       "org/billbird",
		IssueNumber:      1,
		DurationMinutes:  120,
		SourceCommentID:  9,
		SourceCommentURL: "https://example/comment/9",
		CreatedBy:        "user",
	}); err != nil {
		t.Fatalf("create time entry: %v", err)
	}
	resp = withAuth(t, srv.URL+"/api/v1/issues/org/billbird/1/plan-vs-actual", tok.Plaintext)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on plan-vs-actual, got %d", resp.StatusCode)
	}
	var pva map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&pva); err != nil {
		t.Fatalf("decode pva: %v", err)
	}
	resp.Body.Close()
	if pva["planned_minutes"].(float64) != 240 {
		t.Errorf("planned: got %v want 240", pva["planned_minutes"])
	}
	if pva["logged_minutes"].(float64) != 120 {
		t.Errorf("logged: got %v want 120", pva["logged_minutes"])
	}
	if pva["status"] != "under" {
		t.Errorf("status: got %v want under", pva["status"])
	}

	// 5. Revoke the token, then expect 401
	if err := tokenStore.Revoke(ctx, tok.ID, "alice"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	resp = withAuth(t, srv.URL+"/api/v1/plans", tok.Plaintext)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 after revoke, got %d", resp.StatusCode)
	}
}

// TestAPIAuthBearerDeniesNonMembers proves that a user removed from
// every allowed org loses access immediately even with a valid token,
// because the membership policy is consulted on every token request.
func TestAPIAuthBearerDeniesNonMembers(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	tokenStore := apitoken.NewStore(pool)
	timeStore := timeentry.NewStore(pool)
	planStore := planentry.NewStore(pool)

	apiHandler := api.NewHandler(pool, timeStore, planStore, tokenStore)

	deps := auth.APIAuthDependencies{
		Tokens:     tokenStore,
		Membership: fixedMembership(false), // user has been removed from the org
	}
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux, deps.RequireAPIAuth)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tok, err := tokenStore.Generate(ctx, 7, "alice", "integration-test")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	resp := withAuth(t, srv.URL+"/api/v1/plans", tok.Plaintext)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for ex-member, got %d", resp.StatusCode)
	}
}

func withAuth(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}
