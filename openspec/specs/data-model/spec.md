# data-model Specification

## Purpose
TBD - created by archiving change billbird-v1. Update Purpose after archive.
## Requirements
### Requirement: Time entry storage
The system SHALL store time entries in a Postgres table with the following attributes: entry ID, GitHub user ID, GitHub username, repository (owner/name), issue number, duration in minutes, description (nullable), client ID (nullable), source GitHub comment ID, source GitHub comment URL, entry status (active, superseded, deleted), superseded-by entry ID (nullable), created-at timestamp, and created-by type (user or admin).

#### Scenario: Time entry record created
- **WHEN** a `/log 2h` command is processed
- **THEN** a row is inserted with status "active", the correct duration, user, issue, and comment reference

#### Scenario: Entry superseded by correction
- **WHEN** a `/correct 3h` command supersedes entry #42
- **THEN** entry #42's status changes to "superseded" and its superseded_by field points to the new entry

#### Scenario: Entry soft-deleted
- **WHEN** a `/delete` command targets entry #42
- **THEN** entry #42's status changes to "deleted" and no rows are physically removed

### Requirement: Client storage
The system SHALL store client records with: client ID, name, active flag, created-at, and updated-at timestamps.

#### Scenario: Client record
- **WHEN** an admin creates client "City of Amsterdam"
- **THEN** a row is inserted with active=true

### Requirement: Label mapping storage
The system SHALL store label-to-client mappings with: mapping ID, label pattern, client ID, repository (nullable for global mappings), created-at, and updated-at timestamps.

#### Scenario: Label mapping record
- **WHEN** an admin maps label `client:amsterdam` to client ID 1 for repo `org/repo`
- **THEN** a row is inserted with the label, client reference, and repository

### Requirement: Cycle time storage
The system SHALL store cycle time records separately from time entries, with: record ID, repository, issue number, start timestamp (nullable), end timestamp (nullable), start source (board column or PR event), end source, created-at, and updated-at timestamps.

#### Scenario: Cycle time start recorded
- **WHEN** an issue moves to "In Progress"
- **THEN** a cycle time record is created (or updated) with the start timestamp

#### Scenario: Cycle time end recorded
- **WHEN** the issue moves to "Done"
- **THEN** the cycle time record is updated with the end timestamp

### Requirement: Webhook delivery tracking
The system SHALL store processed webhook delivery IDs to support idempotent processing. Records SHALL include: delivery ID, event type, processed-at timestamp.

#### Scenario: Delivery ID stored
- **WHEN** a webhook with delivery ID `abc-123` is processed
- **THEN** a record is stored so future duplicates are detected

### Requirement: No physical deletes
The system SHALL NEVER physically delete time entry records. All state changes (corrections, deletions) SHALL be represented through status fields and correction chain references. This ensures full auditability.

#### Scenario: Audit trail preserved
- **WHEN** an entry is corrected and then the correction is deleted
- **THEN** all three records (original, correction, deletion) exist in the database with correct status values and chain references

### Requirement: Label snapshot column on entries

The `time_entries` and `plan_entries` tables SHALL each carry a `labels TEXT[] NOT NULL DEFAULT '{}'` column. At the moment an entry is created, the system SHALL populate the column with the labels currently attached to the GitHub issue the entry references. The column SHALL be indexed with a GIN index to keep containment queries (`labels @> ARRAY[...]`) cheap as the table grows.

#### Scenario: Label snapshot at /log time
- **WHEN** a `/log 2h` is processed on an issue with labels `client:amsterdam`, `type:development`, `wbso:speur`
- **THEN** the inserted `time_entries` row has `labels = {client:amsterdam, type:development, wbso:speur}`

#### Scenario: Label snapshot at /plan time
- **WHEN** a `/plan 8h` is processed on an issue with labels `client:amsterdam`, `strippenkaart:acme-2026q1`
- **THEN** the inserted `plan_entries` row has `labels = {client:amsterdam, strippenkaart:acme-2026q1}`

#### Scenario: Entry on an unlabelled issue
- **WHEN** a `/log 1h` is processed on an issue with no labels
- **THEN** the row has `labels = '{}'` (empty array), never NULL

### Requirement: Labels do not change retroactively

Once an entry has been created, the system SHALL NOT update its `labels` column in response to label changes on the underlying GitHub issue. The column is a historical snapshot. Admin corrections that adjust an entry's labels SHALL follow the existing correction-chain pattern (new row, supersede old).

#### Scenario: Issue relabelled after the fact
- **WHEN** an issue had labels `{type:bugfix}` at the time of a `/log` entry, and later an operator adds `strippenkaart:acme-2026q1` to the issue
- **THEN** the existing entry's `labels` column remains `{type:bugfix}`; only entries created after the relabel see the new value

### Requirement: client_id resolution is unchanged

The `clients` table and `label_mappings` table remain authoritative for the `client_id` column on `time_entries`. The new `labels` column SHALL NOT replace this resolution; both coexist on the same row.

#### Scenario: Client-attributed entry also carries the raw label
- **WHEN** a `/log` on an issue with label `client:amsterdam` is processed and the resolver maps that label to client_id 1
- **THEN** the row has `client_id = 1` AND `labels` contains the literal `client:amsterdam`

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

