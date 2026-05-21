package auth

import (
	"fmt"
	"log"
	"sync"
	"time"

	gh "github.com/mwesterweel/billbird/internal/github"
)

// AccountType values matching the GitHub API's installation.account.type
// field.
const (
	accountTypeUser = "User"
	accountTypeOrg  = "Organization"
)

// membershipGHClient is the slice of gh.Client that the membership
// checker depends on. The interface exists so tests can substitute a
// deterministic fake without spinning up an HTTPS mock for the GitHub
// API. *gh.Client satisfies it in production.
type membershipGHClient interface {
	ListInstallations() ([]gh.Installation, error)
	IsOrgMember(installationID int64, org, username string) (bool, error)
}

// MembershipChecker re-verifies that a GitHub user is still allowed to
// act through Billbird. For each entry in ALLOWED_ORGS:
//
//   - if the App is installed on an Organization with that login, call
//     the /orgs/{org}/members/{user} API;
//   - if the App is installed on a User account with that login, the
//     owner of that personal namespace is the only "member" — allow
//     when the requesting username equals the account login.
//
// Decisions are cached per username with a TTL so token-authenticated
// API requests do not hammer the GitHub API.
type MembershipChecker struct {
	gh          membershipGHClient
	allowedOrgs []string
	ttl         time.Duration

	mu         sync.Mutex
	orgInstall map[string]installation // account login -> installation details
	installAt  time.Time
	cache      map[string]membershipEntry // username -> decision
}

// installation captures just what the membership check needs from a
// gh.Installation. The full gh.Installation may carry more fields in
// future; we copy the ones we use.
type installation struct {
	ID          int64
	AccountType string
}

type membershipEntry struct {
	isMember  bool
	checkedAt time.Time
}

// NewMembershipChecker builds a checker. Pass the same gh.Client used
// by the webhook handler so installation tokens are shared.
func NewMembershipChecker(client *gh.Client, allowedOrgs []string, ttl time.Duration) *MembershipChecker {
	return newMembershipChecker(client, allowedOrgs, ttl)
}

// newMembershipChecker accepts the narrower interface so unit tests can
// inject a fake. Production code calls NewMembershipChecker.
func newMembershipChecker(client membershipGHClient, allowedOrgs []string, ttl time.Duration) *MembershipChecker {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &MembershipChecker{
		gh:          client,
		allowedOrgs: allowedOrgs,
		ttl:         ttl,
		orgInstall:  make(map[string]installation),
		cache:       make(map[string]membershipEntry),
	}
}

// PrimeInstallations loads the (account -> installation) map once.
// Cheap and idempotent; call at startup. If the App is installed on a
// new account later, the map auto-refreshes on the next cache miss.
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
	m.orgInstall = make(map[string]installation, len(installs))
	for _, inst := range installs {
		m.orgInstall[inst.Account] = installation{
			ID:          inst.ID,
			AccountType: inst.AccountType,
		}
	}
	m.installAt = time.Now()
	return nil
}

// IsAllowed returns true when the given username is currently authorised
// by at least one ALLOWED_ORGS entry. Results are cached for the
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
	mapping := make(map[string]installation, len(m.orgInstall))
	for account, inst := range m.orgInstall {
		mapping[account] = inst
	}
	m.mu.Unlock()

	for _, allowed := range m.allowedOrgs {
		inst, ok := mapping[allowed]
		if !ok {
			continue
		}
		switch inst.AccountType {
		case accountTypeUser:
			// Personal namespace: the only member is the account owner.
			// Allow when the requesting login matches the account login.
			if username == allowed {
				return true
			}
		case accountTypeOrg:
			isMember, err := m.gh.IsOrgMember(inst.ID, allowed, username)
			if err != nil {
				log.Printf("membership: error checking %s in %s: %v", username, allowed, err)
				continue
			}
			if isMember {
				return true
			}
		default:
			// Unknown account type — refuse rather than guess. Surfaces
			// in logs so the operator notices.
			log.Printf("membership: unknown account type %q for %s; skipping", inst.AccountType, allowed)
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
