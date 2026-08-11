## Why

Billbird v1 records actual hours through `/log`, but teams have no way to register forecast hours alongside actuals. Without a plan figure the system cannot answer the question every manager asks: "how is this issue tracking against estimate?" Adding `/plan` makes Billbird a plan-vs-actual tool without leaving the GitHub workflow.

Separately, a manager-facing AI front-end is on the horizon (Manager-MCP, built in Gitsweeper). That consumer must reach Billbird's API without holding a user's session cookie. Long-lived API tokens are the missing piece. Bundling them with `/plan` is deliberate: a Manager-MCP that cannot also read plan-vs-actual data isn't useful.

## What Changes

- New `/plan <duration> [description]` command on issue comments. Creates or supersedes a plan entry for that issue.
- New `/unplan` command. Soft-deletes the active plan entry on the issue. Same chain semantics as `/delete` for log entries.
- New Postgres table `plan_entries`. Same shape as `time_entries` for status/correction-chain compatibility, but **no `client_id` column** — plans are forecasts, not billable artefacts. One active plan per (repository, issue_number).
- New REST endpoints under `/api/v1/`:
  - `GET /plans` — list plan entries with filters (repo, issue, status, period)
  - `GET /plans/{id}` — single plan with correction chain
  - `GET /issues/{owner}/{repo}/{number}/plan-vs-actual` — aggregated planned minutes, logged minutes, variance, status (under/on/over)
- New API token capability: long-lived tokens scoped to a user, usable as `Authorization: Bearer <token>` on `/api/v1/*` routes. Managed in the admin panel. Tokens are hashed at rest (bcrypt or argon2id), shown once at creation.
- Admin panel: "Plan vs Logged" column on the dashboard; token management page under user settings.
- Confirmation comments: `Planned 8h on this issue by @user (plan #12)`; corrections mirror `/correct`'s format.

## Capabilities

### New Capabilities
- `time-planning`: `/plan` and `/unplan` command parsing, plan-entry creation, plan supersede / soft-delete chain, plan-vs-actual aggregation.
- `api-tokens`: Long-lived bearer tokens for API access, hashed at rest, scoped to a user, revocable via the admin panel.

### Modified Capabilities
- `data-model`: Add `plan_entries` table and `api_tokens` table; document that `time_entries.client_id` semantics stay log-only.
- `admin-panel`: Add plan-vs-actual dashboard view and token management page.

## Impact

- **Code (Go)**: new packages `internal/planentry`, `internal/apitoken`; extensions to `internal/commands` (parser), `internal/api` (handlers + bearer auth middleware), `internal/admin` (templates + routes), `internal/db` (two migrations).
- **Schema**: two new migrations (`plan_entries`, `api_tokens`). No backfill.
- **Auth**: REST API gains a second auth path (bearer token) alongside the existing session cookie. Both paths produce the same `User` context.
- **Docs**: `docs/commands.md` covers `/plan` and `/unplan`. New `docs/api-tokens.md`. `docs/architecture.md` updates the data-flow diagram.
- **Operational**: zero new external dependencies. Token hashing uses `golang.org/x/crypto/bcrypt` (already in dependency-graph-eligible stdlib-adjacent set).
- **Backwards compatibility**: additive only. Existing webhooks, commands, and API routes unchanged.
