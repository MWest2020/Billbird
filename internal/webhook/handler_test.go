package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGHClient records calls so tests can assert which GitHub interactions
// the handler triggered without touching the real API.
type fakeGHClient struct {
	mu          sync.Mutex
	postedBody  []string
	postedRepo  []string
	labels      []string
	memberCheck func(org, username string) bool
}

func (f *fakeGHClient) PostComment(installID int64, repo string, issueNumber int, body string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.postedRepo = append(f.postedRepo, repo)
	f.postedBody = append(f.postedBody, body)
	return 1, nil
}

func (f *fakeGHClient) GetIssueLabels(installID int64, repo string, issueNumber int) ([]string, error) {
	return f.labels, nil
}

func (f *fakeGHClient) IsOrgMember(installID int64, org, username string) (bool, error) {
	if f.memberCheck == nil {
		return false, nil
	}
	return f.memberCheck(org, username), nil
}

// fakeDeliveries dedupes in-memory; no DB required.
type fakeDeliveries struct {
	mu        sync.Mutex
	processed map[string]bool
}

func newFakeDeliveries() *fakeDeliveries {
	return &fakeDeliveries{processed: map[string]bool{}}
}

func (f *fakeDeliveries) Claim(_ context.Context, id, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.processed[id] {
		return false, nil
	}
	f.processed[id] = true
	return true, nil
}

const testSecret = "shh"

