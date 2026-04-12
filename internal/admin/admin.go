package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mwesterweel/billbird/internal/auth"
	"github.com/mwesterweel/billbird/internal/commands"
	"github.com/mwesterweel/billbird/internal/timeentry"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool      *pgxpool.Pool
	entries   *timeentry.Store
	templates *template.Template
}

func NewHandler(pool *pgxpool.Pool, entries *timeentry.Store) (*Handler, error) {
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return &Handler{pool: pool, entries: entries, templates: tmpl}, nil
}

// RegisterRoutes registers admin routes. All routes require auth middleware.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authMiddleware func(http.Handler) http.Handler) {
	wrap := func(fn http.HandlerFunc) http.Handler {
		return authMiddleware(fn)
	}
	mux.Handle("GET /admin/", wrap(h.Dashboard))
	mux.Handle("GET /admin/clients", wrap(h.Clients))
	mux.Handle("POST /admin/clients", wrap(h.CreateClient))
	mux.Handle("PATCH /admin/clients/{id}", wrap(h.UpdateClient))
	mux.Handle("GET /admin/mappings", wrap(h.Mappings))
	mux.Handle("POST /admin/mappings", wrap(h.CreateMapping))
	mux.Handle("DELETE /admin/mappings/{id}", wrap(h.DeleteMapping))
	mux.Handle("GET /admin/partials/entries", wrap(h.EntriesPartial))
}

// --- Dashboard ---

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)

	q := r.URL.Query()
	f := timeentry.ListFilter{
		Status:   q.Get("status"),
		Repo:     q.Get("repo"),
		Username: q.Get("username"),
	}
	if f.Status == "" {
		f.Status = "active"
	}

	filterFrom := q.Get("from")
	filterTo := q.Get("to")
	if filterFrom != "" {
		t, _ := time.Parse("2006-01-02", filterFrom)
		f.DateFrom = &t
	}
	if filterTo != "" {
		t, _ := time.Parse("2006-01-02", filterTo)
		end := t.AddDate(0, 0, 1)
		f.DateTo = &end
	}

	entries, err := h.entries.List(r.Context(), f)
	if err != nil {
		log.Printf("error listing entries: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Stats
	var totalMinutes int
	devs := map[string]bool{}
	for _, e := range entries {
		totalMinutes += e.DurationMinutes
		devs[e.GitHubUsername] = true
	}

	var clientCount int
	h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM clients WHERE active = true`).Scan(&clientCount)

	// Render entries partial
	entriesHTML := h.renderEntriesTable(entries)

	data := layoutData{
		Title:    "Dashboard",
		Active:   "dashboard",
		Username: session.Username,
		Content: h.renderTemplate("dashboard.html", map[string]any{
			"TotalHours":     fmt.Sprintf("%.1f", float64(totalMinutes)/60),
			"EntryCount":     len(entries),
			"DevCount":       len(devs),
			"ClientCount":    clientCount,
			"FilterFrom":     filterFrom,
			"FilterTo":       filterTo,
			"FilterUsername":  q.Get("username"),
			"FilterRepo":     q.Get("repo"),
			"EntriesHTML":    entriesHTML,
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) EntriesPartial(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := timeentry.ListFilter{
		Status:   "active",
		Repo:     q.Get("repo"),
		Username: q.Get("username"),
	}
	if v := q.Get("from"); v != "" {
		t, _ := time.Parse("2006-01-02", v)
		f.DateFrom = &t
	}
	if v := q.Get("to"); v != "" {
		t, _ := time.Parse("2006-01-02", v)
		end := t.AddDate(0, 0, 1)
		f.DateTo = &end
	}

	entries, err := h.entries.List(r.Context(), f)
	if err != nil {
		log.Printf("error listing entries: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderEntriesTable(entries))
}

// --- Clients ---

type clientView struct {
	ID        int64
	Name      string
	Active    bool
	CreatedAt time.Time
}

func (h *Handler) Clients(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)
	data := layoutData{
		Title:    "Clients",
		Active:   "clients",
		Username: session.Username,
		Content:  h.renderTemplate("clients.html", map[string]any{
			"ClientsHTML": h.renderClientsTable(r),
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO clients (name) VALUES ($1)`, name)
	if err != nil {
		log.Printf("error creating client: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderClientsTable(r))
}

func (h *Handler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	// Accept both JSON and form values
	var active *bool
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" || contentType == "" {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if v, ok := req["active"]; ok {
			b, _ := v.(bool)
			active = &b
		}
	}
	// Also check HX-Vals which sends JSON in the body
	if active == nil {
		r.ParseForm()
		if v := r.FormValue("active"); v != "" {
			b := v == "true"
			active = &b
		}
	}

	if active != nil {
		h.pool.Exec(r.Context(),
			`UPDATE clients SET active = $1, updated_at = now() WHERE id = $2`, *active, id)
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderClientsTable(r))
}

