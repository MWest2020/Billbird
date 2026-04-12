# Contributing

Billbird is open source and contributions are welcome.

## Development setup

### Prerequisites

- Go 1.25+
- Docker and Docker Compose (for PostgreSQL)
- A GitHub App for testing (see [setup.md](setup.md))

### Get started

```bash
git clone https://github.com/mwesterweel/billbird.git
cd billbird

# Start PostgreSQL
docker compose up db -d

# Set environment variables
export DATABASE_URL="postgres://billbird:billbird@localhost:5432/billbird?sslmode=disable"
export GITHUB_APP_ID="your-app-id"
export GITHUB_PRIVATE_KEY="$(cat path/to/private-key.pem)"
export GITHUB_WEBHOOK_SECRET="your-secret"

# Run the application
go run ./cmd/billbird

# Or run tests
go test ./...
```

### Webhook testing

For local webhook testing, use [smee.io](https://smee.io) to forward GitHub webhooks to your machine:

```bash
# Install smee client
npm install -g smee-client

# Create a channel at https://smee.io and forward to localhost
smee -u https://smee.io/your-channel-id -t http://localhost:8080/webhook
```

Set your GitHub App's webhook URL to the smee.io channel URL.

## Project structure

```
cmd/billbird/           main entrypoint
internal/
  api/                  REST API handlers
  admin/                HTMX admin panel
  auth/                 OAuth and sessions
  client/               Client attribution
  commands/             Slash command parser
  config/               Environment config
  cycletime/            Cycle time tracking
  db/                   Database layer
  github/               GitHub API client
  timeentry/            Time entry logic
  webhook/              Webhook handler
migrations/             SQL migration files
templates/              HTML templates
docs/                   Documentation
```

## Running tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/commands/ -v

# With race detection
go test -race ./...
```

## Adding a migration

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate). To add a new migration:

```bash
# Create migration files
migrate create -ext sql -dir migrations -seq description_of_change
```

This creates two files: `NNNNNN_description_of_change.up.sql` and `NNNNNN_description_of_change.down.sql`. Write the forward migration in the `.up.sql` file and the rollback in the `.down.sql` file.

Migrations run automatically on application startup.

## Code style

- Standard Go formatting (`gofmt`)
- No ORM --- raw SQL with `pgx`
- No web framework --- standard library `net/http`
- Keep dependencies minimal
- Write tests for parser logic and business rules

## Architecture principles

- **API-first**: Every feature must be accessible through the REST API. The admin panel is a thin consumer.
- **No physical deletes**: All state changes through status fields and correction chains.
- **UTC everywhere**: Never store local time without offset.
- **Boring and auditable**: Prefer well-understood approaches over clever ones.
