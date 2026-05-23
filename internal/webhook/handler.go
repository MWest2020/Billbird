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
	"github.com/mwesterweel/billbird/internal/planentry"
	"github.com/mwesterweel/billbird/internal/timeentry"
)

type Handler struct {
	webhookSecret  string
	membership     MembershipPolicy
	deliveries     DeliveryTracker
	ghClient       GHClient
	timeEntries    *timeentry.Store
	planEntries    *planentry.Store
	clientResolver *client.Resolver
}

func NewHandler(
	webhookSecret string,
	membership MembershipPolicy,
	deliveries DeliveryTracker,
	ghClient GHClient,
	timeEntries *timeentry.Store,
	planEntries *planentry.Store,
) *Handler {
	return &Handler{
		webhookSecret: webhookSecret,
		membership:    membership,
		deliveries:    deliveries,
		ghClient:      ghClient,
		timeEntries:   timeEntries,
		planEntries:   planEntries,
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

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	eventType := r.Header.Get("X-GitHub-Event")

	// Atomic idempotency claim: a single INSERT ... ON CONFLICT DO NOTHING
	// either wins the row (we process) or sees an existing row (we skip).
	// Two concurrent receivers for the same delivery_id cannot both win, so
	// duplicate /log entries from GitHub webhook retries are impossible.
	// See docs/webhook-idempotency.md for the failure mode this closes.
	if deliveryID != "" {
		claimed, err := h.deliveries.Claim(r.Context(), deliveryID, eventType)
		if err != nil {
			log.Printf("error claiming delivery: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !claimed {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	switch eventType {
	case "issue_comment":
		h.handleIssueComment(r.Context(), body)
	case "pull_request_review_comment":
		h.handlePullRequestReviewComment(r.Context(), body)
	case "projects_v2_item":
		// cycle time tracking — not yet implemented
	case "pull_request":
		// cycle time tracking — not yet implemented
	default:
		// Unknown event, acknowledge and ignore
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

type commentPayload struct {
	ID      int64  `json:"id"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	User    struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"user"`
}

type repositoryPayload struct {
	FullName string `json:"full_name"`
}

type installationPayload struct {
	ID int64 `json:"id"`
}

type issueCommentEvent struct {
	Action  string         `json:"action"`
	Comment commentPayload `json:"comment"`
	Issue   struct {
		Number int `json:"number"`
	} `json:"issue"`
	Repository   repositoryPayload   `json:"repository"`
	Installation installationPayload `json:"installation"`
}

// pullRequestReviewCommentEvent has the same comment + repository + installation
// shape as issueCommentEvent, but the target number lives on pull_request
// instead of issue. We normalize into issueCommentEvent so the downstream
// command dispatch handles both event types identically — GitHub's API treats
// PR numbers and issue numbers as interchangeable on the comments endpoint.
type pullRequestReviewCommentEvent struct {
	Action      string         `json:"action"`
	Comment     commentPayload `json:"comment"`
	PullRequest struct {
		Number int `json:"number"`
	} `json:"pull_request"`
	Repository   repositoryPayload   `json:"repository"`
	Installation installationPayload `json:"installation"`
}

func (h *Handler) handlePullRequestReviewComment(ctx context.Context, body []byte) {
	var pr pullRequestReviewCommentEvent
	if err := json.Unmarshal(body, &pr); err != nil {
		log.Printf("error parsing pull_request_review_comment event: %v", err)
		return
	}
	event := issueCommentEvent{
		Action:       pr.Action,
		Comment:      pr.Comment,
		Repository:   pr.Repository,
		Installation: pr.Installation,
	}
	event.Issue.Number = pr.PullRequest.Number
	h.dispatchCommentCommand(ctx, event)
}

func (h *Handler) handleIssueComment(ctx context.Context, body []byte) {
	var event issueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("error parsing issue_comment event: %v", err)
		return
	}
	h.dispatchCommentCommand(ctx, event)
}

func (h *Handler) dispatchCommentCommand(ctx context.Context, event issueCommentEvent) {
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
	case commands.CmdPlan:
		h.handlePlan(ctx, event, cmd)
	case commands.CmdUnplan:
		h.handleUnplan(ctx, event)
	}
}

func (h *Handler) handlePlan(ctx context.Context, event issueCommentEvent, cmd *commands.Command) {
	repo := event.Repository.FullName
	installID := event.Installation.ID

	if h.planEntries == nil {
		log.Printf("plan store not configured; ignoring /plan on %s#%d", repo, event.Issue.Number)
		return
	}

	prev, err := h.planEntries.FindActive(ctx, repo, event.Issue.Number)
	if err != nil {
		log.Printf("error finding active plan: %v", err)
		h.postError(ctx, event, "Failed to look up existing plan. Please try again.")
		return
	}

	labels := h.fetchIssueLabels(installID, repo, event.Issue.Number)

	newPlan := &planentry.Entry{
		GitHubUserID:     event.Comment.User.ID,
		GitHubUsername:   event.Comment.User.Login,
		Repository:       repo,
		IssueNumber:      event.Issue.Number,
		DurationMinutes:  cmd.Minutes,
		Description:      cmd.Description,
		SourceCommentID:  event.Comment.ID,
		SourceCommentURL: event.Comment.HTMLURL,
		CreatedBy:        "user",
		Labels:           labels,
	}

	var prevID *int64
	if prev != nil {
		prevID = &prev.ID
	}
	newPlan, err = h.planEntries.ReplacePlan(ctx, prevID, newPlan)
	if err != nil {
		log.Printf("error recording plan: %v", err)
		h.postError(ctx, event, "Failed to record plan. Please try again.")
		return
	}

	var comment string
	if prev != nil {
		comment = fmt.Sprintf("Updated @%s's plan from %s to %s (plan #%d supersedes #%d)",
			event.Comment.User.Login,
			commands.FormatDuration(prev.DurationMinutes),
			commands.FormatDuration(cmd.Minutes),
			newPlan.ID, prev.ID)
	} else {
		comment = fmt.Sprintf("Planned %s on this issue by @%s (plan #%d)",
			commands.FormatDuration(cmd.Minutes),
			event.Comment.User.Login,
			newPlan.ID)
	}
	if cmd.Description != "" {
		comment += " — " + cmd.Description
	}

	if _, err := h.ghClient.PostComment(installID, repo, event.Issue.Number, comment); err != nil {
		log.Printf("error posting plan confirmation comment: %v", err)
	}
}

func (h *Handler) handleUnplan(ctx context.Context, event issueCommentEvent) {
	repo := event.Repository.FullName
	installID := event.Installation.ID

	if h.planEntries == nil {
		log.Printf("plan store not configured; ignoring /unplan on %s#%d", repo, event.Issue.Number)
		return
	}

	prev, err := h.planEntries.FindActive(ctx, repo, event.Issue.Number)
	if err != nil {
		log.Printf("error finding active plan: %v", err)
		h.postError(ctx, event, "Failed to look up active plan. Please try again.")
		return
	}
	if prev == nil {
		h.postError(ctx, event, "No active plan found on this issue to remove.")
		return
	}

	if err := h.planEntries.SoftDelete(ctx, prev.ID, event.Comment.ID, event.Comment.HTMLURL); err != nil {
		log.Printf("error soft-deleting plan %d: %v", prev.ID, err)
		h.postError(ctx, event, "Failed to remove plan. Please try again.")
		return
	}

	comment := fmt.Sprintf("Removed @%s's plan of %s (plan #%d)",
		event.Comment.User.Login,
		commands.FormatDuration(prev.DurationMinutes),
		prev.ID)

	if _, err := h.ghClient.PostComment(installID, repo, event.Issue.Number, comment); err != nil {
		log.Printf("error posting unplan confirmation comment: %v", err)
	}
}

// fetchIssueLabels reads the issue's current labels. Returns an empty
// slice on error and logs the failure — a labels-fetch hiccup must not
// block the /log itself, only mean we record an empty snapshot.
func (h *Handler) fetchIssueLabels(installID int64, repo string, issueNumber int) []string {
	labels, err := h.ghClient.GetIssueLabels(installID, repo, issueNumber)
	if err != nil {
		log.Printf("error fetching labels for %s#%d: %v", repo, issueNumber, err)
		return []string{}
	}
	return labels
}

func (h *Handler) handleLog(ctx context.Context, event issueCommentEvent, cmd *commands.Command) {
	repo := event.Repository.FullName
	installID := event.Installation.ID

	labels := h.fetchIssueLabels(installID, repo, event.Issue.Number)

	var clientID *int64
	if h.clientResolver != nil {
		var err error
		clientID, err = h.clientResolver.ResolveClient(ctx, labels, repo)
		if err != nil {
			log.Printf("error resolving client for %s#%d: %v", repo, event.Issue.Number, err)
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
		Labels:           labels,
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

	labels := h.fetchIssueLabels(installID, repo, event.Issue.Number)

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
		Labels:           labels,
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

// isAuthorized defers to the shared MembershipPolicy (auth.MembershipChecker
// in production). The policy caches decisions per user with a TTL, so this
// is one map lookup in the common case — no GitHub API call per /log.
//
// On a transient GitHub outage the policy returns false, which can produce
// spurious "not a member" replies. That's the same behaviour the previous
// inline-check had; better surfaces would route those through a separate
// "transient error" reply and skip the comment-post entirely. See the
// follow-up in docs/operations.md.
func (h *Handler) isAuthorized(event issueCommentEvent) bool {
	if h.membership == nil {
		// Fail closed: a misconfigured deployment must not silently let
		// every command through. main.go always wires a policy.
		log.Printf("authorization: no membership policy configured; rejecting %s", event.Comment.User.Login)
		return false
	}
	return h.membership.IsAllowed(event.Comment.User.Login)
}

func (h *Handler) postError(ctx context.Context, event issueCommentEvent, msg string) {
	comment := fmt.Sprintf("**Billbird error:** %s", msg)
	if _, err := h.ghClient.PostComment(event.Installation.ID, event.Repository.FullName, event.Issue.Number, comment); err != nil {
		log.Printf("error posting error comment: %v", err)
	}
}
