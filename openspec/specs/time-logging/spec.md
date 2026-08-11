# time-logging Specification

## Purpose
TBD - created by archiving change billbird-v1. Update Purpose after archive.
## Requirements
### Requirement: Log time via slash command
The system SHALL parse `/log <duration>` commands from GitHub issue comments. Duration SHALL support hours (`h`), minutes (`m`), and combined formats (e.g., `1h30m`, `2h`, `45m`). The system SHALL create a time entry linked to the commenting user, the issue, and the repository.

#### Scenario: Log hours on an issue
- **WHEN** a user comments `/log 2h` on a GitHub issue
- **THEN** the system creates a time entry of 120 minutes for that user on that issue and posts a confirmation comment

#### Scenario: Log combined hours and minutes
- **WHEN** a user comments `/log 1h30m` on a GitHub issue
- **THEN** the system creates a time entry of 90 minutes

#### Scenario: Log minutes only
- **WHEN** a user comments `/log 45m` on a GitHub issue
- **THEN** the system creates a time entry of 45 minutes

#### Scenario: Invalid duration format
- **WHEN** a user comments `/log abc` or `/log` with no duration
- **THEN** the system posts a comment explaining the valid format and does not create a time entry

### Requirement: Log time with description
The system SHALL support an optional description after the duration in a `/log` command. The description SHALL be stored with the time entry.

#### Scenario: Log with description
- **WHEN** a user comments `/log 2h Fixed authentication bug`
- **THEN** the system creates a time entry of 120 minutes with description "Fixed authentication bug"

#### Scenario: Log without description
- **WHEN** a user comments `/log 2h`
- **THEN** the system creates a time entry of 120 minutes with a null description

### Requirement: Correct time via slash command
The system SHALL parse `/correct <duration>` commands from GitHub issue comments. A correction SHALL create a new time entry that supersedes the user's most recent active entry on that issue. The previous entry SHALL be marked as superseded but not deleted.

#### Scenario: Correct a previous entry
- **WHEN** a user who logged 2h comments `/correct 3h` on the same issue
- **THEN** the system creates a new 180-minute entry, marks the previous 120-minute entry as superseded, and posts a confirmation comment showing old and new values

#### Scenario: Correct with no prior entry
- **WHEN** a user comments `/correct 1h` on an issue where they have no prior time entry
- **THEN** the system posts a comment explaining there is no entry to correct

### Requirement: Correction with description
The system SHALL support an optional description after the duration in a `/correct` command.

#### Scenario: Correct with updated description
- **WHEN** a user comments `/correct 3h Revised estimate after code review`
- **THEN** the correction entry stores the new description

### Requirement: Delete time via slash command
The system SHALL parse `/delete` commands from GitHub issue comments. A delete SHALL mark the user's most recent active entry on that issue as deleted. The entry SHALL NOT be physically removed from the database.

#### Scenario: Delete a time entry
- **WHEN** a user comments `/delete` on an issue where they have an active time entry
- **THEN** the system marks that entry as deleted and posts a confirmation comment

#### Scenario: Delete with no prior entry
- **WHEN** a user comments `/delete` on an issue where they have no active time entry
- **THEN** the system posts a comment explaining there is no entry to delete

### Requirement: Confirmation comments
The system SHALL post a GitHub comment on the issue confirming every successful `/log`, `/correct`, and `/delete` action. The confirmation SHALL include the action taken, the duration, and the entry ID for reference.

#### Scenario: Log confirmation
- **WHEN** a `/log 2h` command is successfully processed
- **THEN** the bot posts a comment like "Logged 2h for @user (entry #42)"

#### Scenario: Correction confirmation
- **WHEN** a `/correct 3h` command is successfully processed
- **THEN** the bot posts a comment like "Corrected @user's entry from 2h to 3h (entry #43 supersedes #42)"

### Requirement: Comment provenance
Every time entry SHALL store the GitHub comment ID and URL that created it. This links the database record back to the audit trail in the issue thread.

#### Scenario: Entry links to source comment
- **WHEN** a time entry is created from a `/log` command
- **THEN** the entry record includes the GitHub comment ID and permalink URL

### Requirement: Alternative time-source via commit-message footer

The project documentation SHALL describe a complementary time-source: a pre-commit hook on the developer's machine that estimates time elapsed since the previous commit, prompts the developer to confirm or edit, and writes a `Time: <duration>` footer into the commit message. The hook SHALL NOT replace `/log` issue comments — both sources coexist so a later A/B reconciliation tool (planned for Gitsweeper) can surface discrepancies.

The hook SHALL be deliverable as documentation only: a JSON snippet for `~/.claude/settings.json` and a short shell script that the developer copies into their local Claude Code configuration. No code in Billbird itself is required, and no Billbird behaviour changes when the hook is or isn't installed.

#### Scenario: Hook produces a Time footer
- **WHEN** a developer with the hook installed runs `git commit` on a repo where work has been done since the last commit
- **THEN** the commit message gains a `Time: 1h30m` footer (or similar duration), suggested by the hook and confirmed by the developer

#### Scenario: Hook is optional
- **WHEN** a developer does not install the hook
- **THEN** their workflow is unchanged; `/log` issue comments remain the only path Billbird sees and reports normally

#### Scenario: Hook does not contact Billbird
- **WHEN** the hook runs
- **THEN** it operates entirely on the dev's local working copy and the `claude` CLI; no Billbird API call is made and no token is required at the developer machine

### Requirement: Documentation cross-references

`docs/commands.md` and `README.md` SHALL each link to the new `docs/dev-time-hook.md` so devs discover the optional second time-source from the same entry points they use to learn `/log`.

#### Scenario: Dev follows the docs from the README
- **WHEN** a developer reads `README.md` looking for how to record time
- **THEN** they see both `/log` (the primary) and a link to the dev-time hook (the optional commit-side complement)

### Requirement: Filter time entries by label containment

The system SHALL accept a `label` query parameter (repeatable) on `GET /api/v1/time-entries`. When supplied, the result set SHALL contain only entries whose `labels` column contains every requested value. The matching SHALL be case-sensitive (GitHub labels are case-sensitive in their canonical form).

#### Scenario: Single label filter
- **WHEN** a client calls `GET /api/v1/time-entries?label=wbso:speur`
- **THEN** the response contains every active entry whose labels array includes `wbso:speur`

#### Scenario: Multiple labels (AND semantics)
- **WHEN** a client calls `GET /api/v1/time-entries?label=client:amsterdam&label=type:bugfix`
- **THEN** the response contains entries that have *both* labels, not entries that have one or the other

### Requirement: Filter time entries by label prefix

The system SHALL accept a `label_prefix` query parameter on `GET /api/v1/time-entries`. When supplied, the result set SHALL contain only entries that have at least one label starting with the given prefix.

#### Scenario: Prefix matches a dimension
- **WHEN** a client calls `GET /api/v1/time-entries?label_prefix=wbso:`
- **THEN** the response contains every active entry that has any `wbso:*` label, regardless of the suffix

#### Scenario: Prefix without trailing colon
- **WHEN** a client calls `GET /api/v1/time-entries?label_prefix=client`
- **THEN** the response includes entries with `client:amsterdam`, `client:rotterdam`, etc.

### Requirement: Response payload exposes labels

The JSON response shape for `GET /api/v1/time-entries` and `GET /api/v1/time-entries/{id}` SHALL include the `labels` array on every entry, in addition to existing fields.

#### Scenario: Field present even when empty
- **WHEN** an entry was logged on an issue with no labels
- **THEN** the response row carries `"labels": []` — never `null`, never absent

