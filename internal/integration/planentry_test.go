//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

// TestPlanLifecycle drives the full chain: plan → re-plan (supersede)
// → unplan (soft-delete), verifying that the partial unique index
// gates duplicate active rows and that the chain is walkable.
func TestPlanLifecycle(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	store := planentry.NewStore(pool)
	const (
		userID = int64(42)
		login  = "alice"
		repo   = "org/billbird"
		issue  = 100
	)

	// 1. Initial plan: 8h
	first, err := store.Create(ctx, &planentry.Entry{
		GitHubUserID:     userID,
		GitHubUsername:   login,
		Repository:       repo,
		IssueNumber:      issue,
		DurationMinutes:  480,
		Description:      "Initial scope",
		SourceCommentID:  111,
		SourceCommentURL: "https://example/comment/111",
		CreatedBy:        "user",
	})
	if err != nil {
		t.Fatalf("create first plan: %v", err)
	}
	if first.Status != "active" {
		t.Errorf("expected status active, got %q", first.Status)
	}

	// 2. Duplicate active insert must be blocked by the partial unique index.
	_, err = store.Create(ctx, &planentry.Entry{
		GitHubUserID:     userID,
		GitHubUsername:   login,
		Repository:       repo,
		IssueNumber:      issue,
		DurationMinutes:  120,
		SourceCommentID:  112,
		SourceCommentURL: "https://example/comment/112",
		CreatedBy:        "user",
	})
	if err == nil {
		t.Fatalf("expected unique-index violation, got nil")
	}
	if !strings.Contains(err.Error(), "uniq_active_plan") {
		t.Errorf("expected uniq_active_plan in error, got %v", err)
	}

	// 3. Proper supersede flow: flip status, insert new, link chain.
	if err := store.MarkSuperseded(ctx, first.ID); err != nil {
		t.Fatalf("mark superseded: %v", err)
	}
	second, err := store.Create(ctx, &planentry.Entry{
		GitHubUserID:     userID,
		GitHubUsername:   login,
		Repository:       repo,
		IssueNumber:      issue,
		DurationMinutes:  720,
		Description:      "Scope grew",
		SourceCommentID:  113,
		SourceCommentURL: "https://example/comment/113",
		CreatedBy:        "user",
	})
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}
	if err := store.LinkSupersedeChain(ctx, first.ID, second.ID); err != nil {
		t.Fatalf("link chain: %v", err)
	}

	// 4. Verify the chain walks both directions
	chain, err := store.GetChain(ctx, first.ID)
	if err != nil {
		t.Fatalf("get chain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(chain))
	}
	if chain[0].ID != first.ID || chain[1].ID != second.ID {
		t.Errorf("chain order wrong: %d then %d", chain[0].ID, chain[1].ID)
	}
	if chain[0].SupersededBy == nil || *chain[0].SupersededBy != second.ID {
		t.Errorf("expected first.superseded_by = %d, got %v", second.ID, chain[0].SupersededBy)
	}

	// 5. Active lookup returns the latest plan
	active, err := store.FindActive(ctx, repo, issue)
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	if active == nil || active.ID != second.ID {
		t.Fatalf("expected active plan %d, got %+v", second.ID, active)
	}

	// 6. Soft delete records the closing comment metadata
	if err := store.SoftDelete(ctx, second.ID, 999, "https://example/comment/999"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	finalState, err := store.GetByID(ctx, second.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if finalState.Status != "deleted" {
		t.Errorf("expected status deleted, got %q", finalState.Status)
	}
	if finalState.ClosingCommentID == nil || *finalState.ClosingCommentID != 999 {
		t.Errorf("closing comment id not recorded: %+v", finalState.ClosingCommentID)
	}

	// 7. After unplan a fresh /plan must succeed (no active row exists)
	third, err := store.Create(ctx, &planentry.Entry{
		GitHubUserID:     userID,
		GitHubUsername:   login,
		Repository:       repo,
		IssueNumber:      issue,
		DurationMinutes:  60,
		SourceCommentID:  114,
		SourceCommentURL: "https://example/comment/114",
		CreatedBy:        "user",
	})
	if err != nil {
		t.Fatalf("create third plan after unplan: %v", err)
	}
	if third.Status != "active" {
		t.Errorf("expected status active for third plan")
	}
}

func TestPlanVsActualClassification(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()

	planStore := planentry.NewStore(pool)
	timeStore := timeentry.NewStore(pool)

	type setup struct {
		issue        int
		planMinutes  int // 0 means no plan
		loggedSplit  []int
		wantPlanned  int
		wantLogged   int
		wantVariance int
		wantStatus   string
	}
	cases := []setup{
		{
			issue: 1, planMinutes: 480, loggedSplit: []int{120, 60},
			wantPlanned: 480, wantLogged: 180, wantVariance: -300, wantStatus: "under",
		},
		{
			issue: 2, planMinutes: 0, loggedSplit: []int{120},
			wantPlanned: 0, wantLogged: 120, wantVariance: 120, wantStatus: "no_plan",
		},
		{
			issue: 3, planMinutes: 480, loggedSplit: []int{240, 240}, // exactly on plan
			wantPlanned: 480, wantLogged: 480, wantVariance: 0, wantStatus: "on_target",
		},
		{
			issue: 4, planMinutes: 480, loggedSplit: []int{420, 240}, // 660 > 480 by 180min (37%)
			wantPlanned: 480, wantLogged: 660, wantVariance: 180, wantStatus: "over",
		},
		{
			issue: 5, planMinutes: 480, loggedSplit: []int{120, 360, 14}, // 494 within 5% (24min tolerance)
			wantPlanned: 480, wantLogged: 494, wantVariance: 14, wantStatus: "on_target",
		},
	}

	const repo = "org/billbird"

	for _, c := range cases {
		if c.planMinutes > 0 {
			if _, err := planStore.Create(ctx, &planentry.Entry{
				GitHubUserID:     1,
				GitHubUsername:   "alice",
				Repository:       repo,
				IssueNumber:      c.issue,
				DurationMinutes:  c.planMinutes,
				SourceCommentID:  int64(1000 + c.issue),
				SourceCommentURL: "https://example/comment",
				CreatedBy:        "user",
			}); err != nil {
				t.Fatalf("create plan issue %d: %v", c.issue, err)
			}
		}
		for i, minutes := range c.loggedSplit {
			if _, err := timeStore.Create(ctx, &timeentry.Entry{
				GitHubUserID:     1,
				GitHubUsername:   "alice",
				Repository:       repo,
				IssueNumber:      c.issue,
				DurationMinutes:  minutes,
				SourceCommentID:  int64(2000 + c.issue*100 + i),
				SourceCommentURL: "https://example/log",
				CreatedBy:        "user",
			}); err != nil {
				t.Fatalf("create time entry issue %d #%d: %v", c.issue, i, err)
			}
		}
	}

	for _, c := range cases {
		pva, err := planStore.ComputePlanVsActual(ctx, repo, c.issue)
		if err != nil {
			t.Fatalf("compute pva issue %d: %v", c.issue, err)
		}
		if pva.PlannedMins != c.wantPlanned {
			t.Errorf("issue %d: planned got %d want %d", c.issue, pva.PlannedMins, c.wantPlanned)
		}
		if pva.LoggedMins != c.wantLogged {
			t.Errorf("issue %d: logged got %d want %d", c.issue, pva.LoggedMins, c.wantLogged)
		}
		if pva.VarianceMins != c.wantVariance {
			t.Errorf("issue %d: variance got %d want %d", c.issue, pva.VarianceMins, c.wantVariance)
		}
		if pva.Status != c.wantStatus {
			t.Errorf("issue %d: status got %q want %q", c.issue, pva.Status, c.wantStatus)
		}
	}
}

func TestPlanListFilters(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()
	store := planentry.NewStore(pool)

	// Insert across two repos and three issues.
	for _, e := range []planentry.Entry{
		{GitHubUserID: 1, GitHubUsername: "a", Repository: "org/r1", IssueNumber: 10, DurationMinutes: 60, SourceCommentID: 1, SourceCommentURL: "u", CreatedBy: "user"},
		{GitHubUserID: 1, GitHubUsername: "a", Repository: "org/r1", IssueNumber: 11, DurationMinutes: 120, SourceCommentID: 2, SourceCommentURL: "u", CreatedBy: "user"},
		{GitHubUserID: 1, GitHubUsername: "a", Repository: "org/r2", IssueNumber: 20, DurationMinutes: 30, SourceCommentID: 3, SourceCommentURL: "u", CreatedBy: "user"},
	} {
		e := e
		if _, err := store.Create(ctx, &e); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	rows, err := store.List(ctx, planentry.ListFilter{Repo: "org/r1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("repo filter: got %d want 2", len(rows))
	}

	issue := 20
	rows, err = store.List(ctx, planentry.ListFilter{IssueNumber: &issue})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].Repository != "org/r2" {
		t.Errorf("issue filter: got %+v", rows)
	}
}

// suppress unused-import warning if pool unused in some test files
var _ = pgxpool.Pool{}
