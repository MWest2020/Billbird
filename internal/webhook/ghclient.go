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
	// Claim atomically records the delivery as being processed and returns
	// true if the caller is the first (and only) goroutine to do so. Returns
	// false if another delivery with the same ID has already been claimed —
	// in that case the caller must skip processing.
	Claim(ctx context.Context, deliveryID, eventType string) (bool, error)
}

// MembershipPolicy decides whether a GitHub user is still allowed to issue
// commands through Billbird. Implementations are expected to be cheap to
// call (the caller does no caching of its own) — in production the auth
// package's MembershipChecker caches per-user decisions with a TTL.
type MembershipPolicy interface {
	IsAllowed(username string) bool
}