// --- Mappings ---

type mappingView struct {
	ID         int64
	Label      string
	ClientName string
	Repository string
	CreatedAt  time.Time
}

func (h *Handler) Mappings(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)

	clients := h.loadClients(r)

	data := layoutData{
		Title:    "Label Mappings",
		Active:   "mappings",
		Username: session.Username,
		Content:  h.renderTemplate("mappings.html", map[string]any{
			"Clients":     clients,
			"MappingsHTML": h.renderMappingsTable(r),
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) CreateMapping(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	label := r.FormValue("label_pattern")
	clientIDStr := r.FormValue("client_id")
	repo := r.FormValue("repository")

	clientID, _ := strconv.ParseInt(clientIDStr, 10, 64)
	if label == "" || clientID == 0 {
		http.Error(w, "label and client required", http.StatusBadRequest)
		return
	}

	var repoPtr *string
	if repo != "" {
		repoPtr = &repo
	}

	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO label_mappings (label_pattern, client_id, repository) VALUES ($1, $2, $3)`,
		label, clientID, repoPtr)
	if err != nil {
		log.Printf("error creating mapping: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderMappingsTable(r))
}

func (h *Handler) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	h.pool.Exec(r.Context(), `DELETE FROM label_mappings WHERE id = $1`, id)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderMappingsTable(r))
}

// --- Rendering helpers ---

type layoutData struct {
	Title    string
	Active   string
	Username string
	Content  template.HTML
}

type entryView struct {
	ID               int64
	CreatedAt        time.Time
	GitHubUsername   string
	Repository       string
	IssueNumber      int
	Duration         string
	Description      string
	Status           string
	SourceCommentURL string
}

func (h *Handler) renderLayout(w http.ResponseWriter, data layoutData) {
	w.Header().Set("Content-Type", "text/html")
	if err := h.templates.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("error rendering layout: %v", err)
	}
}

func (h *Handler) renderTemplate(name string, data any) template.HTML {
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("error rendering %s: %v", name, err)
		return template.HTML("<p>Error rendering template</p>")
	}
	return template.HTML(buf.String())
}

func (h *Handler) renderEntriesTable(entries []timeentry.Entry) template.HTML {
	views := make([]entryView, len(entries))
	for i, e := range entries {
		views[i] = entryView{
			ID:               e.ID,
			CreatedAt:        e.CreatedAt,
			GitHubUsername:   e.GitHubUsername,
			Repository:       e.Repository,
			IssueNumber:      e.IssueNumber,
			Duration:         commands.FormatDuration(e.DurationMinutes),
			Description:      e.Description,
			Status:           e.Status,
			SourceCommentURL: e.SourceCommentURL,
		}
	}
	return h.renderTemplate("entries_table.html", map[string]any{"Entries": views})
}

func (h *Handler) renderClientsTable(r *http.Request) template.HTML {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, active, created_at FROM clients ORDER BY name`)
	if err != nil {
		return template.HTML("<p>Error loading clients</p>")
	}
	defer rows.Close()

	var clients []clientView
	for rows.Next() {
		var c clientView
		rows.Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt)
		clients = append(clients, c)
	}
	return h.renderTemplate("clients_table.html", map[string]any{"Clients": clients})
}

func (h *Handler) renderMappingsTable(r *http.Request) template.HTML {
	rows, err := h.pool.Query(r.Context(), `
		SELECT lm.id, lm.label_pattern, c.name, COALESCE(lm.repository, ''), lm.created_at
		FROM label_mappings lm
		JOIN clients c ON c.id = lm.client_id
		ORDER BY lm.label_pattern`)
	if err != nil {
		return template.HTML("<p>Error loading mappings</p>")
	}
	defer rows.Close()

	var mappings []mappingView
	for rows.Next() {
		var m mappingView
		rows.Scan(&m.ID, &m.Label, &m.ClientName, &m.Repository, &m.CreatedAt)
		mappings = append(mappings, m)
	}
	return h.renderTemplate("mappings_table.html", map[string]any{"Mappings": mappings})
}

func (h *Handler) loadClients(r *http.Request) []clientView {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, active, created_at FROM clients WHERE active = true ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var clients []clientView
	for rows.Next() {
		var c clientView
		rows.Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt)
		clients = append(clients, c)
	}
	return clients
}
