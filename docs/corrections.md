# Corrections and deletions

Billbird uses a non-destructive correction chain. Nothing is ever physically deleted from the database. This ensures full auditability --- every change is traceable back to a specific GitHub comment.

## The correction chain

Every time entry has a **status**:

| Status | Meaning |
|--------|---------|
| `active` | The current, valid entry |
| `superseded` | Replaced by a newer entry |
| `deleted` | Soft-deleted by the user or admin |

### How /correct works

When a user runs `/correct 3h` on an issue where they previously logged `2h`:

1. The original entry (2h) is marked as `superseded`
2. A new entry (3h) is created with status `active`
3. The original entry's `superseded_by` field points to the new entry
4. Both entries remain in the database

```
Entry #42: 2h, status=superseded, superseded_by=#43
Entry #43: 3h, status=active
```

Multiple corrections form a chain:

```
Entry #42: 2h, status=superseded, superseded_by=#43
Entry #43: 3h, status=superseded, superseded_by=#44
Entry #44: 4h, status=active
```

### How /delete works

When a user runs `/delete`:

1. The most recent `active` entry is marked as `deleted`
2. The entry stays in the database
3. It no longer counts toward reported hours

```
Entry #42: 2h, status=deleted
```

### Scope

`/correct` and `/delete` always target the user's most recent `active` entry on the current issue. A user cannot correct or delete another user's entries through slash commands.

## Admin corrections

Admins can adjust any entry through the admin panel. Admin corrections:

- Follow the same non-destructive pattern
- Record the admin's identity
- Require a reason

```
Entry #42: 2h, status=superseded, superseded_by=#45
Entry #45: 3h, status=active, created_by=admin, reason="Developer forgot review time"
```

## Audit trail

Every entry stores:

- `source_comment_id`: The GitHub comment that created it
- `source_comment_url`: Direct link to the GitHub comment
- `created_by`: Whether it was created by a `user` (slash command) or `admin` (panel)
- `created_at`: UTC timestamp

The issue thread itself serves as the audit log. The database records link back to the exact comments, and the comments link forward to the entries via the bot's confirmation messages.

## Reporting

When generating reports or exports, only `active` entries are counted toward totals. The full chain is available for audit purposes --- admins can view the complete correction history for any entry through the admin panel.
