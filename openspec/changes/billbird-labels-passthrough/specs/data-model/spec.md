## ADDED Requirements

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
