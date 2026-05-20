## ADDED Requirements

### Requirement: Plan time via slash command
The system SHALL parse `/plan <duration> [description]` commands from GitHub issue comments. Duration SHALL accept the same formats as `/log` (hours, minutes, combined). On a successful `/plan`, the system SHALL create a plan entry linked to the commenting user, the issue, and the repository, and SHALL post a confirmation comment.

#### Scenario: Plan hours on an issue without an existing plan
- **WHEN** a user comments `/plan 8h` on an issue with no active plan
- **THEN** the system creates a plan entry of 480 minutes with status `active` and posts a confirmation comment naming the plan ID

#### Scenario: Plan with description
- **WHEN** a user comments `/plan 4h Initial scope estimate`
- **THEN** the plan entry stores the description "Initial scope estimate"

#### Scenario: Invalid plan duration
- **WHEN** a user comments `/plan` or `/plan abc`
- **THEN** no plan entry is created and the system posts an error comment explaining the valid format

### Requirement: Re-planning supersedes the active plan
A second `/plan` on the same issue SHALL create a new plan entry, mark the previously active plan as `superseded`, and set the previous plan's `superseded_by` field to the new plan's ID. At all times at most one plan per (repository, issue) is `active`.

#### Scenario: Re-plan an issue
- **WHEN** the active plan for issue #42 is 8h, and a user comments `/plan 12h`
- **THEN** the previous plan is marked superseded with `superseded_by` set to the new plan, a new active 720-minute plan exists, and the confirmation comment names both plan IDs

#### Scenario: Same plan duration restated
- **WHEN** the active plan is 8h and any user comments `/plan 8h`
- **THEN** the previous plan is still superseded by a new active 8h entry — duration equality does not short-circuit the chain

### Requirement: Remove a plan via slash command
The system SHALL parse `/unplan` commands. The command SHALL mark the issue's active plan entry as `deleted` and SHALL NOT physically remove any row. If no active plan exists, the system SHALL post an error comment.

#### Scenario: Remove the active plan
- **WHEN** a user comments `/unplan` on an issue with an active 8h plan
- **THEN** that plan's status changes to `deleted` and the system posts a confirmation comment naming the deleted plan ID

#### Scenario: Unplan with no active plan
- **WHEN** a user comments `/unplan` on an issue that has no active plan
- **THEN** no rows change and the system posts an error comment explaining there is nothing to unplan

### Requirement: Plan commands authorised through ALLOWED_ORGS
The system SHALL apply the same membership check used for `/log` to `/plan` and `/unplan`. Only members of an organisation listed in `ALLOWED_ORGS` SHALL succeed; non-members SHALL receive the existing not-authorised error comment.

#### Scenario: Non-member uses /plan
- **WHEN** a GitHub user who is not a member of any allowed organisation comments `/plan 4h`
- **THEN** no plan entry is created and the bot posts an authorisation-error comment identical in tone to the existing `/log` error

### Requirement: Plan confirmation comments
The system SHALL post a confirmation comment for every successful `/plan` and `/unplan`. The comment SHALL distinguish a new plan from a superseding plan and SHALL reference plan IDs (not entry IDs).

#### Scenario: New plan confirmation
- **WHEN** a `/plan 8h` command succeeds on an issue with no existing plan
- **THEN** the bot posts a comment like "Planned 8h on this issue by @user (plan #12)"

#### Scenario: Re-plan confirmation
- **WHEN** a `/plan 12h` command supersedes plan #12 with the new plan #13
- **THEN** the bot posts a comment like "Updated @user's plan from 8h to 12h (plan #13 supersedes #12)"

#### Scenario: Unplan confirmation
- **WHEN** a `/unplan` command soft-deletes plan #12 (8h)
- **THEN** the bot posts a comment like "Removed @user's plan of 8h (plan #12)"

### Requirement: Plan comment provenance
Every plan entry SHALL store the GitHub comment ID and comment URL that produced it. Both the originating `/plan` and `/unplan` actions create comment-bound rows (the unplan does this by updating the status of the existing row and recording the unplan comment on a `closed_by_comment` reference).

#### Scenario: Plan entry links to source comment
- **WHEN** a plan entry is created from a `/plan` command
- **THEN** the entry record stores the comment ID and the permalink URL

#### Scenario: Unplan records its source comment
- **WHEN** an `/unplan` command soft-deletes plan #12
- **THEN** plan #12's record stores a reference to the unplan comment alongside its original source comment

### Requirement: Plan-vs-actual aggregation
The system SHALL expose a per-issue plan-vs-actual view through the REST API. The view SHALL return planned minutes (from the active plan, or zero if none), logged minutes (sum of active log entries on that issue), variance in minutes (logged minus planned), and a status field with one of `no_plan`, `under` (logged < planned), `on_target` (within 5 percent of planned), or `over` (logged > planned by more than 5 percent).

#### Scenario: Issue with plan and logs under target
- **WHEN** an issue has an active 8h plan and 3h logged
- **THEN** the API returns `{planned: 480, logged: 180, variance: -300, status: "under"}`

#### Scenario: Issue with no plan
- **WHEN** an issue has 2h logged but no active plan
- **THEN** the API returns `{planned: 0, logged: 120, variance: 120, status: "no_plan"}`

#### Scenario: Issue logged slightly above plan
- **WHEN** an issue has an active 8h plan and 8h12m logged (482 minutes vs 480 minutes, within 5 percent)
- **THEN** the API returns `status: "on_target"`

#### Scenario: Issue logged well above plan
- **WHEN** an issue has an active 8h plan and 12h logged
- **THEN** the API returns `status: "over"` and a positive variance value

### Requirement: Plan listing API
The system SHALL expose `/api/v1/plans` returning plan entries with filters for repository, issue number, plan status, and creation period. The endpoint SHALL return chain references (superseded_by, soft-delete state) so consumers can reconstruct history.

#### Scenario: Filter plans by repository and period
- **WHEN** a client calls `GET /api/v1/plans?repository=org/repo&since=2026-05-01`
- **THEN** the response contains every plan entry for that repository created on or after the given date, with chain references intact

### Requirement: Single plan retrieval API
The system SHALL expose `GET /api/v1/plans/{id}` returning the plan entry and its full correction chain (predecessors and successors).

#### Scenario: Fetch a superseded plan with chain
- **WHEN** plan #12 was superseded by plan #13
- **THEN** `GET /api/v1/plans/12` returns plan #12 plus a `chain` array referencing plan #13 (and any further descendants)
