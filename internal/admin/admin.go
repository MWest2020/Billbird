// Package admin serves the HTML admin UI for Billbird at /admin/*.
//
// The package is intentionally split across one file per feature area:
//
//	admin.go       — Handler composition root and route registration
//	dashboard.go   — /admin/ time-entry overview and stats
//	plans.go       — /admin/plans plan-vs-actual and plan history
//	tokens.go      — /admin/tokens REST API token lifecycle
//	clients.go     — /admin/clients client list and edits
//	mappings.go    — /admin/mappings label-to-client mappings
//	render.go      — shared layoutData and template helpers
//
// All routes are wrapped in the auth middleware passed to RegisterRoutes;
// the handlers in this package therefore assume an authenticated session.
package admin

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/mwesterweel/billbird/internal/apitoken"
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
