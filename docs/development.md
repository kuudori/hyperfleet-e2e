# Writing E2E Tests

This guide explains how to write E2E tests for HyperFleet.

> **Before adding a test here**, check the [test placement strategy](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/e2e-testing/test-placement-strategy.md) — it may belong in the API repo's unit or integration tests instead.

## Before You Start

### 1. Understand the Test Structure

Tests are organized by resource type:

```text
e2e/
├── e2e.go              # Test suite registration
├── cluster/
│   └── creation.go     # Cluster lifecycle tests
└── nodepool/
    └── creation.go     # NodePool lifecycle tests
```

### 2. Read Existing Tests

Start by reading existing tests to understand the patterns:

- [`e2e/cluster/creation.go`](../e2e/cluster/creation.go) - Cluster creation example
- [`e2e/nodepool/creation.go`](../e2e/nodepool/creation.go) - NodePool creation example

### 3. Prepare Test Data

Test payloads are stored in `testdata/payloads/`:

```
testdata/payloads/
├── clusters/
│   └── cluster-request.json        # resource cluster payload
└── nodepools/
    └── nodepool-request.json        # resource nodepool payload
```

#### Payload Templates

Payload files support Go template syntax for dynamic values. This prevents naming conflicts when running tests multiple times in long-running environments.

**Example** (`testdata/payloads/clusters/cluster-request.json`):

```json
{
  "kind": "Cluster",
  "name": "hp-cluster-{{.Random}}",
  "labels": {
    "environment": "production",
    "created-at": "{{.Timestamp}}"
  },
  "spec": { ... }
}
```

Each time the payload is loaded, template variables are replaced with fresh values, ensuring unique resource names. See `pkg/client/payload.go` for available template variables.

## Test File Format

### File Naming Convention

- **File extension**: Use `.go` (NOT `_test.go`)
- **File name**: Descriptive, e.g., `creation.go`, `lifecycle.go`
- **Location**: Under `e2e/{resource-type}/`

### Basic Test Structure

```go
package cluster

import (
    "context"

    "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var testName = "[Suite: cluster][baseline] Create Cluster via API"

var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier0),
    func() {
        var h *helper.Helper
        var clusterID string

        ginkgo.BeforeEach(func() {
            h = helper.New()
        })

        ginkgo.It("should create cluster successfully", func(ctx context.Context) {
            ginkgo.By("submitting cluster creation request")
            cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
            Expect(err).NotTo(HaveOccurred())
            clusterID = *cluster.Id
            ginkgo.DeferCleanup(func(ctx context.Context) {
                if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
                    ginkgo.GinkgoWriter.Printf("Warning: cleanup failed: %v\n", err)
                }
            })

            ginkgo.By("waiting for cluster to become Reconciled")
            Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
                Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
        })
    },
)
```

## Required Elements

### 1. Test Name

```go
var lifecycleTestName = "[Suite: cluster][baseline] Full Cluster Creation Flow"
```

- Format: `[Suite: component][category] Description`
- Suite represents the HyperFleet component being tested (cluster, nodepool, adapter)
- Category describes the test type: `baseline`, `update`, `delete`, `concurrent`, `negative`
- Use clear, descriptive names

### 2. Labels

All tests must use labels for categorization. See `pkg/labels/labels.go` for complete definitions.

**Required labels (1)**:

- **Severity**: `Tier0` | `Tier1` | `Tier2`

**Optional labels**:

- **Scenario**: `Negative` | `Performance`
- **Functionality**: `Upgrade`
- **Constraint**: `Disruptive` | `Slow`

**Example**:

```go
import "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"

var testName = "[Suite: cluster][baseline] Full Cluster Creation Flow"
var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier0),
    func() { ... }
)
```

**Example with optional labels**:

```go
// Negative test case with slow execution
var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier1, labels.Negative, labels.Slow),
    func() { ... }
)
```

### 3. BeforeEach Setup

```go
ginkgo.BeforeEach(func() {
    h = helper.New()
})
```

- Create Helper instance (automatically loads configuration)
- Initialize test context

### 4. Test Steps with ginkgo.By

```go
ginkgo.By("submitting cluster creation request")
// ... perform action

ginkgo.By("waiting for cluster to become Reconciled")
// ... wait for condition

ginkgo.By("verifying adapter conditions")
// ... verify conditions
```

