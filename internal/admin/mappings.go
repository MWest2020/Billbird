package admin

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mwesterweel/billbird/internal/auth"
)

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
		Content: h.renderTemplate("mappings.html", map[string]any{
			"Clients":      clients,
			"MappingsHTML": h.renderMappingsTable(r),
		}),
	}
	h.renderLayout(w, data)
}

func (h *Handler) CreateMapping(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	label := r.FormValue("label_pattern")
	repo := r.FormValue("repository")

	clientID, err := strconv.ParseInt(r.FormValue("client_id"), 10, 64)
	if err != nil || label == "" || clientID == 0 {
		http.Error(w, "label and client required", http.StatusBadRequest)
		return
	}

	var repoPtr *string
	if repo != "" {
		repoPtr = &repo
	}

	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO label_mappings (label_pattern, client_id, repository) VALUES ($1, $2, $3)`,
		label, clientID, repoPtr); err != nil {
		log.Printf("error creating mapping: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderMappingsTable(r))
}

func (h *Handler) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM label_mappings WHERE id = $1`, id); err != nil {
		log.Printf("admin delete mapping %d: %v", id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, h.renderMappingsTable(r))
}

func (h *Handler) renderMappingsTable(r *http.Request) template.HTML {
	rows, err := h.pool.Query(r.Context(), `
		SELECT lm.id, lm.label_pattern, c.name, COALESCE(lm.repository, ''), lm.created_at
		FROM label_mappings lm
		JOIN clients c ON c.id = lm.client_id
		ORDER BY lm.label_pattern`)
	if err != nil {
		log.Printf("admin mappings query: %v", err)
		return template.HTML("<p>Error loading mappings</p>")
	}
	defer rows.Close()

	var mappings []mappingView
	for rows.Next() {
		var m mappingView
		if err := rows.Scan(&m.ID, &m.Label, &m.ClientName, &m.Repository, &m.CreatedAt); err != nil {
			log.Printf("admin mappings scan: %v", err)
			continue
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("admin mappings iterate: %v", err)
		return template.HTML("<p>Error loading mappings</p>")
	}
	return h.renderTemplate("mappings_table.html", map[string]any{"Mappings": mappings})
}
