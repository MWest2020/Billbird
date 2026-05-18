## Context

Billbird v1 is in production-shape: webhooks, slash commands, correction chains, client attribution, REST API and HTMX admin panel are all wired up against Postgres. The v1 design.md called out "Multi-tenancy or SaaS hosting" as a non-goal; per a follow-up decision on 2026-05-18, Billbird stays single-tenant per instance (one self-hosted deployment per organisation, `ALLOWED_ORGS` set to that single org). That decision is captured in a sibling change (`billbird-org-scoping`) and informs the API-token design here: tokens are user-scoped, not org-scoped, because every Billbird instance already belongs to one org.

This change adds two capabilities that are intentionally bundled. `/plan` gives teams a forecast value to compare against logged hours. API tokens give Gitsweeper's Manager-MCP (and any future non-browser client) a way to read that comparison without a browser session. Shipping `/plan` without the token capability would leave the manager-MCP roadmap stranded; shipping the token capability without `/plan` would expose only the existing log data, which is half the story.

## Goals / Non-Goals

**Goals:**
- Forecast hours per issue using the same audit-trail pattern as `/log` — every plan change traces back to a GitHub comment.
- Surface plan-vs-actual variance through the existing REST API contract.
- Provide bearer-token authentication on `/api/v1/*` that an external Python MCP server can use safely.
- Stay additive: no behaviour change to `/log`, `/correct`, `/delete`, or existing endpoints.

**Non-Goals:**
- Multi-plan-per-issue (sprint plans, milestone plans, per-developer plans). v1 = one active plan per issue.
- Plans on artefacts other than issues (PRs, milestones, project items).
- Token scopes / fine-grained permissions. v1 token = "act as the issuing user".
- Auto-derived plans from story-point labels or Project board estimates. Plans are explicit `/plan` commands only.
- Webhook outputs (Slack, email) when a plan is exceeded. Reporting only.

## Decisions

### Separate `plan_entries` table, not a `kind` column on `time_entries`

**Choice:** New table `plan_entries` with the same shape as `time_entries` minus `client_id`. Both tables carry the same status enum (`active`, `superseded`, `deleted`) and the same `superseded_by` self-reference.

**Rationale:** Plans and logs have different semantics (forecast vs actual), different consumers (planning views vs invoicing reports), and likely different RBAC trajectories (a future "only leads can plan" role would not touch log entries). Mixing them in one table forces a `kind` filter on every query and complicates the `client_id` foreign key (plans should not be billable). A separate table keeps each query path single-purpose and matches the boring/auditable preference recorded for this project.

**Alternatives considered:**
- Single `time_entries` table with `kind enum ('log','plan')`: one schema, but every read needs `WHERE kind = ...`; client-attribution code must learn to skip plans; correction-chain queries become more conditional. Saves a table at the cost of conditional logic in many places.
- Plan stored as a column on an issue-level record: rejected — issue-level state has no audit trail today and would need its own correction chain.

### One active plan per (repository, issue_number)

**Choice:** Enforce uniqueness with a partial unique index: `CREATE UNIQUE INDEX uniq_active_plan ON plan_entries (repository, issue_number) WHERE status = 'active';`. A second `/plan` on the same issue supersedes the first; nothing concurrent stays active.

**Rationale:** Mirrors the developer's mental model — "the plan for this issue right now is X". Aggregation queries don't need a "latest plan per issue" subquery. The partial index makes the constraint a database-level guarantee instead of an application invariant.

**Alternatives considered:**
- Allow multiple active plans, take the most recent at read time: simpler write path, but every reader must implement the same "latest-active" rule, and disagreements are easy to introduce.

### `/plan` is open to all `ALLOWED_ORGS` members in v1

**Choice:** Same authorisation gate as `/log`. Any org member can plan, correct their plan, or unplan. Admin override (`created_by = 'admin'`) is available through the panel exactly like log entries.

**Rationale:** Adding a "planners only" role to v1 requires either GitHub Teams integration or a Billbird-side role table, both of which are bigger than this change. Teams that need plan-write restriction can rely on social convention until a role capability lands. Audit trail makes misuse traceable.

**Open question:** Whether to add a `roles` capability in the next change. Tracked, not blocking.

### Bearer tokens stored as `bcrypt` hash, shown once

