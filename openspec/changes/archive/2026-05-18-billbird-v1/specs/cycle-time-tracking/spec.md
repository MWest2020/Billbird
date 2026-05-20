## ADDED Requirements

### Requirement: Track start time from project board
The system SHALL record a start timestamp when an issue is moved to a column designated as "in progress" on a GitHub Project board. The column name matching SHALL be case-insensitive and configurable (default: "In Progress").

#### Scenario: Issue moved to In Progress
- **WHEN** an issue is moved to the "In Progress" column on a configured project board
- **THEN** the system records a cycle time start timestamp for that issue

#### Scenario: Issue already has a start timestamp
- **WHEN** an issue that already has a start timestamp is moved to "In Progress" again
- **THEN** the system does not overwrite the existing start timestamp

### Requirement: Track end time from project board
The system SHALL record an end timestamp when an issue is moved to a column designated as "done" on a GitHub Project board. The column name matching SHALL be case-insensitive and configurable (default: "Done").

#### Scenario: Issue moved to Done
- **WHEN** an issue is moved to the "Done" column on a configured project board
- **THEN** the system records a cycle time end timestamp for that issue

### Requirement: Track end time from PR merge
The system SHALL record an end timestamp when a pull request that references an issue (via closing keywords like "closes #123") is merged, if that issue has a start timestamp but no end timestamp.

#### Scenario: PR merged closes issue with start time
- **WHEN** a PR with body "closes #42" is merged and issue #42 has a cycle time start but no end
- **THEN** the system records the merge time as the cycle time end for issue #42

#### Scenario: PR merged but issue has no start time
- **WHEN** a PR referencing an issue is merged but the issue has no cycle time start
- **THEN** the system takes no action

### Requirement: Cycle time is separate from logged hours
Cycle time records SHALL be stored separately from manual time entries. They represent wall-clock flow time, not billable hours. The admin panel SHALL display them as a distinct metric.

#### Scenario: Issue has both logged hours and cycle time
- **WHEN** an issue has 4h of logged time and a cycle time of 2 days
- **THEN** both values are visible independently in the admin panel and reports
