## ADDED Requirements

### Requirement: Docker Compose for local development
The system SHALL provide a Docker Compose configuration that starts the application and a Postgres database with a single `docker compose up` command. The configuration SHALL support hot-reload for development.

#### Scenario: Start local development environment
- **WHEN** a developer runs `docker compose up`
- **THEN** the application and Postgres start, the database schema is applied, and the app is accessible on a local port

### Requirement: Helm chart for Kubernetes deployment
The system SHALL provide a Helm chart for production deployment on Kubernetes. The chart SHALL support configurable values for: database connection, GitHub App credentials (via secrets), replica count, ingress configuration, and resource limits.

#### Scenario: Deploy to Kubernetes
- **WHEN** an operator runs `helm install billbird ./charts/billbird` with a values file
- **THEN** the application deploys with the configured Postgres connection, GitHub App credentials from Kubernetes secrets, and ingress routing

### Requirement: Database migrations
The system SHALL manage database schema changes through versioned migration files. Migrations SHALL run automatically on application startup (or as an init container in Kubernetes).

#### Scenario: Fresh deployment
- **WHEN** the application starts against an empty database
- **THEN** all migrations run in order, creating the full schema

#### Scenario: Upgrade deployment
- **WHEN** the application starts against a database with existing migrations
- **THEN** only new migrations run

### Requirement: Configuration via environment variables
The system SHALL be configured entirely through environment variables: database URL, GitHub App ID, GitHub App private key, GitHub webhook secret, OAuth client ID/secret, and admin organization name. No configuration files SHALL be required at runtime.

#### Scenario: Application starts with valid environment
- **WHEN** all required environment variables are set
- **THEN** the application starts successfully

#### Scenario: Missing required variable
- **WHEN** a required environment variable is missing
- **THEN** the application fails to start with a clear error message naming the missing variable

### Requirement: No external dependencies beyond Postgres and GitHub
The system SHALL NOT require any services beyond Postgres and a GitHub App registration. No Redis, no message queues, no external caches. This minimizes operational complexity for self-hosted deployments.

#### Scenario: Minimal infrastructure
- **WHEN** an operator has Postgres and a GitHub App configured
- **THEN** the system is fully functional without additional infrastructure
