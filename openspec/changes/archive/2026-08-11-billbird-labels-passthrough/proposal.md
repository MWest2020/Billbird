## Why

After the live smoke on 2026-05-21 we have a clear picture of the dimensions teams want to slice time reports on: client (already covered), but also **WBSO category**, **work type** (development / bugfix / beheer), **strippenkaart** (a budget identifier), and **internal hours** (verlof / ziek / scholing / overhead). Today Billbird only resolves `client_id` from a single labelled mapping; every other dimension is invisible to the database.

The natural answer is to mirror **all** issue labels into Billbird at log/plan time, with no opinion on what each label means. GitHub stays the source of truth for labels; Billbird snapshots them per entry; reports filter on label-prefix in SQL. One mechanism, four (and more, later) dimensions.

This deliberately does **not** introduce a strippenkaart-budgets table: budgets are negotiated with the customer and live in Excel / contract. Billbird answers "how many hours got logged against this label?" and the operator compares that to the negotiated total. If automated burn-down alerts ever become necessary, a `budgets` table can be added without disturbing the label column.

## What Changes

- New column `labels TEXT[]` on `time_entries` and on `plan_entries`. Snapshots the labels present on the issue at the moment the comment was posted; never updated retroactively.
- New migration `000009_add_labels_to_time_entries` and `000010_add_labels_to_plan_entries`. GIN indexes on each `labels` column so containment queries (`labels @> ARRAY['wbso:speur']`) stay cheap as data grows.
- The webhook handler, which already fetches issue labels for client resolution, now passes the same list through to the entry record. No extra GitHub API call.
- `timeentry.Entry` and `planentry.Entry` gain a `Labels []string` field.
- `timeentry.ListFilter` and `planentry.ListFilter` gain a `Labels []string` field (containment — all of the listed labels must be present) and `LabelPrefix string` (any label starting with the given prefix).
- REST API `GET /api/v1/time-entries` and `GET /api/v1/plans` accept `?label=foo&label=bar` (repeatable) and `?label_prefix=wbso:`. Response payloads expose the `labels` array on every row.
- `client_id` resolution stays exactly as it is — labels-passthrough is additive, not a replacement for the `label_mappings` table.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `data-model`: add the `labels TEXT[]` column and GIN index on `time_entries` and `plan_entries`. Reaffirm that label state is snapshotted, not joined live, so a label removed from the issue later does not retro-edit historical entries.
- `time-logging`: confirm that `/log` and `/correct` snapshot the issue's labels alongside the existing client attribution.
- `time-planning`: same for `/plan` and `/unplan`-aware chain entries.

## Impact

- **Code (Go)**: one column added to two structs; webhook handler call-site adjusted; store CRUD updated to pass `labels` through; one filter clause added to `List` SQL.
- **Schema**: two additive migrations. No backfill — existing rows get an empty array (NULL coalesced).
- **REST API**: backwards compatible; response includes a new optional field, request gains two new optional query params.
- **MCP / Gitsweeper**: the existing `billbird_hours_summary` and `billbird_plan_vs_actual` can be extended to accept a `label_prefix` argument in a follow-up. Not required for v1 of this change — the REST surface is enough.
- **Reports**: a single new pattern — group by label-prefix — covers strippenkaarten, WBSO categories, work types, and internal categories without per-dimension code.
- **Backwards compatibility**: fully additive.
