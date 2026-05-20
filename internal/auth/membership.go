package auth

import (
	"fmt"
	"log"
	"sync"
	"time"

	gh "github.com/mwesterweel/billbird/internal/github"
)

// MembershipChecker re-verifies that a GitHub user is still a member of at
// least one ALLOWED_ORGS organisation, using the GitHub App's installation
// tokens. Decisions are cached per user with a TTL so token-authenticated
// API requests do not hammer the GitHub API.
type MembershipChecker struct {
	gh          *gh.Client
	allowedOrgs []string
	ttl         time.Duration

	mu         sync.Mutex
	orgInstall map[string]int64 // org login -> installation ID
	installAt  time.Time
	cache      map[string]membershipEntry // username -> decision
}

type membershipEntry struct {
	isMember bool
	checkedAt time.Time
}

// NewMembershipChecker builds a checker. Pass the same gh.Client used by the
// webhook handler so installation tokens are shared.
func NewMembershipChecker(client *gh.Client, allowedOrgs []string, ttl time.Duration) *MembershipChecker {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &MembershipChecker{
		gh:          client,
		allowedOrgs: allowedOrgs,
		ttl:         ttl,
		orgInstall:  make(map[string]int64),
		cache:       make(map[string]membershipEntry),
	}
}

// PrimeInstallations loads the (org -> installation ID) map once. Cheap and
// idempotent; call at startup. If the app is added to an org later, the
// map auto-refreshes on the next cache miss for that user.
func (m *MembershipChecker) PrimeInstallations() error {
	return m.refreshInstallationsLocked(false)
}

func (m *MembershipChecker) refreshInstallationsLocked(force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !force && len(m.orgInstall) > 0 && time.Since(m.installAt) < time.Hour {
		return nil
	}
	installs, err := m.gh.ListInstallations()
	if err != nil {
		return fmt.Errorf("listing app installations: %w", err)
	}
	m.orgInstall = make(map[string]int64, len(installs))
	for _, inst := range installs {
		m.orgInstall[inst.Account] = inst.ID
	}
	m.installAt = time.Now()
	return nil
}

// IsAllowed returns true when the given username is a current member of at
// least one ALLOWED_ORGS organisation. Results are cached for the
// configured TTL (default 5 minutes).
func (m *MembershipChecker) IsAllowed(username string) bool {
	now := time.Now()

	m.mu.Lock()
	entry, seen := m.cache[username]
	if seen && now.Sub(entry.checkedAt) < m.ttl {
		m.mu.Unlock()
		return entry.isMember
	}
	m.mu.Unlock()

	isMember := m.checkOrgs(username)

	m.mu.Lock()
	m.cache[username] = membershipEntry{isMember: isMember, checkedAt: now}
	m.mu.Unlock()
	return isMember
}

func (m *MembershipChecker) checkOrgs(username string) bool {
	m.mu.Lock()
	stale := len(m.orgInstall) == 0
	m.mu.Unlock()
	if stale {
		if err := m.refreshInstallationsLocked(true); err != nil {
			log.Printf("membership: cannot refresh installations: %v", err)
			return false
		}
	}

	m.mu.Lock()
	mapping := make(map[string]int64, len(m.orgInstall))
	for org, id := range m.orgInstall {
		mapping[org] = id
	}
	m.mu.Unlock()

	for _, org := range m.allowedOrgs {
		installID, ok := mapping[org]
		if !ok {
			continue
		}
		isMember, err := m.gh.IsOrgMember(installID, org, username)
		if err != nil {
			log.Printf("membership: error checking %s in %s: %v", username, org, err)
			continue
		}
		if isMember {
			return true
		}
	}
	return false
}

// Invalidate drops any cached decision for a user. Use after revocation
// flows when we want the next request to recheck immediately.
func (m *MembershipChecker) Invalidate(username string) {
	m.mu.Lock()
	delete(m.cache, username)
	m.mu.Unlock()
}
