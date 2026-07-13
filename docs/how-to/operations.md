---
status: draft
last_reviewed: 2026-07-13
---

# Operations notes

Small operational details about how Billbird behaves under load and during incidents. This is not a deployment guide — see [self-hosting.md](self-hosting.md) for that.

## `billbird doctor`

One-shot diagnostic that checks everything Billbird depends on without writing any state. Run it whenever something stops working or after changing GitHub App settings:

```bash
docker compose exec app billbird doctor
# or, locally:
go run ./cmd/billbird doctor
```

What it checks, in order:

1. **Config** — every required environment variable is set.
2. **Postgres** — can connect + ping.
3. **GitHub App credentials** — `GITHUB_PRIVATE_KEY` parses as PEM, signing a JWT succeeds, and `GET /app` returns the same `App ID` the env claims.
4. **App-level permissions** — `issues` and `pull_requests` are both `write`. Anything less and bot replies will silently 403.
5. **App-level event subscriptions** — `issue_comment` is subscribed (hard requirement); `pull_request_review_comment` (soft — only matters if you want `/log` inline on PR reviews).
6. **Per-installation permissions** — for each install, both `issues` and `pull_requests` are `write`. A mismatch here (App says write, install says read) means the install still has pending permissions to accept at `https://github.com/settings/installations`.
7. **Recent webhook deliveries** — last 10 events with their HTTP status codes, flagged in red when 4xx/5xx. A `404` here is almost always a wrong Webhook URL field; a `401` is a webhook secret mismatch.
8. **Effective URLs** — the `BASE_URL` Billbird will use and the matching Callback URL + Webhook URL the GitHub App must be configured with.

The command exits non-zero on any concrete problem so it composes cleanly into shell scripts (`billbird doctor && deploy ...`).

## GitHub installation tokens

## GitHub installation tokens

Billbird authenticates to GitHub as a GitHub App, not as a user. The flow:

1. Billbird signs a short-lived JWT with the App's private key.
2. Billbird POSTs the JWT to `/app/installations/{id}/access_tokens` to get a 1-hour-valid installation token scoped to one specific install.
3. Billbird uses the installation token for every API call (posting comments, fetching labels, checking org membership) until it expires, then refreshes.

The installation token is cached in-memory keyed by installation ID. The cache survives until the process restarts.

### Why fetch is wrapped in `singleflight`

GitHub rate-limits installation-token requests per App. A naive implementation looks like this:

```go
if cached, ok := cache[id]; ok && stillValid(cached) {
    return cached.token
}
return fetchFromGitHub(id)   // racy
```

The window between the cache miss and the cache write is wide enough that N concurrent webhooks all see "no cached token" and all call `/app/installations/.../access_tokens`. Each request counts against the App's rate limit; the responses race each other into the cache; only the last-writer-wins entry is kept. Wasteful and order-sensitive.

`getInstallationToken` in `internal/github/client.go` wraps the fetch in `golang.org/x/sync/singleflight`:

```go
key := strconv.FormatInt(installationID, 10)
tok, err, _ := c.tokenFlight.Do(key, func() (any, error) {
    // double-check cache, then fetch if still missing
    ...
})
```

Singleflight guarantees: **at most one fetch per installation ID runs at a time**. Concurrent callers block on the in-flight call and all share the resulting token. The first caller does the GitHub round-trip; the rest get the cached value back instantly. Rate-limit pressure stays proportional to *distinct installations × token-expiry rate*, not to *webhook QPS*.

Inside the singleflight slot we re-check the cache: while a caller was waiting for its slot, a previous winner may have populated the cache, so the fetch isn't needed at all.

### When the cache is invalidated

There is no manual invalidation. The cache only refreshes when:

- The cached entry is within 60 seconds of its `expires_at` timestamp, or
- The process restarts.

**If the App's permissions change** (e.g. you grant `pull_requests: write` after starting up), existing cached install tokens keep their old scope and continue to fail with `403 "Resource not accessible by integration"` for the new permission until they expire. Either wait up to one hour for natural rotation, or restart Billbird to drop the cache immediately. The restart is the recommended path during setup; the natural expiry is fine for any change that lands without anyone watching.

## Webhook deliveries

See [webhook-idempotency.md](../reference/webhook-idempotency.md) for how Billbird ensures every `/log` is processed exactly once even though GitHub may deliver the same event multiple times.

## Sensitive operations and audit

- **Time entries are never physically deleted.** `/delete` and `/correct` create new rows that supersede prior rows. The full chain stays in the database for audit.
- **API tokens** are bcrypt-hashed; the plaintext is shown exactly once on the page that the create-form returns to, never persisted, and never put into a URL (see [api-tokens.md](../reference/api-tokens.md)).
- **Webhook secret** verifies HMAC-SHA256 with `hmac.Equal` (constant-time compare) and fail-closes when the configured secret is empty. There is no way to disable signature verification.
