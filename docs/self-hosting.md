# Self-hosting

Billbird is designed to be self-hosted. You own your data completely. The only external dependencies are PostgreSQL and a GitHub App registration.

## Docker Compose

The simplest way to run Billbird.

### Prerequisites

- Docker and Docker Compose
- A registered GitHub App (see [setup.md](setup.md))

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

### Reverse proxy

For production use, put Billbird behind a reverse proxy with TLS. Example nginx configuration:

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

## Kubernetes (Helm)

For production deployments on Kubernetes.

### Prerequisites

- A Kubernetes cluster
- Helm 3
- A PostgreSQL instance (managed or self-hosted)
- A registered GitHub App

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
