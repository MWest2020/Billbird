# Client attribution

Billbird can automatically attribute logged hours to clients based on GitHub issue labels. This keeps the billing workflow entirely inside GitHub --- developers don't need to think about which client they're working for.

## How it works

1. An admin creates a **client** (e.g., "City of Amsterdam")
2. An admin creates a **label mapping** that connects a GitHub label to that client (e.g., label `client:amsterdam` maps to "City of Amsterdam")
3. When a developer logs time on an issue, Billbird checks the issue's labels
4. If a label matches a mapping, the time entry is attributed to that client

The developer sees no difference in their workflow. They comment `/log 2h` as usual, and the entry is silently attributed.

## Label mappings

A label mapping connects a GitHub label to a client record. Mappings can be:

- **Repository-specific**: Only applies to a specific repository
- **Global**: Applies to all repositories

### Precedence

When both a global and repository-specific mapping exist for the same label, the **repository-specific mapping takes precedence**.

### Multiple labels

If an issue has multiple labels that match different clients, Billbird uses the first match and includes a note in the confirmation comment about the ambiguity.

## Example

Given this setup:

| Label | Client | Repository |
|-------|--------|------------|
| `client:amsterdam` | City of Amsterdam | *(global)* |
| `client:amsterdam` | Amsterdam IT Dept | `org/internal-tools` |
| `client:rotterdam` | Port of Rotterdam | *(global)* |

Then:

- `/log 2h` on issue `org/website#42` with label `client:amsterdam` attributes to **City of Amsterdam** (global mapping)
- `/log 2h` on issue `org/internal-tools#10` with label `client:amsterdam` attributes to **Amsterdam IT Dept** (repo-specific wins)
- `/log 2h` on issue `org/website#43` with no client label has **no client attribution**

## Managing clients and mappings

Clients and label mappings are managed through the admin panel or the REST API.

### Admin panel

Navigate to the **Clients** page to create and manage clients. Navigate to **Label Mappings** to create and manage label-to-client mappings.

### REST API

```bash
# Create a client
POST /api/v1/clients
{"name": "City of Amsterdam"}

# Create a label mapping
POST /api/v1/label-mappings
{"label_pattern": "client:amsterdam", "client_id": 1}

# Create a repo-specific mapping
POST /api/v1/label-mappings
{"label_pattern": "client:amsterdam", "client_id": 2, "repository": "org/internal-tools"}
```

## Deactivating clients

When a client is deactivated, no new time entries will be attributed to them. Existing entries retain their attribution for historical accuracy.