- Use `ginkgo.By()` to mark major test steps
- Makes test output readable
- **DO NOT** use `ginkgo.By()` inside `Eventually` closures

### 5. Resource Cleanup

Prefer `ginkgo.DeferCleanup` inline right after resource creation:

```go
clusterID, err := h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
Expect(err).NotTo(HaveOccurred())
ginkgo.DeferCleanup(func(ctx context.Context) {
    if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
        ginkgo.GinkgoWriter.Printf("Warning: cleanup failed: %v\n", err)
    }
})
```

- Register cleanup inline right after creating the resource
- `DeferCleanup` runs in LIFO order and is scoped to the current node
- Guard against empty IDs to avoid unnecessary cleanup calls
- Log cleanup failures as warnings

## Writing Assertions

### Use Gomega Matchers

```go
// Basic assertions
Expect(err).NotTo(HaveOccurred())
Expect(cluster.Id).NotTo(BeNil())
Expect(cluster.Generation).To(Equal(int32(1)))

// Async: use pollers + custom matchers (preferred)
Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

// Async: use func(g Gomega) for complex one-off assertions
Eventually(func(g Gomega) {
    statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
    g.Expect(err).NotTo(HaveOccurred())
    // multi-field validation...
}, timeout, h.Cfg.Polling.Interval).Should(Succeed())
```

**Important**: Inside `Eventually` closures, use `g.Expect()` instead of `Expect()`

## Using Pollers and Matchers

The framework uses **pollers** (functions that fetch current state) and **custom matchers** (reusable Gomega assertions) to compose async checks. This avoids a combinatorial explosion of `WaitFor*` helper functions.

### Wait for Resource Condition

```go
// Cluster
Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

// NodePool (same matcher, different poller)
Eventually(h.PollNodePool(ctx, clusterID, npID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
```

### Wait for Adapter Conditions

```go
// All adapters finalized
Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersWithCondition(h.Cfg.Adapters.Cluster, client.ConditionTypeFinalized, client.AdapterConditionStatusTrue))

// All adapters at a specific generation with Applied+Available+Health=True
Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, expectedGen))
```

### Wait for Hard-Delete

```go
Eventually(h.PollClusterHTTPStatus(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(Equal(http.StatusNotFound))
```

### Check Conditions Synchronously

```go
hasReconciled := h.HasResourceCondition(cluster.Status.Conditions, client.ConditionTypeReconciled, client.ResourceConditionStatusTrue)
Expect(hasReconciled).To(BeTrue())

hasApplied := h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeApplied, client.AdapterConditionStatusTrue)
Expect(hasApplied).To(BeTrue())
```

Available pollers: see `pkg/helper/pollers.go`. Available matchers: see `pkg/helper/matchers.go`.

## Best Practices

### DO ✅

