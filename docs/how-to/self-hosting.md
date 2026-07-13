---
status: draft
last_reviewed: 2026-07-13
---

# Self-hosting

Billbird is designed to be self-hosted. You own your data completely. The only external dependencies are PostgreSQL and a GitHub App registration.

## Per-organisation deployment pattern

**Read this before configuring anything.** Billbird is intentionally single-tenant: one Billbird instance per organisation. Two organisations sharing a single Billbird deployment is not a supported pattern and is explicitly out of scope for v2 (see [the architecture doc](../explanation/architecture.md#deployment-topology)).

In practice this means each organisation gets:

- **Its own Postgres database.** No multi-tenant column lives in the schema; data isolation is provided by the deployment boundary. A shared database would commingle organisations' time data, which the schema does not protect against.
- **Its own GitHub App registration.** GitHub Apps are organisation-scoped; this is the natural unit anyway.
- **Its own secret store.** Webhook secret, App private key, OAuth client secret, and session secret should never be reused across organisations.
- **Its own backup cadence.** Billbird never physically deletes rows, so backups are the audit trail. Per-organisation retention requirements apply per backup.

The `ALLOWED_ORGS` environment variable typically holds **one** organisation login. Comma-separated multi-org values stay legal — for example a consulting team that operates from one GitHub org but logs time against client GitHub orgs — but that is a conscious operator choice, not the default model.

A team that genuinely needs multi-tenant SaaS hosting today should fork the project; the scope for v2 does not include it.

## Docker Compose

The simplest way to run Billbird.

### Prerequisites

- Docker and Docker Compose
- A registered GitHub App **dedicated to this organisation** (see [setup.md](setup.md))
- A Postgres database **dedicated to this organisation** (Docker Compose provisions one out of the box)

### Steps

```bash
git clone https://github.com/mwesterweel/billbird.git
cd billbird

# Create your environment file
cp env.example .env
# Edit .env with your GitHub App credentials and a database password

# Start everything
docker compose up -d
```

This starts:
- **PostgreSQL 17** on port 5432 (data persisted in a Docker volume)
- **Billbird** on port 8080

Database migrations run automatically on startup.

### Verify

```bash
curl http://localhost:8080/healthz
# ok
```

### Updating

```bash
git pull
docker compose up -d --build
```

Migrations run automatically --- new schema changes are applied on startup.

### Exposing Billbird publicly

GitHub's webhook delivery and the OAuth admin-panel callback both need a public HTTPS URL. Two common options:

#### Option A: Reverse proxy with TLS (nginx)

If you already operate a reverse proxy on a host with a public IP, point a vhost at Billbird:

```nginx
server {
    listen 443 ssl;
    server_name billbird.example.com;

    ssl_certificate     /etc/ssl/certs/billbird.crt;
    ssl_certificate_key /etc/ssl/private/billbird.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

#### Option B: Cloudflare Tunnel

If your Billbird host has no public IP (homelab, NAT, VPN-only network), Cloudflare Tunnel exposes it through Cloudflare's edge with no port forwarding. Free TLS, no exposed IP.

1. In **Cloudflare Zero Trust → Networks → Tunnels**, create a tunnel and copy the install token.
2. On the host, install and run `cloudflared` as a service (Cloudflare provides install commands per OS in the dashboard).
3. In the tunnel's **Public Hostnames** tab, add:
   - **Subdomain:** `billbird`
   - **Domain:** your Cloudflare-managed domain
   - **Service:** `http://localhost:8080`
4. Cloudflare auto-provisions a DNS record for `billbird.<your-domain>` pointing at the tunnel.

In Billbird's `.env`:

```
BASE_URL=https://billbird.your-domain.example
```

In the GitHub App settings:

- **Callback URL:** `https://billbird.your-domain.example/auth/callback`
- **Webhook URL:** `https://billbird.your-domain.example/webhook`

#### Development tunneling

For short-lived local testing without a public URL, [smee.io](https://smee.io) or `ngrok` work for the webhook path. They're not suitable for production because the URL changes per session.

## Kubernetes (Helm)

For production deployments on Kubernetes.

### Prerequisites

- A Kubernetes cluster
- Helm 3
- A PostgreSQL instance **dedicated to this organisation** (managed or self-hosted)
- A registered GitHub App **dedicated to this organisation**

> If you operate Billbird for multiple organisations, install the chart into a separate namespace per organisation with separate secrets and a separate database. Do not point two installs at the same database.

### Install

```bash
helm install billbird ./charts/billbird \
  --set database.url="postgres://user:pass@postgres:5432/billbird?sslmode=require" \
  --set github.appId="123456" \
  --set github.webhookSecret="your-secret" \
  --set github.clientId="Iv1.abc123" \
  --set github.clientSecret="your-secret" \
  --set admin.orgName="your-org"
```

Or use a values file:

```bash
helm install billbird ./charts/billbird -f values-production.yaml
```

### Helm values

| Value | Description | Default |
|-------|-------------|---------|
| `replicaCount` | Number of app replicas | `1` |
| `database.url` | PostgreSQL connection string | *(required)* |
| `database.existingSecret` | Name of existing K8s secret with DATABASE_URL | *(none)* |
| `github.appId` | GitHub App ID | *(required)* |
| `github.privateKey` | GitHub App private key (PEM) | *(required)* |
| `github.privateKeySecret` | Existing K8s secret name for the private key | *(none)* |
| `github.webhookSecret` | Webhook verification secret | *(required)* |
| `github.clientId` | OAuth client ID | *(none)* |
| `github.clientSecret` | OAuth client secret | *(none)* |
| `admin.orgName` | GitHub org for admin access | *(none)* |
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.host` | Ingress hostname | *(none)* |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |

### Migrations

Migrations run as an init container before the main application starts. This ensures the schema is up to date before any webhook processing begins.

### Multiple replicas

Billbird supports running multiple replicas behind a load balancer. Webhook idempotency (via `X-GitHub-Delivery` tracking) ensures no double-processing even when GitHub sends the same webhook to different replicas.

## Backup

Back up your PostgreSQL database regularly. The database contains all time entries, client records, and correction history. Since Billbird never physically deletes data, a database backup captures the complete audit trail.

```bash
# Docker Compose
docker compose exec db pg_dump -U billbird billbird > backup.sql

# Kubernetes
kubectl exec -it postgres-pod -- pg_dump -U billbird billbird > backup.sql
```
