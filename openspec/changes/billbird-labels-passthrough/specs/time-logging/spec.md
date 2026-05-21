## ADDED Requirements

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
