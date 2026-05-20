## ADDED Requirements

### Requirement: Issue API tokens for authenticated users
The admin panel SHALL allow any authenticated user (member of `ALLOWED_ORGS`) to create personal API tokens. Each created token SHALL be returned in plaintext exactly once, in the response body of the create call. The system SHALL NOT persist the plaintext.

#### Scenario: User creates a token
- **WHEN** an authenticated user submits the token creation form with a label "Manager-MCP"
- **THEN** the panel response shows the plaintext token once, the database stores only its hash and the label, and subsequent reloads of the token list never reveal the plaintext again

#### Scenario: Unauthenticated request to create a token
- **WHEN** an unauthenticated request hits the token-creation endpoint
- **THEN** the request is rejected with HTTP 401 and no token row is created

### Requirement: Bearer token authentication on `/api/v1/*`
The system SHALL accept `Authorization: Bearer <token>` headers on every `/api/v1/*` route. A valid token SHALL produce the same user context as the session-cookie auth path; both paths SHALL be interchangeable from the handler's perspective.

#### Scenario: Token grants API access
- **WHEN** a request to `GET /api/v1/time-entries` includes a valid bearer token issued for user @alice
- **THEN** the response is identical to the response @alice would receive via a session cookie

#### Scenario: Cookie still works
- **WHEN** a browser request includes only a valid session cookie and no Authorization header
- **THEN** the existing cookie auth path continues to grant access

#### Scenario: Invalid token rejected
- **WHEN** a request includes `Authorization: Bearer bb_invalidvalue`
- **THEN** the response is HTTP 401 and no handler-level work runs

### Requirement: Token hashing at rest
The system SHALL store only a one-way hash of every token's plaintext, using bcrypt with cost factor at least 12 (or argon2id with comparable parameters). The hash output SHALL be the only column that the application uses to verify a presented token; no plaintext or reversible derivation SHALL be persisted.

#### Scenario: Token verification uses hash compare
- **WHEN** a bearer token is presented
- **THEN** the system computes the hash and compares against the stored value; no plaintext column exists

#### Scenario: Database leak does not yield usable tokens
- **WHEN** a database backup is exfiltrated
- **THEN** the rows contain no value usable as a bearer token without an offline brute-force attack against bcrypt

### Requirement: Token revocation
Token owners SHALL be able to revoke their own tokens from the admin panel. Admins SHALL be able to revoke any token. A revoked token SHALL fail authentication immediately on the next request, with no cache-induced delay.

#### Scenario: Owner revokes a token
- **WHEN** @alice revokes her token labelled "Manager-MCP"
- **THEN** subsequent requests bearing that token receive HTTP 401

#### Scenario: Admin revokes another user's token
- **WHEN** an admin revokes a token belonging to @bob
- **THEN** subsequent requests bearing that token receive HTTP 401

### Requirement: Token authorisation re-checks org membership
Bearer token authentication SHALL re-check the owning user's membership in `ALLOWED_ORGS` against the GitHub API. The check MAY be cached for up to 5 minutes. A user removed from every allowed org SHALL lose token-based API access within the cache TTL.

#### Scenario: Removed user loses access
- **WHEN** @charlie's bearer token is used 6 minutes after @charlie is removed from the only allowed org
- **THEN** the request is rejected with HTTP 401 and the token is treated as not authorised

#### Scenario: Fresh membership check within cache window
- **WHEN** @charlie's token is used within 5 minutes of the previous successful membership check
- **THEN** the cached membership decision is used and no GitHub API call is made

### Requirement: Token visibility in the admin panel
The admin panel SHALL list tokens for the current user (and for any user, for admins) with: label, creation timestamp, last-used timestamp, the non-secret prefix of the token (first 8 base64 characters), and the revoked flag. The full plaintext SHALL NEVER appear in the listing.

#### Scenario: Token list excludes plaintext
- **WHEN** a user views their token list
- **THEN** each row displays metadata and the 8-character prefix, never the full token value

#### Scenario: Last-used timestamp updates on use
- **WHEN** a token successfully authenticates a request
- **THEN** the token's `last_used_at` is updated (no more often than once per minute to avoid write amplification)

### Requirement: Audit visibility of token-driven writes
Every write through `/api/v1/*` made with a bearer token SHALL record the token's ID alongside the user ID in the affected row's audit fields (where audit fields exist, such as admin corrections). Token-driven reads SHALL NOT require audit row writes.

#### Scenario: Admin correction made via token
- **WHEN** a write happens via a bearer token (e.g. an admin correction through MCP)
- **THEN** the correction row records both the user ID and the token ID that performed it
