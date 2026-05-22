package webhook

import "context"

// GHClient is the subset of the GitHub client the webhook handler needs.
// It exists as an interface so tests can stub the GitHub side without
// requiring a real installation or network.
type GHClient interface {
	PostComment(installationID int64, repo string, issueNumber int, body string) (int64, error)
	GetIssueLabels(installationID int64, repo string, issueNumber int) ([]string, error)
	IsOrgMember(installationID int64, org, username string) (bool, error)
}

// DeliveryTracker dedupes webhook deliveries by GitHub's X-GitHub-Delivery
// header. Backed by Postgres at runtime; tests stub it.
type DeliveryTracker interface {
	IsProcessed(ctx context.Context, deliveryID string) (bool, error)
	MarkProcessed(ctx context.Context, deliveryID, eventType string) error
}
