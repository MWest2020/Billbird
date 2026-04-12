## ADDED Requirements

### Requirement: GitHub App webhook endpoint
The system SHALL expose an HTTP POST endpoint that receives GitHub webhook payloads. The endpoint SHALL verify the webhook signature using the configured webhook secret (HMAC-SHA256) before processing any event. Invalid signatures SHALL be rejected with HTTP 401.

#### Scenario: Valid webhook received
- **WHEN** GitHub sends a POST request with a valid `X-Hub-Signature-256` header
- **THEN** the system accepts the payload and returns HTTP 200

#### Scenario: Invalid webhook signature
- **WHEN** a POST request arrives with an invalid or missing `X-Hub-Signature-256` header
- **THEN** the system rejects the request with HTTP 401 and does not process the event

### Requirement: Event routing
The system SHALL route incoming webhook events to the appropriate handler based on the `X-GitHub-Event` header. The system SHALL handle the following event types: `issue_comment`, `project_card` (or `projects_v2_item` for Projects V2), and `pull_request`. Unrecognized event types SHALL be acknowledged with HTTP 200 and ignored.

#### Scenario: Issue comment event routed
- **WHEN** a webhook arrives with `X-GitHub-Event: issue_comment` and action `created`
- **THEN** the system routes the payload to the slash command parser

#### Scenario: Project card event routed
- **WHEN** a webhook arrives with `X-GitHub-Event: projects_v2_item`
- **THEN** the system routes the payload to the cycle time tracker

#### Scenario: Pull request event routed
- **WHEN** a webhook arrives with `X-GitHub-Event: pull_request` and action `closed` with `merged: true`
- **THEN** the system routes the payload to the cycle time tracker

#### Scenario: Unknown event type
- **WHEN** a webhook arrives with an unrecognized `X-GitHub-Event` header value
- **THEN** the system returns HTTP 200 and takes no further action

### Requirement: Idempotent event processing
The system SHALL track processed webhook delivery IDs (`X-GitHub-Delivery` header) and skip duplicate deliveries. This prevents double-processing when GitHub retries webhook delivery.

#### Scenario: Duplicate webhook delivery
- **WHEN** a webhook arrives with an `X-GitHub-Delivery` ID that has already been processed
- **THEN** the system returns HTTP 200 and does not create duplicate records

#### Scenario: First-time webhook delivery
- **WHEN** a webhook arrives with a new `X-GitHub-Delivery` ID
- **THEN** the system processes the event normally and records the delivery ID
