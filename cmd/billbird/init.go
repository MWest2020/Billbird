package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const initUsage = `usage: billbird init [flags]

Bootstrap a new GitHub App for Billbird via GitHub's App Manifest flow.
Creates the App with correct permissions and event subscriptions, generates
all secrets, and writes them to your .env file.

This skips the ~15 manual form fields and 5 secret-management steps the
normal GitHub App setup requires.

Prerequisites:
  - BASE_URL must be set (env var or --base-url flag). Must match the
    public URL where Billbird will be reachable; GitHub redirects there
    after App creation.

Flags:
  --base-url <url>     Public URL where Billbird is reachable. Required
                       unless BASE_URL is set in the environment.
  --name <string>      App name. Default: "Billbird".
  --org <string>       Create as an organization-owned App. Default:
                       owned by the GitHub user who clicks Create.
  --env-file <path>    Path to .env. Default: .env (in CWD).
  --force              Overwrite existing values in the .env file.
                       Default behaviour is to preserve existing keys.

Example:
  BASE_URL=https://billbird.example.com billbird init
  billbird init --base-url https://billbird.example.com --org acme
`

// runInit drives the manifest bootstrap flow.
func runInit() int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, initUsage) }
	var (
		baseURL = fs.String("base-url", os.Getenv("BASE_URL"), "")
		appName = fs.String("name", "Billbird", "")
		org     = fs.String("org", "", "")
		envFile = fs.String("env-file", ".env", "")
		force   = fs.Bool("force", false, "")
	)
	if err := fs.Parse(os.Args[2:]); err != nil {
		return 2
	}
	if *baseURL == "" {
		fmt.Fprintln(os.Stderr, "error: BASE_URL not set; pass --base-url or export BASE_URL")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		return 2
	}
	*baseURL = strings.TrimRight(*baseURL, "/")

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot generate state token: %v\n", err)
		return 1
	}
	state := hex.EncodeToString(stateBytes)

	manifest := buildManifest(*appName, *baseURL)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot marshal manifest: %v\n", err)
		return 1
	}

	formAction := "https://github.com/settings/apps/new"
	if *org != "" {
		formAction = "https://github.com/organizations/" + *org + "/settings/apps/new"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	done := make(chan initResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /init", renderInitPage(*appName, manifestJSON, state, formAction))
	mux.HandleFunc("GET /init/callback", handleInitCallback(state, done))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			done <- initResult{err: fmt.Errorf("listen: %w", err)}
		}
	}()
	defer srv.Shutdown(context.Background())

	fmt.Println("=== Billbird init ===")
	fmt.Println()
	fmt.Println("Open this URL in your browser:")
	fmt.Println()
	fmt.Printf("    %s/init\n", *baseURL)
	fmt.Println()
	fmt.Println("You'll see a confirm button. Click it once → GitHub creates the App")
	fmt.Println("with the right permissions, then sends you back here with the secrets.")
	fmt.Println()
	fmt.Println("Waiting for the callback (Ctrl+C to abort)...")

	select {
	case res := <-done:
		if res.err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", res.err)
			return 1
		}
		return writeEnvAndPrintNext(*envFile, *force, *baseURL, res.app)
	case <-time.After(10 * time.Minute):
		fmt.Fprintln(os.Stderr, "\ntimed out after 10 minutes waiting for callback")
		return 1
	}
}

type initResult struct {
	app *manifestConversion
	err error
}

// manifestConversion is the subset of fields GitHub returns from
// POST /app-manifests/{code}/conversions that init actually uses.
type manifestConversion struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	HTMLURL       string `json:"html_url"`
	PEM           string `json:"pem"`
	WebhookSecret string `json:"webhook_secret"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
}

func buildManifest(name, baseURL string) map[string]any {
	return map[string]any{
		"name":        name,
		"url":         baseURL,
		"description": "Self-hosted time tracking driven by GitHub slash commands. https://github.com/MWest2020/Billbird",
		"hook_attributes": map[string]any{
			"url":    baseURL + "/webhook",
			"active": true,
		},
		// redirect_url is where GitHub sends the operator after the App is
		// created. It is *not* the OAuth callback — that's callback_urls below.
		"redirect_url":            baseURL + "/init/callback",
		"callback_urls":           []string{baseURL + "/auth/callback"},
		"request_oauth_on_install": true,
		"public":                  false,
		"default_permissions": map[string]string{
			"issues":        "write",
			"pull_requests": "write",
			"metadata":      "read",
			"members":       "read",
		},
		"default_events": []string{"issue_comment", "pull_request_review_comment"},
	}
}

// renderInitPage shows a single confirm button that POSTs the manifest to
// GitHub. No auto-submit — operator gets one chance to read what's about to
// happen before they click.
func renderInitPage(appName string, manifestJSON []byte, state, formAction string) http.HandlerFunc {
	tmpl := `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>Billbird init</title>
