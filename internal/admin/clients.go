package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mwesterweel/billbird/internal/auth"
)

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
		Content: h.renderTemplate("clients.html", map[string]any{
			"ClientsHTML": h.renderClientsTable(r),
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO clients (name) VALUES ($1)`, name); err != nil {
		log.Printf("error creating client: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderClientsTable(r))
}

func (h *Handler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Accept both JSON and form values.
	var active *bool
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" || contentType == "" {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			log.Printf("admin update client: decode body: %v", err)
		}
		if v, ok := req["active"]; ok {
			b, _ := v.(bool)
			active = &b
		}
	}
	if active == nil {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if v := r.FormValue("active"); v != "" {
			b := v == "true"
			active = &b
		}
	}

	if active != nil {
		tag, err := h.pool.Exec(r.Context(),
			`UPDATE clients SET active = $1, updated_at = now() WHERE id = $2`, *active, id)
		if err != nil {
			log.Printf("admin update client %d: %v", id, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderClientsTable(r))
}

func (h *Handler) renderClientsTable(r *http.Request) template.HTML {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, active, created_at FROM clients ORDER BY name`)
	if err != nil {
		log.Printf("admin clients query: %v", err)
		return template.HTML("<p>Error loading clients</p>")
	}
	defer rows.Close()

	var clients []clientView
	for rows.Next() {
		var c clientView
		if err := rows.Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt); err != nil {
			log.Printf("admin clients scan: %v", err)
			continue
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("admin clients iterate: %v", err)
		return template.HTML("<p>Error loading clients</p>")
	}
	return h.renderTemplate("clients_table.html", map[string]any{"Clients": clients})
}

// loadClients returns only the active clients, for select-dropdowns in other
// admin pages (e.g. the mapping-create form).
func (h *Handler) loadClients(r *http.Request) []clientView {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, active, created_at FROM clients WHERE active = true ORDER BY name`)
	if err != nil {
		log.Printf("admin load clients query: %v", err)
		return nil
	}
	defer rows.Close()

	var clients []clientView
	for rows.Next() {
		var c clientView
		if err := rows.Scan(&c.ID, &c.Name, &c.Active, &c.CreatedAt); err != nil {
			log.Printf("admin load clients scan: %v", err)
			continue
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("admin load clients iterate: %v", err)
	}
	return clients
}