**Choice:** Token format `bb_<base64-32-random-bytes>`. At creation, plaintext returned in the response and never persisted. Server stores `bcrypt(plaintext)` plus a non-secret prefix (first 8 chars of the base64 portion) for UI display. Tokens have no expiry by default; admin panel supports manual revocation.

**Rationale:** Standard pattern for personal-access-token style auth (matches GitHub's own PAT model in spirit). bcrypt is in `golang.org/x/crypto`, already an idiomatic Go dependency. Showing the token once forces users to capture it at creation, eliminating "where do I find my token again" UX traps and limiting the blast radius of an admin-panel breach.

**Alternatives considered:**
- HMAC-signed tokens (no DB lookup): faster, but revocation requires either a deny-list or short-lived tokens + refresh — too much machinery for v1.
- Argon2id instead of bcrypt: stronger but adds tuning surface; bcrypt at cost 12 is plenty for this rate.

### Bearer middleware reuses the session-cookie user context

**Choice:** `internal/auth` exposes one middleware that accepts either a valid session cookie or a valid `Authorization: Bearer bb_...` header. Both produce the same `User` value downstream. Token's owning user must still be a member of `ALLOWED_ORGS` at request time (cached for 5 minutes to avoid hammering the GitHub API).

**Rationale:** Handlers don't need to care which auth path was used. Revoking org membership in GitHub immediately disables the token within the cache TTL. The 5-minute cache matches the existing OAuth session staleness budget.

**Alternatives considered:**
- Cache org membership for token lifetime: faster but a removed user could keep access for days.
- Re-check on every request: correct but doubles the GitHub API budget per call.

### Confirmation-comment surface area

**Choice:** New confirmation strings, but reuse `/correct`'s superseded-chain phrasing for plan corrections:
- Plan: `Planned 8h on this issue by @user (plan #12)`
- Plan correction: `Updated @user's plan from 8h to 12h (plan #13 supersedes #12)`
- Unplan: `Removed @user's plan of 8h (plan #12)`

**Rationale:** Identical conventions across `/log` and `/plan` make the comment thread easy to scan. The word "plan" instead of "entry" keeps the two concepts visually distinct.

## Risks / Trade-offs

**[`/plan` collides with downstream tools using `/plan`]** → Mitigation: command parser already only matches at start-of-line and against a fixed verb list. Conflicts with other GitHub apps would surface as duplicate comments on the same trigger, which Billbird cannot prevent and which is acceptable.

**[Plan-vs-actual is a single number per issue, hiding distribution over time]** → Mitigation: variance is a derived view; richer reporting (e.g., burn-down) can be added in Gitsweeper's MCP layer without schema changes.

**[Bearer tokens widen the auth surface]** → Mitigation: tokens are user-scoped (no "service account" tier), org-membership is re-checked through GitHub, all `/api/v1/*` writes already require explicit handlers (no PostgREST-style auto-CRUD). The blast radius of a leaked token equals "that user logged in".

**[Cached org membership for 5 minutes means revocation lags]** → Mitigation: documented behaviour; admins can also explicitly revoke a token in the panel, which takes effect immediately.

**[`plan_entries` shape will drift from `time_entries` over time]** → Mitigation: accepted. The shared shape is a starting point, not a contract. If a column (e.g. `target_date`) is meaningful only for plans, it lives only on `plan_entries`.

## Migration Plan

1. Apply migration `000007_create_plan_entries.up.sql` and `000008_create_api_tokens.up.sql`. Both are additive — no data backfill, no existing-row updates.
2. Roll out the binary with parser, store, REST handlers, and admin pages enabled.
3. Surface plan-vs-actual in the admin dashboard as a new column; existing dashboard queries unchanged.
4. Document `/plan` and `/unplan` in `docs/commands.md`. Document tokens in new `docs/api-tokens.md`.

**Rollback:** Stop the new binary, run the `down` migrations. Existing `/log` data is untouched in either direction.

## Open Questions

1. Should plan deltas (e.g. "scope grew by 4h") get a dedicated `/replan` verb or piggyback on `/correct` semantics? Current decision: `/plan` always supersedes — same as `/correct` for logs — and we revisit if usage shows people want delta-style updates.
2. Token lifetime: ship without expiry (admin revokes manually) or default 90 days? Current decision: no expiry in v1, revisit when MCP usage data exists.
3. Should an exceeded-plan event surface back into the GitHub thread as a bot comment? Out of scope here; capturable as a follow-up change.
