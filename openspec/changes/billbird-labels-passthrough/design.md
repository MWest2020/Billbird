## Context

Today `time_entries.client_id` is the only structured-from-labels dimension in Billbird. It is populated through a small relational dance: `label_mappings` maps a literal label (`client:amsterdam`) to a `clients` row, the webhook handler resolves it, the resolved foreign key lands on the entry. That's elegant for the *one* dimension we knew we needed in v1.

Two days of conversation about strippenkaarten, WBSO, work-types, and internal hours has shown that the team wants several more label-driven dimensions, with the open list growing over time. Building a `label_mappings`-style table per dimension would multiply schema and code with each new use case. Even one extra (`work_types`) is more code than this is worth.

## Goals / Non-Goals

**Goals:**
- One mechanism that covers every present and future label-driven dimension.
- GitHub remains the source of truth for what labels exist; Billbird does not maintain a label registry.
- Reports can slice on any label or label-prefix at SQL speed.
- Backwards compatible — existing client_id resolution untouched.

**Non-Goals:**
- A budgets table. Strippenkaart totals live in the contract / Excel; comparison is the operator's job.
- A label classification system. Billbird stores raw labels; "this label means WBSO" is a convention the team agrees on, not code Billbird enforces.
- Live joining issue labels. Each entry snapshots the labels visible at log time — retro-edits on the issue do not retro-edit historical entries.
- Updating the MCP tools' input schemas in this change. The REST surface is enough; MCP can pick this up in a follow-up.

## Decisions

### One `labels TEXT[]` column per entry table

**Choice:** Add `labels TEXT[]` to `time_entries` and `plan_entries`. GIN index on each for fast containment queries.

**Rationale:** Postgres arrays are first-class, support `@>` (contains) and `ANY()`, and a single GIN index covers all dimensions. No join, no schema growth per new label dimension. Storage is cheap (labels are short strings, GitHub allows up to ~64 chars; an issue rarely has more than a dozen labels).

**Alternatives considered:**
- *JSONB column* — same query power, slightly more flexible (key-value pairs), but heavier. We only need strings, not nested structure.
- *Separate `entry_labels(entry_id, label)` table* — most normalised but every query needs a join and the row count grows by entry-count × labels-per-entry. CRAP-wise, two-orders-of-magnitude more rows, and a containment query needs a self-join trick. Rejected on simplicity.
- *Per-dimension columns (`type`, `wbso_category`, `strippenkaart_id`)* — schema grows with every new dimension. We just spent the work to avoid that.

### Snapshot at write, never re-join

**Choice:** At `/log`, `/correct`, `/plan` time, capture the labels currently on the issue. Never recompute. If labels change on the issue later, only entries created **after** that change reflect it.

**Rationale:** Audit integrity. The whole point of the existing correction-chain pattern is that what was recorded is what was true at the moment of recording. Letting label changes retro-edit historical reports would break that promise.

**Trade-off:** A retroactive relabel ("oh, this issue was always strippenkaart-X") needs admin intervention (the existing admin-correction path) rather than just relabeling on GitHub. Documented; not a code problem.

### `client_id` resolution stays

**Choice:** Keep the existing `clientResolver` flow. After this change, an entry has both a resolved `client_id` AND a raw `labels` array containing `client:amsterdam`.

**Rationale:** The two columns serve different consumers. `client_id` joins to `clients.name` for the billing-side reports and the admin panel; `labels` is for the new free-form aggregation. Removing `client_id` would force every billing query to do a string-prefix scan, which is worse than what we have. They cost a few bytes per row to coexist.

### Filter API: containment plus prefix

**Choice:** `ListFilter.Labels []string` means "every label in the list must be present" (Postgres `labels @> $1`); `ListFilter.LabelPrefix string` is `EXISTS (SELECT 1 FROM unnest(labels) l WHERE l LIKE $1 || '%')`.

**Rationale:** Two operators cover the two real questions: "this exact dimension value" (containment) and "any value in this dimension" (prefix). No need for more expressive query languages at v1 — if a real second dimension-prefix query lands later we add `LabelPrefixes []string` then.

**Alternatives considered:**
- *Full predicate language* — over-engineered. Most users will want exact match or single prefix.
- *Subqueries from MCP layer* — would push complexity into Gitsweeper for no gain. Better to centralise the SQL in Billbird's store.

### REST API extension

**Choice:** `GET /api/v1/time-entries` and `GET /api/v1/plans` accept `?label=foo&label=bar` (repeatable) and `?label_prefix=wbso:`. Response rows expose `labels`.

**Rationale:** Matches the filter shape one-to-one; consumers (Gitsweeper MCP, admin panel, future Nextcloud) get the same expressiveness as the store. No new endpoint needed.

## Risks / Trade-offs

**[GIN index size grows with label cardinality]** → Mitigation: at expected scale (a few thousand entries × <20 distinct labels) the index is negligible. Periodic `REINDEX` if we ever see bloat.

**[Operators relying on relabeled issues to fix historical data]** → Mitigation: documented in `docs/commands.md` (or a new `docs/labels.md`) that labels are snapshotted. The admin correction path can edit an entry's labels if a real correction is needed.

**[Two sources of "client" information (`client_id` and `client:*` label)]** → Mitigation: prefer `client_id` in billing queries. The label is for slicing within a billing report (e.g. "of these client-X hours, how many are WBSO-eligible?"). Document the precedence; no enforcement code.

**[MCP tools could go stale relative to the REST surface]** → Mitigation: out of scope for this change. The follow-up MCP change is small and clearly framed.

## Migration Plan

1. Apply migrations 000009 and 000010 on a running database; both are additive (`ADD COLUMN labels TEXT[] NOT NULL DEFAULT '{}'`). No backfill needed.
2. Deploy the new binary. New entries land with non-empty `labels`; old entries have `'{}'` (empty array) and continue to work.
3. The admin panel and REST API expose the new `labels` field; clients that don't read it ignore it (additive JSON).

**Rollback:** `DROP COLUMN labels` per migration `down`. No data loss to anything that existed before this change.

## Open Questions

1. Should the admin panel surface labels as a column in the entries table? *Default decision: yes, small chips next to the description. Implementation in this change.*
2. Should the MCP tool inputs gain a `label_prefix` field in this PR or the next? *Default decision: next. Keep this PR focused on Billbird's surface.*
3. Should we expose labels on `plan-vs-actual` aggregations too? *Default decision: not yet — it's an issue-level view; labels are entry-level. Revisit if a real query asks for it.*
