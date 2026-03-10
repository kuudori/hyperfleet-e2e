# Test Data

This directory contains test data payloads used by E2E tests.

## Directory Structure

```text
testdata/
└── payloads/
    ├── clusters/       # Cluster creation payloads
    └── nodepools/      # NodePool creation payloads
```

## Cluster Payloads

### Valid Payloads

| File                   | Purpose | Use Case |
|------------------------|---------|----------|
| `cluster-request.json` | Standard cluster | General testing, cluster lifecycle |

## NodePool Payloads

### Valid Payloads

| File                    | Purpose | Use Case |
|-------------------------|---------|----------|
| `nodepool-request.json` | Standard compute nodepool | General-purpose testing |

## Usage in Tests

### Loading from File

```go
// Create cluster from payload
cluster, err := h.Client.CreateClusterFromPayload(ctx, "testdata/payloads/clusters/cluster-request.json")

// Create nodepool from payload
nodepool, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, "testdata/payloads/nodepools/nodepool-request.json")
```

### Payload Templates

Payloads support Go template syntax for dynamic values. For example:

```json
{
  "kind": "Cluster",
  "name": "hp-cluster-{{.Random}}",
  "labels": {
    "created-at": "{{.Timestamp}}"
  }
}
```

Template variables (e.g., `{{.Random}}`, `{{.UUID}}`, `{{.Timestamp}}`) are automatically replaced when the payload is loaded, ensuring unique resource names for each test run. See [`pkg/client/payload.go`](../pkg/client/payload.go) for the complete list of available variables.

## Payload Naming Conventions

- **Resource type prefix**: `cluster-request`, `nodepool-request`
- **Optional variant suffix**: `-variant` for specialized payloads (e.g., `-gpu`, `-minimal`)
- **Lowercase with hyphens**: `cluster-request.json`, `cluster-request-gpu.json` (future), not `CLUSTER-REQUEST.json`

## Adding New Payloads

When adding new payloads:

1. **Follow naming convention**: `{platform}.json` or `{platform}_{variant}.json`
2. **Add to appropriate directory**: `clusters/` or `nodepools/`
3. **Update this README**: Add entry to relevant table
4. **Validate JSON**: Ensure valid JSON syntax
5. **Document purpose**: Clear description of use case

## Maintenance Notes

- **Keep payloads minimal**: Only include necessary fields
- **Use realistic values**: Especially for production-like tests
- **Sync with API spec**: Update when API schema changes

## Relationship to Test Implementation

Payloads map to test implementation files:

| Payload | Test File | Description |
|---------|-----------|-------------|
| `clusters/cluster-request.json` | `e2e/cluster/creation.go` | Cluster creation lifecycle |
| `nodepools/nodepool-request.json` | `e2e/nodepool/creation.go` | NodePool creation lifecycle |

## See Also

- [Test Cases](../test-design/testcases/) - Test specifications using these payloads
- [Client API](../pkg/client/) - API client methods for loading payloads
- [Helper API](../pkg/helper/) - Helper functions for test data
