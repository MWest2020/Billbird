## ADDED Requirements

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
