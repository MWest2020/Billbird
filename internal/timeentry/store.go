package timeentry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID               int64
	GitHubUserID     int64
	GitHubUsername   string
	Repository       string
	IssueNumber      int
	DurationMinutes  int
	Description      string
	ClientID         *int64
	SourceCommentID  int64
	SourceCommentURL string
	Status           string
	SupersededBy     *int64
	CreatedBy        string
	CreatedAt        time.Time
	// Labels is a snapshot of the issue's GitHub labels at the moment
	// this entry was created. Never NULL — empty issues serialise as [].
	// Used for arbitrary slicing in reports (strippenkaart, wbso, etc.).
	// JSON tag keeps the response shape lowercase to align with the
	// established API convention for new fields.
	Labels []string `json:"labels"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// columns enumerates the time_entries columns scanned by every Get/List
// path. Kept as a constant so adding a column does not drift between
// SELECT and Scan.
const columns = `id, github_user_id, github_username, repository, issue_number,
	duration_minutes, description, client_id,
	source_comment_id, source_comment_url, status, superseded_by, created_by, created_at, labels`

// Create inserts a new time entry and returns the created entry.
func (s *Store) Create(ctx context.Context, e *Entry) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO time_entries (
			github_user_id, github_username, repository, issue_number,
			duration_minutes, description, client_id,
			source_comment_id, source_comment_url, status, created_by, labels
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11)
		RETURNING id, created_at`,
		e.GitHubUserID, e.GitHubUsername, e.Repository, e.IssueNumber,
		e.DurationMinutes, nilIfEmpty(e.Description), e.ClientID,
		e.SourceCommentID, e.SourceCommentURL, e.CreatedBy, labelsForInsert(e.Labels),
	)

	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting time entry: %w", err)
	}
	e.Status = "active"
	return e, nil
}

// labelsForInsert returns the slice that should land in the labels
// column. Postgres `text[] NOT NULL DEFAULT '{}'` rejects NULL, so we
// coerce nil to an empty slice here once.
func labelsForInsert(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// rowScanner abstracts pgx.Row and pgx.Rows so scanEntry handles both.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanEntry decodes a row produced by SELECT columns ... into an Entry.
// Returns (nil, nil) on pgx.ErrNoRows.
func scanEntry(row rowScanner) (*Entry, error) {
	e := &Entry{}
	var desc *string
	err := row.Scan(
		&e.ID, &e.GitHubUserID, &e.GitHubUsername, &e.Repository, &e.IssueNumber,
		&e.DurationMinutes, &desc, &e.ClientID,
		&e.SourceCommentID, &e.SourceCommentURL, &e.Status, &e.SupersededBy, &e.CreatedBy, &e.CreatedAt, &e.Labels,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning time entry: %w", err)
	}
	if desc != nil {
		e.Description = *desc
	}
	if e.Labels == nil {
		e.Labels = []string{}
	}
	return e, nil
}

// FindLatestActive returns the most recent active entry for a user on an issue.
func (s *Store) FindLatestActive(ctx context.Context, userID int64, repo string, issueNumber int) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+columns+`
		FROM time_entries
		WHERE github_user_id = $1 AND repository = $2 AND issue_number = $3 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID, repo, issueNumber,
	)
	return scanEntry(row)
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
	Status      string // "active", "all" — default "active"
	Repo        string
	ClientID    *int64
	Username    string
	DateFrom    *time.Time
	DateTo      *time.Time
	Labels      []string // AND semantics — every label must be present on the entry
	LabelPrefix string   // entry has at least one label starting with this prefix
}

// List returns time entries matching the filter, ordered by created_at desc.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Entry, error) {
	query := `SELECT ` + columns + ` FROM time_entries WHERE 1=1`
	args := []any{}
	argN := 1

	if f.Status == "" || f.Status == "active" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, "active")
		argN++
	}
	if f.Repo != "" {
		query += fmt.Sprintf(" AND repository = $%d", argN)
		args = append(args, f.Repo)
		argN++
	}
	if f.ClientID != nil {
		query += fmt.Sprintf(" AND client_id = $%d", argN)
		args = append(args, *f.ClientID)
		argN++
	}
	if f.Username != "" {
		query += fmt.Sprintf(" AND github_username = $%d", argN)
		args = append(args, f.Username)
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
		return nil, fmt.Errorf("listing time entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, *e)
	}
	return entries, nil
}

// GetByID returns a single time entry by ID.
func (s *Store) GetByID(ctx context.Context, id int64) (*Entry, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+columns+` FROM time_entries WHERE id = $1`, id)
	return scanEntry(row)
}

// GetCorrectionChain returns the full chain for an entry (all entries linked via superseded_by).
func (s *Store) GetCorrectionChain(ctx context.Context, id int64) ([]Entry, error) {
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