func signBody(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newTestHandler(gh GHClient, deliveries DeliveryTracker, allowedOrgs []string) *Handler {
	return &Handler{
		webhookSecret: testSecret,
		allowedOrgs:   allowedOrgs,
		deliveries:    deliveries,
		ghClient:      gh,
		// timeEntries and planEntries are nil; these tests don't exercise
		// the create paths — they cover routing, auth, and idempotency.
	}
}

func postWebhook(t *testing.T, h *Handler, event, deliveryID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", signBody(body))
	if deliveryID != "" {
		req.Header.Set("X-GitHub-Delivery", deliveryID)
	}
	rec := httptest.NewRecorder()
	h.Handle(rec, req)
	return rec
}

func TestVerifySignature(t *testing.T) {
	h := &Handler{webhookSecret: testSecret}
	body := []byte(`{"hello":"world"}`)

	if !h.verifySignature(body, signBody(body)) {
		t.Error("valid signature was rejected")
	}
	if h.verifySignature(body, "sha256=deadbeef") {
		t.Error("bad signature accepted")
	}
	if h.verifySignature(body, "") {
		t.Error("empty signature accepted")
	}
	if h.verifySignature(body, "sha1=abc") {
		t.Error("wrong-prefix signature accepted")
	}
}

func TestVerifySignature_NoSecretConfigured(t *testing.T) {
	h := &Handler{webhookSecret: ""}
	if h.verifySignature([]byte(`{}`), "sha256=anything") {
		t.Error("handler accepted signature with empty secret")
	}
}

func TestHandle_BadSignature_Returns401(t *testing.T) {
	h := newTestHandler(&fakeGHClient{}, newFakeDeliveries(), nil)

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandle_UnknownEvent_OK(t *testing.T) {
	h := newTestHandler(&fakeGHClient{}, newFakeDeliveries(), nil)
	rec := postWebhook(t, h, "star", "del-1", []byte(`{}`))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown event, got %d", rec.Code)
	}
}

const issueCommentPayload = `{
	"action": "created",
	"comment": {
		"id": 100,
		"body": "/log 5m smoke",
		"html_url": "https://github.com/o/r/issues/1#issuecomment-100",
		"user": {"id": 7, "login": "alice"}
	},
	"issue": {"number": 1},
	"repository": {"full_name": "o/r"},
	"installation": {"id": 42}
}`

const prReviewCommentPayload = `{
	"action": "created",
	"comment": {
		"id": 200,
		"body": "/log 7m review smoke",
		"html_url": "https://github.com/o/r/pull/9#discussion_r200",
		"user": {"id": 7, "login": "alice"}
	},
	"pull_request": {"number": 9},
	"repository": {"full_name": "o/r"},
	"installation": {"id": 42}
}`

// TestHandle_IssueComment_LogCommand_ReachesAuthCheck confirms the issue_comment
// route parses the payload and the command, and then enforces ALLOWED_ORGS
// membership. Without any allowed orgs, the user is rejected and Billbird
// posts an explanatory comment back — observing that comment confirms the
// handler walked end-to-end through parse → auth.
func TestHandle_IssueComment_LogCommand_ReachesAuthCheck(t *testing.T) {
	gh := &fakeGHClient{}
	h := newTestHandler(gh, newFakeDeliveries(), nil)

	rec := postWebhook(t, h, "issue_comment", "del-issue", []byte(issueCommentPayload))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(gh.postedBody) != 1 {
		t.Fatalf("expected one error comment posted, got %d", len(gh.postedBody))
	}
	if !strings.Contains(gh.postedBody[0], "not a member of an authorized organization") {
		t.Errorf("expected org-membership error, got %q", gh.postedBody[0])
	}
	if gh.postedRepo[0] != "o/r" {
		t.Errorf("expected repo o/r, got %q", gh.postedRepo[0])
	}
}

func TestHandle_PullRequestReviewComment_LogCommand_ReachesAuthCheck(t *testing.T) {
	gh := &fakeGHClient{}
	h := newTestHandler(gh, newFakeDeliveries(), nil)

	rec := postWebhook(t, h, "pull_request_review_comment", "del-pr", []byte(prReviewCommentPayload))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(gh.postedBody) != 1 {
		t.Fatalf("expected one error comment posted, got %d", len(gh.postedBody))
	}
	if !strings.Contains(gh.postedBody[0], "not a member of an authorized organization") {
		t.Errorf("expected org-membership error, got %q", gh.postedBody[0])
	}
}

func TestHandle_IssueComment_EditedAction_Ignored(t *testing.T) {
	gh := &fakeGHClient{}
	h := newTestHandler(gh, newFakeDeliveries(), nil)

	body := strings.Replace(issueCommentPayload, `"action": "created"`, `"action": "edited"`, 1)
	rec := postWebhook(t, h, "issue_comment", "del-edit", []byte(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(gh.postedBody) != 0 {
		t.Errorf("expected no GitHub calls for edited action, got %d", len(gh.postedBody))
	}
}

func TestHandle_NonCommandComment_NoOp(t *testing.T) {
	gh := &fakeGHClient{}
	h := newTestHandler(gh, newFakeDeliveries(), nil)

	body := strings.Replace(issueCommentPayload, "/log 5m smoke", "just a regular comment", 1)
	rec := postWebhook(t, h, "issue_comment", "del-regular", []byte(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(gh.postedBody) != 0 {
		t.Errorf("expected no GitHub calls for non-command comment, got %d", len(gh.postedBody))
	}
}

func TestHandle_Idempotency_DuplicateDeliveryShortCircuits(t *testing.T) {
	gh := &fakeGHClient{}
	deliveries := newFakeDeliveries()
	h := newTestHandler(gh, deliveries, nil)

	// First delivery — should process and post the auth-error comment.
	rec1 := postWebhook(t, h, "issue_comment", "dup-id", []byte(issueCommentPayload))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first delivery: expected 200, got %d", rec1.Code)
	}
	if len(gh.postedBody) != 1 {
		t.Fatalf("first delivery: expected one GitHub call, got %d", len(gh.postedBody))
	}

	// Second delivery with the same X-GitHub-Delivery ID — must short-circuit.
	rec2 := postWebhook(t, h, "issue_comment", "dup-id", []byte(issueCommentPayload))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second delivery: expected 200, got %d", rec2.Code)
	}
	if len(gh.postedBody) != 1 {
		t.Errorf("second delivery should be skipped; saw %d GitHub calls (expected 1)", len(gh.postedBody))
	}
}
