## 1. Project Setup

- [ ] 1.1 Initialize Go module (`go mod init`), set up project directory structure per design doc (including `internal/api/` for REST endpoints)
- [ ] 1.2 Set up Dockerfile (multi-stage build: build Go binary, copy into scratch/alpine image)
- [ ] 1.3 Set up docker-compose.yml with app and Postgres services, hot-reload via `air` or similar
- [ ] 1.4 Add `golang-migrate` dependency and create migrations directory structure
- [ ] 1.5 Write main entrypoint (`cmd/billbird/main.go`) with config loading from environment variables, fail-fast on missing required vars

## 2. Database Schema and Migrations

- [ ] 2.1 Create migration: `clients` table (id, name, active, created_at, updated_at)
- [ ] 2.2 Create migration: `label_mappings` table (id, label_pattern, client_id FK, repository nullable, created_at, updated_at)
- [ ] 2.3 Create migration: `time_entries` table (id, github_user_id, github_username, repository, issue_number, duration_minutes, description nullable, client_id nullable FK, source_comment_id, source_comment_url, status enum, superseded_by nullable FK, created_by_type enum, created_at)
- [ ] 2.4 Create migration: `cycle_time_records` table (id, repository, issue_number, start_at nullable, end_at nullable, start_source, end_source, created_at, updated_at)
- [ ] 2.5 Create migration: `webhook_deliveries` table (id, delivery_id unique, event_type, processed_at)
- [ ] 2.6 Create migration: `sessions` table for admin panel auth (id, github_user_id, github_username, org_member bool, created_at, expires_at)
- [ ] 2.7 Write database connection setup and migration runner in `internal/db`

## 3. Webhook Ingestion

- [ ] 3.1 Implement webhook signature verification (HMAC-SHA256 using `X-Hub-Signature-256`)
- [ ] 3.2 Implement webhook HTTP handler: verify signature, parse `X-GitHub-Event` header, route to handlers
- [ ] 3.3 Implement idempotent delivery tracking: check/store `X-GitHub-Delivery` IDs
- [ ] 3.4 Wire up webhook endpoint in main router

## 4. Slash Command Parsing

- [ ] 4.1 Implement command parser: extract `/log`, `/correct`, `/delete` from issue comment body with duration parsing (hours, minutes, combined)
- [ ] 4.2 Write unit tests for command parser covering valid formats, invalid formats, descriptions, and edge cases

## 5. GitHub API Client

- [ ] 5.1 Implement GitHub App authentication (JWT generation from private key, installation token exchange)
- [ ] 5.2 Implement post-comment function (create issue comment via REST API)
- [ ] 5.3 Implement get-issue-labels function (list labels on an issue via REST API)
- [ ] 5.4 Implement helper to parse closing issue references from PR body

## 6. Time Entry Domain Logic

- [ ] 6.1 Implement `/log` handler: parse command, resolve client from labels, insert time entry, post confirmation comment
- [ ] 6.2 Implement `/correct` handler: find most recent active entry for user+issue, create correction entry, mark previous as superseded, post confirmation
- [ ] 6.3 Implement `/delete` handler: find most recent active entry for user+issue, mark as deleted, post confirmation
- [ ] 6.4 Implement error handling: no prior entry for correct/delete, invalid duration, post error comments to issue
- [ ] 6.5 Write integration tests for the correction chain logic (log -> correct -> correct -> delete)

## 7. Client Attribution

- [ ] 7.1 Implement label-to-client resolution: given an issue's labels and repository, find the matching client via label_mappings
- [ ] 7.2 Handle precedence: repository-specific mapping over global mapping
- [ ] 7.3 Handle ambiguity: multiple client labels on one issue (use first match, log warning)

## 8. Cycle Time Tracking

- [ ] 8.1 Implement project card/item event handler: detect column transitions, supplementary GraphQL query to resolve column names from field IDs
- [ ] 8.2 Implement PR merge event handler: parse closing references, record end timestamp for referenced issues
- [ ] 8.3 Implement cycle time record creation/update in database
- [ ] 8.4 Handle edge cases: no existing start time, duplicate start events

## 9. REST API

- [ ] 9.1 Define REST API route structure under `/api/v1/` (time entries, clients, label mappings, cycle time, export)
- [ ] 9.2 Implement time entries API: list (with filters: date range, repo, client, developer), get single entry with correction chain
- [ ] 9.3 Implement clients API: list, create, update, deactivate
- [ ] 9.4 Implement label mappings API: list, create, update, delete
- [ ] 9.5 Implement manual adjustment API: create admin correction with reason
- [ ] 9.6 Implement CSV export API: respect query filters, return CSV
- [ ] 9.7 Implement cycle time API: per-issue and aggregated per developer/repository
- [ ] 9.8 Implement auth middleware for API routes (session cookie for POC, API token placeholder for future)

## 10. Admin Panel Authentication

- [ ] 10.1 Implement GitHub OAuth flow: redirect to GitHub, handle callback, exchange code for token
- [ ] 10.2 Implement org membership check using the OAuth token
- [ ] 10.3 Implement session creation, signed session cookie, and session validation middleware
- [ ] 10.4 Implement logout (clear session)

## 11. Admin Panel Views (HTMX, consuming REST API)

- [ ] 11.1 Create base HTML layout template with navigation (dashboard, clients, export)
- [ ] 11.2 Implement hours overview dashboard: calls REST API for time entries, renders with filters
- [ ] 11.3 Implement HTMX partial updates for filter changes on the dashboard
- [ ] 11.4 Implement correction history view: calls REST API for entry chain, renders with GitHub comment links
- [ ] 11.5 Implement manual entry adjustment form: posts to REST API adjustment endpoint
- [ ] 11.6 Implement client management page: calls REST API for CRUD operations
- [ ] 11.7 Implement label mapping management page: calls REST API for CRUD operations
- [ ] 11.8 Implement CSV export page: triggers REST API export endpoint download
- [ ] 11.9 Implement cycle time display: calls REST API for cycle time data

## 12. Deployment

- [ ] 12.1 Finalize Dockerfile with production configuration (non-root user, health check endpoint)
- [ ] 12.2 Create Helm chart: deployment, service, ingress, configmap, secret references, migration init container
- [ ] 12.3 Add health check endpoint (`/healthz`) that verifies database connectivity
- [ ] 12.4 Document required environment variables and GitHub App setup in README

## 13. End-to-End Testing

- [ ] 13.1 Write end-to-end test: webhook delivery -> `/log` command -> time entry created -> confirmation comment posted
- [ ] 13.2 Write end-to-end test: correction chain (`/log` -> `/correct` -> `/delete`) with database state verification
- [ ] 13.3 Write end-to-end test: client attribution via label mapping
- [ ] 13.4 Write end-to-end test: REST API endpoints (time entries, clients, export)
- [ ] 13.5 Write end-to-end test: admin panel login, view dashboard via API, export CSV
