package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ClientID     string
	ClientSecret string
	AllowedOrgs  []string
	SessionSecret string
	BaseURL      string // e.g. http://localhost:8080
}

type Handler struct {
	cfg        Config
	pool       *pgxpool.Pool
	httpClient *http.Client
}

func NewHandler(cfg Config, pool *pgxpool.Pool) *Handler {
	return &Handler{
		cfg:        cfg,
		pool:       pool,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// RegisterRoutes registers auth routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", h.Login)
	mux.HandleFunc("GET /auth/callback", h.Callback)
	mux.HandleFunc("GET /auth/logout", h.Logout)
}

// Login redirects to GitHub OAuth.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	state := generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	url := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s/auth/callback&state=%s&scope=read:org",
		h.cfg.ClientID, h.cfg.BaseURL, state)
	http.Redirect(w, r, url, http.StatusFound)
}

// Callback handles the OAuth callback from GitHub.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	// Exchange code for token
	token, err := h.exchangeCode(code)
	if err != nil {
		log.Printf("oauth token exchange error: %v", err)
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Get user info
	user, err := h.getUser(token)
	if err != nil {
		log.Printf("oauth get user error: %v", err)
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Check org membership
	isMember := false
	for _, org := range h.cfg.AllowedOrgs {
		if h.checkOrgMembership(token, org, user.Login) {
			isMember = true
			break
		}
	}

	if !isMember {
		http.Error(w, "You are not a member of an authorized organization.", http.StatusForbidden)
		return
	}

	// Create session
	expiresAt := time.Now().Add(24 * time.Hour)
	var sessionID int64
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO sessions (github_user_id, github_username, org_member, expires_at)
		VALUES ($1, $2, true, $3) RETURNING id`,
		user.ID, user.Login, expiresAt).Scan(&sessionID)
	if err != nil {
		log.Printf("error creating session: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	cookieValue := h.signSession(sessionID)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    cookieValue,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// Logout clears the session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

// Session represents a validated session.
type Session struct {
	UserID   int64
	Username string
}

// RequireAuth is middleware that checks for a valid session.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := h.ValidateSession(r)
		if err != nil {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), sessionKey{}, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type sessionKey struct{}

// GetSession returns the session from the request context.
func GetSession(r *http.Request) *Session {
	s, _ := r.Context().Value(sessionKey{}).(*Session)
	return s
}

// ValidateSession checks the session cookie and returns the session if valid.
func (h *Handler) ValidateSession(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil, fmt.Errorf("no session cookie")
	}

	sessionID, err := h.verifySession(cookie.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid session: %w", err)
	}

	var s Session
	err = h.pool.QueryRow(r.Context(), `
		SELECT github_user_id, github_username FROM sessions
		WHERE id = $1 AND expires_at > now()`,
		sessionID).Scan(&s.UserID, &s.Username)
	if err != nil {
		return nil, fmt.Errorf("session expired or not found")
	}

	return &s, nil
}

// --- GitHub API helpers ---

func (h *Handler) exchangeCode(code string) (string, error) {
	payload := fmt.Sprintf(`{"client_id":"%s","client_secret":"%s","code":"%s"}`,
		h.cfg.ClientID, h.cfg.ClientSecret, code)

	req, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token",
		strings.NewReader(payload))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("oauth error: %s", result.Error)
	}
	return result.AccessToken, nil
}

type ghUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func (h *Handler) getUser(token string) (*ghUser, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user failed (%d): %s", resp.StatusCode, body)
	}

	var user ghUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (h *Handler) checkOrgMembership(token, org, username string) bool {
	req, _ := http.NewRequest("GET",
		fmt.Sprintf("https://api.github.com/orgs/%s/members/%s", org, username), nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusNoContent
}

// --- Session signing ---

func (h *Handler) signSession(sessionID int64) string {
	data := fmt.Sprintf("%d", sessionID)
	mac := hmac.New(sha256.New, []byte(h.cfg.SessionSecret))
	mac.Write([]byte(data))
	sig := hex.EncodeToString(mac.Sum(nil))
	return data + "." + sig
}

func (h *Handler) verifySession(cookie string) (int64, error) {
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("malformed session cookie")
	}

	data, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(h.cfg.SessionSecret))
	mac.Write([]byte(data))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return 0, fmt.Errorf("invalid signature")
	}

	id, err := fmt.Sscanf(data, "%d", new(int64))
	if err != nil || id != 1 {
		return 0, fmt.Errorf("invalid session id")
	}

	var sessionID int64
	fmt.Sscanf(data, "%d", &sessionID)
	return sessionID, nil
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
