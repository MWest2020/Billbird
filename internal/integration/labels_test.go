//go:build integration

package integration

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

// TestTimeEntryLabelsRoundTrip stores entries with various label sets
// and exercises both List filters against real Postgres + GIN index.
func TestTimeEntryLabelsRoundTrip(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()
	store := timeentry.NewStore(pool)

	type want struct {
		labels []string
	}
	rows := []want{
		{labels: []string{"client:amsterdam", "type:development", "wbso:speur"}},
		{labels: []string{"client:rotterdam", "type:bugfix"}},
		{labels: []string{"strippenkaart:acme-2026q1"}},
		{labels: nil}, // unlabelled issue
	}

	for i, r := range rows {
		_, err := store.Create(ctx, &timeentry.Entry{
			GitHubUserID:     1,
			GitHubUsername:   "alice",
			Repository:       "org/repo",
			IssueNumber:      i + 1,
			DurationMinutes:  60,
			SourceCommentID:  int64(1000 + i),
			SourceCommentURL: "https://example/comment",
			CreatedBy:        "user",
			Labels:           r.labels,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	// 1. Round-trip via GetByID — empty issue must come back as [] not nil.
	allFromList, err := store.List(ctx, timeentry.ListFilter{Status: "all"})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(allFromList) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(allFromList))
	}
	for _, e := range allFromList {
		if e.Labels == nil {
			t.Errorf("entry %d Labels must never be nil after read; got nil for issue %d",
				e.ID, e.IssueNumber)
		}
	}

	// 2. Single-label containment.
	filtered, err := store.List(ctx, timeentry.ListFilter{
		Status: "all",
		Labels: []string{"wbso:speur"},
	})
	if err != nil {
		t.Fatalf("list by label: %v", err)
	}
	if len(filtered) != 1 || filtered[0].IssueNumber != 1 {
		t.Errorf("expected issue 1 only, got %+v", filtered)
	}

	// 3. AND containment — both labels must be present.
	andFiltered, err := store.List(ctx, timeentry.ListFilter{
		Status: "all",
		Labels: []string{"client:amsterdam", "type:development"},
	})
	if err != nil {
		t.Fatalf("AND filter: %v", err)
	}
	if len(andFiltered) != 1 || andFiltered[0].IssueNumber != 1 {
		t.Errorf("AND should match only issue 1, got %+v", andFiltered)
	}

	// 4. AND with two labels that do not co-occur → empty result.
	zero, err := store.List(ctx, timeentry.ListFilter{
		Status: "all",
		Labels: []string{"client:amsterdam", "client:rotterdam"},
	})
	if err != nil {
		t.Fatalf("AND zero: %v", err)
	}
	if len(zero) != 0 {
		t.Errorf("expected 0 results for impossible AND, got %d", len(zero))
	}

	// 5. Prefix filter.
	prefixed, err := store.List(ctx, timeentry.ListFilter{
		Status:      "all",
		LabelPrefix: "client:",
	})
	if err != nil {
		t.Fatalf("prefix filter: %v", err)
	}
	gotIssues := []int{}
	for _, e := range prefixed {
		gotIssues = append(gotIssues, e.IssueNumber)
	}
	sort.Ints(gotIssues)
	if !reflect.DeepEqual(gotIssues, []int{1, 2}) {
		t.Errorf("expected issues 1+2 with client:* prefix, got %v", gotIssues)
	}

	// 6. Strippenkaart-style prefix.
	strippenkaart, err := store.List(ctx, timeentry.ListFilter{
		Status:      "all",
		LabelPrefix: "strippenkaart:",
	})
	if err != nil {
		t.Fatalf("strippenkaart filter: %v", err)
	}
	if len(strippenkaart) != 1 || strippenkaart[0].IssueNumber != 3 {
		t.Errorf("expected issue 3 for strippenkaart:* prefix, got %+v", strippenkaart)
	}

	// 7. Empty filter returns everything.
	noFilter, err := store.List(ctx, timeentry.ListFilter{Status: "all"})
	if err != nil {
		t.Fatalf("no filter: %v", err)
	}
	if len(noFilter) != 4 {
		t.Errorf("expected 4 entries with no label filter, got %d", len(noFilter))
	}
}

func TestPlanEntryLabelsRoundTrip(t *testing.T) {
	ensureLinux(t)
	pool, shutdown := startPostgres(t)
	defer shutdown()
	ctx := context.Background()
	store := planentry.NewStore(pool)

	plans := []struct {
		issue  int
		labels []string
	}{
		{1, []string{"strippenkaart:acme-2026q1", "wbso:speur"}},
		{2, []string{"strippenkaart:acme-2026q1"}},
		{3, []string{"strippenkaart:other-budget", "type:bugfix"}},
		{4, nil},
	}
	for _, p := range plans {
		if _, err := store.Create(ctx, &planentry.Entry{
			GitHubUserID:     1,
			GitHubUsername:   "alice",
			Repository:       "org/repo",
			IssueNumber:      p.issue,
			DurationMinutes:  60,
			SourceCommentID:  int64(2000 + p.issue),
			SourceCommentURL: "https://example/plan",
			CreatedBy:        "user",
			Labels:           p.labels,
		}); err != nil {
			t.Fatalf("create plan %d: %v", p.issue, err)
		}
	}

	// Strippenkaart-budget query: two plans share `strippenkaart:acme-2026q1`.
	acme, err := store.List(ctx, planentry.ListFilter{
		Labels: []string{"strippenkaart:acme-2026q1"},
	})
	if err != nil {
		t.Fatalf("acme list: %v", err)
	}
	if len(acme) != 2 {
		t.Errorf("expected 2 plans on acme-2026q1, got %d", len(acme))
	}

	// WBSO prefix — only one plan is WBSO-eligible.
	wbso, err := store.List(ctx, planentry.ListFilter{
		LabelPrefix: "wbso:",
	})
	if err != nil {
		t.Fatalf("wbso list: %v", err)
	}
	if len(wbso) != 1 || wbso[0].IssueNumber != 1 {
		t.Errorf("expected plan on issue 1, got %+v", wbso)
	}

	// Round-trip GetByID — empty labels come back as [].
	chain, err := store.GetChain(ctx, acme[0].ID)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	for _, p := range chain {
		if p.Labels == nil {
			t.Errorf("chain entry labels must never be nil, got nil on plan %d", p.ID)
		}
	}
}
