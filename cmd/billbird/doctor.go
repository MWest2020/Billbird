package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mwesterweel/billbird/internal/config"
	"github.com/mwesterweel/billbird/internal/db"
)

// runDoctor performs a one-shot health check of Billbird's external
// dependencies (Postgres, GitHub App credentials, installation permissions,
// recent webhook deliveries). Designed to surface every common setup mistake
// in a single command, so operators don't have to script the GitHub App
// API by hand.
//
// Returns 0 if everything looks good, non-zero on any concrete problem.
func runDoctor() int {
	fmt.Println("=== Billbird doctor ===")
	fmt.Println()

	problems := 0
	warn := func(format string, args ...any) {
		fmt.Print("⚠  ")
		fmt.Printf(format+"\n", args...)
	}
	fail := func(format string, args ...any) {
		fmt.Print("✗  ")
		fmt.Printf(format+"\n", args...)
		problems++
	}
	ok := func(format string, args ...any) {
		fmt.Print("✓  ")
		fmt.Printf(format+"\n", args...)
	}

	// --- 1. Config ---
	cfg, err := config.Load()
	if err != nil {
		fail("config: %v", err)
		return 1
	}
	ok("config loaded — app_id=%s allowed_orgs=%v", cfg.GitHubAppID, cfg.AllowedOrgs)

	// --- 2. Database ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if pool, err := db.Connect(ctx, cfg.DatabaseURL); err != nil {
		fail("database: connect: %v", err)
	} else {
		if err := pool.Ping(ctx); err != nil {
			fail("database: ping: %v", err)
		} else {
			ok("database: connect + ping")
		}
		pool.Close()
	}

	// --- 3. App credentials (PEM parsing + JWT round-trip) ---
	appJWT, err := signAppJWT(cfg.GitHubAppID, cfg.GitHubPrivateKey)
	if err != nil {
		fail("app credentials: %v", err)
		return printSummary(problems)
	}
	appInfo, err := ghGetMap(appJWT, "https://api.github.com/app")
	if err != nil {
		fail("app: GET /app failed: %v", err)
		return printSummary(problems)
	}
	ok("app: id=%d name=%q slug=%v", intAt(appInfo, "id"), appInfo["name"], appInfo["slug"])

	// Required permissions on the App itself.
	appPerms := mapAt(appInfo, "permissions")
	checkPerm := func(name, want string) {
		got, _ := appPerms[name].(string)
		switch {
		case got == want:
			ok("app permissions: %s = %s", name, got)
		case got == "":
			fail("app permissions: %s is missing (must be %q)", name, want)
		default:
			fail("app permissions: %s = %q (must be %q for bot replies)", name, got, want)
		}
	}
	checkPerm("issues", "write")
	checkPerm("pull_requests", "write")

	// Required event subscriptions on the App.
	appEvents := sliceAt(appInfo, "events")
	checkEvent := func(name string, hard bool) {
		if sliceContainsString(appEvents, name) {
			ok("app events: %s subscribed", name)
			return
		}
		if hard {
			fail("app events: %s not subscribed (required)", name)
		} else {
			warn("app events: %s not subscribed (optional — needed only for PR review-comment /log)", name)
		}
	}
	checkEvent("issue_comment", true)
	checkEvent("pull_request_review_comment", false)

	// --- 4. Installations ---
	insts, err := ghGetSlice(appJWT, "https://api.github.com/app/installations?per_page=100")
	if err != nil {
		fail("installations: GET /app/installations: %v", err)
		return printSummary(problems)
	}
	ok("installations: %d found", len(insts))
	for _, ix := range insts {
		inst, _ := ix.(map[string]any)
		account := stringAt(mapAt(inst, "account"), "login")
		instID := intAt(inst, "id")
		perms := mapAt(inst, "permissions")
		events := sliceAt(inst, "events")
		issuesPerm, _ := perms["issues"].(string)
		prsPerm, _ := perms["pull_requests"].(string)

		fmt.Printf("    install id=%d account=%s issues=%s pull_requests=%s events=%d\n",
			instID, account, issuesPerm, prsPerm, len(events))

		if issuesPerm != "write" {
			fail("    install on %s has issues=%q (must be write — accept pending permissions at https://github.com/settings/installations)", account, issuesPerm)
		}
		if prsPerm != "write" {
			fail("    install on %s has pull_requests=%q (must be write — accept pending permissions at https://github.com/settings/installations)", account, prsPerm)
		}
	}

	// --- 5. Recent webhook deliveries ---
	delivs, err := ghGetSlice(appJWT, "https://api.github.com/app/hook/deliveries?per_page=10")
	if err != nil {
		warn("deliveries: GET /app/hook/deliveries: %v", err)
	} else {
		fmt.Printf("✓  recent webhook deliveries (last %d):\n", len(delivs))
		for _, d := range delivs {
			dd, _ := d.(map[string]any)
			status, _ := dd["status_code"].(float64)
			marker := "  "
			if int(status) >= 400 {
				marker = "⚠ "
			}
			fmt.Printf("    %s %v  %-32s status=%-3v  %v\n",
				marker, dd["delivered_at"], dd["event"], int(status), dd["status"])
		}
	}

	// --- 6. Effective URLs ---
	fmt.Println()
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		warn("BASE_URL is not set — falling back to http://localhost:%s (only works for local dev)", cfg.Port)
		baseURL = "http://localhost:" + cfg.Port
	}
	fmt.Printf("    BASE_URL              = %s\n", baseURL)
	fmt.Printf("    Expected Callback URL = %s/auth/callback\n", baseURL)
	fmt.Printf("    Expected Webhook URL  = %s/webhook\n", baseURL)
	fmt.Println("    Verify these match the values configured in the GitHub App settings.")

	return printSummary(problems)
}

func printSummary(problems int) int {
	fmt.Println()
	if problems == 0 {
		fmt.Println("doctor: all checks passed")
		return 0
	}
	fmt.Printf("doctor: %d problem(s) found — see ✗ lines above\n", problems)
	return 1
}

// signAppJWT builds the App-level JWT GitHub expects on /app and
// /app/installations endpoints. PEM-parsing failures here mean the
// GITHUB_PRIVATE_KEY environment variable is malformed.
func signAppJWT(appID, privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("GITHUB_PRIVATE_KEY: cannot decode PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("GITHUB_PRIVATE_KEY: parse RSA private key: %w", err)
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
		Issuer:    appID,
	})
	return tok.SignedString(key)
}

func ghGet(appJWT, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func ghGetMap(appJWT, url string) (map[string]any, error) {
	body, err := ghGet(appJWT, url)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

func ghGetSlice(appJWT, url string) ([]any, error) {
	body, err := ghGet(appJWT, url)
	if err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

func mapAt(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, _ := m[key].(map[string]any)
	return v
}

func sliceAt(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

func stringAt(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// intAt extracts an integer value from a JSON-decoded map. JSON numbers
// arrive as float64 by default; this helper renders them back as int64
// without the scientific-notation that %v gives for large floats.
func intAt(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	f, _ := m[key].(float64)
	return int64(f)
}

func sliceContainsString(s []any, want string) bool {
	for _, v := range s {
		if str, ok := v.(string); ok && str == want {
			return true
		}
	}
	return false
}
