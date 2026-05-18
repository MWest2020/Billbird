## Context

Billbird is a greenfield application. There is no existing codebase. The target audience is small-to-medium software teams already using GitHub for project management who need low-friction time tracking. The system must be simple to self-host, operationally minimal, and avoid scope creep into invoicing or project management.

## Goals / Non-Goals

**Goals:**
- API-first architecture: billbird-core exposes a REST API that any UI can consume
- Zero-config developer experience: developers interact only through GitHub comments
- Full auditability: every time entry traces back to a GitHub comment, every change is preserved
- Simple self-hosting: Postgres + GitHub App + one container for the core
- Multiple UI consumers: HTMX admin panel (POC), Nextcloud app (MVP)

**Non-Goals:**
- Invoicing or payment processing
- Timer functionality (start/stop real-time tracking)
- Integration with tools outside GitHub (Jira, Linear, Slack)
- Multi-tenancy or SaaS hosting
- Mobile app or desktop client
- PostgREST or auto-generated API — API shape must be controlled by billbird-core

## Decisions

### Language and framework: Go with standard library HTTP server

**Choice:** Go with `net/http`, no web framework.

**Rationale:** Go produces a single static binary, has excellent Postgres drivers (`pgx`), first-class HTTP support, and is easy to containerize. The application has straightforward request handling (webhooks + HTMX pages) that doesn't benefit from a framework. Keeping dependencies minimal reduces supply chain risk and simplifies auditing.

**Alternatives considered:**
- Node.js/Express: More ecosystem for GitHub integrations, but heavier runtime, more complex containerization
- Python/FastAPI: Good ergonomics but slower for webhook processing under load, GIL concerns
- Rust: Performance overkill for this use case, slower development velocity

### Database access: Raw SQL with pgx

**Choice:** Use `pgx` directly with hand-written SQL queries. No ORM.

**Rationale:** The data model is straightforward (6-7 tables). An ORM adds complexity without meaningful benefit. Raw SQL is auditable, explicit, and avoids the N+1 problem by design. Migrations use `golang-migrate`.

**Alternatives considered:**
- sqlc (SQL-to-Go codegen): Good option, may adopt later if query count grows. For v1 the overhead of the tool isn't justified.
- GORM: Hides query behavior, makes the correction chain logic harder to reason about

### Architecture: API-first with thin UI layer

**Choice:** billbird-core exposes a REST API as the primary interface. The HTMX admin panel (billbird-admin) is the POC's default UI consumer, built as a thin layer on top of the API. A Nextcloud app will be the second consumer post-POC, replacing billbird-admin as the primary UI.

**Rationale:** The API is the product. Multiple consumers need the same data (admin panel now, Nextcloud app next). Building the HTMX panel directly against the database would create a rewrite when the Nextcloud app arrives. API-first ensures every operation is available to any client from day one.

**Constraints:**
- PostgREST is explicitly out — API shape must be hand-controlled
- billbird-core owns all business logic; UI layers are purely presentational
- The HTMX admin panel lives in the same binary for POC simplicity but only calls the REST API routes

**Alternatives considered:**
- Server-side-only HTMX (no API layer): Simpler for POC but creates a rewrite for Nextcloud integration
- Separate API service + separate admin service: Over-engineered for POC; the API routes and admin routes coexist in one binary
- PostgREST: Insufficient control over API shape, business logic placement

### Admin panel (POC): Server-side rendered HTML with HTMX

**Choice:** Go `html/template` for rendering, HTMX for interactivity. Consumes billbird-core's REST API internally.

**Rationale:** The admin panel has simple CRUD operations and tabular data. SSR with HTMX eliminates a build step and a JavaScript framework. The panel calls the same API endpoints that the future Nextcloud app will use, validating the API design early.

**Alternatives considered:**
- React/Next.js SPA: Overkill for the POC feature set
- Templ (Go templating library): Adds a code generation step; `html/template` is sufficient

### Webhook processing: Synchronous in-process

**Choice:** Process webhooks synchronously in the HTTP handler. No background job queue.

**Rationale:** Each webhook does minimal work: parse command, one-two DB writes, one GitHub API call (confirmation comment). This completes in under 500ms. A job queue (Redis, etc.) adds operational complexity for no benefit at the expected scale. If processing latency becomes an issue, the handler can be refactored to enqueue work later without changing the external interface.

**Alternatives considered:**
- Background job queue (Redis + worker): Adds infrastructure dependency, unnecessary at expected scale
- Async goroutine pool: Risk of lost work on crash; DB write + GitHub API call should be atomic from the user's perspective

