# Feature: Adapter Framework - Error Handling and Resilience

## Table of Contents

1. [Adapter can detect and report invalid K8s resource failures](#test-title-adapter-can-detect-and-report-invalid-k8s-resource-failures)
2. [Adapter can detect and handle precondition timeouts](#test-title-adapter-can-detect-and-handle-precondition-timeouts)
3. [Adapter can recover from crash and process redelivered events](#test-title-adapter-can-recover-from-crash-and-process-redelivered-events)
4. [Adapter can process pending events after restart](#test-title-adapter-can-process-pending-events-after-restart)
5. [API can handle incomplete adapter status reports gracefully](#test-title-api-can-handle-incomplete-adapter-status-reports-gracefully)

---

## Test Title: Adapter can detect and report invalid K8s resource failures

### Description

This test validates that the adapter framework correctly detects and reports failures when attempting to create invalid Kubernetes resources on the target cluster. It ensures that when an adapter's configuration contains invalid K8s resource objects, the framework properly handles the API server rejection and reports the failure status back to the HyperFleet API with appropriate condition states and error details.


---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-01-30 |
| **Updated** | 2026-01-30 |


---

### Preconditions
1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully

---

### Test Steps

#### Step 1: Deploy dedicated test adapter with invalid K8s resource configuration
**Action:**
- Deploy a test adapter via Helm with AdapterConfig containing invalid K8s resource objects

**Expected Result:**
- Test adapter is deployed and running successfully

#### Step 2: Send POST request to create a new cluster
**Action:**
- Execute cluster creation request:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response

#### Step 3: Verify adapter status reports failure
**Action:**
- Poll adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- The test adapter reports `Available` condition with `status: "False"`, with reason indicating invalid K8s resource

#### Step 4: Cleanup resources

**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```
- Uninstall the test adapter Helm release

**Expected Result:**
- Namespace and all associated resources are deleted successfully
- Test adapter deployment is removed

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---

## Test Title: Adapter can detect and handle precondition timeouts

### Description

This test validates that the adapter framework correctly detects and handles resource timeouts when adapter Jobs exceed configured timeout limits.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-01-30 |
| **Updated** | 2026-01-30 |


---

### Preconditions
1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully

---

### Test Steps

#### Step 1: Deploy dedicated timeout-adapter with unsatisfiable preconditions
**Action:**
- Deploy a timeout-adapter via Helm with AdapterConfig containing preconditions that cannot be met, for example:
```yaml
preconditions:
  - name: "clusterStatus"
    apiCall:
      method: "GET"
      url: "{{ .hyperfleetApiBaseUrl }}/api/hyperfleet/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterId }}"
      timeout: 10s
      retryAttempts: 3
      retryBackoff: "exponential"
    capture:
      - name: "clusterName"
        field: "name"
      - name: "clusterPhase"
        field: "status.phase"
      - name: "generationId"
        field: "generation"
    conditions:
      - field: "clusterPhase"
        operator: "in"
        values: ["NotReady", "Ready"]
```

**Expected Result:**
- timeout-adapter is deployed and running successfully

#### Step 2: Send POST request to create a new cluster
**Action:**
- Execute cluster creation request:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response

#### Step 3: Verify adapter status reports timeout
**Action:**
- Poll adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- The timeout-adapter reports `Available` condition with `status: "False"`, with reason indicating timeout (e.g., `"reason": "JobTimeout"`)

#### Step 4: Cleanup resources

**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```
- Uninstall the timeout-adapter Helm release

**Expected Result:**
- Namespace and all associated resources are deleted successfully
- Timeout-adapter deployment is removed

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---


## Test Title: Adapter can recover from crash and process redelivered events

### Description

This test validates that when an adapter crashes during event processing, the system ensures that pending events are eventually processed after the adapter recovers. This ensures that no events are lost due to adapter failures and the system maintains eventual consistency.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-11 |
| **Updated** | 2026-03-04 |


---

### Preconditions
1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra)
2. HyperFleet API, Sentinel, and Adapter services are deployed
3. Message broker is configured with appropriate acknowledgment deadline and retry policy

---

### Test Steps

#### Step 1: Deploy dedicated crash-adapter with pre-configured crash behavior
**Action:**
- Deploy a crash-adapter via Helm with `SIMULATE_RESULT=crash`, separate from the normal adapters used in other tests

**Expected Result:**
- crash-adapter is deployed and running successfully

#### Step 2: Create a cluster to trigger event
**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response with cluster ID

**Note:** After the cluster is created, Sentinel will detect the new cluster during its polling cycle and publish an event to the broker, which triggers the crash-adapter to receive and process the event.

#### Step 3: Verify crash-adapter crashes on event receipt
**Action:**
- Check cluster adapter statuses via API:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses | jq '.items[].adapter'
```
- Check crash-adapter pod status (for manual verification):
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=crash-adapter --no-headers
```

**Expected Result:**
- API: statuses response does not contain an entry for `crash-adapter` (it crashed before reporting status)
- kubectl: crash-adapter pod shows CrashLoopBackOff or Error state

**Note:** The unacknowledged message will be redelivered by the broker, which is verified in Step 5.

#### Step 4: Restore crash-adapter to normal mode
**Action:**
- Upgrade crash-adapter Helm release with `SIMULATE_RESULT=success`

**Expected Result:**
- crash-adapter pod starts and remains Running

#### Step 5: Verify message redelivery and processing
**Action:**
- Wait for crash-adapter to process redelivered message
- Check crash-adapter status via API:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses \
  | jq '.items[] | select(.adapter == "crash-adapter")'
```

**Expected Result:**
- crash-adapter status entry is now present in the statuses response (confirming the redelivered message was processed)
- crash-adapter reports all three condition types with `status: "True"`: `Applied`, `Available`, `Health`
- `observed_generation` is set to `1`

#### Step 6: Cleanup resources
**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```
- Uninstall the crash-adapter Helm release

**Expected Result:**
- Namespace and all associated resources are deleted successfully
- crash-adapter deployment is removed

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---

### Notes

- Message redelivery behavior depends on the message broker configuration:
  - For Google Pub/Sub: Uses acknowledgment deadline and retry policy
  - If adapter crashes before acknowledging message, broker will redeliver after the acknowledgment deadline expires
- Sentinel may also republish events during its polling cycle if generation > observed_generation

---

## Test Title: Adapter can process pending events after restart

### Description

This test validates that adapters can recover from restarts and continue processing pending events. Pending events should be eventually processed after the adapter restarts.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-11 |
| **Updated** | 2026-03-04 |


---

### Preconditions
1. HyperFleet system is deployed and running
2. Adapter is running normally

---

### Test Steps

#### Step 1: Create cluster and verify initial processing
**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for adapter to process and verify status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Cluster created and processed
- Adapter status shows all three condition types with `status: "True"`: `Applied`, `Available`, `Health`

---

#### Step 2: Restart adapter pod
**Action:**
- Delete an adapter pod to trigger restart:
```bash
kubectl delete pod -n hyperfleet -l app.kubernetes.io/instance=<adapter-release-name>
```

**Expected Result:**
- Adapter pod is terminated
- New adapter pod starts up automatically

---

#### Step 3: Create another cluster after adapter restart
**Action:**
- Create another cluster via API:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster created successfully (API is independent of adapter)

**Note:** The cluster can be created during or after the adapter restart. In either case, Sentinel will publish an event during its next polling cycle, and the adapter should eventually process it.

---

#### Step 4: Verify adapter processes pending events after restart
**Action:**
- Wait for adapter to fully restart
- Check status of the new cluster:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Both clusters have adapter statuses with all three condition types `status: "True"`: `Applied`, `Available`, `Health`

#### Step 5: Cleanup resources
**Action:**
- Delete the namespaces created for both clusters:
```bash
kubectl delete namespace {cluster_id_1}
kubectl delete namespace {cluster_id_2}
```

**Expected Result:**
- Namespaces and all associated resources are deleted successfully

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---

## Test Title: API can handle incomplete adapter status reports gracefully

### Description

This test validates that the HyperFleet API can gracefully handle adapter status reports that are missing expected fields. By deploying dedicated adapters whose AdapterConfig `post` section intentionally omits certain fields (e.g., `reason`, `message`, `observed_generation`, or conditions entirely), the test verifies that the API accepts incomplete status reports without crashing and stores what is available, rather than rejecting the entire status update.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-11 |
| **Updated** | 2026-03-04 |


---

### Preconditions
1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully

---

### Test Steps

#### Step 1: Deploy three incomplete-adapter variants
**Action:**
- Deploy three dedicated adapters via Helm, each with an AdapterConfig whose `post` section is intentionally incomplete:
  - `incomplete-no-reason`: omits `reason` and `message` fields from condition reporting
  - `incomplete-no-generation`: omits `observed_generation` field
  - `incomplete-empty-conditions`: produces an empty conditions array

**Expected Result:**
- All three adapters are deployed and running successfully

#### Step 2: Create a cluster to trigger event processing
**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response with cluster ID

#### Step 3: Verify API handles each incomplete status report
**Action:**
- Poll adapter statuses for the created cluster:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses | jq '.items[]'
```

**Expected Result:**
- **incomplete-no-reason**: status entry is present; `reason` and `message` fields default to empty or null
- **incomplete-no-generation**: status entry is present; `observed_generation` defaults to 0 or null
- **incomplete-empty-conditions**: API either accepts the report with empty conditions, or the adapter status entry is absent if API returned a validation error (HTTP 400)
- API does not crash or return HTTP 500 for any of the above

#### Step 4: Verify cluster state is not corrupted
**Action:**
- Retrieve the cluster and its conditions:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} | jq '.conditions'
```

**Expected Result:**
- Cluster conditions remain consistent and valid
- Incomplete status entries do not interfere with cluster Ready/Available evaluation

#### Step 5: Cleanup resources
**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```
- Uninstall all three incomplete-adapter Helm releases

**Expected Result:**
- Namespace and all associated resources are deleted successfully
- All three adapter deployments are removed

**Note:** Namespace deletion is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---
