## ADDED Requirements

### Requirement: Plan entry storage
The system SHALL store plan entries in a Postgres table `plan_entries` with the following attributes: entry ID, GitHub user ID, GitHub username, repository (owner/name), issue number, planned duration in minutes, description (nullable), source GitHub comment ID, source GitHub comment URL, plan status (`active`, `superseded`, `deleted`), superseded-by entry ID (nullable, foreign key to `plan_entries.id`), closing-comment ID (nullable, set when `/unplan` soft-deletes the row), closing-comment URL (nullable, paired with the closing comment ID), created-at timestamp, and created-by type (`user` or `admin`). Plan entries SHALL NOT carry a `client_id` column; plans are forecasts and are never billable in their own right.

#### Scenario: Plan entry record created
- **WHEN** a `/plan 8h` command is processed
- **THEN** a row is inserted in `plan_entries` with status `active`, the correct planned minutes, user, issue, and source comment reference

#### Scenario: Plan entry superseded by re-plan
- **WHEN** a `/plan 12h` command supersedes plan #12
- **THEN** plan #12's status changes to `superseded` and its `superseded_by` field points to the new plan's ID

#### Scenario: Plan entry soft-deleted by unplan
- **WHEN** a `/unplan` command targets plan #12
- **THEN** plan #12's status changes to `deleted`, the closing comment ID and URL are stored, and no rows are physically removed

### Requirement: At most one active plan per issue
The `plan_entries` table SHALL enforce that at most one row per `(repository, issue_number)` carries status `active` at any time, via a partial unique index.

#### Scenario: Duplicate active plan rejected at the database level
- **WHEN** a write attempts to set a second row with status `active` for the same repository and issue number while another active row exists
- **THEN** the database raises a unique-constraint violation and the write is rejected

### Requirement: API token storage
The system SHALL store API tokens in a Postgres table `api_tokens` with: token ID, owning GitHub user ID, owning GitHub username, label (free-text user-provided), prefix (first 8 base64 characters of the plaintext, for display), bcrypt hash of the full plaintext, created-at timestamp, last-used-at timestamp (nullable), revoked flag (boolean), and revoked-at timestamp (nullable).

#### Scenario: Token row at creation time
- **WHEN** a user creates a token labelled "Manager-MCP"
- **THEN** a row is inserted with prefix, bcrypt hash, label, created-at set, last-used-at null, revoked false

#### Scenario: Token row updates on use
- **WHEN** a bearer token is used to authenticate a request
- **THEN** the corresponding row's `last_used_at` is updated, throttled to at most once per minute

#### Scenario: Token row marked revoked
- **WHEN** an owner or admin revokes a token
- **THEN** the row's `revoked` flag becomes true and `revoked_at` is set; the bcrypt hash is retained for audit purposes

### Requirement: Plan and token tables do not break existing audit guarantees
The `plan_entries` table SHALL follow the same no-physical-delete invariant as `time_entries`. The `api_tokens` table SHALL retain rows for revoked tokens; a revoked token row SHALL never be physically removed.

#### Scenario: Soft-deleted plan still exists
- **WHEN** a plan is deleted via `/unplan` and then a new plan is created on the same issue
- **THEN** the deleted plan row, the new active row, and any chain rows in between all coexist with correct status values

#### Scenario: Revoked token row retained
- **WHEN** a token is revoked
- **THEN** the row remains in `api_tokens` with `revoked = true` and no physical delete occurs