<style>
  body { font-family: -apple-system, sans-serif; max-width: 640px; margin: 4em auto; padding: 0 1em; color: #222; }
  h1 { font-size: 1.4em; }
  pre { background: #f4f4f4; padding: 1em; overflow-x: auto; font-size: 0.85em; }
  button { font-size: 1.1em; padding: 0.6em 1.4em; cursor: pointer; }
</style>
</head><body>
<h1>Create the {{.AppName}} GitHub App</h1>
<p>Clicking the button below sends the manifest below to GitHub, which creates
a new GitHub App on your account with these settings — no manual form fields.</p>
<ul>
  <li>Webhook URL: <code>{{.WebhookURL}}</code></li>
  <li>OAuth callback: <code>{{.CallbackURL}}</code></li>
  <li>Permissions: <code>issues=write</code>, <code>pull_requests=write</code>, <code>metadata=read</code>, <code>members=read</code></li>
  <li>Events: <code>issue_comment</code>, <code>pull_request_review_comment</code></li>
</ul>
<p>After GitHub creates the App you'll be redirected back here, and the
secrets (App ID, private key, webhook secret, OAuth client) get written
into your <code>.env</code> file.</p>
<form action="{{.FormAction}}?state={{.State}}" method="post">
  <input type="hidden" name="manifest" value='{{.Manifest}}'>
  <button type="submit">Create GitHub App on GitHub</button>
</form>
<details style="margin-top: 2em;">
  <summary>See the raw manifest</summary>
  <pre>{{.ManifestPretty}}</pre>
</details>
</body></html>`

	t := template.Must(template.New("init").Parse(tmpl))
	pretty := prettyJSON(manifestJSON)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := struct {
			AppName        string
			WebhookURL     string
			CallbackURL    string
			FormAction     string
			State          string
			Manifest       string
			ManifestPretty string
		}{
			AppName:        appName,
			WebhookURL:     manifestField(manifestJSON, "hook_attributes", "url"),
			CallbackURL:    manifestCallback(manifestJSON),
			FormAction:     formAction,
			State:          state,
			Manifest:       string(manifestJSON),
			ManifestPretty: pretty,
		}
		_ = t.Execute(w, data)
	}
}

func handleInitCallback(expectedState string, done chan<- initResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if state != expectedState {
			http.Error(w, "state mismatch — refusing", http.StatusBadRequest)
			done <- initResult{err: fmt.Errorf("state mismatch in callback (CSRF guard tripped)")}
			return
		}
		if code == "" {
			http.Error(w, "missing 'code' parameter from GitHub", http.StatusBadRequest)
			done <- initResult{err: fmt.Errorf("missing 'code' in callback")}
			return
		}
		app, err := exchangeManifestCode(code)
		if err != nil {
			http.Error(w, "exchange failed: "+err.Error(), http.StatusInternalServerError)
			done <- initResult{err: err}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><body style="font-family:sans-serif;max-width:640px;margin:4em auto;">
<h1>%s created ✓</h1>
<p>The App is registered. Secrets have been written to your local <code>.env</code> file.</p>
<p>Two more steps to go (see the CLI output for details):</p>
<ol>
  <li><strong>Install the App:</strong> <a href="%s/installations/new">%s/installations/new</a></li>
  <li><strong>Set <code>ALLOWED_ORGS</code></strong> in <code>.env</code>, then <code>docker compose up -d</code>.</li>
</ol>
<p>You can close this tab.</p>
</body></html>`, app.Name, app.HTMLURL, app.HTMLURL)
		done <- initResult{app: app}
	}
}

// exchangeManifestCode trades the short-lived code GitHub sent in the
// callback for the App's secrets. The code expires in ~60 seconds, so we
// give the HTTP call a generous timeout but not infinite.
func exchangeManifestCode(code string) (*manifestConversion, error) {
	url := "https://api.github.com/app-manifests/" + code + "/conversions"
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitHub returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out manifestConversion
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode conversion response: %w", err)
	}
	return &out, nil
}

