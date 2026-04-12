# Changelog

## 2026-04-12 — REST API, OAuth, admin panel, org-gated auth, CLI wrapper

### Added
- **REST API** (`internal/api`): Full JSON API under `/api/v1/` — time entries (list with filters, get single, correction chain), clients CRUD, label mappings CRUD, CSV export
- **GitHub OAuth** (`internal/auth`): OAuth flow, org membership check against `ALLOWED_ORGS`, signed session cookies, `RequireAuth` middleware
- **Admin panel** (`internal/admin`): HTMX dashboard with hours overview (stats + filterable entries table), client management (add/activate/deactivate), label mapping management (add/remove), all consuming REST API via internal calls
- **Org-gated authorization**: Slash commands (`/log`, `/correct`, `/delete`) now require the commenter to be a member of one of the `ALLOWED_ORGS`. Non-members get an error comment. Replaces old `ADMIN_ORG_NAME` single-org config.
- **CLI wrapper** (`bin/billbird`): Shell script for terminal-based time logging via `gh`. Auto-detects repo from git remote. Tested against MWest2020/Billbird#1.
- **Templates** (`templates/`): layout, dashboard, entries table, clients, clients table, mappings, mappings table — all with HTMX partial updates

### Changed
- `ADMIN_ORG_NAME` replaced by `ALLOWED_ORGS` (comma-separated, required). Used for both slash command authorization and admin panel access.
- `main.go` fully wired: webhook, API, auth, and admin routes all registered with appropriate middleware

## 2026-04-11 — Project scaffold, database migrations, webhook ingestion, /log handler

### Added
- **Project structure**: Go module `github.com/mwesterweel/billbird` with `cmd/billbird`, `internal/` packages (webhook, commands, timeentry, client, github, api, admin, auth, config, db, cycletime)
- **Configuration** (`internal/config`): Environment-variable-based config with fail-fast validation for required vars (DATABASE_URL, GITHUB_APP_ID, GITHUB_PRIVATE_KEY, GITHUB_WEBHOOK_SECRET)
- **Database migrations** (6 migrations): clients, label_mappings, time_entries, cycle_time_records, webhook_deliveries, sessions — using `golang-migrate`
- **Database connection** (`internal/db`): pgxpool connection setup + auto-migration on startup
- **Webhook ingestion** (`internal/webhook`): HMAC-SHA256 signature verification, event routing by X-GitHub-Event header, idempotent delivery tracking via X-GitHub-Delivery
- **Slash command parser** (`internal/commands`): Parses `/log`, `/correct`, `/delete` from issue comments with duration support (h, m, combined) and optional descriptions — 16 unit tests, all passing
- **GitHub API client** (`internal/github`): GitHub App JWT authentication, installation token caching, post-comment, get-issue-labels
- **Time entry store** (`internal/timeentry`): Create, FindLatestActive, Supersede, SoftDelete — non-destructive correction chain
- **Client attribution** (`internal/client`): Label-to-client resolver with repo-specific precedence over global mappings
- **Command handlers**: Full `/log`, `/correct`, `/delete` flow with confirmation comments posted back to GitHub issues
- **Health check**: `GET /healthz` endpoint with database connectivity check
- **Docker**: Multi-stage Dockerfile (Go build → Alpine runtime, non-root user), docker-compose.yml with Postgres
- **OpenSpec**: Initialized with `billbird-v1` change — proposal, design, specs (7 capabilities), tasks (13 groups)

- **Documentation** (`docs/`): Setup guide, commands reference, client attribution, corrections, architecture, configuration, self-hosting, contributing
- **README**: Project overview with simplest use case, quick start, and doc links

### Architecture decisions recorded
- API-first: REST API is the primary interface, HTMX admin panel is a thin consumer (POC), Nextcloud app is the next consumer (MVP)
- UTC-only timestamps, optional timezone on user profile for display
- GitHub Projects V2: supplementary GraphQL query for column name resolution
- Admin access v1: org membership sufficient, no role-based access
