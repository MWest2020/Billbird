## Why

Software teams that use GitHub for issue tracking are forced to context-switch to separate tools for time registration. This creates friction, reduces compliance, and disconnects time data from the work it describes. Billbird eliminates this by embedding time tracking directly into GitHub issue comments using slash commands, making honest time registration the path of least resistance.

## What Changes

- Introduce a GitHub App that listens for issue comment webhooks and responds to `/log` and `/correct` slash commands
- Store time entries in Postgres, linked to GitHub users, issues, repositories, and optionally clients
- Support non-destructive correction chains — corrections supersede previous entries without deleting them
- Enable client attribution through GitHub label-to-client mappings (e.g. label `client:amsterdam` maps to a client record)
- Passively track cycle time by observing GitHub Project board column transitions and PR merge events
- Provide a minimal server-side rendered admin panel (HTMX) with GitHub OAuth for viewing hours, managing client mappings, and exporting CSV
- Package for self-hosting via Docker Compose (dev/small deployments) and Helm chart (Kubernetes production)

## Capabilities

### New Capabilities
- `webhook-ingestion`: GitHub App webhook receiver that handles issue comment, project card, and pull request events
- `time-logging`: Slash command parsing (`/log`, `/correct`) and time entry creation with correction chain semantics
- `client-attribution`: Label-to-client mapping system that automatically attributes logged hours to clients based on issue labels
- `cycle-time-tracking`: Passive start/stop timestamp recording from project board movements and PR merges
- `admin-panel`: Server-side rendered admin UI with GitHub OAuth, hours overview, correction history, client management, and CSV export
- `data-model`: Postgres schema covering time entries, corrections, clients, label mappings, users, and repositories
- `deployment`: Docker Compose and Helm chart configurations for self-hosted deployment

### Modified Capabilities
<!-- None — greenfield project -->

## Impact

- **New codebase**: Entire application is net-new
- **External dependencies**: Postgres, GitHub App registration (webhook secret + private key)
- **APIs**: GitHub Webhooks API (incoming), GitHub REST API (posting confirmation comments, reading labels/project boards)
- **Auth**: GitHub OAuth for admin panel access
- **Data**: All time data is owned by the deploying team — no external SaaS dependencies
