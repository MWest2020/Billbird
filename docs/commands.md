# Commands reference

Billbird recognizes slash commands in GitHub issue comments. Commands must appear at the start of a line.

## /log

Log time on the current issue.

```
/log <duration> [description]
```

**Examples:**

```
/log 2h
/log 45m
/log 1h30m
/log 2h Fixed the authentication bug
```

**Duration formats:**

| Format | Meaning |
|--------|---------|
| `2h` | 2 hours (120 minutes) |
| `45m` | 45 minutes |
| `1h30m` | 1 hour 30 minutes (90 minutes) |
| `12h` | 12 hours |

**Behavior:**
- Creates a new time entry with status `active`
- Links the entry to the commenting user, the issue, and the repository
- If the issue has a label matching a [client mapping](client-attribution.md), the entry is automatically attributed to that client
- Bot replies with a confirmation comment

**Confirmation:**
> Logged 2h for @developer (entry #42)

Or with a description:
> Logged 2h for @developer (entry #42) --- Fixed the authentication bug

## /correct

Replace your most recent entry on the current issue.

```
/correct <duration> [description]
```

**Examples:**

```
/correct 3h
/correct 1h30m Revised after code review
```

**Behavior:**
- Finds your most recent `active` entry on this issue
- Creates a new entry with the corrected duration
- Marks the previous entry as `superseded` (not deleted)
- The previous entry's `superseded_by` field points to the new entry
- Client attribution carries over from the original entry

**Confirmation:**
> Corrected @developer's entry from 2h to 3h (entry #43 supersedes #42)

**Errors:**
- If you have no active entry on this issue, the bot replies with an error

## /delete

Remove your most recent entry on the current issue.

```
/delete
```

**Behavior:**
- Finds your most recent `active` entry on this issue
- Marks it as `deleted` (soft delete --- the row stays in the database)
- No physical deletion ever occurs

**Confirmation:**
> Deleted @developer's entry of 2h (entry #42)

**Errors:**
- If you have no active entry on this issue, the bot replies with an error

## How commands are parsed

- Commands must start at the beginning of a line
- Only the first command in a comment is processed
- Text before or after the command line is ignored
- The bot only responds to newly created comments (not edits)

```
Here is some context about what I did.

/log 2h Implemented the feature

This text is ignored by Billbird.
```

## Error messages

When a command fails, Billbird posts a comment explaining what went wrong:

> **Billbird error:** missing duration: use /log \<duration\> (e.g. /log 2h, /log 30m, /log 1h30m)

> **Billbird error:** No active time entry found on this issue to correct.
