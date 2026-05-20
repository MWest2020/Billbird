## ADDED Requirements

### Requirement: Plan-vs-actual dashboard column
The admin panel dashboard SHALL display a `Plan vs Logged` column for every issue surfaced in the entries view. The column SHALL show planned minutes (or a dash when no active plan exists), logged minutes summed across active log entries, and a status badge with one of `no_plan`, `under`, `on_target`, `over`. Values SHALL come from the plan-vs-actual REST endpoint, not from a separate database query.

#### Scenario: Issue with both a plan and logs
- **WHEN** an admin views the dashboard with an issue that has an active 8h plan and 3h logged
- **THEN** the row shows `8h / 3h` with an `under` badge

#### Scenario: Issue with no plan
- **WHEN** an admin views the dashboard with an issue that has logs but no active plan
- **THEN** the row shows `— / <logged>` with a `no_plan` badge

#### Scenario: Issue with no logs yet
- **WHEN** an admin views the dashboard with an issue that has an active plan and no logs yet
- **THEN** the row shows `<planned> / 0h` with an `under` badge

### Requirement: Plan history view
The admin panel SHALL provide a view of the full plan chain for any issue, similar to the existing correction-history view for log entries. The view SHALL list every plan entry in chronological order, marking the active one and linking each plan to its source GitHub comment.

#### Scenario: View plan chain
- **WHEN** an admin opens the plan-history view for an issue whose plan has been changed twice and then unplanned
- **THEN** the view lists all three plan rows with their statuses (`superseded`, `superseded`, `deleted`), the supersede pointers, and direct links to the originating GitHub comments

### Requirement: Token management page
The admin panel SHALL include a token management page accessible from the authenticated user's account menu. The page SHALL list the current user's tokens with the columns defined in the api-tokens capability (label, created-at, last-used-at, prefix, revoked flag). Admins SHALL additionally be able to view and revoke any user's tokens.

#### Scenario: User creates a token from the panel
- **WHEN** an authenticated user submits the create-token form with a label
- **THEN** the panel displays the plaintext token exactly once on the next page, and the token then appears in the listing with its prefix only

#### Scenario: User revokes a token from the panel
- **WHEN** an authenticated user clicks revoke next to their own token
- **THEN** the panel marks the token as revoked and a subsequent API call using that token fails with HTTP 401

#### Scenario: Admin revokes another user's token
- **WHEN** an admin opens the token management page for user @bob and clicks revoke on one of his tokens
- **THEN** the token is marked revoked and the action is recorded with the admin's identity in the audit fields
