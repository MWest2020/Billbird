---
status: draft
last_reviewed: 2026-07-13
---

# Configuration reference

Billbird is configured entirely through environment variables. No configuration files are required at runtime.

## Required variables

These must be set for the application to start. If any are missing, the application exits immediately with an error naming the missing variable.

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@host:5432/billbird?sslmode=disable` |
| `GITHUB_APP_ID` | Numeric ID of your GitHub App | `123456` |
| `GITHUB_PRIVATE_KEY` | PEM-encoded private key for the GitHub App | `-----BEGIN RSA PRIVATE KEY-----\n...` |
| `GITHUB_WEBHOOK_SECRET` | Secret used to verify webhook signatures | `whsec_abc123...` |
| `ALLOWED_ORGS` | Comma-separated list of GitHub org names whose members can use slash commands | `my-org` or `org-a,org-b` |

## Optional variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `BASE_URL` | Public URL Billbird is reachable on. Used to build the OAuth callback (`BASE_URL/auth/callback`); must match the "Callback URL" configured in the GitHub App. | `http://localhost:PORT` |
| `GITHUB_CLIENT_ID` | OAuth client ID (for admin panel login) | *(none)* |
| `GITHUB_CLIENT_SECRET` | OAuth client secret (for admin panel login) | *(none)* |
| `SESSION_SECRET` | Secret for signing session cookies | *(none)* |

## Development-only flags

| Variable | What it does | Why |
|----------|--------------|-----|
| `BILLBIRD_DEV_MEMBERSHIP_BYPASS` | When set to `true`, every bearer token is treated as belonging to an allowed-org member. The startup log prints a banner so the override is visible. | Local smoke testing without a registered GitHub App. **Must not be set in production.** The setting widens API access from "ALLOWED_ORGS members" to "anyone with any valid token". |

## Notes

### Private key format

The `GITHUB_PRIVATE_KEY` value should contain the full PEM-encoded key, including the `-----BEGIN RSA PRIVATE KEY-----` and `-----END RSA PRIVATE KEY-----` markers. In Docker Compose or shell environments, wrap the value in quotes and use literal newlines.

### Database URL

The `DATABASE_URL` must be a valid PostgreSQL connection string. For local development with Docker Compose, the default is:

```
postgres://billbird:billbird@db:5432/billbird?sslmode=disable
```

For production, use TLS:

```
postgres://user:pass@host:5432/billbird?sslmode=require
```

### Allowed orgs

`ALLOWED_ORGS` controls who can use Billbird slash commands. Only members of the listed GitHub organizations can `/log`, `/correct`, or `/delete`. Non-members get an error comment. Multiple orgs are comma-separated: `org-a,org-b`.

Billbird checks org membership via the GitHub API on every command. No user registration in Billbird itself is needed --- membership is managed entirely through GitHub.

### Admin panel variables

The admin panel requires `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and `SESSION_SECRET` to be set. Admin access is granted to members of any org listed in `ALLOWED_ORGS`. Without the panel variables, the webhook endpoint and health check still function, but the admin panel is unavailable.

### Public URL (`BASE_URL`)

When admin panel users log in, Billbird sends them through GitHub OAuth. GitHub redirects back to `BASE_URL/auth/callback`, and that URL must match the **Callback URL** configured in the GitHub App's settings exactly — including scheme (`https`) and port.

If `BASE_URL` is unset Billbird falls back to `http://localhost:PORT`, which works for local development but produces a `redirect_uri mismatch` error from GitHub for any other deployment. Set `BASE_URL` to your real public URL — for example `https://billbird.example.com` — and configure the matching Callback URL in the GitHub App.

The webhook receiver (`POST /webhook`) does not use `BASE_URL`; GitHub's servers reach it directly via the **Webhook URL** field in the GitHub App. Both URLs share the same public host in a typical deployment.

### Kubernetes

In Kubernetes deployments, sensitive variables (`GITHUB_PRIVATE_KEY`, `GITHUB_WEBHOOK_SECRET`, `GITHUB_CLIENT_SECRET`, `SESSION_SECRET`, `DATABASE_URL`) should be stored in Kubernetes Secrets and referenced in the pod spec. See [self-hosting.md](../how-to/self-hosting.md) for Helm chart configuration.
