package planentry

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID                int64
	GitHubUserID      int64
	GitHubUsername    string
	Repository        string
	IssueNumber       int
	DurationMinutes   int
	Description       string
	SourceCommentID   int64
	SourceCommentURL  string
	ClosingCommentID  *int64
	ClosingCommentURL *string
	Status            string
	SupersededBy      *int64
	CreatedBy         string
	CreatedAt         time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a new plan entry and returns it. The caller is responsible
// for superseding any prior active plan first if needed.
func (s *Store) Create(ctx context.Context, e *Entry) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO plan_entries (
			github_user_id, github_username, repository, issue_number,
			duration_minutes, description,
			source_comment_id, source_comment_url, status, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9)
		RETURNING id, created_at`,
		e.GitHubUserID, e.GitHubUsername, e.Repository, e.IssueNumber,
		e.DurationMinutes, nilIfEmpty(e.Description),
		e.SourceCommentID, e.SourceCommentURL, e.CreatedBy,
	)

	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting plan entry: %w", err)
	}
	e.Status = "active"
	return e, nil
}

// FindActive returns the active plan for an issue, or nil if none exists.
func (s *Store) FindActive(ctx context.Context, repo string, issueNumber int) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, github_user_id, github_username, repository, issue_number,
			duration_minutes, description,
			source_comment_id, source_comment_url,
			closing_comment_id, closing_comment_url,
			status, superseded_by, created_by, created_at
		FROM plan_entries
		WHERE repository = $1 AND issue_number = $2 AND status = 'active'
		LIMIT 1`,
		repo, issueNumber,
	)

	return scanEntry(row)
}

// MarkSuperseded flips a plan's status to 'superseded' without setting the
// superseded_by foreign key. This is the first half of a supersede flow —
// freeing the partial-unique-index slot so a new active plan can be inserted
// before the new plan's ID exists.
func (s *Store) MarkSuperseded(ctx context.Context, oldID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plan_entries SET status = 'superseded' WHERE id = $1`,
		oldID,
	)
	if err != nil {
		return fmt.Errorf("marking plan %d superseded: %w", oldID, err)
	}
	return nil
}

// LinkSupersedeChain sets the superseded_by foreign key after the new plan
// has been inserted. Pair with MarkSuperseded.
func (s *Store) LinkSupersedeChain(ctx context.Context, oldID, newID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plan_entries SET superseded_by = $1 WHERE id = $2`,
		newID, oldID,
	)
	if err != nil {
		return fmt.Errorf("linking chain %d -> %d: %w", oldID, newID, err)
	}
	return nil
}

// SoftDelete marks a plan as deleted and records the closing comment.
func (s *Store) SoftDelete(ctx context.Context, planID int64, closingCommentID int64, closingCommentURL string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE plan_entries
		SET status = 'deleted',
		    closing_comment_id = $1,
		    closing_comment_url = $2
		WHERE id = $3`,
		closingCommentID, closingCommentURL, planID,
	)
	if err != nil {
		return fmt.Errorf("soft-deleting plan %d: %w", planID, err)
	}
	return nil
}

// GetByID returns a single plan entry by ID.
func (s *Store) GetByID(ctx context.Context, id int64) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, github_user_id, github_username, repository, issue_number,
			duration_minutes, description,
			source_comment_id, source_comment_url,
			closing_comment_id, closing_comment_url,
			status, superseded_by, created_by, created_at
		FROM plan_entries WHERE id = $1`, id)

	return scanEntry(row)
}

// GetChain walks the supersede chain forwards and backwards from any entry
// in the chain and returns every plan entry in chronological order.
func (s *Store) GetChain(ctx context.Context, id int64) ([]Entry, error) {
	rootID := id
	for {
		row := s.pool.QueryRow(ctx, `
			SELECT id FROM plan_entries WHERE superseded_by = $1`, rootID)
		var prevID int64
		err := row.Scan(&prevID)
		if err == pgx.ErrNoRows {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("walking chain back: %w", err)
		}
		rootID = prevID
	}

	var chain []Entry
	currentID := rootID
	for {
		e, err := s.GetByID(ctx, currentID)
		if err != nil {
			return nil, err
		}
		if e == nil {
			break
		}
		chain = append(chain, *e)
		if e.SupersededBy == nil {
			break
		}
		currentID = *e.SupersededBy
	}
	return chain, nil
}

// ListFilter holds query parameters for listing plan entries.
type ListFilter struct {
	Repo        string
	IssueNumber *int
	Status      string // "active", "all" — default "active"
	DateFrom    *time.Time
	DateTo      *time.Time
}