// writeEnvAndPrintNext merges the new secrets into .env (preserving any
// pre-existing values unless --force) and prints the operator's next steps.
func writeEnvAndPrintNext(envFile string, force bool, baseURL string, app *manifestConversion) int {
	sessionSecret, err := randomHex(32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot generate SESSION_SECRET: %v\n", err)
		return 1
	}
	updates := map[string]string{
		"GITHUB_APP_ID":         fmt.Sprintf("%d", app.ID),
		"GITHUB_PRIVATE_KEY":    quoteMultiline(app.PEM),
		"GITHUB_WEBHOOK_SECRET": app.WebhookSecret,
		"GITHUB_CLIENT_ID":      app.ClientID,
		"GITHUB_CLIENT_SECRET":  app.ClientSecret,
		"SESSION_SECRET":        sessionSecret,
		"BASE_URL":              baseURL,
	}
	written, skipped, err := mergeEnvFile(envFile, updates, force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	abs, _ := filepath.Abs(envFile)
	fmt.Println()
	fmt.Printf("✓ GitHub App created: %s (id=%d)\n", app.Name, app.ID)
	fmt.Printf("✓ Secrets written to %s\n", abs)
	if len(skipped) > 0 {
		fmt.Println()
		fmt.Println("  Existing values preserved (pass --force to overwrite):")
		for _, k := range skipped {
			fmt.Printf("    %s\n", k)
		}
	}
	_ = written
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Install the App on your org or account:\n     %s/installations/new\n", app.HTMLURL)
	fmt.Println()
	fmt.Println("  2. Set ALLOWED_ORGS in .env to the GitHub org or user login that should")
	fmt.Println("     be allowed to /log via Billbird. Example: ALLOWED_ORGS=acme,another-org")
	fmt.Println()
	fmt.Println("  3. Start Billbird:")
	fmt.Println("       docker compose up -d")
	fmt.Println()
	fmt.Println("  4. Verify the setup:")
	fmt.Println("       docker compose exec app billbird doctor")
	return 0
}

// mergeEnvFile reads an existing .env (if any), updates lines matching the
// updates map (or appends new keys), and writes the result back. Returns the
// list of keys that were written and the list that already had a value and
// were preserved (when force=false).
func mergeEnvFile(path string, updates map[string]string, force bool) (written, skipped []string, err error) {
	var lines []string
	seen := map[string]bool{}

	if existing, err := os.Open(path); err == nil {
		defer existing.Close()
		sc := bufio.NewScanner(existing)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line)
				continue
			}
			eq := strings.IndexByte(trimmed, '=')
			if eq < 0 {
				lines = append(lines, line)
				continue
			}
			key := strings.TrimSpace(trimmed[:eq])
			newVal, want := updates[key]
			if !want {
				lines = append(lines, line)
				continue
			}
			seen[key] = true
			existingVal := strings.TrimSpace(trimmed[eq+1:])
			if existingVal != "" && !force {
				skipped = append(skipped, key)
				lines = append(lines, line)
				continue
			}
			lines = append(lines, key+"="+newVal)
			written = append(written, key)
		}
		if err := sc.Err(); err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}

	// Append keys that did not exist yet.
	for k, v := range updates {
		if seen[k] {
			continue
		}
		lines = append(lines, k+"="+v)
		written = append(written, k)
	}

	out := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return nil, nil, fmt.Errorf("write %s: %w", path, err)
	}
	return written, skipped, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// quoteMultiline wraps a multi-line value (the PEM) in double-quotes so
// docker-compose's .env loader preserves the newlines.
func quoteMultiline(s string) string {
	if !strings.ContainsRune(s, '\n') {
		return s
	}
	return `"` + s + `"`
}

func prettyJSON(b []byte) string {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(b)
	}
	return string(pretty)
}

// manifestField looks up a nested string field in a JSON manifest.
func manifestField(b []byte, keys ...string) string {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return ""
	}
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[k]
	}
	s, _ := v.(string)
	return s
}

// manifestCallback extracts the first OAuth callback URL from a manifest.
func manifestCallback(b []byte) string {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return ""
	}
	arr, _ := v["callback_urls"].([]any)
	if len(arr) == 0 {
		return ""
	}
	s, _ := arr[0].(string)
	return s
}
