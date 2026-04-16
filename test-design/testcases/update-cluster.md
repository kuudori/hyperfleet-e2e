# Feature: Cluster Update Lifecycle (PATCH)

## Table of Contents

1. [Cluster update via PATCH triggers reconciliation and reaches Reconciled](#test-title-cluster-update-via-patch-triggers-reconciliation-and-reaches-reconciled)
2. [Adapter statuses transition during update reconciliation](#test-title-adapter-statuses-transition-during-update-reconciliation)
3. [Multiple rapid updates coalesce to latest generation](#test-title-multiple-rapid-updates-coalesce-to-latest-generation)
4. [PATCH with invalid payload is rejected without changing cluster state](#test-title-patch-with-invalid-payload-is-rejected-without-changing-cluster-state)

---

## Test Title: Cluster update via PATCH triggers reconciliation and reaches Reconciled

### Description

This test validates the cluster update lifecycle end-to-end. It verifies that when a PATCH request modifies a cluster's spec, the API increments the `generation`, Sentinel detects the generation change and publishes a reconciliation event, adapters reconcile to the new generation reporting updated `observed_generation`, and the cluster reaches `Reconciled=True` at the new generation. This confirms the complete update-reconciliation pipeline works correctly.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. PATCH endpoint is deployed and operational
5. Reconciled status aggregation is implemented

---

### Test Steps

#### Step 1: Create a cluster and wait for Ready state at generation 1

**Action:**
- Create a cluster and wait for Ready:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Ready` condition `status: "True"` and `Available` condition `status: "True"`
- Cluster `generation` equals 1
- `Reconciled` condition `status: "True"` with `observed_generation: 1`
- All required adapters report `observed_generation: 1`

#### Step 2: Send PATCH request to update the cluster spec

**Action:**
- Submit a PATCH request to modify the cluster:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"updated-key": "new-value"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- Response body shows `generation` incremented from 1 to 2
- The spec change is reflected in the response

#### Step 3: Verify adapters reconcile to the new generation

**Action:**
- Poll adapter statuses until all adapters report the new generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `observed_generation: 2`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`
- **Adapter condition metadata validation** (for each condition):
  - `reason`: Non-empty string
  - `message`: Non-empty string
  - `last_transition_time`: Valid RFC3339 timestamp
- **Adapter status metadata validation** (for each required adapter):
  - `last_report_time`: Updated to a timestamp after the PATCH request

#### Step 4: Verify cluster reaches Reconciled=True at new generation

**Action:**
- Retrieve the cluster status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 2`
- Cluster `Ready` condition `status: "True"`
- Cluster `Available` condition `status: "True"`
- `generation` equals 2

#### Step 5: Verify adapter statuses reflect the update

**Action:**
- Retrieve adapter statuses to confirm all adapters reconciled the update:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `observed_generation: 2`
- All adapters report `Applied: True` (confirming managed K8s resources were updated)
- This implicitly confirms K8s resources (e.g., namespace annotation `hyperfleet.io/generation`) reflect the update

#### Step 6: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: Adapter statuses transition during update reconciliation

### Description

This test validates the intermediate status transitions during update reconciliation. When a cluster spec is updated, there is a window where adapters have not yet reconciled to the new generation. During this window, `Reconciled` should be `False` (indicating stale adapter statuses relative to the new generation). This test captures and validates these intermediate states.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. PATCH endpoint is deployed and operational
5. Reconciled status aggregation is implemented

---

### Test Steps

#### Step 1: Create a cluster and wait for Ready and Reconciled at generation 1

**Action:**
- Create a cluster and wait for full convergence:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"` at `generation: 1`
- All adapters report `observed_generation: 1`

#### Step 2: Send PATCH request and immediately poll for intermediate state

**Action:**
- Send PATCH to trigger generation increment:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"trigger-reconcile": "true"}}'
```
- Immediately poll cluster status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Immediately poll adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Cluster `generation` is now 2
- **Intermediate state** (captured if polled before adapters reconcile):
  - `Reconciled` condition `status: "False"` (adapters have not yet reported at generation 2)
  - Some or all adapters still report `observed_generation: 1` (stale relative to generation 2)

**Note:** This intermediate state may be very brief depending on adapter reconciliation speed. The test should poll immediately after PATCH to maximize the chance of capturing it.

#### Step 3: Wait for full convergence and verify final state

**Action:**
- Continue polling until all adapters report the new generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- All adapters report `observed_generation: 2`
- Cluster `Reconciled` condition transitions to `status: "True"`
- Full state transition observed: `Reconciled: True (gen 1)` -> `Reconciled: False (gen 2 pending)` -> `Reconciled: True (gen 2)`

#### Step 4: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: Multiple rapid updates coalesce to latest generation

### Description

This test validates that when multiple PATCH requests are sent in rapid succession, the system handles generation increments correctly and adapters eventually reconcile to the final generation. Intermediate generations may be skipped by adapters (coalesced), which is expected behavior since adapters reconcile the latest desired state.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. PATCH endpoint is deployed and operational
5. Reconciled status aggregation is implemented

---

### Test Steps

#### Step 1: Create a cluster and wait for Ready at generation 1

**Action:**
- Create a cluster and wait for Ready:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Ready: True`, `Reconciled: True` at `generation: 1`

#### Step 2: Send three PATCH requests in rapid succession

**Action:**
- Send three updates without waiting for reconciliation between them:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update": "first"}}'
```
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update": "second"}}'
```
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update": "third"}}'
```

**Expected Result:**
- Each PATCH returns HTTP 200 with incrementing `generation` values: 2, 3, 4
- The final cluster state reflects the last update (`{"update": "third"}`)

#### Step 3: Wait for adapters to reconcile to the final generation

**Action:**
- Poll adapter statuses until all report the final generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `observed_generation: 4`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`
- Adapters may skip intermediate generations (2, 3) and reconcile directly to generation 4 -- this is acceptable and expected behavior

#### Step 4: Verify cluster reaches Reconciled=True at final generation

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `generation` equals 4
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 4`
- Cluster `Ready` condition `status: "True"`
- Cluster spec contains `{"update": "third"}` (the last applied value)

#### Step 5: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: PATCH with invalid payload is rejected without changing cluster state

### Description

This test validates that the API rejects malformed or constraint-violating PATCH requests on a cluster with an HTTP 4xx response, does not increment `generation`, and does not trigger reconciliation. It complements the happy-path update tests by locking down the negative API contract for cluster spec mutations (malformed JSON, type violations, read-only field mutation).

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. PATCH endpoint is deployed and operational with request validation enabled

---

### Test Steps

#### Step 1: Create a cluster and wait for Ready at generation 1

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for Ready and `Reconciled: True` with `observed_generation: 1`

**Expected Result:**
- Cluster reaches `Ready: True`, `Reconciled: True` at `generation: 1`

#### Step 2: Capture baseline state and per-adapter `last_report_time`

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  | jq '{id, generation, created_at, deleted_time, conditions, labels, spec}'
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses \
  | jq '[.items[] | {adapter, observed_generation, last_report_time}]'
```

**Expected Result:**
- Baseline captured for comparison in Step 4

#### Step 3: Attempt PATCH with invalid payloads

For each case, submit the PATCH and capture the response:

**Case A: Malformed JSON (truncated body, missing closing brace)**
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"k": "v"'
```

**Case B: Wrong type on a typed spec field**
Submit a PATCH that replaces a typed spec field with a value of the wrong type (e.g., provide a string where the schema expects an object or an array). Concrete field choice depends on the cluster OpenAPI schema -- pick any required typed subfield under `spec`.
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"release": "not-an-object"}}'
```

**Case C: Attempt to mutate an immutable/server-controlled field (e.g., `id`, `generation`, `created_at`, `deleted_time`)**
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"id": "different-id", "generation": 99, "created_at": "1970-01-01T00:00:00Z"}'
```

**Expected Result (for every case):**
- Response is HTTP 400 (Bad Request) or 422 (Unprocessable Entity) -- no 5xx
- Response body contains a structured validation error message identifying the offending field
- For Case C, the server either rejects the request or silently ignores the read-only fields (both are acceptable, but read-only fields must remain unchanged in Step 4)

#### Step 4: Verify cluster state is unchanged

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  | jq '{id, generation, created_at, deleted_time, conditions, labels, spec}'
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses \
  | jq '[.items[] | {adapter, observed_generation, last_report_time}]'
```

**Expected Result:**
- `generation` is still 1 (no invalid PATCH incremented it)
- `id`, `created_at`, and `deleted_time` are unchanged from baseline
- Cluster `Reconciled` condition remains `status: "True"` with `observed_generation: 1`
- Cluster `Ready` condition remains `status: "True"`
- All adapter `last_report_time` values are unchanged vs baseline (no spurious reconciliation was triggered)

#### Step 5: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---
