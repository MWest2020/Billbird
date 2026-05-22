package admin

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mwesterweel/billbird/internal/apitoken"
	"github.com/mwesterweel/billbird/internal/auth"
	"github.com/mwesterweel/billbird/internal/commands"
	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Handler struct {
	pool      *pgxpool.Pool
	entries   *timeentry.Store
	plans     *planentry.Store
	tokens    *apitoken.Store
	templates *template.Template
}

func NewHandler(pool *pgxpool.Pool, entries *timeentry.Store, plans *planentry.Store, tokens *apitoken.Store) (*Handler, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	return &Handler{pool: pool, entries: entries, plans: plans, tokens: tokens, templates: tmpl}, nil
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
	mux.Handle("GET /admin/plans", wrap(h.Plans))
	mux.Handle("GET /admin/plans/{repo}/{number}", wrap(h.PlanHistory))
	mux.Handle("GET /admin/tokens", wrap(h.Tokens))
	mux.Handle("POST /admin/tokens", wrap(h.CreateTokenForm))
	mux.Handle("POST /admin/tokens/{id}/revoke", wrap(h.RevokeTokenForm))
}

// --- Plans ---

type planView struct {
	ID               int64
	CreatedAt        time.Time
	GitHubUsername   string
	Repository       string
	IssueNumber      int
	Duration         string
	Description      string
	Status           string
	SourceCommentURL string
	ClosingURL       string
}

type planVsActualView struct {
	Repository  string
	IssueNumber int
	Planned     string
	Logged      string
	Variance    string
	StatusBadge string // under | on_target | over | no_plan
}

