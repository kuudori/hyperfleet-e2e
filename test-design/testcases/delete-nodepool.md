# Feature: Nodepool Deletion Lifecycle

## Table of Contents

1. [Nodepool deletion happy path -- soft-delete through hard-delete](#test-title-nodepool-deletion-happy-path----soft-delete-through-hard-delete)
2. [Nodepool deletion does not affect sibling nodepools](#test-title-nodepool-deletion-does-not-affect-sibling-nodepools)
3. [Re-DELETE on already-deleted nodepool is idempotent](#test-title-re-delete-on-already-deleted-nodepool-is-idempotent)
4. [DELETE non-existent nodepool returns 404](#test-title-delete-non-existent-nodepool-returns-404)
5. [PATCH to soft-deleted nodepool returns 409 Conflict](#test-title-patch-to-soft-deleted-nodepool-returns-409-conflict)

---

## Test Title: Nodepool deletion happy path -- soft-delete through hard-delete

### Description

This test validates the complete nodepool deletion lifecycle. It verifies that when a DELETE request is sent for a single nodepool, the API sets `deleted_time`, nodepool adapters clean up their managed resources and report `Finalized=True`, the nodepool reaches `Reconciled=True`, and hard-delete permanently removes the nodepool record. Critically, the parent cluster must remain unaffected by the nodepool deletion.

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
4. DELETE and hard-delete endpoints are deployed and operational
5. Reconciled status aggregation is implemented

---

### Test Steps

#### Step 1: Create a cluster and a nodepool, wait for Ready state

**Action:**
- Create a cluster:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Create a nodepool under the cluster:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for both cluster and nodepool to reach Ready state

**Expected Result:**
- Cluster `Ready` condition `status: "True"`
- Nodepool `Ready` condition `status: "True"`

#### Step 2: Send DELETE request for the nodepool only

**Action:**
- Submit a DELETE request for the nodepool (not the cluster):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted)
- Response body includes the nodepool with `deleted_time` set to a valid RFC3339 timestamp
- Nodepool `generation` is incremented

#### Step 3: Verify nodepool adapters report Finalized=True

**Action:**
- Poll nodepool adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- All nodepool adapters report `Finalized` condition `status: "True"`
- `Applied` condition transitions to `status: "False"` (managed resources deleted)
- `Available` condition transitions to `status: "False"`
- `observed_generation` matches the post-DELETE generation

#### Step 4: Verify nodepool reaches Reconciled=True and is hard-deleted

**Action:**
- Poll nodepool status, then attempt GET after hard-delete:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool `Reconciled` condition transitions to `status: "True"`
- After hard-delete: GET returns HTTP 404 (Not Found)

#### Step 5: Verify parent cluster is unaffected

**Action:**
- Retrieve the parent cluster:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster does NOT have `deleted_time` set
- Cluster `Ready` condition remains `status: "True"`
- Cluster `Available` condition remains `status: "True"`
- Cluster is fully operational and unaffected by the nodepool deletion

#### Step 6: Cleanup resources

**Action:**
- Delete the cluster (which cleans up remaining resources):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all remaining resources are cleaned up

---

## Test Title: Nodepool deletion does not affect sibling nodepools

### Description

This test validates isolation between sibling nodepools during deletion. When one nodepool is deleted, other nodepools under the same cluster must remain in their current state with no `deleted_time` set and no disruption to their adapter statuses.

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
4. DELETE endpoint is deployed and operational

---

### Test Steps

#### Step 1: Create a cluster with two nodepools and wait for Ready state

**Action:**
- Create a cluster and two nodepools (each call generates a unique name via `{{.Random}}` template):
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
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for all to reach Ready state

**Expected Result:**
- Cluster and both nodepools reach `Ready` condition `status: "True"`

#### Step 2: Delete one nodepool

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_1}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set on nodepool_1

#### Step 3: Verify sibling nodepool is unaffected

**Action:**
- Retrieve the sibling nodepool:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}
```
- Retrieve sibling nodepool adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}/statuses
```

**Expected Result:**
- Sibling nodepool does NOT have `deleted_time` set
- Sibling nodepool `Ready` condition remains `status: "True"`
- Sibling nodepool adapter statuses are unchanged (`Applied: True`, `Available: True`, `Health: True`)

#### Step 4: Verify parent cluster is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster does NOT have `deleted_time` set
- Cluster `Ready` condition remains `status: "True"`

#### Step 5: Cleanup resources

**Action:**
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster, remaining nodepool, and all associated resources are cleaned up

---

## Test Title: Re-DELETE on already-deleted nodepool is idempotent

### Description

This test validates that calling DELETE on a nodepool that has already been soft-deleted returns the same result without error. The `deleted_time` should remain unchanged from the first DELETE call, and the cascade uses a `WHERE deleted_time IS NULL` guard so repeat calls are safe no-ops.

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
4. DELETE endpoint is deployed and operational

---

### Test Steps

#### Step 1: Create a cluster and nodepool, wait for Ready state

**Action:**
- Create a cluster and nodepool, wait for Ready:
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

**Expected Result:**
- Cluster and nodepool reach Ready state

#### Step 2: Send first DELETE request

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- Record `{original_deleted_time}` and `{original_generation}`

#### Step 3: Send second DELETE request

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted)
- `deleted_time` is identical to `{original_deleted_time}`
- `generation` is identical to `{original_generation}` (not incremented again)

#### Step 4: Cleanup resources

**Action:**
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: DELETE non-existent nodepool returns 404

### Description

This test validates that sending a DELETE request for a nodepool ID that does not exist under a valid cluster returns HTTP 404 Not Found.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API is deployed and running successfully
3. A valid cluster exists (for the cluster_id path parameter)

---

### Test Steps

#### Step 1: Create a cluster (for valid cluster_id)

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster is created with a valid `cluster_id`

#### Step 2: Send DELETE request for a non-existent nodepool ID

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/non-existent-nodepool-id-12345
```

**Expected Result:**
- Response returns HTTP 404 (Not Found)
- Response body includes an error message indicating the nodepool was not found

#### Step 3: Cleanup resources

**Action:**
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: PATCH to soft-deleted nodepool returns 409 Conflict

### Description

This test validates that the API rejects mutation requests (PATCH) to nodepools that have been soft-deleted. Once a nodepool has `deleted_time` set, no spec modifications should be allowed to prevent new generation events from triggering reconciliation while deletion cleanup is in progress.

**Note:** Same mechanism as the cluster PATCH 409 test case — a PATCH on a tombstoned nodepool bumps `generation`, creating a mismatch that blocks hard-delete until adapters re-process at the new generation. The adapter won't recreate K8s resources (deletion check short-circuits apply), but the round-trip through Sentinel and adapter delays hard-delete. A 409 guard prevents this.

**Status note:** This test case requires the API to implement a mutation guard for tombstoned resources. Until then, PATCH will succeed on soft-deleted nodepools.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. DELETE and PATCH endpoints are deployed and operational
5. Mutation guard for tombstoned resources is implemented in the API

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
- Nodepool at `generation: 1`

#### Step 2: Send DELETE request to soft-delete the nodepool

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- Nodepool `generation` incremented to 2

#### Step 3: Attempt PATCH on the soft-deleted nodepool

**Action:**
- Send a PATCH request to modify the nodepool spec:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"updated-label": "should-not-work"}}'
```

**Expected Result:**
- Response returns HTTP 409 (Conflict)
- Response body includes an error message indicating the resource is pending deletion
- The nodepool's `generation` remains at 2 (not incremented)

#### Step 4: Verify nodepool state is unchanged

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool spec does not contain the attempted label change
- `generation` remains at 2
- `deleted_time` is still set (deletion not affected)

#### Step 5: Cleanup resources

**Action:**
- Delete the cluster (cleans up remaining resources):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---
