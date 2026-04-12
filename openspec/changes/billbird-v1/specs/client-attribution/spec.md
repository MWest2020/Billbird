## ADDED Requirements

### Requirement: Client-label mapping
Admins SHALL be able to create mappings between GitHub labels and client records. A mapping associates a label pattern (e.g., `client:amsterdam`) with a specific client in the system. Mappings are scoped to a repository or apply globally across all repositories.

#### Scenario: Create a label-to-client mapping
- **WHEN** an admin maps label `client:amsterdam` to client "City of Amsterdam" for repository `org/repo`
- **THEN** the system stores the mapping and uses it for future time entry attribution

#### Scenario: Global label mapping
- **WHEN** an admin creates a label mapping without specifying a repository
- **THEN** the mapping applies to all repositories

#### Scenario: Repository mapping takes precedence
- **WHEN** both a global and repository-specific mapping exist for the same label
- **THEN** the repository-specific mapping takes precedence

### Requirement: Automatic client attribution on time entry
When a time entry is created, the system SHALL check the issue's labels and automatically associate the entry with the matching client. If multiple client labels are present, the system SHALL use the first match and log a warning.

#### Scenario: Issue has a client label
- **WHEN** a user logs time on an issue labeled `client:amsterdam`
- **THEN** the time entry is automatically attributed to client "City of Amsterdam"

#### Scenario: Issue has no client label
- **WHEN** a user logs time on an issue with no client-matching labels
- **THEN** the time entry has no client attribution (null client)

#### Scenario: Issue has multiple client labels
- **WHEN** a user logs time on an issue with labels `client:amsterdam` and `client:rotterdam`
- **THEN** the system attributes to the first matching client and includes a note in the confirmation comment about the ambiguity

### Requirement: Client management
Admins SHALL be able to create, update, and deactivate client records. Deactivated clients SHALL NOT receive new time attribution but their historical entries SHALL remain intact.

#### Scenario: Create a client
- **WHEN** an admin creates a client named "City of Amsterdam"
- **THEN** the client is available for label mapping and time attribution

#### Scenario: Deactivate a client
- **WHEN** an admin deactivates a client
- **THEN** new time entries are not attributed to that client, but existing entries retain their attribution
