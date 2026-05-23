package admin

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mwesterweel/billbird/internal/apitoken"
	"github.com/mwesterweel/billbird/internal/auth"
)

type tokenRow struct {
	ID         int64
	Label      string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	Revoked    bool
}

func (h *Handler) Tokens(w http.ResponseWriter, r *http.Request) {
	h.renderTokensPage(w, r, "")
}

// renderTokensPage loads the current token list for the session user and
// renders the page. newPlaintext is shown exactly once on the page that the
// generate form returns; it is never persisted, never stored in the URL or a
// cookie, and never logged. Putting the plaintext in a query string (as a
// prior version did) leaked it to reverse-proxy access logs, browser
// history, and Referer headers — see docs/api-tokens.md.
func (h *Handler) renderTokensPage(w http.ResponseWriter, r *http.Request, newPlaintext string) {
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

	// Render the tokens page in-place with the new plaintext. No redirect, so
	// the plaintext never lands in a URL.
	h.renderTokensPage(w, r, t.Plaintext)
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
