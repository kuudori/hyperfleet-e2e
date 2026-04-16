# Feature: Nodepool Update Lifecycle (PATCH)

## Table of Contents

1. [Nodepool update via PATCH triggers reconciliation and reaches Reconciled](#test-title-nodepool-update-via-patch-triggers-reconciliation-and-reaches-reconciled)
2. [PATCH with invalid payload is rejected without changing nodepool state](#test-title-patch-with-invalid-payload-is-rejected-without-changing-nodepool-state)

---

## Test Title: Nodepool update via PATCH triggers reconciliation and reaches Reconciled

### Description

This test validates the nodepool update lifecycle. It verifies that when a PATCH request modifies a nodepool's spec, the nodepool's `generation` is incremented independently of the parent cluster, nodepool adapters reconcile to the new generation, and the nodepool reaches `Reconciled=True`. The parent cluster must remain unaffected.

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

#### Step 1: Create a cluster and nodepool, wait for Ready state

**Action:**
- Create a cluster and nodepool:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for both to reach Ready state

**Expected Result:**
- Cluster and nodepool reach `Ready` condition `status: "True"`
- Both at `generation: 1`, `Reconciled: True`

#### Step 2: Send PATCH request to update the nodepool spec

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"updated-key": "new-value"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- Nodepool `generation` incremented from 1 to 2

#### Step 3: Verify nodepool adapters reconcile to the new generation

**Action:**
- Poll nodepool adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- All nodepool adapters report `observed_generation: 2`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`

#### Step 4: Verify nodepool reaches Reconciled=True at new generation

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool `Reconciled` condition `status: "True"` with `observed_generation: 2`
- Nodepool `Ready` condition `status: "True"`

#### Step 5: Verify parent cluster is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `generation` remains at 1 (unchanged)
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 1`
- Cluster `Ready` condition `status: "True"`

#### Step 6: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster, nodepool, and all associated resources are cleaned up

---

## Test Title: PATCH with invalid payload is rejected without changing nodepool state

### Description

This test validates that the API rejects malformed or constraint-violating PATCH requests on a nodepool with an HTTP 4xx response, does not increment `generation`, and does not trigger reconciliation. It complements the happy-path update test by covering the negative API contract for nodepool spec mutations.

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

#### Step 1: Create a cluster and nodepool, wait for Ready state

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for both to reach Ready state

**Expected Result:**
- Nodepool at `generation: 1`, `Reconciled: True` with `observed_generation: 1`

#### Step 2: Capture baseline state and `last_report_time` per adapter

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  | jq '{id, cluster_id, generation, conditions, labels, spec}'
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses \
  | jq '[.items[] | {adapter, observed_generation, last_report_time}]'
```

**Expected Result:**
- Baseline captured for comparison in Step 4

#### Step 3: Attempt PATCH with invalid payloads

For each of the following cases, submit a PATCH and verify it is rejected without any state change:

**Case A: Malformed JSON**
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"replicas": 3'
```

**Case B: Wrong type on a typed spec field (e.g., `replicas` as a string)**
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"replicas": "not-a-number"}'
```

**Case C: Attempt to mutate an immutable/server-controlled field (e.g., `id`, `generation`, `cluster_id`)**
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"id": "nodepool-other", "generation": 99, "cluster_id": "cluster-other"}'
```

**Expected Result (for every case):**
- Response is HTTP 400 (Bad Request) or 422 (Unprocessable Entity) — no 5xx
- Response body contains a structured validation error message identifying the offending field
- For Case C, the server either rejects the request or silently ignores the read-only fields (both are acceptable, but read-only fields must remain unchanged)

#### Step 4: Verify nodepool state is unchanged

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  | jq '{id, cluster_id, generation, conditions, labels, spec}'
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses \
  | jq '[.items[] | {adapter, observed_generation, last_report_time}]'
```

**Expected Result:**
- `generation` is still 1 (no invalid PATCH incremented it)
- `id` and `cluster_id` are unchanged from baseline
- Nodepool `Reconciled` condition remains `status: "True"` with `observed_generation: 1`
- All adapter `last_report_time` values are unchanged vs baseline (no spurious reconciliation was triggered)

#### Step 5: Verify parent cluster is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `generation` remains at 1
- Cluster `Reconciled: True` with `observed_generation: 1`

#### Step 6: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster, nodepool, and all associated resources are cleaned up

---
