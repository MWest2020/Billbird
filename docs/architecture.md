# Architecture

## Overview

Billbird follows an **API-first** design. The REST API is the primary interface. All business logic lives in `billbird-core`. UI layers are thin consumers.

```
                         GitHub
                           |
                      webhooks (POST /webhook)
                           |
                    +------+------+
                    | billbird-core |
                    |   (Go)       |
                    |              |
                    | - Webhook handler
                    | - Command parser
                    | - Time entry logic
                    | - Client attribution
                    | - REST API
                    +------+------+
                      |         |
               Postgres     GitHub API
                            (comments, labels)
```

### Consumers

| Consumer | Status | Description |
|----------|--------|-------------|
| **billbird-admin** | POC | HTMX admin panel, consumes REST API |
| **Nextcloud app** | Planned (MVP) | Will replace billbird-admin as primary UI |

The HTMX admin panel calls the same API endpoints that future consumers will use. This validates the API design early.

## Components

### Webhook handler (`internal/webhook`)

Receives GitHub webhook payloads, verifies HMAC-SHA256 signatures, tracks delivery IDs for idempotency, and routes events to handlers.

### Command parser (`internal/commands`)

Extracts `/log`, `/correct`, `/delete` from issue comment bodies. Parses durations. Pure function with no side effects.

### Time entry store (`internal/timeentry`)

Manages the time entries table. Handles the correction chain (create, supersede, soft-delete). All writes go through this store.

### Plan entry store (`internal/planentry`)

Manages the `plan_entries` table for `/plan` and `/unplan`. Same correction-chain pattern as `timeentry`, but plans carry no `client_id` — they are forecasts, not billable. A partial unique index enforces at most one `active` plan per `(repository, issue_number)`. Also exposes `ComputePlanVsActual` which is the source of truth for the dashboard's variance column and the `plan-vs-actual` API.

### API token store (`internal/apitoken`)

Issues, verifies, and revokes bearer tokens used on `/api/v1/*` by non-browser consumers. Tokens are hashed at rest with bcrypt (cost 12); the plaintext is shown to the user exactly once at creation.

### Client resolver (`internal/client`)

Matches issue labels to client records via label mappings. Handles repository-specific vs. global precedence.

### GitHub client (`internal/github`)

Authenticates as a GitHub App (JWT + installation tokens). Posts confirmation comments. Fetches issue labels. Caches installation tokens.

### REST API (`internal/api`)

JSON endpoints for all operations. Consumed by the admin panel and future UIs.

### Admin panel (`internal/admin`)

HTMX-based server-side rendered UI. Calls the REST API internally. GitHub OAuth for authentication. New pages: **Plans** (active plans + plan-vs-actual badges per issue) and **API tokens** (create/list/revoke; plaintext shown once at creation).

## Data flow: /log command

```
1. GitHub sends issue_comment webhook
2. Handler verifies HMAC-SHA256 signature
3. Handler checks X-GitHub-Delivery for idempotency
4. Parser extracts /log 2h from comment body
5. Handler checks user is a member of an allowed org
6. GitHub client fetches issue labels
7. Client resolver maps labels to client
8. Time entry store inserts row (status=active)
9. GitHub client posts confirmation comment
10. Handler marks delivery as processed
```

## Design constraints

- **No external dependencies beyond Postgres**: No Redis, no message queues, no external caches
- **Synchronous webhook processing**: Each webhook completes in <500ms, no job queue needed
- **No physical deletes**: All state changes through status fields and correction chains
- **UTC timestamps everywhere**: Optional timezone on user profile for display only
- **API shape controlled by billbird-core**: PostgREST explicitly out
- **Single-tenant per instance**: one Billbird deployment serves exactly one organisation (see Deployment topology)

## Deployment topology

Billbird is deployed **one instance per organisation**. Each instance owns its own Postgres database, its own GitHub App registration, and its own secret store. There is no `organization_id` column on any table — tenant isolation is provided by the deployment boundary, not by the schema.

This shape has three consequences that ripple through the rest of the design:

- **API tokens are user-scoped, not org-scoped.** Each instance already belongs to one org, so a token only needs to identify a user.
- **Backups are per-instance.** No data needs to be sliced out of a shared database when an organisation rotates retention policy.
- **Future tables follow the same rule.** A new domain table introduced by a future change does not carry a tenant-discriminator column. Multi-tenant SaaS hosting is explicitly out of scope for v2; any proposal that would change this must explicitly revise the [`billbird-org-scoping` change](../openspec/changes/archive/) and provide a migration plan for existing single-tenant deployments.

`ALLOWED_ORGS` may still contain multiple comma-separated organisations, for the consulting-team case where the same operator works under more than one GitHub org but shares one Billbird instance by design. The single-tenant rule still holds: a Billbird instance belongs to one operator, even when that operator's work spans multiple GitHub orgs.

## Project structure

```
billbird/
  cmd/billbird/           main entrypoint
  internal/
    api/                  REST API handlers (JSON)
    admin/                HTMX admin panel (consumes API)
    apitoken/             Bearer-token issue/verify/revoke
    auth/                 OAuth flow, session management, bearer middleware
    client/               Client and label mapping logic
    commands/             Slash command parsing
    config/               Environment variable loading
    cycletime/            Cycle time tracking
    db/                   Database connection, migrations
    github/               GitHub API client
    planentry/            Plan entry domain logic (/plan, /unplan)
    timeentry/            Time entry domain logic
    webhook/              Webhook handler, signature verification
  migrations/             SQL migration files
  templates/              HTML templates for admin panel
  docs/                   Documentation
  charts/billbird/        Helm chart
```