func (h *Handler) Plans(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)
	if h.plans == nil {
		http.Error(w, "plans not configured", http.StatusServiceUnavailable)
		return
	}

	plans, err := h.plans.List(r.Context(), planentry.ListFilter{Status: "active"})
	if err != nil {
		log.Printf("admin plans list error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Compute plan-vs-actual rows
	rows := make([]planVsActualView, 0, len(plans))
	for _, p := range plans {
		pva, err := h.plans.ComputePlanVsActual(r.Context(), p.Repository, p.IssueNumber)
		if err != nil {
			log.Printf("plan-vs-actual error %s#%d: %v", p.Repository, p.IssueNumber, err)
			continue
		}
		rows = append(rows, planVsActualView{
			Repository:  pva.Repository,
			IssueNumber: pva.IssueNumber,
			Planned:     commands.FormatDuration(pva.PlannedMins),
			Logged:      commands.FormatDuration(pva.LoggedMins),
			Variance:    formatVariance(pva.VarianceMins),
			StatusBadge: pva.Status,
		})
	}

	data := layoutData{
		Title:    "Plans",
		Active:   "plans",
		Username: session.Username,
		Content: h.renderTemplate("plans.html", map[string]any{
			"Rows": rows,
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) PlanHistory(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)
	if h.plans == nil {
		http.Error(w, "plans not configured", http.StatusServiceUnavailable)
		return
	}
	repoEnc := r.PathValue("repo")
	repo, err := decodeRepo(repoEnc)
	if err != nil {
		http.Error(w, "invalid repo", http.StatusBadRequest)
		return
	}
	issueNumber, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "invalid issue number", http.StatusBadRequest)
		return
	}

	// Get the latest plan entry (active or otherwise) for the issue, then chain from it.
	plans, err := h.plans.List(r.Context(), planentry.ListFilter{
		Repo:        repo,
		IssueNumber: &issueNumber,
		Status:      "all",
	})
	if err != nil {
		log.Printf("admin plan history list error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(plans) == 0 {
		http.Error(w, "no plans for this issue", http.StatusNotFound)
		return
	}

	chain, err := h.plans.GetChain(r.Context(), plans[0].ID)
	if err != nil {
		log.Printf("admin plan chain error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	views := make([]planView, len(chain))
	for i, p := range chain {
		v := planView{
			ID:               p.ID,
			CreatedAt:        p.CreatedAt,
			GitHubUsername:   p.GitHubUsername,
			Repository:       p.Repository,
			IssueNumber:      p.IssueNumber,
			Duration:         commands.FormatDuration(p.DurationMinutes),
			Description:      p.Description,
			Status:           p.Status,
			SourceCommentURL: p.SourceCommentURL,
		}
		if p.ClosingCommentURL != nil {
			v.ClosingURL = *p.ClosingCommentURL
		}
		views[i] = v
	}

	data := layoutData{
		Title:    "Plan history",
		Active:   "plans",
		Username: session.Username,
		Content: h.renderTemplate("plan_history.html", map[string]any{
			"Plans":       views,
			"Repository":  repo,
			"IssueNumber": issueNumber,
		}),
	}
	h.renderLayout(w, data)
}

// decodeRepo accepts either a path-style "owner__repo" or a literal
// "owner/repo" wrapped as a path segment (which works on Go 1.22+ as
// "owner/repo" because of pattern matching). The encoded form uses a
// double-underscore separator for safety with the path router.
func decodeRepo(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	if strings.Contains(s, "/") {
		return s, nil
	}
	if i := strings.Index(s, "__"); i > 0 {
		return s[:i] + "/" + s[i+2:], nil
	}
	return "", fmt.Errorf("malformed repo %q", s)
}

func formatVariance(minutes int) string {
	if minutes == 0 {
		return "0m"
	}
	if minutes > 0 {
		return "+" + commands.FormatDuration(minutes)
	}
	return "-" + commands.FormatDuration(-minutes)
}

// --- Tokens ---

type tokenRow struct {
	ID         int64
	Label      string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	Revoked    bool
}

func (h *Handler) Tokens(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)
	if h.tokens == nil {
		http.Error(w, "tokens not configured", http.StatusServiceUnavailable)
		return
	}

	rows, err := h.tokens.ListForUser(r.Context(), session.UserID)
	if err != nil {
		log.Printf("admin tokens list error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	views := make([]tokenRow, len(rows))
	for i, t := range rows {
		views[i] = tokenRow{
			ID:         t.ID,
			Label:      t.Label,
			Prefix:     t.Prefix,
			CreatedAt:  t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
			Revoked:    t.Revoked,
		}
	}

	// A freshly minted plaintext is delivered via the query string
	// exactly once. We never persist it; we never reload it.
	newPlaintext := r.URL.Query().Get("new_token")

	data := layoutData{
		Title:    "API tokens",
		Active:   "tokens",
		Username: session.Username,
		Content: h.renderTemplate("tokens.html", map[string]any{
			"Tokens":       views,
			"NewPlaintext": newPlaintext,
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) CreateTokenForm(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)
	if h.tokens == nil {
		http.Error(w, "tokens not configured", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}

	t, err := h.tokens.Generate(r.Context(), session.UserID, session.Username, label)
	if err != nil {
		log.Printf("admin token generate error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Redirect back to the listing carrying the plaintext exactly once.
	http.Redirect(w, r, "/admin/tokens?new_token="+t.Plaintext, http.StatusSeeOther)
}

func (h *Handler) RevokeTokenForm(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSession(r)
	if h.tokens == nil {
		http.Error(w, "tokens not configured", http.StatusServiceUnavailable)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	target, err := h.tokens.GetByID(r.Context(), id)
	if err == apitoken.ErrTokenNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("admin token lookup error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if target.GitHubUserID != session.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := h.tokens.Revoke(r.Context(), id, session.Username); err != nil {
		log.Printf("admin token revoke error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/tokens", http.StatusSeeOther)
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
	entriesHTML := h.renderEntriesTable(r.Context(), entries)

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
	fmt.Fprint(w, h.renderEntriesTable(r.Context(), entries))
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
	PlanBadge        string // under | on_target | over | no_plan | "" when plans not loaded
	PlanSummary      string // e.g. "8h / 6h"
	Labels           []string
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

func (h *Handler) renderEntriesTable(ctx context.Context, entries []timeentry.Entry) template.HTML {
	views := make([]entryView, len(entries))

	// Deduplicate (repo, issue) lookups so a dashboard page with many entries
	// on the same issue only hits the database once per issue.
	type issueKey struct {
		Repo string
		Num  int
	}
	cache := map[issueKey]planentry.PlanVsActual{}
	if h.plans != nil {
		for _, e := range entries {
			key := issueKey{Repo: e.Repository, Num: e.IssueNumber}
			if _, ok := cache[key]; ok {
				continue
			}
			pva, err := h.plans.ComputePlanVsActual(ctx, e.Repository, e.IssueNumber)
			if err != nil {
				log.Printf("plan-vs-actual for %s#%d: %v", e.Repository, e.IssueNumber, err)
				continue
			}
			cache[key] = *pva
		}
	}

	for i, e := range entries {
		v := entryView{
			ID:               e.ID,
			CreatedAt:        e.CreatedAt,
			GitHubUsername:   e.GitHubUsername,
			Repository:       e.Repository,
			IssueNumber:      e.IssueNumber,
			Duration:         commands.FormatDuration(e.DurationMinutes),
			Description:      e.Description,
			Status:           e.Status,
			SourceCommentURL: e.SourceCommentURL,
			Labels:           e.Labels,
		}
		if pva, ok := cache[issueKey{Repo: e.Repository, Num: e.IssueNumber}]; ok {
			v.PlanBadge = pva.Status
			if pva.Status == "no_plan" {
				v.PlanSummary = fmt.Sprintf("— / %s", commands.FormatDuration(pva.LoggedMins))
			} else {
				v.PlanSummary = fmt.Sprintf("%s / %s",
					commands.FormatDuration(pva.PlannedMins),
					commands.FormatDuration(pva.LoggedMins))
			}
		}
		views[i] = v
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
