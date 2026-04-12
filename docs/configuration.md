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
| `GITHUB_CLIENT_ID` | OAuth client ID (for admin panel login) | *(none)* |
| `GITHUB_CLIENT_SECRET` | OAuth client secret (for admin panel login) | *(none)* |
| `SESSION_SECRET` | Secret for signing session cookies | *(none)* |

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

### Kubernetes

In Kubernetes deployments, sensitive variables (`GITHUB_PRIVATE_KEY`, `GITHUB_WEBHOOK_SECRET`, `GITHUB_CLIENT_SECRET`, `SESSION_SECRET`, `DATABASE_URL`) should be stored in Kubernetes Secrets and referenced in the pod spec. See [self-hosting.md](self-hosting.md) for Helm chart configuration.
