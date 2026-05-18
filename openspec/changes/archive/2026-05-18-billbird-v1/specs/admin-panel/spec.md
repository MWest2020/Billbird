## ADDED Requirements

### Requirement: GitHub OAuth authentication
The admin panel SHALL require authentication via GitHub OAuth. Only users who are members of the configured GitHub organization (or repository collaborators) SHALL be granted access. Admin roles SHALL be configurable.

#### Scenario: Authorized user logs in
- **WHEN** a GitHub user who is a member of the configured organization visits the admin panel
- **THEN** they are redirected through GitHub OAuth and granted access

#### Scenario: Unauthorized user attempts login
- **WHEN** a GitHub user who is not a member of the configured organization attempts to access the admin panel
- **THEN** they are denied access with a clear error message

### Requirement: Hours overview dashboard
The admin panel SHALL display a dashboard showing logged hours aggregated by developer, by client, and by time period. Admins SHALL be able to filter by date range, repository, client, and developer.

#### Scenario: View hours by developer
- **WHEN** an admin views the dashboard filtered by a specific developer
- **THEN** they see all active time entries for that developer, grouped by issue and client

#### Scenario: View hours by client
- **WHEN** an admin views the dashboard filtered by a specific client
- **THEN** they see all active time entries attributed to that client, grouped by developer and issue

#### Scenario: Filter by date range
- **WHEN** an admin filters the dashboard to a specific week
- **THEN** only time entries within that date range are shown

### Requirement: Correction history view
The admin panel SHALL display the full correction chain for any time entry. This includes the original entry, all corrections, and any deletions, with timestamps and links to the source GitHub comments.

#### Scenario: View correction history
- **WHEN** an admin views a time entry that has been corrected twice
- **THEN** they see the original entry, both corrections, and which entry is currently active, each linked to its GitHub comment

### Requirement: Manual entry adjustment
Admins SHALL be able to manually adjust time entries from the admin panel. Manual adjustments SHALL be recorded with the admin's identity and a required reason, following the same non-destructive correction chain pattern.

#### Scenario: Admin adjusts an entry
- **WHEN** an admin changes a developer's 2h entry to 3h with reason "Developer forgot to include review time"
- **THEN** a new correction entry is created with the admin's identity and reason, superseding the previous entry

### Requirement: Client-label mapping management
The admin panel SHALL provide an interface for creating, editing, and removing client-label mappings as defined in the client-attribution capability.

#### Scenario: Admin creates a label mapping
- **WHEN** an admin maps label `client:rotterdam` to client "Port of Rotterdam"
- **THEN** the mapping is saved and immediately active for new time entries

### Requirement: CSV export
The admin panel SHALL allow admins to export time entry data as CSV. The export SHALL respect the current filters (date range, client, developer, repository) and include: date, developer, repository, issue, client, duration, description, and entry type (original/correction/deletion).

#### Scenario: Export filtered data
- **WHEN** an admin exports CSV with filters set to client "City of Amsterdam" for March 2026
- **THEN** a CSV file is downloaded containing only matching entries with all specified columns

### Requirement: Cycle time display
The admin panel SHALL display cycle time data alongside logged hours. Cycle time SHALL be shown per issue and aggregated per developer and per repository.

#### Scenario: View cycle time for a repository
- **WHEN** an admin views cycle time for repository `org/repo`
- **THEN** they see average and per-issue cycle times for that repository
