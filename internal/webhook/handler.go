package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/mwesterweel/billbird/internal/client"
	"github.com/mwesterweel/billbird/internal/commands"
	gh "github.com/mwesterweel/billbird/internal/github"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

type Handler struct {
	webhookSecret  string
	allowedOrgs    []string
	deliveries     *DeliveryStore
	ghClient       *gh.Client
	timeEntries    *timeentry.Store
	clientResolver *client.Resolver
}

func NewHandler(
	webhookSecret string,
	allowedOrgs []string,
	deliveries *DeliveryStore,
	ghClient *gh.Client,
	timeEntries *timeentry.Store,
) *Handler {
	return &Handler{
		webhookSecret: webhookSecret,
		allowedOrgs:   allowedOrgs,
		deliveries:    deliveries,
		ghClient:      ghClient,
		timeEntries:   timeEntries,
	}
}

// SetClientResolver sets the client resolver. Called after construction because
// of the circular dependency between handler and resolver sharing the pool.
func (h *Handler) SetClientResolver(r *client.Resolver) {
	h.clientResolver = r
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature
	if !h.verifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Check idempotency
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID != "" {
		processed, err := h.deliveries.IsProcessed(r.Context(), deliveryID)
		if err != nil {
			log.Printf("error checking delivery: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if processed {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")

	// Route by event type
	switch eventType {
	case "issue_comment":
		h.handleIssueComment(r.Context(), body)
	case "projects_v2_item":
		// cycle time tracking — not yet implemented
	case "pull_request":
		// cycle time tracking — not yet implemented
	default:
		// Unknown event, acknowledge and ignore
	}

	// Mark delivery as processed
	if deliveryID != "" {
		if err := h.deliveries.MarkProcessed(r.Context(), deliveryID, eventType); err != nil {
			log.Printf("error marking delivery processed: %v", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) verifySignature(body []byte, signature string) bool {
	if h.webhookSecret == "" {
		return false
	}

	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

type issueCommentEvent struct {
	Action  string `json:"action"`
	Comment struct {
		ID      int64  `json:"id"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		User    struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
	Issue struct {
		Number int `json:"number"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func (h *Handler) handleIssueComment(ctx context.Context, body []byte) {
	var event issueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("error parsing issue_comment event: %v", err)
		return
	}

	if event.Action != "created" {
		return
	}

	cmd, err := commands.Parse(event.Comment.Body)
	if err != nil {
		h.postError(ctx, event, err.Error())
		return
	}
	if cmd == nil {
		return // not a billbird command
	}

	// Check org membership before processing any command
	if !h.isAuthorized(event) {
		h.postError(ctx, event, fmt.Sprintf(
			"@%s is not a member of an authorized organization. Contact your admin.",
			event.Comment.User.Login))
		return
	}

	switch cmd.Type {
	case commands.CmdLog:
		h.handleLog(ctx, event, cmd)
	case commands.CmdCorrect:
		h.handleCorrect(ctx, event, cmd)
	case commands.CmdDelete:
		h.handleDelete(ctx, event)
	}
}

func (h *Handler) handleLog(ctx context.Context, event issueCommentEvent, cmd *commands.Command) {
	repo := event.Repository.FullName
	installID := event.Installation.ID

	// Resolve client from labels
	var clientID *int64
	if h.clientResolver != nil {
		labels, err := h.ghClient.GetIssueLabels(installID, repo, event.Issue.Number)
		if err != nil {
			log.Printf("error fetching labels for %s#%d: %v", repo, event.Issue.Number, err)
			// Continue without client attribution
		} else {
			clientID, err = h.clientResolver.ResolveClient(ctx, labels, repo)
			if err != nil {
				log.Printf("error resolving client for %s#%d: %v", repo, event.Issue.Number, err)
			}
		}
	}

	entry := &timeentry.Entry{
		GitHubUserID:     event.Comment.User.ID,
		GitHubUsername:   event.Comment.User.Login,
		Repository:       repo,
		IssueNumber:      event.Issue.Number,
		DurationMinutes:  cmd.Minutes,
		Description:      cmd.Description,
		ClientID:         clientID,
		SourceCommentID:  event.Comment.ID,
		SourceCommentURL: event.Comment.HTMLURL,
		CreatedBy:        "user",
	}

	entry, err := h.timeEntries.Create(ctx, entry)
	if err != nil {
		log.Printf("error creating time entry: %v", err)
		h.postError(ctx, event, "Failed to log time. Please try again.")
		return
	}

	comment := fmt.Sprintf("Logged %s for @%s (entry #%d)",
		commands.FormatDuration(cmd.Minutes), event.Comment.User.Login, entry.ID)
	if cmd.Description != "" {
		comment += fmt.Sprintf(" — %s", cmd.Description)
	}

	if _, err := h.ghClient.PostComment(installID, repo, event.Issue.Number, comment); err != nil {
		log.Printf("error posting confirmation comment: %v", err)
	}
}

func (h *Handler) handleCorrect(ctx context.Context, event issueCommentEvent, cmd *commands.Command) {
	repo := event.Repository.FullName
	installID := event.Installation.ID

	prev, err := h.timeEntries.FindLatestActive(ctx, event.Comment.User.ID, repo, event.Issue.Number)
	if err != nil {
		log.Printf("error finding previous entry: %v", err)
		h.postError(ctx, event, "Failed to find previous entry. Please try again.")
		return
	}
	if prev == nil {
		h.postError(ctx, event, "No active time entry found on this issue to correct.")
		return
	}

	entry := &timeentry.Entry{
		GitHubUserID:     event.Comment.User.ID,
		GitHubUsername:   event.Comment.User.Login,
		Repository:       repo,
		IssueNumber:      event.Issue.Number,
		DurationMinutes:  cmd.Minutes,
		Description:      cmd.Description,
		ClientID:         prev.ClientID,
		SourceCommentID:  event.Comment.ID,
		SourceCommentURL: event.Comment.HTMLURL,
		CreatedBy:        "user",
	}

	entry, err = h.timeEntries.Create(ctx, entry)
	if err != nil {
		log.Printf("error creating correction entry: %v", err)
		h.postError(ctx, event, "Failed to correct time. Please try again.")
		return
	}

	if err := h.timeEntries.Supersede(ctx, prev.ID, entry.ID); err != nil {
		log.Printf("error superseding entry %d: %v", prev.ID, err)
	}

	comment := fmt.Sprintf("Corrected @%s's entry from %s to %s (entry #%d supersedes #%d)",
		event.Comment.User.Login,
		commands.FormatDuration(prev.DurationMinutes),
		commands.FormatDuration(cmd.Minutes),
		entry.ID, prev.ID)

	if _, err := h.ghClient.PostComment(installID, repo, event.Issue.Number, comment); err != nil {
		log.Printf("error posting correction comment: %v", err)
	}
}

func (h *Handler) handleDelete(ctx context.Context, event issueCommentEvent) {
	repo := event.Repository.FullName
	installID := event.Installation.ID

	prev, err := h.timeEntries.FindLatestActive(ctx, event.Comment.User.ID, repo, event.Issue.Number)
	if err != nil {
		log.Printf("error finding previous entry: %v", err)
		h.postError(ctx, event, "Failed to find entry. Please try again.")
		return
	}
	if prev == nil {
		h.postError(ctx, event, "No active time entry found on this issue to delete.")
		return
	}

	if err := h.timeEntries.SoftDelete(ctx, prev.ID); err != nil {
		log.Printf("error deleting entry %d: %v", prev.ID, err)
		h.postError(ctx, event, "Failed to delete entry. Please try again.")
		return
	}

	comment := fmt.Sprintf("Deleted @%s's entry of %s (entry #%d)",
		event.Comment.User.Login, commands.FormatDuration(prev.DurationMinutes), prev.ID)

	if _, err := h.ghClient.PostComment(installID, repo, event.Issue.Number, comment); err != nil {
		log.Printf("error posting delete comment: %v", err)
	}
}

func (h *Handler) isAuthorized(event issueCommentEvent) bool {
	username := event.Comment.User.Login
	installID := event.Installation.ID

	for _, org := range h.allowedOrgs {
		isMember, err := h.ghClient.IsOrgMember(installID, org, username)
		if err != nil {
			log.Printf("error checking org membership for %s in %s: %v", username, org, err)
			continue
		}
		if isMember {
			return true
		}
	}
	return false
}

func (h *Handler) postError(ctx context.Context, event issueCommentEvent, msg string) {
	comment := fmt.Sprintf("**Billbird error:** %s", msg)
	if _, err := h.ghClient.PostComment(event.Installation.ID, event.Repository.FullName, event.Issue.Number, comment); err != nil {
		log.Printf("error posting error comment: %v", err)
	}
}
