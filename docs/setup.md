# Setup guide

This guide walks you from zero to a Billbird instance that processes `/log` comments end-to-end. The flow is sequenced so you fill every form field with a real value the first time — no "come back later to update this URL" loops.

## 0. Decide how Billbird will be reachable

GitHub's webhook deliveries need a public HTTPS URL. Pick one option **before** you touch the GitHub App settings, because that URL is the first thing GitHub will ask for and you cannot install the App without it.

| Option | When | What you get |
|---|---|---|
| **Cloudflare Tunnel** | Self-hosting on a box without a public IP (NAT, VPN-only, homelab) | `https://billbird.<your-domain>` via `cloudflared`; no port forwarding, free TLS |
| **Reverse proxy (nginx/Caddy/Traefik) with Let's Encrypt** | Self-hosting on a box with a public IP | Standard vhost + cert |
| **smee.io or ngrok** | Local development, ephemeral testing only | Throwaway public URL that proxies to localhost; **do not use for production** — the URL changes per session |

See [self-hosting.md](self-hosting.md#exposing-billbird-publicly) for the configs.

**Write down your chosen URL** (without trailing slash). Examples: `https://billbird.westerweel.work`, `https://billbird.example.com`. This is your `BASE_URL`; every following step references it.

## 1. Register a GitHub App

Go to **Settings → Developer settings → GitHub Apps → New GitHub App** (in your personal settings, or in the org's settings for an org-owned App).

### Identifying information

| Field | Value |
|---|---|
| **App name** | Billbird (or anything you prefer) |
| **Homepage URL** | `https://<your BASE_URL>` |
| **Callback URL** | `https://<your BASE_URL>/auth/callback` |
| **Setup URL** | leave empty |
| **Webhook URL** | `https://<your BASE_URL>/webhook` |
| **Webhook secret** | Generate a random string with `openssl rand -hex 32` and copy it; you cannot view it again |

Tick **"Request user authorization (OAuth) during installation"** so the admin panel login works.

### Repository permissions

| Permission | Access | Why |
|---|---|---|
| **Issues** | Read & write | Read for label fetch; write to post the bot's `Logged 1h …` confirmation comments |
| **Pull requests** | Read & write | Same as issues, but for `/log` on PR conversations and PR review comments. **Read-only will silently fail** with `403 Resource not accessible by integration` whenever the bot tries to reply on a PR |
| **Metadata** | Read only | Required for any App |

### Organization permissions

| Permission | Access | Why |
|---|---|---|
| **Members** | Read only | Membership check against `ALLOWED_ORGS` |

### Events

Tick the events Billbird needs:

| Event | Why |
|---|---|
| **Issue comment** | The `/log`, `/correct`, `/delete`, `/plan`, `/unplan` commands on issues |
| **Pull request review comment** | The same commands inline on PR review threads |

The "Pull request" and "Projects v2 item" events are placeholders for future cycle-time tracking — skip them for v1.

### After saving

On the App's General settings page, capture:

1. **App ID** (numeric, e.g. `3365040`)
2. Click **Generate a private key** → downloads a `.pem` file
3. **Client ID** (already shown, e.g. `Iv23ligs…`)
4. **Generate a new client secret** → copy once, you cannot view it again

You now have the five secrets you need: webhook secret, App ID, private key, client ID, client secret.

## 2. Configure environment variables

```bash
git clone https://github.com/mwesterweel/billbird.git
cd billbird
cp env.example .env
```

Edit `.env`:

```bash
# Public URL — MUST match what you registered in the GitHub App
BASE_URL=https://billbird.example.com

# Database (Docker Compose provisions Postgres on db:5432)
DATABASE_URL=postgres://billbird:billbird@db:5432/billbird?sslmode=disable

# GitHub App credentials
GITHUB_APP_ID=123456
GITHUB_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
...full PEM contents, literal newlines...
-----END RSA PRIVATE KEY-----"
GITHUB_WEBHOOK_SECRET=the-random-string-from-step-1

# Comma-separated GitHub orgs/users whose members can log time
ALLOWED_ORGS=your-org,your-personal-handle

# Admin panel OAuth
GITHUB_CLIENT_ID=Iv23ligs...
GITHUB_CLIENT_SECRET=the-client-secret-from-step-1
SESSION_SECRET=$(openssl rand -hex 32)
```

See [configuration.md](configuration.md) for every variable.

## 3. Run Billbird

```bash
docker compose up -d
docker compose logs -f app
```

Wait for `billbird listening on :8080` in the logs. Database migrations run automatically.

## 4. Install the App

In the GitHub App settings, click **Install App** in the left sidebar, pick the account or org, and choose which repos. Pick a single test repo first if you want a low-blast-radius smoke.

## 5. Smoke test

```bash
# Liveness
curl https://<your BASE_URL>/healthz   # → ok

# Admin panel — should redirect to GitHub OAuth, then land on /admin/
open https://<your BASE_URL>/admin/
```

Then open an issue in a repo where the App is installed and post a comment:

```
/log 5m setup smoke test
```

Within ~5 seconds the bot replies with `Logged 5m for @you (entry #1) — setup smoke test`. Refresh `/admin/` to see the entry.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `/auth/login` redirects to GitHub with `redirect_uri mismatch` error | App's Callback URL field doesn't match Billbird's `BASE_URL/auth/callback` | Update either side so they match exactly (scheme + host + port + path) |
| Webhook deliveries 404 in App "Recent Deliveries" | Webhook URL points to `/webhooks` (plural) or wrong path | Fix to `<BASE_URL>/webhook` (singular, no trailing slash) |
| Bot doesn't reply but webhook returns 200 | The App's installation token has `pull_requests: read` (App permission change not yet accepted on the install) | Accept new permissions at `/settings/installations` |
| Bot replies on real issues but not on PRs | `Pull requests` permission is `Read only` | Bump to `Read & write` in App settings, then accept on install |
| After changing App permissions, bot still gets 403 | Cached installation token has the old scope; it refreshes within 1 hour | `docker compose down && docker compose up -d` clears the cache immediately. See [operations.md](operations.md#when-the-cache-is-invalidated) |

## Next steps

- [Configure client attribution](client-attribution.md) to map labels to clients
- [Deploy to Kubernetes](self-hosting.md) for production use
- Run `billbird doctor` to validate App credentials, install permissions, and webhook deliveries from the command line (see [operations.md](operations.md))
