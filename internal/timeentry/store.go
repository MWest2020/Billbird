package timeentry

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID              int64
	GitHubUserID    int64
	GitHubUsername  string
	Repository      string
	IssueNumber     int
	DurationMinutes int
	Description     string
	ClientID        *int64
	SourceCommentID int64
	SourceCommentURL string
	Status          string
	SupersededBy    *int64
	CreatedBy       string
	CreatedAt       time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a new time entry and returns the created entry.
func (s *Store) Create(ctx context.Context, e *Entry) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO time_entries (
			github_user_id, github_username, repository, issue_number,
			duration_minutes, description, client_id,
			source_comment_id, source_comment_url, status, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10)
		RETURNING id, created_at`,
		e.GitHubUserID, e.GitHubUsername, e.Repository, e.IssueNumber,
		e.DurationMinutes, nilIfEmpty(e.Description), e.ClientID,
		e.SourceCommentID, e.SourceCommentURL, e.CreatedBy,
	)

	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting time entry: %w", err)
	}
	e.Status = "active"
	return e, nil
}

// FindLatestActive returns the most recent active entry for a user on an issue.
func (s *Store) FindLatestActive(ctx context.Context, userID int64, repo string, issueNumber int) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, github_user_id, github_username, repository, issue_number,
			duration_minutes, description, client_id,
			source_comment_id, source_comment_url, status, superseded_by, created_by, created_at
		FROM time_entries
		WHERE github_user_id = $1 AND repository = $2 AND issue_number = $3 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID, repo, issueNumber,
	)

	e := &Entry{}
	var desc *string
	err := row.Scan(
		&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
		&e.DurationMinutes, &desc, &e.ClientID,
		&e.SourceCommentID, &e.SourceCommentURL, &e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding latest active entry: %w", err)
	}
	if desc != nil {
		e.Description = *desc
	}
	return e, nil
}

// Supersede marks an entry as superseded and links it to the new entry.
func (s *Store) Supersede(ctx context.Context, oldID, newID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries SET status = 'superseded', superseded_by = $1 WHERE id = $2`,
		newID, oldID,
	)
	if err != nil {
		return fmt.Errorf("superseding entry %d: %w", oldID, err)
	}
	return nil
}

// SoftDelete marks an entry as deleted.
func (s *Store) SoftDelete(ctx context.Context, entryID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries SET status = 'deleted' WHERE id = $1`,
		entryID,
	)
	if err != nil {
		return fmt.Errorf("soft-deleting entry %d: %w", entryID, err)
	}
	return nil
}

// ListFilter holds query parameters for listing time entries.
type ListFilter struct {
	Status   string // "active", "all" — default "active"
	Repo     string
	ClientID *int64
	Username string
	DateFrom *time.Time
	DateTo   *time.Time
}

// List returns time entries matching the filter, ordered by created_at desc.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Entry, error) {
	query := `
		SELECT te.id, te.github_user_id, te.github_username, te.repository, te.issue_number,
			te.duration_minutes, te.description, te.client_id,
			te.source_comment_id, te.source_comment_url, te.status, te.superseded_by, te.created_by, te.created_at
		FROM time_entries te
		WHERE 1=1`
	args := []any{}
	argN := 1

	if f.Status == "" || f.Status == "active" {
		query += fmt.Sprintf(" AND te.status = $%d", argN)
		args = append(args, "active")
		argN++
	}
	if f.Repo != "" {
		query += fmt.Sprintf(" AND te.repository = $%d", argN)
		args = append(args, f.Repo)
		argN++
	}
	if f.ClientID != nil {
		query += fmt.Sprintf(" AND te.client_id = $%d", argN)
		args = append(args, *f.ClientID)
		argN++
	}
	if f.Username != "" {
		query += fmt.Sprintf(" AND te.github_username = $%d", argN)
		args = append(args, f.Username)
		argN++
	}
	if f.DateFrom != nil {
		query += fmt.Sprintf(" AND te.created_at >= $%d", argN)
		args = append(args, *f.DateFrom)
		argN++
	}
	if f.DateTo != nil {
		query += fmt.Sprintf(" AND te.created_at < $%d", argN)
		args = append(args, *f.DateTo)
		argN++
	}

	query += " ORDER BY te.created_at DESC LIMIT 500"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing time entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var desc *string
		if err := rows.Scan(
			&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
			&e.DurationMinutes, &desc, &e.ClientID,
			&e.SourceCommentID, &e.SourceCommentURL, &e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning time entry: %w", err)
		}
		if desc != nil {
			e.Description = *desc
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GetByID returns a single time entry by ID.
func (s *Store) GetByID(ctx context.Context, id int64) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, github_user_id, github_username, repository, issue_number,
			duration_minutes, description, client_id,
			source_comment_id, source_comment_url, status, superseded_by, created_by, created_at
		FROM time_entries WHERE id = $1`, id)

	e := &Entry{}
	var desc *string
	err := row.Scan(
		&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
		&e.DurationMinutes, &desc, &e.ClientID,
		&e.SourceCommentID, &e.SourceCommentURL, &e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting entry %d: %w", id, err)
	}
	if desc != nil {
		e.Description = *desc
	}
	return e, nil
}

// GetCorrectionChain returns the full chain for an entry (all entries linked via superseded_by).
func (s *Store) GetCorrectionChain(ctx context.Context, id int64) ([]Entry, error) {
	// Walk backwards to find the root
	rootID := id
	for {
		row := s.pool.QueryRow(ctx, `
			SELECT id FROM time_entries
			WHERE superseded_by = $1`, rootID)
		var prevID int64
		err := row.Scan(&prevID)
		if err == pgx.ErrNoRows {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("walking chain: %w", err)
		}
		rootID = prevID
	}

	// Walk forward from root
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

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
