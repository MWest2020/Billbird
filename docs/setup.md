# Setup guide

This guide walks you through registering a GitHub App and running Billbird.

## 1. Register a GitHub App

Go to your GitHub organization settings (or personal settings) and create a new GitHub App:

**Settings > Developer settings > GitHub Apps > New GitHub App**

Fill in the following:

| Field | Value |
|-------|-------|
| **App name** | Billbird (or any name you prefer) |
| **Homepage URL** | Your Billbird instance URL |
| **Webhook URL** | `https://your-domain.com/webhook` |
| **Webhook secret** | Generate a random string (save it for later) |

### Permissions

Under **Repository permissions**:

| Permission | Access |
|------------|--------|
| Issues | Read & write |
| Pull requests | Read only |
| Metadata | Read only |

Under **Organization permissions**:

| Permission | Access |
|------------|--------|
| Members | Read only |

### Events

Subscribe to these webhook events:

- Issue comments
- Projects v2 items (for cycle time tracking)
- Pull requests (for cycle time tracking)

### Post-creation

After creating the app:

1. Note the **App ID** from the app's general settings page
2. Generate a **private key** and download the `.pem` file
3. Note the **Client ID** and generate a **Client secret** (for admin panel OAuth)
4. Install the app on your organization/repositories

## 2. Configure environment variables

Billbird is configured entirely through environment variables. Create a `.env` file:

```bash
# Required
DATABASE_URL=postgres://billbird:billbird@localhost:5432/billbird?sslmode=disable
GITHUB_APP_ID=123456
GITHUB_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
...your private key contents...
-----END RSA PRIVATE KEY-----"
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# Required — comma-separated GitHub org names whose members can log time
ALLOWED_ORGS=your-org

# Optional (required for admin panel)
GITHUB_CLIENT_ID=Iv1.abc123
GITHUB_CLIENT_SECRET=your-client-secret
SESSION_SECRET=a-random-secret-for-signing-cookies

# Optional
PORT=8080
```

See [configuration.md](configuration.md) for the full reference.

## 3. Run with Docker Compose

```bash
cp env.example .env
# Edit .env with your values

docker compose up
```

This starts:
- PostgreSQL 17 on port 5432
- Billbird on port 8080

Database migrations run automatically on startup.

## 4. Verify

Check the health endpoint:

```bash
curl http://localhost:8080/healthz
# ok
```

Create a test issue in a repository where the app is installed. Comment `/log 1h test` and verify the bot replies with a confirmation.

## 5. Expose Billbird publicly

GitHub's webhook deliveries and the OAuth admin-panel callback both need a public HTTPS URL. Pick one of the deployment options in [self-hosting.md](self-hosting.md#exposing-billbird-publicly):

- **Reverse proxy with TLS** (nginx, Caddy, …) if your host has a public IP
- **Cloudflare Tunnel** if your host is behind NAT or VPN-only
- **Development**: [smee.io](https://smee.io) or `ngrok` for short-lived local testing

Once you have a public URL, set `BASE_URL` in `.env` and update the GitHub App's **Callback URL** and **Webhook URL** to match. See [configuration.md](configuration.md#public-url-base_url) for details.

## Next steps

- [Configure client attribution](client-attribution.md) to map labels to clients
- [Deploy to Kubernetes](self-hosting.md) for production use
