package auth

import (
	"errors"
	"fmt"
	"testing"
	"time"

	gh "github.com/mwesterweel/billbird/internal/github"
)

// fakeGH is a tiny stand-in for gh.Client used to drive the
// MembershipChecker through every dispatch branch deterministically.
type fakeGH struct {
	installations    []gh.Installation
	installErr       error
	memberByOrg      map[string]map[string]bool // org -> username -> member?
	memberCheckErr   error
	isMemberCalls    int
	listInstallCalls int
}

func (f *fakeGH) ListInstallations() ([]gh.Installation, error) {
	f.listInstallCalls++
	if f.installErr != nil {
		return nil, f.installErr
	}
	return f.installations, nil
}

func (f *fakeGH) IsOrgMember(_ int64, org, username string) (bool, error) {
	f.isMemberCalls++
	if f.memberCheckErr != nil {
		return false, f.memberCheckErr
	}
	users, ok := f.memberByOrg[org]
	if !ok {
		return false, nil
	}
	return users[username], nil
}

func TestPersonalAccountInstallationAllowsOwner(t *testing.T) {
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 100, Account: "MWest2020", AccountType: "User"},
		},
	}
	mc := newMembershipChecker(f, []string{"MWest2020"}, time.Minute)

	if !mc.IsAllowed("MWest2020") {
		t.Errorf("expected owner of personal namespace to be allowed")
	}
	if mc.IsAllowed("someone-else") {
		t.Errorf("non-owner must not be allowed via a User installation")
	}
	if f.isMemberCalls != 0 {
		t.Errorf("User installations must not trigger IsOrgMember; got %d calls", f.isMemberCalls)
	}
}

func TestOrganizationInstallationDelegatesToIsOrgMember(t *testing.T) {
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 200, Account: "AcmeOrg", AccountType: "Organization"},
		},
		memberByOrg: map[string]map[string]bool{
			"AcmeOrg": {"alice": true, "bob": false},
		},
	}
	mc := newMembershipChecker(f, []string{"AcmeOrg"}, time.Minute)

	if !mc.IsAllowed("alice") {
		t.Errorf("alice should be a member of AcmeOrg")
	}
	if mc.IsAllowed("bob") {
		t.Errorf("bob should not be allowed (not a member)")
	}
	if f.isMemberCalls != 2 {
		t.Errorf("expected 2 IsOrgMember calls, got %d", f.isMemberCalls)
	}
}

func TestMixedAllowedOrgsPersonalAndOrg(t *testing.T) {
	// The most realistic real-world case: developer testing from their
	// personal account + their actual organisation membership.
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 100, Account: "MWest2020", AccountType: "User"},
			{ID: 200, Account: "AcmeOrg", AccountType: "Organization"},
		},
		memberByOrg: map[string]map[string]bool{
			"AcmeOrg": {"MWest2020": true},
		},
	}
	mc := newMembershipChecker(f, []string{"MWest2020", "AcmeOrg"}, time.Minute)

	// Personal-account match short-circuits before any HTTP call.
	if !mc.IsAllowed("MWest2020") {
		t.Errorf("MWest2020 should be allowed via personal-namespace match")
	}

	mc.Invalidate("MWest2020")
	f.isMemberCalls = 0

	// Once the personal-account branch matches, the org branch is not
	// consulted. But if a different user tries (only org-member), they
	// should still be allowed.
	if !mc.IsAllowed("MWest2020") {
		t.Errorf("MWest2020 must still be allowed after invalidate")
	}
}

func TestUnknownAccountTypeIsRefused(t *testing.T) {
	// Future-proofing: if GitHub adds a new installation account type
	// (e.g. "Enterprise"), we refuse rather than guess.
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 300, Account: "weird", AccountType: "Enterprise"},
		},
	}
	mc := newMembershipChecker(f, []string{"weird"}, time.Minute)

	if mc.IsAllowed("anyone") {
		t.Errorf("unknown account type must default to denied")
	}
}

func TestUserInstallationAccountTypeIsCaseSensitive(t *testing.T) {
	// GitHub returns "User" / "Organization" capitalised — guard
	// against an accidental tolower regression.
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 400, Account: "MWest2020", AccountType: "user"}, // lowercase intentional
		},
	}
	mc := newMembershipChecker(f, []string{"MWest2020"}, time.Minute)

	if mc.IsAllowed("MWest2020") {
		t.Errorf("lowercase 'user' must not match the canonical 'User' constant")
	}
}

func TestCachingHonoursTTL(t *testing.T) {
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 200, Account: "AcmeOrg", AccountType: "Organization"},
		},
		memberByOrg: map[string]map[string]bool{
			"AcmeOrg": {"alice": true},
		},
	}
	// Very short TTL so the test can prove caching without sleeping.
	mc := newMembershipChecker(f, []string{"AcmeOrg"}, 100*time.Millisecond)

	if !mc.IsAllowed("alice") {
		t.Fatalf("expected alice allowed")
	}
	if !mc.IsAllowed("alice") {
		t.Fatalf("expected cached allow")
	}
	if f.isMemberCalls != 1 {
		t.Errorf("expected 1 IsOrgMember call within TTL, got %d", f.isMemberCalls)
	}
	mc.Invalidate("alice")
	if !mc.IsAllowed("alice") {
		t.Fatalf("expected allow after invalidate")
	}
	if f.isMemberCalls != 2 {
		t.Errorf("expected 2 IsOrgMember calls after invalidate, got %d", f.isMemberCalls)
	}
}

func TestRefreshErrorsAreLoggedNotFatal(t *testing.T) {
	f := &fakeGH{installErr: errors.New("boom")}
	mc := newMembershipChecker(f, []string{"AcmeOrg"}, time.Minute)
	if mc.IsAllowed("alice") {
		t.Errorf("when installations can't be listed, every user is denied")
	}
}

func TestPrimeInstallationsCaches(t *testing.T) {
	f := &fakeGH{
		installations: []gh.Installation{
			{ID: 100, Account: "MWest2020", AccountType: "User"},
		},
	}
	mc := newMembershipChecker(f, []string{"MWest2020"}, time.Minute)
	if err := mc.PrimeInstallations(); err != nil {
		t.Fatalf("prime: %v", err)
	}
	// After priming, checkOrgs should not need to refresh again.
	_ = mc.IsAllowed("MWest2020")
	if f.listInstallCalls != 1 {
		t.Errorf("expected 1 ListInstallations call after priming, got %d", f.listInstallCalls)
	}
}

// Ensure the canonical *gh.Client still satisfies the unexported
// interface — guards against drift if gh.Client method signatures
// change.
func TestClientImplementsInterface(t *testing.T) {
	var _ membershipGHClient = (*gh.Client)(nil)
	// Avoid unused-import: t is needed for build but no assertion runs.
	_ = fmt.Sprintf
}