### Authorization: Org-membership gate for slash commands

**Choice:** Only members of configured GitHub organizations can use `/log`, `/correct`, `/delete`. Configured via `ALLOWED_ORGS` (comma-separated, supports multiple orgs). Membership is checked against the GitHub API on every command. No user registration in Billbird.

**Rationale:** Prevents arbitrary GitHub users from logging time. Piggybacks on GitHub's existing access control --- no separate user management to maintain. Multiple orgs supported because teams may span orgs (e.g., a consulting firm with client orgs).

**Alternatives considered:**
- Explicit user registration in Billbird: Double maintenance alongside GitHub org management
- Repo collaborator check: Too granular --- a user might have access to one repo but not another, yet still need to log time across the org
- GitHub Teams: More granular but adds config complexity; can be added later if needed

### Authentication: GitHub OAuth with session cookies (POC) / API tokens (future)

**Choice:** Standard GitHub OAuth 2.0 flow for the HTMX admin panel. Session stored in a signed cookie with the GitHub user ID and org membership. The REST API authenticates via the same session cookie for the POC; API token auth will be added for the Nextcloud app consumer.

**Rationale:** The admin panel already requires GitHub identity. OAuth is the natural fit. Server-side sessions in Postgres avoid the need for Redis. Signed cookies prevent tampering. API token support is deferred until the Nextcloud consumer needs it.

**Admin role granularity:** Org membership is sufficient for v1. No viewer/editor role distinction.

**Alternatives considered:**
- GitHub App installation token: Doesn't provide user identity for the admin panel
- JWT tokens: Adds complexity for token refresh; cookies are simpler for a server-rendered app

### Timezone handling: UTC always

**Choice:** Store all timestamps in UTC. Optional timezone field on user profile for display purposes only. Never store local time without offset.

### Project structure: Flat with clear package boundaries

```
billbird/
  cmd/billbird/         # main entrypoint
  internal/
    webhook/            # webhook handler, signature verification, event routing
    commands/           # slash command parsing (/log, /correct, /delete)
    timeentry/          # time entry domain logic, correction chains
    client/             # client and label mapping logic
    cycletime/          # cycle time tracking
    github/             # GitHub API client (posting comments, reading labels)
    api/                # REST API handlers (JSON endpoints)
    admin/              # HTMX admin panel (thin layer consuming API)
    auth/               # OAuth flow, session management
    db/                 # database queries, migrations
  templates/            # html/template files for admin panel
  migrations/           # SQL migration files
  charts/billbird/      # Helm chart
  docker-compose.yml
  Dockerfile
```

## Risks / Trade-offs

**[Synchronous webhook processing may not scale]** -> At very high volume (hundreds of webhooks/second), synchronous processing could bottleneck. Mitigation: the architecture allows adding a queue later without changing the webhook endpoint contract. Expected v1 scale (< 10 webhooks/minute) is well within limits.

**[GitHub API rate limits]** -> Posting confirmation comments consumes GitHub API quota (5000/hour for GitHub Apps). Mitigation: at expected scale this is not a concern. If needed, confirmation comments can be batched or made optional.

**[Projects V2 API requires supplementary GraphQL]** -> GitHub Projects V2 webhooks deliver field IDs, not column names. A supplementary GraphQL query is required in the webhook handler to resolve column names from field IDs. This is known behaviour. Mitigation: isolate GraphQL usage in the cycletime package.

**[Single container = single point of failure]** -> No redundancy in the default deployment. Mitigation: Helm chart supports replica count > 1. Webhook idempotency ensures no double-processing with multiple replicas behind a load balancer.

**[Label-based client attribution is simple but rigid]** -> Some teams may want client attribution at the repository level or via other mechanisms. Mitigation: label mapping is the right starting point; the mapping table can be extended later without schema changes to time entries.

## Migration Plan

Not applicable — greenfield deployment. The application creates its own schema on first start via migration files.

**Rollback strategy:** Since this is a new deployment, rollback means stopping the container and optionally dropping the database. No existing system is being replaced.

## Resolved Questions

1. **GitHub Projects V2 webhook support**: Supplementary GraphQL query required to resolve column names from field IDs. Known behaviour, handled in the webhook handler.
2. **Admin role granularity**: Org membership is sufficient for v1. No viewer/editor roles.
3. **Time zone handling**: UTC always. Optional timezone field on user profile for display only. Never store local time without offset.
