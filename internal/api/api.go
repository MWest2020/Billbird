package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mwesterweel/billbird/internal/commands"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

type Handler struct {
	pool        *pgxpool.Pool
	timeEntries *timeentry.Store
}

func NewHandler(pool *pgxpool.Pool, timeEntries *timeentry.Store) *Handler {
	return &Handler{pool: pool, timeEntries: timeEntries}
}

// RegisterRoutes registers all API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/time-entries", h.ListTimeEntries)
	mux.HandleFunc("GET /api/v1/time-entries/{id}", h.GetTimeEntry)
	mux.HandleFunc("GET /api/v1/time-entries/{id}/chain", h.GetCorrectionChain)
	mux.HandleFunc("GET /api/v1/clients", h.ListClients)
	mux.HandleFunc("POST /api/v1/clients", h.CreateClient)
	mux.HandleFunc("PATCH /api/v1/clients/{id}", h.UpdateClient)
	mux.HandleFunc("GET /api/v1/label-mappings", h.ListLabelMappings)
	mux.HandleFunc("POST /api/v1/label-mappings", h.CreateLabelMapping)
	mux.HandleFunc("DELETE /api/v1/label-mappings/{id}", h.DeleteLabelMapping)
	mux.HandleFunc("GET /api/v1/export/csv", h.ExportCSV)
}

// --- Time Entries ---

func (h *Handler) ListTimeEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := timeentry.ListFilter{
		Status:   q.Get("status"),
		Repo:     q.Get("repo"),
		Username: q.Get("username"),
	}

	if v := q.Get("client_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			jsonError(w, "invalid client_id", http.StatusBadRequest)
			return
		}
		f.ClientID = &id
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			jsonError(w, "invalid from date, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		f.DateFrom = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			jsonError(w, "invalid to date, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		// Include the full day
		end := t.AddDate(0, 0, 1)
		f.DateTo = &end
	}

	entries, err := h.timeEntries.List(r.Context(), f)
	if err != nil {
		log.Printf("error listing time entries: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, entries)
}

func (h *Handler) GetTimeEntry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	entry, err := h.timeEntries.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("error getting entry: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if entry == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, entry)
}

func (h *Handler) GetCorrectionChain(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	chain, err := h.timeEntries.GetCorrectionChain(r.Context(), id)
	if err != nil {
		log.Printf("error getting chain: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, chain)
}

// --- Clients ---

type clientRow struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, active, created_at, updated_at FROM clients ORDER BY name`)
	if err != nil {
		log.Printf("error listing clients: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var clients []clientRow
	for rows.Next() {
		var c clientRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt, &c.UpdatedAt); err != nil {
			log.Printf("error scanning client: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		clients = append(clients, c)
	}
	writeJSON(w, clients)
}

func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}

	var c clientRow
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO clients (name) VALUES ($1) RETURNING id, name, active, created_at, updated_at`,
		req.Name).Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		log.Printf("error creating client: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, c)
}

func (h *Handler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req struct {
		Name   *string `json:"name"`
		Active *bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name != nil {
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE clients SET name = $1, updated_at = now() WHERE id = $2`, *req.Name, id); err != nil {
			log.Printf("error updating client name: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	if req.Active != nil {
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE clients SET active = $1, updated_at = now() WHERE id = $2`, *req.Active, id); err != nil {
			log.Printf("error updating client active: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	var c clientRow
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, active, created_at, updated_at FROM clients WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, c)
}

// --- Label Mappings ---

type labelMappingRow struct {
	ID         int64     `json:"id"`
	Label      string    `json:"label_pattern"`
	ClientID   int64     `json:"client_id"`
	ClientName string    `json:"client_name,omitempty"`
	Repository *string   `json:"repository"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handler) ListLabelMappings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT lm.id, lm.label_pattern, lm.client_id, c.name, lm.repository, lm.created_at
		FROM label_mappings lm
		JOIN clients c ON c.id = lm.client_id
		ORDER BY lm.label_pattern`)
	if err != nil {
		log.Printf("error listing mappings: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var mappings []labelMappingRow
	for rows.Next() {
		var m labelMappingRow
		if err := rows.Scan(&m.ID, &m.Label, &m.ClientID, &m.ClientName, &m.Repository, &m.CreatedAt); err != nil {
			log.Printf("error scanning mapping: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		mappings = append(mappings, m)
	}
	writeJSON(w, mappings)
}

func (h *Handler) CreateLabelMapping(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label      string  `json:"label_pattern"`
		ClientID   int64   `json:"client_id"`
		Repository *string `json:"repository"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" || req.ClientID == 0 {
		jsonError(w, "label_pattern and client_id are required", http.StatusBadRequest)
		return
	}

	var m labelMappingRow
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO label_mappings (label_pattern, client_id, repository)
		VALUES ($1, $2, $3)
		RETURNING id, label_pattern, client_id, repository, created_at`,
		req.Label, req.ClientID, req.Repository).
		Scan(&m.ID, &m.Label, &m.ClientID, &m.Repository, &m.CreatedAt)
	if err != nil {
		log.Printf("error creating mapping: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, m)
}

func (h *Handler) DeleteLabelMapping(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	tag, err := h.pool.Exec(r.Context(), `DELETE FROM label_mappings WHERE id = $1`, id)
	if err != nil {
		log.Printf("error deleting mapping: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- CSV Export ---

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := timeentry.ListFilter{
		Status:   "active",
		Repo:     q.Get("repo"),
		Username: q.Get("username"),
	}

	if v := q.Get("client_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			f.ClientID = &id
		}
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			f.DateFrom = &t
		}
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			end := t.AddDate(0, 0, 1)
			f.DateTo = &end
		}
	}

	entries, err := h.timeEntries.List(r.Context(), f)
	if err != nil {
		log.Printf("error exporting: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Build client name lookup
	clientNames := map[int64]string{}
	rows, err := h.pool.Query(r.Context(), `SELECT id, name FROM clients`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err == nil {
				clientNames[id] = name
			}
		}
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=billbird-export.csv")

	cw := csv.NewWriter(w)
	cw.Write([]string{"date", "developer", "repository", "issue", "client", "duration", "description", "entry_id"})

	for _, e := range entries {
		clientName := ""
		if e.ClientID != nil {
			clientName = clientNames[*e.ClientID]
		}
		cw.Write([]string{
			e.CreatedAt.Format("2006-01-02"),
			e.GitHubUsername,
			e.Repository,
			fmt.Sprintf("#%d", e.IssueNumber),
			clientName,
			commands.FormatDuration(e.DurationMinutes),
			e.Description,
			fmt.Sprintf("%d", e.ID),
		})
	}
	cw.Flush()
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if v == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
