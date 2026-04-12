# Billbird

Time tracking for teams that live in GitHub.

Developers log hours with slash commands in issue comments. Billbird stores the entries, attributes them to clients, and gets out of the way.

## The simplest use case

A developer finishes work on an issue. They comment:

```
/log 2h
```

Billbird's bot replies:

> Logged 2h for @developer (entry #1)

That's it. The time is recorded, linked to the issue, and visible to admins. No app to open, no tab to switch to, no form to fill out.

Need to fix a mistake? Comment `/correct 3h`. Need to remove it entirely? Comment `/delete`. Every change is tracked, nothing is ever lost.

## Commands

| Command | Example | What it does |
|---------|---------|--------------|
| `/log <duration> [description]` | `/log 1h30m Code review` | Log time on the current issue |
| `/correct <duration> [description]` | `/correct 2h` | Replace your last entry on this issue |
| `/delete` | `/delete` | Remove your last entry on this issue |

Durations support hours (`2h`), minutes (`45m`), or both (`1h30m`).

## How it works

Billbird is a [GitHub App](https://docs.github.com/en/apps) that listens to issue comment webhooks. When it sees a slash command, it:

1. Parses the command and duration
2. Checks the issue's labels to auto-attribute a client (if configured)
3. Stores the time entry in Postgres
4. Posts a confirmation comment on the issue

Corrections create a new entry that supersedes the old one. Deletes mark entries as deleted. Nothing is ever physically removed from the database --- the issue thread is the audit log.

## Quick start

```bash
# Clone and start
git clone https://github.com/mwesterweel/billbird.git
cd billbird
cp env.example .env  # fill in your GitHub App credentials
docker compose up
```

See [docs/setup.md](docs/setup.md) for GitHub App registration and configuration.

## Documentation

- [Setup guide](docs/setup.md) --- GitHub App registration, configuration, deployment
- [Commands reference](docs/commands.md) --- Slash command syntax and behavior
- [Client attribution](docs/client-attribution.md) --- Mapping GitHub labels to clients
- [Corrections and deletions](docs/corrections.md) --- How the correction chain works
- [Architecture](docs/architecture.md) --- System design and API-first approach
- [Configuration reference](docs/configuration.md) --- All environment variables
- [Self-hosting](docs/self-hosting.md) --- Docker Compose and Kubernetes deployment
- [Contributing](docs/contributing.md) --- Development setup and guidelines

## License

[MIT](LICENSE)
