package admin

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mwesterweel/billbird/internal/auth"
	"github.com/mwesterweel/billbird/internal/commands"
	"github.com/mwesterweel/billbird/internal/planentry"
)

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
