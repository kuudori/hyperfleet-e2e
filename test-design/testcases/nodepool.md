# Feature: Nodepools Resource Type Lifecycle Management

## Table of Contents

1. [Nodepools Resource Type - Workflow Validation](#test-title-nodepools-resource-type---workflow-validation)
2. [Nodepools Resource Type - K8s Resource Check Aligned with Preinstalled NodePool Related Adapters Specified](#test-title-nodepools-resource-type---k8s-resource-check-aligned-with-preinstalled-nodepool-related-adapters-specified)

---

## Test Title: Nodepools Resource Type - Workflow Validation

### Description

This test validates that the workflow can work correctly for nodepools resource type. It verifies that when a nodepool resource is created via the HyperFleet API, the system correctly processes the resource through its lifecycle, required adapters (configured in the test config) execute successfully, and accurately reports status transitions back to the API. The test validates required adapters first to identify specific failures, then confirms the nodepool reaches the final Ready and Available state. This approach ensures the complete workflow of CLM can successfully handle nodepools resource type requests end-to-end.

---

| **Field** | **Value**     |
|-----------|---------------|
| **Pos/Neg** | Positive      |
| **Priority** | Tier0         |
| **Status** | Automated     |
| **Automation** | Automated     |
| **Version** | MVP           |
| **Created** | 2026-02-04    |
| **Updated** | 2026-03-02    |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. A cluster resource creation request has been submitted and its cluster_id is available
    - **Note**: Cluster does not need to be Ready before creating nodepool
    - **Cleanup**: Cluster resource cleanup should be handled in test suite teardown where cluster was created

---

### Test Steps

**Setup (BeforeEach):**
- Get or create test cluster (cluster_id is obtained)
- Submit a POST request to create a NodePool resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Response includes the created nodepool ID for use in test validations

#### Step 1: Verify initial status of nodepool
**Action:**
- Poll nodepool status for initial response
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- NodePool `Ready` condition `status: False`
- NodePool `Available` condition `status: False`

#### Step 2: Verify required adapter execution results

**Action:**
Poll adapter statuses until all required adapters report `Applied/Available/Health=True` or timeout:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- Response returns HTTP 200 (OK) status code
- All required adapters from config are present in the response:
  - `np-configmap`
- Each required adapter has all required condition types: `Applied`, `Available`, `Health`
- Each condition has `status: "True"` indicating successful execution
- **Adapter condition metadata validation** (for each condition in adapter.conditions):
  - `reason`: Non-empty string providing human-readable summary of the condition state
  - `message`: Non-empty string with detailed human-readable description
  - `last_transition_time`: Valid RFC3339 timestamp of the last status change
- **Adapter status metadata validation** (for each required adapter):
  - `created_time`: Valid RFC3339 timestamp when the adapter status was first created
  - `last_report_time`: Valid RFC3339 timestamp when the adapter last reported its status
  - `observed_generation`: Non-nil integer value equal to 1 for new creation requests

**Note:** Required adapters are configurable via:
- Config file: `configs/config.yaml` under `adapters.nodepool`
- Environment variable: `HYPERFLEET_ADAPTERS_NODEPOOL` (comma-separated list)

#### Step 3: Verify final nodepool state

**Action:**
- Wait for nodepool Ready condition to transition to True
- Retrieve final nodepool status information:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- NodePool `Ready` condition transitions from `status: False` to `status: True`
- Final nodepool conditions have `status: True` for both condition `{"type": "Ready"}` and `{"type": "Available"}`
- Validate that the observedGeneration for the Ready and Available conditions is 1 for a new creation request
- Validate adapter-specific conditions in nodepool status (Note: This check will be removed once these adapter-specific conditions are removed in the future):
  - Each required adapter should report its own condition type (e.g., `NpConfigmapSuccessful`) with `status: True`
- This confirms the nodepool has reached the desired end state

#### Step 4: Cleanup Resources (AfterEach)

**Action:**
- Wait for cluster Ready condition with timeout to prevent namespace deletion conflicts:
  - Poll the cluster status via API until Ready=True
  - If timeout occurs, log a warning and continue with best-effort cleanup
- Delete the cluster namespace (cascades to delete nodepool resources):
```bash
# Wait for cluster Ready with timeout (best-effort, pseudo-code)
# Poll: curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} until condition type=Ready, status=True
# If timeout: log "WARN:  The cluster did not reach Ready state before cleanup"

# Delete namespace regardless of Ready state
kubectl delete namespace {cluster_id}
```

**Expected Result:**
- Namespace deletion is attempted regardless of Ready state, with a warning logged if cluster is not Ready
- Namespace and all associated resources (including nodepools) are deleted (best-effort)
- Cleanup never hangs indefinitely

**Note:** This is a workaround cleanup method. Once HyperFleet API supports DELETE operations for "nodepools" and "clusters" resource type, this step should be replaced with:
```bash
# Delete nodepool
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
# Delete cluster
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---

## Test Title: Nodepools Resource Type - K8s Resource Check Aligned with Preinstalled NodePool Related Adapters Specified

### Description

This test verifies that the Kubernetes resources of different types (e.g., configmap) can be successfully created, aligned with the preinstalled adapters specified when submitting a nodepools resource request.

---

| **Field** | **Value**  |
|-----------|------------|
| **Pos/Neg** | Positive   |
| **Priority** | Tier0      |
| **Status** | Automated  |
| **Automation** | Automated  |
| **Version** | MVP        |
| **Created** | 2026-02-04 |
| **Updated** | 2026-03-05 |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. A cluster resource creation request has been submitted and its cluster_id is available
    - **Note**: Cluster does not need to be Ready before creating nodepool
    - **Cleanup**: Cluster resource cleanup should be handled in test suite teardown where cluster was created

---

### Test Steps

**Setup (BeforeEach):**
- Get or create test cluster (cluster_id is obtained)
- Submit a POST request to create a NodePool resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Response includes the created nodepool ID and name for use in test validations

#### Step 1: Verify Kubernetes Resources for Each Required Adapter

**Action:**
- For each adapter configured in `configs/config.yaml` under `adapters.nodepool`, verify the corresponding Kubernetes resource:
  - **np-configmap adapter**: Verify ConfigMap resource exists in cluster namespace
- Use kubectl to verify resources with expected labels and annotations:
```bash
# Example for np-configmap adapter
kubectl get configmap -n {cluster_id} \
  -l hyperfleet.io/cluster-id={cluster_id} \
  -l hyperfleet.io/nodepool-id={nodepool_id} \
  -l hyperfleet.io/nodepool-name={nodepool_name} \
  -l hyperfleet.io/resource-type=configmap
```

**Expected Result:**
- **ConfigMap (np-configmap adapter)**:
  - ConfigMap exists in the cluster namespace (namespace name = cluster_id)
  - ConfigMap has correct labels:
    - `hyperfleet.io/cluster-id`: {cluster_id}
    - `hyperfleet.io/nodepool-id`: {nodepool_id}
    - `hyperfleet.io/nodepool-name`: {nodepool_name}
    - `hyperfleet.io/resource-type`: "configmap"
  - ConfigMap has correct annotations:
    - `hyperfleet.io/generation`: "1" (for new creation request)

#### Step 2: Verify Final NodePool State

**Action:**
- Wait for nodepool Ready condition to transition to True
- Retrieve final nodepool status information:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- NodePool `Ready` condition has `status: True`
- This confirms the nodepool workflow completed successfully and all K8s resources were created

#### Step 3: Cleanup Resources (AfterEach)

**Action:**
- Wait for cluster Ready condition with timeout to prevent namespace deletion conflicts:
  - Poll the cluster status via API until Ready=True
  - If timeout occurs, log a warning and continue with best-effort cleanup
- Delete the cluster namespace (cascades to delete nodepool resources):
```bash
# Wait for cluster Ready with timeout (best-effort, pseudo-code)
# Poll: curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} until condition type=Ready, status=True
# If timeout: log "WARN:  The cluster did not reach Ready state before cleanup"

# Delete namespace regardless of Ready state
kubectl delete namespace {cluster_id}
```

**Expected Result:**
- Namespace deletion is attempted regardless of Ready state, with a warning logged if cluster is not Ready
- Namespace and all associated resources (ConfigMap) are deleted (best-effort)
- Cleanup never hangs indefinitely

**Note:** This is a workaround cleanup method. Once HyperFleet API supports DELETE operations for "nodepools" and "clusters" resource type, this step should be replaced with:
```bash
# Delete nodepool
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
# Delete cluster
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---
