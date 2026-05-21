# Labels

Billbird snapshots every GitHub label on an issue into the time entry or plan entry created at that moment. The snapshot lives on the row as a Postgres `TEXT[]` column and is the basis for all label-driven aggregations — strippenkaart, WBSO, work type, internal hours, anything else.

## Conventions

Use a colon-separated `dimension:value` prefix per label. The dimension makes prefix queries unambiguous; the value is free text. Existing dimensions in use:

| Prefix | Meaning | Example |
|--------|---------|---------|
| `client:` | Customer identifier (also drives `client_id` resolution via `label_mappings`) | `client:amsterdam` |
| `type:` | Work category | `type:development`, `type:bugfix`, `type:beheer` |
| `wbso:` | R&D tax-credit category | `wbso:speur`, `wbso:dev`, `wbso:integratie` |
| `strippenkaart:` | Budget identifier | `strippenkaart:acme-2026q1` |
| `internal:` | Internal hours category, used on the internal-hours repo | `internal:verlof`, `internal:ziek`, `internal:scholing`, `internal:overhead` |

Labels not following this convention still get snapshotted — they just won't be addressable by prefix.

## Semantics

- **Snapshot, not join.** The labels on the entry are frozen at the moment the entry was created. Adding a label to the issue *after* a `/log` does not retro-edit existing entries; only entries created *after* the relabel see the new value. This keeps the audit trail intact.
- **Empty issues are fine.** An issue with no labels produces `labels: []` on the entry, never `null`.
- **`client_id` still works.** The `clients` and `label_mappings` tables remain authoritative for the `client_id` foreign key. The new `labels` column is additive — both coexist on the same row.

## Querying

`GET /api/v1/time-entries` and `GET /api/v1/plans` accept:

- `label=foo&label=bar` (repeatable) — AND containment. The row must contain every listed label.
- `label_prefix=wbso:` — at least one label on the row starts with this prefix.

Examples:

```bash
# All time entries on a strippenkaart
curl -H "Authorization: Bearer $TOKEN" \
  'http://billbird/api/v1/time-entries?label=strippenkaart:acme-2026q1'

# Every WBSO-eligible entry, regardless of subcategory
curl -H "Authorization: Bearer $TOKEN" \
  'http://billbird/api/v1/time-entries?label_prefix=wbso:'

# Bugfix hours for one client
curl -H "Authorization: Bearer $TOKEN" \
  'http://billbird/api/v1/time-entries?label=client:amsterdam&label=type:bugfix'

# Same for plans
curl -H "Authorization: Bearer $TOKEN" \
  'http://billbird/api/v1/plans?label=strippenkaart:acme-2026q1'
```

The response includes a `labels` array on every row so consumers can re-aggregate locally if needed.

## What Billbird does *not* do

- **No budget tracking.** Billbird answers "how many hours got logged against strippenkaart X?". The total agreed with the customer lives in your contract / spreadsheet — comparison is the operator's job. If automated burn-down ever becomes a hard requirement, a future change can add a `budgets` table without disturbing the labels column.
- **No label registry.** Any string can be a label. Conventions above are recommended; nothing enforces them.
- **No relabel-to-fix-history.** If you need to retroactively correct an entry's labels, use the admin correction path (new entry supersedes the old, with the corrected labels).

## Performance notes

Each `labels` column is indexed with a GIN index (`idx_time_entries_labels`, `idx_plan_entries_labels`). At expected scale (thousands of entries, tens of distinct labels) containment queries (`labels @> ARRAY['x']`) return in milliseconds. Prefix queries use a `unnest` + `LIKE` pattern that doesn't use the GIN index, so very wide tables may eventually want a separate trigram index; not necessary for v1.
