# Feature: Adapter Framework - Customization

## Table of Contents

1. [Adapter framework can detect and report failures to cluster API endpoints](#test-title-adapter-framework-can-detect-and-report-failures-to-cluster-api-endpoints)
2. [Adapter framework can detect and handle resource timeouts](#test-title-adapter-framework-can-detect-and-handle-resource-timeouts)

---

## Test Title: Adapter framework can detect and report failures to cluster API endpoints

### Description

This test validates that the adapter framework correctly detects and reports failures when attempting to create invalid Kubernetes resources on the target cluster. It ensures that when an adapter's configuration contains invalid K8s resource objects, the framework properly handles the API server rejection, logs meaningful error messages, and reports the failure status back to the HyperFleet API with appropriate condition states and error details.


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

#### Step 1: Test template rendering errors
**Action:**
- Configure AdapterConfig with invalid AdapterConfig (invalid K8s resource object)
- Deploy the test adapter

**Expected Result:**
- Adapter detects template rendering error
- Log reports failure with clear error message

#### Step 2: Send POST request to create a new cluster
**Action:**
- Execute cluster creation request:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d <cluster_create_payload>
```

**Expected Result:**
- API returns successful response

#### Step 3: Wait for timeout and Verify Timeout Handling
**Action:**
- Wait for some minutes
- Verify adapter status

**Expected Result:**
```bash
   curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/<cluster_id>/statuses \
     | jq -r '.items[] | select(.adapter=="<adapter_name>") | .conditions[] | select(.type=="Available")'

   # Example:
   # {
   #   "type": "Available",
   #   "status": "False",
   #   "reason": "`invalid k8s object` resource is invalid",
   #   "message": "Invalid Kubernetes object"
   # }
```

---


## Test Title: Adapter framework can detect and handle resource timeouts

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

#### Step 1: Configure adapter with timeout setting
**Action:**
- Configure AdapterConfig with non-existed conditions that can't meet the precondition
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
- Deploy the test adapter

**Expected Result:**
- Adapter loads configuration successfully
- Adapter pods are running successfully
- Adapter logs show successful initialization

#### Step 2: Send POST request to create a new cluster
**Action:**
- Execute cluster creation request:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d <cluster_create_payload>
```

**Expected Result:**
- API returns successful response

#### Step 3: Wait for timeout and Verify Timeout Handling
**Action:**
- Wait for some minutes
- Verify adapter status

**Expected Result:**
```bash
   curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/<cluster_id>/statuses \
     | jq -r '.items[] | select(.adapter=="<adapter_name>") | .conditions[] | select(.type=="Available")'

   # Example:
   # {
   #   "type": "Available",
   #   "status": "False",
   #   "reason": "JobTimeout",
   #   "message": "Validation job did not complete within 30 seconds"
   # }
```

---