- Use descriptive test names and labels
- Mark major steps with `ginkgo.By()`
- Use `Eventually` for async operations
- Clean up resources in `AfterEach`
- Use timeout values from config
- Store resource IDs for cleanup
- Use pollers + custom matchers for async waits (see `pkg/helper/pollers.go`, `pkg/helper/matchers.go`)
- Ensure tests are parallel-safe (see [Parallel Safety](#parallel-safety) section)

### DON'T ❌

- Don't use `_test.go` suffix (use `.go`)
- Don't use `ginkgo.By()` inside `Eventually` closures
- Don't hardcode timeouts (use config values)
- Don't skip cleanup
- Don't create `WaitFor*` wrapper functions that hide `Eventually` - use pollers + matchers instead
- Don't use fixed resource names or shared mutable state across specs

## Parallel Safety

The suite is designed to support Ginkgo multi-process parallelism (`--procs=N`). All new tests MUST be parallel-safe unless explicitly marked `ginkgo.Serial`.

### Rules for parallel-safe tests

- **Unique resource names.** Use payload templates (`.Random`, `.UUID`) for all API resources. Never use fixed names.
- **Per-spec cleanup.** Register `ginkgo.DeferCleanup` right after resource creation. Never rely on `AfterSuite` as the primary cleanup path.
- **No shared mutable state.** Each spec gets its own `helper.New()` instance. Never store state in package-level variables that other specs read.
- **Temp adapter names must be unique per spec.** Two specs deploying the same adapter name would race on one Helm release.
- **Never use `os.Setenv` for values that vary per spec.** Pass values via Helm `--set` or struct fields instead.

### When to use `ginkgo.Serial`

Add `ginkgo.Serial` to the `Describe` decorator (not inside an `It`) and include a comment explaining why:

```go
var _ = ginkgo.Describe("...",
    ginkgo.Serial, // Serial: scales down sentinel replicas, blocking all concurrent specs
    ginkgo.Label(labels.Tier1, labels.Disruptive),
    func() { ... },
)
```

Valid reasons to mark a spec `Serial`:

- **Mutates shared infrastructure** (e.g., scales deployments, deletes shared resources)
- **Deploys temporary adapters** that subscribe to all events, causing cross-talk with concurrent specs

Specs already marked `Serial`: sentinel scale-down, force-delete, stuck-deletion, crash-recovery, maestro-unavailability, adapter-failover, adapter-failure, maestro negative scenarios.

Performance specs (`labels.Performance`) are a separate category - they carry no tier label and run in their own dedicated CI job (`--label-filter="perf"`) on a quiet system. They are not marked `Serial` because they never run alongside functional tests.

## Adding New Tests

### 1. Create Test File

```bash
# For cluster tests
touch e2e/cluster/my-new-test.go

# For nodepool tests
touch e2e/nodepool/my-new-test.go
```

### 2. Follow the Template

Copy from existing tests and modify:

- Change test name and ID
- Update labels
- Implement test logic
- Add cleanup

### 3. Register Test (Automatic)

Tests are automatically registered via the package import in `e2e/e2e.go`:

```go
package e2e

import (
    _ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/adapter"
    _ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/cluster"
    _ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/nodepool"
)
```

No need to manually register tests.

## Common Patterns

### Create Resource from Payload

```go
cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
Expect(err).NotTo(HaveOccurred())
```

### Wait for Condition

```go
Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
```

### Wait for All Adapters at Generation

```go
Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, expectedGen))
```

### Verify Adapter Conditions Synchronously

```go
statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
Expect(err).NotTo(HaveOccurred())

for _, adapter := range statuses.Items {
    Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeApplied, client.AdapterConditionStatusTrue)).To(BeTrue(),
        "adapter %s should have Applied=True", adapter.Adapter)
    Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeAvailable, client.AdapterConditionStatusTrue)).To(BeTrue(),
        "adapter %s should have Available=True", adapter.Adapter)
}
```

## Validating New E2E Tests

After writing your test, validate it works properly:

### 1. Set Up Your Development Environment

You need a running HyperFleet environment before running tests. See the [Setup Guide](setup.md) for complete instructions:

- **Kind (local):** Fast setup for local testing (recommended for development)
- **GCP:** Cloud environment for more realistic testing

The environment setup will configure required environment variables:
- `HYPERFLEET_API_URL`
- `MAESTRO_URL`
- `NAMESPACE`
- source `env/env.local` if required

### 2. Run Your Test

```bash
# Run your specific test in parallel (default 4 procs)
make e2e GINKGO_FOCUS="Your Test Description"

# Run by suite
make e2e GINKGO_FOCUS="\[Suite: cluster\]"

# Run single-process to isolate parallelism issues
make e2e PROCS=1 GINKGO_FOCUS="Your Test Description"
```

### 4. Run Pre-Commit Checks

Before committing, ensure your code passes all checks:

```bash
# Run all checks (format, lint, unit tests)
make check
```

### 5. Verify Test Behavior

Ensure your test:
- ✅ Creates resources successfully
- ✅ Waits for expected conditions
- ✅ Cleans up resources (check manually if needed)
- ✅ Passes consistently (run multiple times)
- ✅ Fails appropriately when conditions aren't met

### 6. Check Test Output

Review the test output for:
- Clear step descriptions (via `ginkgo.By()`)
- Appropriate timeout values
- Proper error messages on failure

## Next Steps

- **Architecture**: Understand the framework design in [Architecture](architecture.md)
- **Configuration**: See detailed comments in `configs/config.yaml`
- **Debug Tests**: Learn debugging techniques in [Debugging Guide](debugging.md)
- **Runbook**: Step-by-step operational guide in [Runbook](runbook.md)
