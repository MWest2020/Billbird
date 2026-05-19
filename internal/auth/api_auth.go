package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mwesterweel/billbird/internal/apitoken"
)

// AuthMethod records which auth path produced the session in context.
type AuthMethod string

const (
	AuthMethodCookie AuthMethod = "cookie"
	AuthMethodToken  AuthMethod = "token"
)

// APIAuthContext holds the resolved identity for a /api/v1/* request. The
// session shape stays the same regardless of which auth path was used so
// handlers can read it the same way.
type APIAuthContext struct {
	Session *Session
	Method  AuthMethod
	TokenID *int64 // populated for AuthMethodToken
}

type apiAuthKey struct{}

// GetAPIAuth returns the API auth context if present.
func GetAPIAuth(r *http.Request) *APIAuthContext {
	v, _ := r.Context().Value(apiAuthKey{}).(*APIAuthContext)
	return v
}

// MembershipPolicy decides whether a GitHub user is still allowed to
// act through Billbird. Implemented in production by MembershipChecker
// (which talks to GitHub through the App's installation tokens); test
// code substitutes a deterministic fake.
type MembershipPolicy interface {
	IsAllowed(username string) bool
}

// APIAuthDependencies bundles the collaborators the bearer-and-cookie
// middleware needs. Both fields may be nil in narrow contexts (e.g. tests),
// but the middleware will reject every request in that case.
type APIAuthDependencies struct {
	Cookie     *Handler         // for cookie-path session validation
	Tokens     *apitoken.Store  // for bearer-path token verification
	Membership MembershipPolicy // re-check ALLOWED_ORGS for token requests
}

// RequireAPIAuth accepts either a valid session cookie OR a valid bearer
// token. On both success paths it places an *APIAuthContext on the request
// context. On failure it writes a JSON 401 — never a redirect — because
// the consumers here are non-browser clients.
func (deps APIAuthDependencies) RequireAPIAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bearer takes precedence: explicit caller intent.
		if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
			plaintext := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			if deps.Tokens == nil || deps.Membership == nil {
				writeJSON401(w, "auth not configured")
				return
			}
			tok, err := deps.Tokens.Verify(r.Context(), plaintext)
			if err != nil {
				writeJSON401(w, "auth error")
				return
			}
			if tok == nil {
				writeJSON401(w, "invalid token")
				return
			}
			if !deps.Membership.IsAllowed(tok.GitHubUsername) {
				writeJSON401(w, "user no longer authorised")
				return
			}
			ctx := context.WithValue(r.Context(), apiAuthKey{}, &APIAuthContext{
				Session: &Session{UserID: tok.GitHubUserID, Username: tok.GitHubUsername},
				Method:  AuthMethodToken,
				TokenID: &tok.ID,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fallback to session cookie.
		if deps.Cookie == nil {
			writeJSON401(w, "authentication required")
			return
		}
		session, err := deps.Cookie.ValidateSession(r)
		if err != nil {
			writeJSON401(w, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), apiAuthKey{}, &APIAuthContext{
			Session: session,
			Method:  AuthMethodCookie,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSON401(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
