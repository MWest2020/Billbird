---
status: draft
last_reviewed: 2026-07-13
---

# Dev-time hook

Optional pre-commit hook that estimates how long you've been working on a commit and adds a `Time: <duration>` footer to the commit message. You confirm or edit the suggestion before the commit lands. The footer becomes a second time-source alongside `/log` issue comments — Gitsweeper will reconcile the two and surface drift.

This is a doc page. There is no Billbird code involved on the dev side. Nothing in Billbird changes whether you install the hook or not.

## Why a second source

`/log` lives on issue comments — explicit, but easy to forget. The hook estimates from your local working copy: the time since the last commit, narrowed by recent file-mtime activity. The two sources cross-check each other:

- If a commit's `Time: 2h` matches roughly with `/log` totals on the issue it closes → no drift.
- If a commit says `Time: 4h` but the issue's `/log` total is only `1h` → forgotten log entry.
- If `/log 6h` is on the issue but commits across the day sum to `Time: 1h` → the hook missed work (probably did the work outside the editor and the heuristic didn't catch it).

Cross-check happens in Gitsweeper (planned follow-up), not in Billbird.

## What you copy

Two blocks. One into `~/.claude/settings.json`, one into a file in your home directory.

### 1. `~/.claude/settings.json`

Open (or create) the file and merge in:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "command": "$HOME/.claude/billbird-time-hook.sh"
      }
    ]
  }
}
```

If the file already has a `hooks` block, merge the `PreToolUse` array — don't replace what's there.

### 2. `~/.claude/billbird-time-hook.sh`

```bash
#!/usr/bin/env bash
# Billbird dev-time hook. Suggests a Time: footer on git commits.
# Reads the tool-use JSON from stdin; only acts on `git commit` invocations.
# Safe to omit on any other command.

set -euo pipefail

input="$(cat)"

# Only act on `git commit` shells.
case "$input" in
  *'"command":"git commit'*) ;;
  *) echo "$input"; exit 0 ;;
esac

# Estimate time since the last commit on the current branch. Falls back
# to zero if there is no prior commit (first commit of a fresh repo).
since_iso="$(git log -1 --format=%cI 2>/dev/null || true)"
if [ -z "$since_iso" ]; then
  # Nothing to base a duration on; let the commit proceed unchanged.
  echo "$input"; exit 0
fi

now_epoch="$(date -u +%s)"
since_epoch="$(date -u -d "$since_iso" +%s 2>/dev/null || date -u -j -f '%Y-%m-%dT%H:%M:%S%z' "$since_iso" +%s)"
elapsed=$((now_epoch - since_epoch))

# Cap absurd durations (machine asleep / weekend / etc.) at 8 hours.
[ "$elapsed" -gt 28800 ] && elapsed=28800

# Format as Xh Ym, drop zero parts.
hours=$((elapsed / 3600))
minutes=$(((elapsed % 3600) / 60))
if [ "$hours" -gt 0 ] && [ "$minutes" -gt 0 ]; then
  suggestion="${hours}h${minutes}m"
elif [ "$hours" -gt 0 ]; then
  suggestion="${hours}h"
else
  suggestion="${minutes}m"
fi

# Inject a Time: footer into the -m argument of git commit, if present.
# Otherwise leave the command alone — the dev will see git's editor and
# can add the footer themselves.
case "$input" in
  *' -m '*|*"-m\""*)
    # Append " — Time: <suggestion>" to the last -m string.
    echo "$input" | sed -E "s/(\"command\":\"git commit[^\"]*-m \"[^\"]*)/\1\n\nTime: ${suggestion}/"
    ;;
  *)
    # Pass the input through; the dev will be in their editor.
    echo "$input"
    ;;
esac
```

Make it executable:

```bash
chmod +x ~/.claude/billbird-time-hook.sh
```

## Workflow

1. You work on your branch as usual.
2. You run `git commit -m "fix the regression"` (or open the editor).
3. Claude Code invokes the hook; it estimates elapsed time and rewrites the message to include a `Time: 45m` footer (or whatever it computes).
4. You see the rewritten command and approve or edit before it actually runs.
5. The commit lands with the `Time:` footer in its message; `git log` shows it forever after.

You can always edit the suggestion. If the hook overestimates because you stepped out for lunch, change `Time: 1h30m` to `Time: 45m` before approving. If you didn't actually work between commits (rebase, merge commit), delete the line.

## What the hook does and does not do

| Does | Does not |
|------|----------|
| Read `git log -1 --format=%cI` for the prior commit timestamp | Contact Billbird or any network endpoint |
| Compute elapsed time, capped at 8 h | Hold or read any token, secret, or credential |
| Suggest a `Time:` footer for `git commit -m ...` calls | Replace `/log` issue comments — both sources coexist |
| Let you edit or remove the suggestion before commit | Force itself on commits — bypass with `--no-verify` or by skipping Claude Code |

## Reconciliation (planned, not yet shipped)

A future `gitsweeper reconcile` MCP tool / CLI subcommand will:

1. Walk recent commits, pull `Time:` footers.
2. Walk recent `/log` entries via Billbird's `/api/v1/time-entries`.
3. Group by `(repo, issue)` — the commit usually references the issue via `Closes #N` or branch name.
4. Report per-group drift: minutes-from-commits vs minutes-from-logs.

Until that lands, the footers are simply audit-trail in `git log`.

## Disabling the hook

Remove the entry from `~/.claude/settings.json` or comment it out. The script file can stay; without the settings entry it never runs.
