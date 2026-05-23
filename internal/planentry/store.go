package planentry

import (
	"context"
	"fmt"
	"strings"
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
	// Labels snapshots the issue's GitHub labels at create-time. Empty
	// array if the issue had none; never nil. Lowercase JSON tag aligns
	// with the API convention for new fields.
	Labels []string `json:"labels"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// columns lists every column scanned by Get / List paths. Keep
// SELECT lists and Scan order in sync via this single constant.
const columns = `id, github_user_id, github_username, repository, issue_number,
	duration_minutes, description,
	source_comment_id, source_comment_url,
	closing_comment_id, closing_comment_url,
	status, superseded_by, created_by, created_at, labels`

// Create inserts a new plan entry and returns it. The caller is responsible
// for superseding any prior active plan first if needed.
func (s *Store) Create(ctx context.Context, e *Entry) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO plan_entries (
			github_user_id, github_username, repository, issue_number,
			duration_minutes, description,
			source_comment_id, source_comment_url, status, created_by, labels
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10)
		RETURNING id, created_at`,
		e.GitHubUserID, e.GitHubUsername, e.Repository, e.IssueNumber,
		e.DurationMinutes, nilIfEmpty(e.Description),
		e.SourceCommentID, e.SourceCommentURL, e.CreatedBy, labelsForInsert(e.Labels),
	)

	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting plan entry: %w", err)
	}
	e.Status = "active"
	return e, nil
}

func labelsForInsert(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// FindActive returns the active plan for an issue, or nil if none exists.
func (s *Store) FindActive(ctx context.Context, repo string, issueNumber int) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+columns+`
		FROM plan_entries
		WHERE repository = $1 AND issue_number = $2 AND status = 'active'
		LIMIT 1`,
		repo, issueNumber,
	)
	return scanEntry(row)
}

// ReplacePlan supersedes any active plan on (repo, issueNumber) and creates
// the new one in a single transaction. When prevID is non-nil the previous
// plan is flipped to 'superseded' first, the new plan inserted, then the
// superseded_by foreign key linked — all-or-nothing. A process crash or DB
// error between the steps leaves the database unchanged.
//
// Pass prevID == nil for a fresh plan; ReplacePlan then degenerates to a
// plain insert wrapped in a (cheap) one-statement transaction. The handler
// no longer has to think about the supersede dance.
func (s *Store) ReplacePlan(ctx context.Context, prevID *int64, e *Entry) (*Entry, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after Commit

	if prevID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE plan_entries SET status = 'superseded' WHERE id = $1`,
			*prevID,
		); err != nil {
			return nil, fmt.Errorf("marking plan %d superseded: %w", *prevID, err)
		}
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO plan_entries (
			github_user_id, github_username, repository, issue_number,
			duration_minutes, description,
			source_comment_id, source_comment_url, status, created_by, labels
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10)
		RETURNING id, created_at`,
		e.GitHubUserID, e.GitHubUsername, e.Repository, e.IssueNumber,
		e.DurationMinutes, nilIfEmpty(e.Description),
		e.SourceCommentID, e.SourceCommentURL, e.CreatedBy, labelsForInsert(e.Labels),
	)
	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting plan entry: %w", err)
	}
	e.Status = "active"

	if prevID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE plan_entries SET superseded_by = $1 WHERE id = $2`,
			e.ID, *prevID,
		); err != nil {
			return nil, fmt.Errorf("linking chain %d -> %d: %w", *prevID, e.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return e, nil
}

// MarkSuperseded flips a plan's status to 'superseded' without setting the
// superseded_by foreign key. This is the first half of a supersede flow —
// freeing the partial-unique-index slot so a new active plan can be inserted
// before the new plan's ID exists.
//
// Deprecated: callers should prefer ReplacePlan, which wraps the whole
// supersede dance in a transaction. This method is kept for any direct test
// or admin use that needs the intermediate state.
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
	row := s.pool.QueryRow(ctx, `SELECT `+columns+` FROM plan_entries WHERE id = $1`, id)
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
	Labels      []string // AND containment
	LabelPrefix string   // any label starts with this prefix
}

// List returns plan entries matching the filter, ordered by created_at desc.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Entry, error) {
	query := `SELECT ` + columns + ` FROM plan_entries WHERE 1=1`
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
	if len(f.Labels) > 0 {
		query += fmt.Sprintf(" AND labels @> $%d", argN)
		args = append(args, f.Labels)
		argN++
	}
	if prefix := strings.TrimSpace(f.LabelPrefix); prefix != "" {
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM unnest(labels) l WHERE l LIKE $%d)", argN)
		args = append(args, prefix+"%")
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
func (s *Store) ComputePlanVsActual(ctx context.Context, repo string, issueNumber int) (*PlanVsActual, error) {
	pva := &PlanVsActual{
		Repository:  repo,
		IssueNumber: issueNumber,
	}

	row := s.pool.QueryRow(ctx, `
		SELECT id, duration_minutes FROM plan_entries
		WHERE repository = $1 AND issue_number = $2 AND status = 'active'
		LIMIT 1`, repo, issueNumber)
	var planID int64
	var planMins int
	err := row.Scan(&planID, &planMins)
	if err == pgx.ErrNoRows {
		planID = 0
		planMins = 0
	} else if err != nil {
		return nil, fmt.Errorf("loading active plan: %w", err)
	} else {
		pva.PlanID = &planID
		pva.PlannedMins = planMins
	}

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
	if err := scanInto(row, e); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func scanEntryRow(rows pgx.Rows) (*Entry, error) {
	e := &Entry{}
	if err := scanInto(rows, e); err != nil {
		return nil, err
	}
	return e, nil
}

// scanInto centralises the scan-and-coerce dance. Description and the
// closing-comment fields stay nullable in the DB but become empty
// strings / nil pointers on the struct.
func scanInto(row rowScanner, e *Entry) error {
	var desc, closingURL *string
	var closingID *int64
	err := row.Scan(
		&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
		&e.DurationMinutes, &desc,
		&e.SourceCommentID, &e.SourceCommentURL,
		&closingID, &closingURL,
		&e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt, &e.Labels,
	)
	if err == pgx.ErrNoRows {
		return pgx.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("scanning plan entry: %w", err)
	}
	if desc != nil {
		e.Description = *desc
	}
	e.ClosingCommentID = closingID
	e.ClosingCommentURL = closingURL
	if e.Labels == nil {
		e.Labels = []string{}
	}
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