// List returns plan entries matching the filter, ordered by created_at desc.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Entry, error) {
	query := `
		SELECT id, github_user_id, github_username, repository, issue_number,
			duration_minutes, description,
			source_comment_id, source_comment_url,
			closing_comment_id, closing_comment_url,
			status, superseded_by, created_by, created_at
		FROM plan_entries
		WHERE 1=1`
	args := []any{}
	argN := 1

	if f.Status == "" || f.Status == "active" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, "active")
		argN++
	} else if f.Status != "all" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, f.Status)
		argN++
	}
	if f.Repo != "" {
		query += fmt.Sprintf(" AND repository = $%d", argN)
		args = append(args, f.Repo)
		argN++
	}
	if f.IssueNumber != nil {
		query += fmt.Sprintf(" AND issue_number = $%d", argN)
		args = append(args, *f.IssueNumber)
		argN++
	}
	if f.DateFrom != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, *f.DateFrom)
		argN++
	}
	if f.DateTo != nil {
		query += fmt.Sprintf(" AND created_at < $%d", argN)
		args = append(args, *f.DateTo)
		argN++
	}

	query += " ORDER BY created_at DESC LIMIT 500"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing plan entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		e, err := scanEntryRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, *e)
	}
	return entries, nil
}

// PlanVsActual aggregates plan and log totals for an issue.
type PlanVsActual struct {
	Repository   string `json:"repository"`
	IssueNumber  int    `json:"issue_number"`
	PlannedMins  int    `json:"planned_minutes"`
	LoggedMins   int    `json:"logged_minutes"`
	VarianceMins int    `json:"variance_minutes"`
	Status       string `json:"status"`
	PlanID       *int64 `json:"plan_id,omitempty"`
}

// ComputePlanVsActual returns the plan-vs-actual aggregate for a single issue.
// Logged minutes count active time entries only. Status values:
//   - "no_plan"    — no active plan, regardless of logged minutes
//   - "under"      — logged < planned by more than 5%
//   - "on_target"  — within 5% of planned (in either direction)
//   - "over"       — logged > planned by more than 5%
func (s *Store) ComputePlanVsActual(ctx context.Context, repo string, issueNumber int) (*PlanVsActual, error) {
	pva := &PlanVsActual{
		Repository:  repo,
		IssueNumber: issueNumber,
	}

	// Planned: active plan for the issue
	row := s.pool.QueryRow(ctx, `
		SELECT id, duration_minutes FROM plan_entries
		WHERE repository = $1 AND issue_number = $2 AND status = 'active'
		LIMIT 1`, repo, issueNumber)
	var planID int64
	var planMins int
	err := row.Scan(&planID, &planMins)
	if err == pgx.ErrNoRows {
		// no plan
		planID = 0
		planMins = 0
	} else if err != nil {
		return nil, fmt.Errorf("loading active plan: %w", err)
	} else {
		pva.PlanID = &planID
		pva.PlannedMins = planMins
	}

	// Logged: sum active time entries
	var loggedMins *int
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM time_entries
		WHERE repository = $1 AND issue_number = $2 AND status = 'active'`,
		repo, issueNumber,
	).Scan(&loggedMins)
	if err != nil {
		return nil, fmt.Errorf("summing logged minutes: %w", err)
	}
	if loggedMins != nil {
		pva.LoggedMins = *loggedMins
	}

	pva.VarianceMins = pva.LoggedMins - pva.PlannedMins
	pva.Status = classifyStatus(pva.PlannedMins, pva.LoggedMins)
	return pva, nil
}

func classifyStatus(planned, logged int) string {
	if planned == 0 {
		return "no_plan"
	}
	// 5% tolerance window, rounded outward so single-minute floats don't shift status
	tolerance := planned * 5 / 100
	if tolerance < 1 {
		tolerance = 1
	}
	diff := logged - planned
	switch {
	case diff > tolerance:
		return "over"
	case diff < -tolerance:
		return "under"
	default:
		return "on_target"
	}
}

// --- scanning helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(row rowScanner) (*Entry, error) {
	e := &Entry{}
	var desc, closingURL *string
	var closingID *int64
	err := row.Scan(
		&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
		&e.DurationMinutes, &desc,
		&e.SourceCommentID, &e.SourceCommentURL,
		&closingID, &closingURL,
		&e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning plan entry: %w", err)
	}
	if desc != nil {
		e.Description = *desc
	}
	e.ClosingCommentID = closingID
	e.ClosingCommentURL = closingURL
	return e, nil
}

func scanEntryRow(rows pgx.Rows) (*Entry, error) {
	e := &Entry{}
	var desc, closingURL *string
	var closingID *int64
	err := rows.Scan(
		&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
		&e.DurationMinutes, &desc,
		&e.SourceCommentID, &e.SourceCommentURL,
		&closingID, &closingURL,
		&e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning plan entry: %w", err)
	}
	if desc != nil {
		e.Description = *desc
	}
	e.ClosingCommentID = closingID
	e.ClosingCommentURL = closingURL
	return e, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
