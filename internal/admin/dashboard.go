package admin

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/mwesterweel/billbird/internal/auth"
	"github.com/mwesterweel/billbird/internal/commands"
	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

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
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM clients WHERE active = true`).Scan(&clientCount); err != nil {
		log.Printf("admin dashboard: counting clients: %v", err)
		// non-fatal: render the dashboard with clientCount=0 instead of failing
	}

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
			"FilterUsername": q.Get("username"),
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
