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

## Platform support

Billbird is a **GitHub App** today. Porting to GitLab or Forgejo is on the table but is real work, not a config switch. This section is honest about what is portable, what is not, and what a port would cost — so that anyone asking can pre-screen themselves before opening an issue.

### What is platform-portable

The following components do not know about any specific git host and would survive any platform port without changes:

- The Postgres schema and migrations
- The command parser (`/log`, `/correct`, `/delete`, `/plan`, `/unplan`) — pure string parsing
- The admin UI and templates
- The REST API (`/api/v1/*`)
- The bearer-token system
- The idempotency layer (works on any `delivery_id` string)
- The label snapshot and client-attribution logic
- The plan-vs-actual computation

### What is GitHub-specific

These integration points are tied to GitHub's API shape and would each need replacing per target platform:

| Concern | GitHub | GitLab | Forgejo |
|---|---|---|---|
| Authentication | GitHub App + per-install access tokens (1h TTL, JWT-issued) | GitLab App or PAT — no per-install model with the same shape | PAT or service-account token |
| Webhook signature | HMAC-SHA256 in `X-Hub-Signature-256` | Plaintext shared secret in `X-Gitlab-Token` | HMAC-SHA256 (GitHub-compatible) |
| Event name | `issue_comment`, `pull_request_review_comment` | `Note Hook` with `noteable_type=Issue` / `MergeRequest` | `issue_comment` (GitHub-compatible) |
| Payload fields | `comment.user.login`, `issue.number`, `repository.full_name` | `user.username`, `issue.iid`, `project.path_with_namespace` | Similar to GitHub, not identical |
| Membership check | `GET /orgs/{org}/members/{user}` | `GET /groups/{group}/members/{user_id}` | `GET /orgs/{org}/members/{username}` |
| Bot identity | The App's bot account (`*[bot]`) | Plain service account | Plain service account |
| One-click registration | App Manifest flow (`billbird init`) | Not available | Not available |

The split is roughly: **half the code base is portable, half is tied to GitHub.**

### Path forward, in priority order

1. **GitHub-only (current).** Billbird stays focused. Operators on other platforms either fork or wait. **This is the only state v1 commits to.**
2. **Forgejo port** (~2–3 focused days). Forgejo aims for GitHub API compatibility; webhook payloads are close, signature scheme matches, the membership endpoint is the same shape. The biggest piece of work is replacing GitHub App auth (which Forgejo lacks) with a service-account PAT model. No App Manifest equivalent, so `billbird init` is GitHub-only.
3. **GitLab port** (~4–5 focused days). Larger because the event model differs (`Note Hook`), the signature scheme is plaintext rather than HMAC, and the auth model has no installation token concept. Most of the cost is the auth shim, not the webhook routing.

If we go past option 1, the right shape is a `Provider` interface inside `internal/webhook` and `internal/<provider>` packages — not three forks. The interface needs four methods: `VerifySignature(body, header) bool`, `ParseCommentEvent(body) (*CommentEvent, error)`, `PostComment(ctx, repo, num, body) error`, `IsMember(ctx, group, user) bool`. The handler dispatches on the inbound webhook's source URL or header.

### Concrete demand threshold

Multi-platform refactoring stays on the shadow roadmap until **at least three distinct community members open issues asking for the same target platform**, *and* at least one of them volunteers to maintain the provider. Until then, the code stays single-platform and the architecture stays simple. This rule is here so the project does not end up with three half-maintained platform integrations.

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
