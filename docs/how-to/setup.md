---
status: draft
last_reviewed: 2026-07-13
---

# Setup guide

This guide walks you from zero to a Billbird instance that processes `/log` comments end-to-end.

There are two paths:

- **Automated (recommended)** — `billbird init` registers the GitHub App via GitHub's App Manifest flow, generates every secret, and writes them to `.env` in one round-trip. ~5 minutes start to finish.
- **Manual** — fill out the GitHub App form yourself, paste secrets into `.env` by hand. ~30 minutes. Useful if you want to inspect each field or you already have an App you want to reuse.

Both paths share the same first step: pick how Billbird will be reachable from the public internet.

## 0. Decide how Billbird will be reachable

GitHub's webhook deliveries need a public HTTPS URL. Pick one option **before** running anything, because that URL goes into every step that follows.

| Option | When | What you get |
|---|---|---|
| **Cloudflare Tunnel** | Self-hosting on a box without a public IP (NAT, VPN-only, homelab) | `https://billbird.<your-domain>` via `cloudflared`; no port forwarding, free TLS |
| **Reverse proxy (nginx/Caddy/Traefik) with Let's Encrypt** | Self-hosting on a box with a public IP | Standard vhost + cert |
| **smee.io or ngrok** | Local development, ephemeral testing only | Throwaway public URL that proxies to localhost; **do not use for production** |

See [self-hosting.md](self-hosting.md#exposing-billbird-publicly) for the configs.

**Write down your chosen URL** (without trailing slash). Examples: `https://billbird.westerweel.work`, `https://billbird.example.com`. This is your `BASE_URL`.

---

## Automated path: `billbird init`

### 1. Clone, prepare `.env`

```bash
git clone https://github.com/mwesterweel/billbird.git
cd billbird
cp env.example .env

# Edit .env: set BASE_URL and ALLOWED_ORGS at minimum.
# Database URL has a working default for Docker Compose.
```

`.env` only needs two values from you at this stage:

```bash
BASE_URL=https://billbird.example.com   # your public URL from step 0
ALLOWED_ORGS=your-org                   # GitHub org or user login(s)
```

Leave the GitHub App secrets blank — `init` will fill them in.

### 2. Run `billbird init`

```bash
docker compose run --rm --service-ports app billbird init
```

`init` starts a one-shot HTTP server on port 8080 and prints a URL like:

```
Open this URL in your browser:

    https://billbird.example.com/init
```

Open that URL. You'll see a single confirm button. Click it once:

1. Your browser POSTs a pre-filled manifest to GitHub.
2. GitHub creates the App (correct permissions, correct events, freshly-generated webhook secret, private key, and OAuth client).
3. GitHub redirects you back to `<BASE_URL>/init/callback` with a one-time code.
4. `init` exchanges the code for the App's secrets, writes them into `.env`, and exits.

### 3. Install the App

`init` prints an install URL when it finishes — something like `https://github.com/apps/billbird-<random>/installations/new`. Open it in the browser, pick the account or org, and choose which repos.

### 4. Start Billbird

```bash
docker compose up -d
docker compose exec app billbird doctor
```

`doctor` confirms every dependency is reachable and the App is configured correctly. If it prints all green ticks, you're ready to smoke-test.

### 5. Smoke test

Open an issue in a repo where the App is installed and post a comment:

```
/log 5m setup smoke test
```

Within ~5 seconds the bot replies. Visit `https://<BASE_URL>/admin/` to see the entry.

---

## Manual path

Use this if you want to register the App yourself, or you already have an App and want to point Billbird at it.

### 1. Register a GitHub App

Go to **Settings → Developer settings → GitHub Apps → New GitHub App**.

| Field | Value |
|---|---|
| **App name** | Billbird (or anything you prefer) |
| **Homepage URL** | `https://<BASE_URL>` |
| **Callback URL** | `https://<BASE_URL>/auth/callback` |
| **Webhook URL** | `https://<BASE_URL>/webhook` |
| **Webhook secret** | `openssl rand -hex 32` |

Tick **"Request user authorization (OAuth) during installation"**.

Repository permissions:

| Permission | Access | Why |
|---|---|---|
| **Issues** | Read & write | Read for label fetch; write to post bot confirmations |
| **Pull requests** | Read & write | Same as issues, but for PR comments. **Read-only silently 403s** on every PR-side bot reply |
| **Metadata** | Read only | Required for any App |

Organization permissions:

| Permission | Access | Why |
|---|---|---|
| **Members** | Read only | Membership check against `ALLOWED_ORGS` |

Events:

| Event | Why |
|---|---|
| **Issue comment** | `/log`, `/correct`, `/delete`, `/plan`, `/unplan` on issues |
| **Pull request review comment** | Same commands inline on PR review threads |

After saving:

1. Capture the **App ID** (numeric).
2. Click **Generate a private key** → downloads a `.pem` file.
3. Capture the **Client ID**, then **Generate a new client secret** and copy it once.

### 2. Configure `.env`

```bash
cp env.example .env
```

Fill in:

```bash
BASE_URL=https://billbird.example.com
DATABASE_URL=postgres://billbird:billbird@db:5432/billbird?sslmode=disable
GITHUB_APP_ID=123456
GITHUB_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
...full PEM contents...
-----END RSA PRIVATE KEY-----"
GITHUB_WEBHOOK_SECRET=the-secret-from-step-1
ALLOWED_ORGS=your-org
GITHUB_CLIENT_ID=Iv23...
GITHUB_CLIENT_SECRET=the-client-secret-from-step-1
SESSION_SECRET=$(openssl rand -hex 32)
```

See [configuration.md](../reference/configuration.md) for every variable.

### 3. Run + install + smoke

Same as steps 3–5 of the automated path above.

---

## Troubleshooting

`billbird doctor` is the canonical first stop. It prints `✗` lines for every concrete problem; the table below is the rough mapping from doctor output to underlying cause.

| Symptom (doctor or runtime) | Cause | Fix |
|---|---|---|
| `app permissions: pull_requests = "read"` | App was registered with PR-read-only | Bump to Read & write in App settings, then accept on install at `/settings/installations` |
| `install on X has pull_requests="read"` (but App permissions say write) | App permissions were changed but install hasn't accepted | Accept the pending permissions at `/settings/installations` |
| Recent webhook delivery shows `404` | Webhook URL points to wrong path (e.g. `/webhooks` plural) | Fix to `<BASE_URL>/webhook` (singular) |
| Recent webhook delivery shows `401` | Webhook secret mismatch between App and `.env` | Make them match; restart Billbird |
| `/auth/login` errors with `redirect_uri mismatch` | App's Callback URL doesn't match `<BASE_URL>/auth/callback` | Update either side so they match exactly |
| After changing App permissions, bot still 403s | Cached installation token has the old scope | `docker compose down && docker compose up -d` clears the cache immediately. See [operations.md](operations.md#when-the-cache-is-invalidated) |

## Next steps

- [Configure client attribution](../reference/client-attribution.md) to map labels to clients
- [Deploy to Kubernetes](self-hosting.md) for production use
