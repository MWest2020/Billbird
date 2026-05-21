## ADDED Requirements

### Requirement: Filter plan entries by label containment and prefix

The system SHALL accept the same `label` (repeatable, AND semantics) and `label_prefix` query parameters on `GET /api/v1/plans` as on the time-entries endpoint. The matching semantics SHALL be identical.

#### Scenario: Plans by strippenkaart
- **WHEN** a client calls `GET /api/v1/plans?label=strippenkaart:acme-2026q1`
- **THEN** the response contains every plan entry whose labels array includes that strippenkaart label

#### Scenario: All plans in a WBSO category
- **WHEN** a client calls `GET /api/v1/plans?label_prefix=wbso:`
- **THEN** the response includes every plan entry with any `wbso:*` label

### Requirement: Plan responses expose labels

`GET /api/v1/plans`, `GET /api/v1/plans/{id}`, and `GET /api/v1/plans/{id}/chain` SHALL each include the `labels` array on every plan record they return.

#### Scenario: Plan chain shows label history per generation
- **WHEN** a plan was created with labels `{strippenkaart:acme-2026q1}` and later superseded by a new plan whose issue had labels `{strippenkaart:acme-2026q1, scope:extended}`
- **THEN** the chain response shows both generations with their respective snapshots — old plan with one label, new plan with both
