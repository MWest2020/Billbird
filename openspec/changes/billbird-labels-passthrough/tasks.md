## 1. Schema migrations

- [x] 1.1 Write migration `000009_add_labels_to_time_entries.up.sql` and `.down.sql`: `ADD COLUMN labels TEXT[] NOT NULL DEFAULT '{}'` plus a GIN index `idx_time_entries_labels`
- [x] 1.2 Write migration `000010_add_labels_to_plan_entries.up.sql` and `.down.sql`: same shape for `plan_entries`

## 2. Domain types

- [x] 2.1 Add `Labels []string` to `timeentry.Entry`
- [x] 2.2 Add `Labels []string` to `planentry.Entry`
- [x] 2.3 Make sure both serialise to JSON as `"labels"` (lowercase) and never `null` (empty slice = `[]`)

## 3. Store

- [x] 3.1 `timeentry.Store.Create` writes the labels column; INSERT statement uses `$N::text[]`
- [x] 3.2 `timeentry.Store.List` accepts `ListFilter.Labels []string` (AND containment via `labels @> $N`) and `ListFilter.LabelPrefix string` (via `EXISTS (SELECT 1 FROM unnest(labels) l WHERE l LIKE ...)`)
- [x] 3.3 `timeentry.Store.GetByID` / `GetCorrectionChain` populate the Labels field on every scan
- [x] 3.4 Same set of edits on `planentry.Store`

## 4. Webhook handler

- [x] 4.1 `webhook.handleLog`: pass the already-fetched labels to `timeentry.Entry.Labels` before calling `Create`
- [x] 4.2 `webhook.handleCorrect`: snapshot labels on the new corrective entry too (not copied from the superseded entry — re-fetched at the moment of the new comment)
- [x] 4.3 `webhook.handlePlan`: same for plan_entries
- [x] 4.4 `webhook.handlePlanCorrect` (re-plan): same

## 5. REST API

- [x] 5.1 `GET /api/v1/time-entries` parses repeating `?label=` into `f.Labels` and `?label_prefix=` into `f.LabelPrefix`
- [x] 5.2 `GET /api/v1/plans` same
- [x] 5.3 Verify JSON response shapes (`labels` always present, never null)

## 6. Admin panel (small)

- [x] 6.1 `templates/entries_table.html` shows label chips on each row (small, muted styling, comma-separated)
- [x] 6.2 `templates/plan_history.html` same
- [x] 6.3 Filter input on the dashboard accepts a free-text label-prefix field (optional, keep it minimal)

## 7. Unit tests

- [x] 7.1 `timeentry` Create round-trip: insert with labels, read back, equal
- [x] 7.2 `timeentry` List filter: containment (single + AND), prefix, empty filter
- [x] 7.3 `planentry` mirror of 7.1 and 7.2
- [x] 7.4 JSON shape: marshalled entry with empty labels has `"labels":[]` not `"labels":null`

## 8. Integration tests

- [x] 8.1 New test in `internal/integration` covering: insert two entries (one with labels, one without), verify containment query returns the right one, verify prefix query returns the right one, verify empty-labels entry round-trips clean
- [x] 8.2 New integration test for plans: identical shape

## 9. Live smoke

- [x] 9.1 Apply both migrations against the dev Postgres
- [x] 9.2 Add the labels `wbso:speur`, `type:development`, `strippenkaart:smoke` to test-issue MWest2020/Billbird#... (new one), post `/log 2h`, hit `/api/v1/time-entries?label_prefix=wbso:` and confirm the entry returns with the labels snapshotted

## 10. Documentation

- [x] 10.1 New `docs/labels.md` — convention examples (`client:*`, `type:*`, `wbso:*`, `strippenkaart:*`, `internal:*`) and the snapshot semantics
- [x] 10.2 `docs/commands.md` mentions that `/log` and `/plan` snapshot issue labels
- [x] 10.3 `CHANGELOG.md` entry
