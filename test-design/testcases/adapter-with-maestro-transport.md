# Feature: Adapter Framework - Maestro Transportation Layer

## Table of Contents

1. [Adapter can create ManifestWork and report status via Maestro transport](#test-title-adapter-can-create-manifestwork-and-report-status-via-maestro-transport)
2. [Adapter can skip ManifestWork operation when generation is unchanged](#test-title-adapter-can-skip-manifestwork-operation-when-generation-is-unchanged)
3. [Adapter can route ManifestWork to correct consumer based on targetCluster](#test-title-adapter-can-route-manifestwork-to-correct-consumer-based-on-targetcluster)
4. [Adapter can handle Maestro server unavailability gracefully](#test-title-adapter-can-handle-maestro-server-unavailability-gracefully)
5. [Adapter can handle invalid targetCluster (consumer not found) gracefully](#test-title-adapter-can-handle-invalid-targetcluster-consumer-not-found-gracefully)

---

## Environment Setup

Before running these tests, deploy the full HyperFleet stack on a dedicated GKE cluster. The following Make targets from `hyperfleet-infra` are used:

```bash
# 1. Create GKE cluster
make install-terraform TF_ENV=dev-{name}

# 2. Get kubectl credentials
gcloud container clusters get-credentials hyperfleet-dev-{name} \
  --zone us-central1-a --project hcm-hyperfleet

# 3. Generate Helm values from Terraform outputs
make tf-helm-values TF_ENV=dev-{name}

# 4. Deploy Maestro stack
make install-maestro
# Note: You may need to manually install OCM CRDs if the Helm chart CRD installation fails:
#   kubectl apply -f https://raw.githubusercontent.com/open-cluster-management-io/api/main/work/v1/0000_00_work.open-cluster-management.io_manifestworks.crd.yaml
#   kubectl apply -f https://raw.githubusercontent.com/open-cluster-management-io/api/main/work/v1/0000_01_work.open-cluster-management.io_appliedmanifestworks.crd.yaml
#   kubectl rollout restart deployment/maestro-agent -n maestro

# 5. Create Maestro consumer (represents a target cluster)
make create-maestro-consumer MAESTRO_CONSUMER=cluster1

# 6. Deploy HyperFleet API
make install-api

# 7. Deploy Sentinels
make install-sentinels

# 8. Deploy Maestro transport adapter
# The adapter name here must match ADAPTER_NAME below.
# If using a different adapter (e.g., cl-maestro), update both accordingly.
make install-adapter2

# 9. Set test variables
export ADAPTER_NAME='adapter2'
export MAESTRO_CONSUMER='cluster1'
export API_URL='http://localhost:8000'

# 10. Port-forward HyperFleet API for local access
kubectl port-forward -n hyperfleet svc/hyperfleet-api 8000:8000 &
```

---

## Test Title: Adapter can create ManifestWork and report status via Maestro transport

### Description

This test validates the complete Maestro transport happy path: creating a cluster via the HyperFleet API triggers the adapter to create a ManifestWork (resource bundle) on the Maestro server, the Maestro agent applies the ManifestWork content to the target cluster (verified via kubectl), the adapter discovers the ManifestWork and its nested sub-resources via statusFeedback, evaluates post-processing CEL expressions, and reports the final status back to the HyperFleet API.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-03-02 |

---

### Preconditions

1. HyperFleet API and Sentinel services are deployed and running successfully
2. Maestro is deployed and running successfully with an active agent
3. At least one Maestro consumer is registered (e.g., `${MAESTRO_CONSUMER}`)
4. Adapter is deployed in Maestro transport mode (`transport.client: "maestro"`)
5. Adapter task config defines nestedDiscoveries (`namespace0`, `configmap0`) and post-processing CEL expressions

---

### Test Steps

#### Step 1: Create a cluster via HyperFleet API
**Action:**
```bash
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "maestro-happy-path-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {
        "type": "gcp",
        "gcp": {"projectID": "test-project", "region": "us-central1"}
      },
      "release": {"version": "4.14.0"}
    }
  }' | jq -r '.id')
echo "CLUSTER_ID=${CLUSTER_ID}"
```

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID and `generation: 1`

#### Step 2: Verify ManifestWork was created on Maestro
**Action:**
- Query the Maestro resource-bundles API from inside the maestro pod:
```bash
# Capture resource bundle ID for subsequent steps
RESOURCE_BUNDLE_ID=$(kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles \
  | jq -r --arg cid "${CLUSTER_ID}" \
    '.items[] | select(.metadata.labels["hyperfleet.io/cluster-id"] == $cid) | .id')
echo "RESOURCE_BUNDLE_ID=${RESOURCE_BUNDLE_ID}"

# Display resource bundle details
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/${RESOURCE_BUNDLE_ID} \
  | jq '{id: .id, consumer_name: .consumer_name, version: .version,
       manifest_names: [.manifests[].metadata.name]}'
```

**Expected Result:**
- A resource bundle (ManifestWork) is created on Maestro targeting `${MAESTRO_CONSUMER}`
- The resource bundle contains all expected inline manifests as resources
- `manifest_names` follows the naming pattern `${CLUSTER_ID}-${ADAPTER_NAME}-<resource_type>`:
  - `${CLUSTER_ID}-${ADAPTER_NAME}-namespace` (Namespace)
  - `${CLUSTER_ID}-${ADAPTER_NAME}-configmap` (ConfigMap)

Example output:
```json
{
  "id": "auto-generated unique ID by Maestro",
  "consumer_name": "${MAESTRO_CONSUMER}, the target consumer this ManifestWork is routed to",
  "version": 1,
  "manifest_names": [
    "${CLUSTER_ID}-${ADAPTER_NAME}-namespace",
    "${CLUSTER_ID}-${ADAPTER_NAME}-configmap"
  ]
}
```

#### Step 3: Verify ManifestWork metadata (labels and annotations)
**Action:**
```bash
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/${RESOURCE_BUNDLE_ID} \
  | jq '.metadata | {labels, annotations}'
```

**Expected Result:**

1. **Code logic additions** (dynamically set by adapter code):
   - `consumer_name`: set to the resolved `targetCluster` value (e.g., `${MAESTRO_CONSUMER}`)
   - `hyperfleet.io/generation` (label + annotation): set from the cluster's current generation value

2. **Manifest template configuration** (from adapter task config template):
   - Labels: `hyperfleet.io/cluster-id`, `hyperfleet.io/adapter`
   - Annotations: `hyperfleet.io/managed-by`

Example output:
```json
{
  "labels": {
    "hyperfleet.io/cluster-id": "${CLUSTER_ID}",
    "hyperfleet.io/generation": "1, code logic: set from cluster generation",
    "hyperfleet.io/adapter": "${ADAPTER_NAME}, template config: identifies the adapter"
  },
  "annotations": {
    "hyperfleet.io/generation": "1, code logic: used for idempotency check",
    "hyperfleet.io/managed-by": "${ADAPTER_NAME}, template config: indicates managing adapter"
  }
}
```

#### Step 4: Verify feedbackRules configuration in Maestro resource bundle
**Action:**
```bash
# Query feedbackRules
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/${RESOURCE_BUNDLE_ID} \
  | jq '.manifest_configs'
```

**Expected Result:**
- `manifestConfigs` contains feedbackRules with JSONPaths for status collection:
  - Namespace: `.status.phase`
  - ConfigMap: `.data`, `.metadata.resourceVersion`

Example output:
```json
[
  {
    "resourceIdentifier": {
      "name": "${CLUSTER_ID}-${ADAPTER_NAME}-namespace",
      "group": "",
      "resource": "namespaces"
    },
    "feedbackRules": [
      {"type": "JSONPaths", "jsonPaths": [{"name": "phase", "path": ".status.phase"}]}
    ]
  },
  {
    "resourceIdentifier": {
      "name": "${CLUSTER_ID}-${ADAPTER_NAME}-configmap",
      "group": "",
      "resource": "configmaps",
      "namespace": "${CLUSTER_ID}-${ADAPTER_NAME}-namespace"
    },
    "feedbackRules": [
      {"type": "JSONPaths", "jsonPaths": [
        {"name": "data", "path": ".data"},
        {"name": "resourceVersion", "path": ".metadata.resourceVersion"}
      ]}
    ]
  }
]
```

#### Step 5: Verify K8s resources created by Maestro agent on target cluster

Wait ~15 seconds for the Maestro agent to apply the ManifestWork content to the target cluster.

**Action:**
```bash
# Verify Namespace
kubectl get ns ${CLUSTER_ID}-${ADAPTER_NAME}-namespace

# Verify ConfigMap
kubectl get configmap ${CLUSTER_ID}-${ADAPTER_NAME}-configmap \
  -n ${CLUSTER_ID}-${ADAPTER_NAME}-namespace
```

**Expected Result:**
- Namespace `${CLUSTER_ID}-${ADAPTER_NAME}-namespace` exists and is `Active`
- ConfigMap `${CLUSTER_ID}-${ADAPTER_NAME}-configmap` exists in the namespace

#### Step 6: Verify adapter status report to HyperFleet API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq '.items[] | select(.adapter == "'"${ADAPTER_NAME}"'")'
```

**Expected Result:**
- Status entry with `adapter: "${ADAPTER_NAME}"`
- `observed_generation: 1`
- `observed_time` is present and is a valid timestamp
- Three conditions with expected values:
  - Applied = True, reason = `AppliedManifestWorkComplete`
  - Available = True, reason = `AllResourcesAvailable`
  - Health = True, reason = `Healthy`
- `data.manifestwork.name` = `"${CLUSTER_ID}-${ADAPTER_NAME}"`
- `data.namespace.phase` = `"Active"`
- `data.namespace.name` = `"${CLUSTER_ID}-${ADAPTER_NAME}-namespace"`
- `data.configmap.clusterId` = `"${CLUSTER_ID}"`
- `data.configmap.name` = `"${CLUSTER_ID}-${ADAPTER_NAME}-configmap"`

Example output:
```json
{
  "adapter": "${ADAPTER_NAME}",
  "observed_generation": 1,
  "observed_time": "2026-01-01T00:00:00Z",
  "conditions": [
    {
      "type": "Applied",
      "status": "True",
      "reason": "AppliedManifestWorkComplete"
    },
    {
      "type": "Available",
      "status": "True",
      "reason": "AllResourcesAvailable"
    },
    {
      "type": "Health",
      "status": "True",
      "reason": "Healthy"
    }
  ],
  "data": {
    "manifestwork": {
      "name": "${CLUSTER_ID}-${ADAPTER_NAME}"
    },
    "namespace": {
      "phase": "Active",
      "name": "${CLUSTER_ID}-${ADAPTER_NAME}-namespace"
    },
    "configmap": {
      "clusterId": "${CLUSTER_ID}",
      "name": "${CLUSTER_ID}-${ADAPTER_NAME}-configmap"
    }
  }
}
```

#### Step 7: Cleanup
**Action:**
```bash
# Delete the namespace created by Maestro agent
kubectl delete ns ${CLUSTER_ID}-${ADAPTER_NAME}-namespace --ignore-not-found

# Delete the resource bundle on Maestro (via Maestro API)
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/${RESOURCE_BUNDLE_ID}
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}
> ```

---

## Test Title: Adapter can skip ManifestWork operation when generation is unchanged

### Description

This test validates the generation-based idempotency mechanism for ManifestWork operations via Maestro transport. When a ManifestWork does not exist, it should be created. When the same event is reprocessed with the same generation, the operation should be skipped.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet API, Sentinel, and Adapter (Maestro mode) are deployed and running
2. Maestro server is accessible with at least one registered consumer

---

### Test Steps

#### Step 1: Create a cluster (triggers initial ManifestWork creation)
**Action:**
```bash
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "gen-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq -r '.id')
echo "CLUSTER_ID=${CLUSTER_ID}"
```

**Expected Result:**
- Cluster created with `generation: 1`

#### Step 2: Verify "Skip" operation on subsequent processing (same generation)
**Action:**
- The Sentinel continuously polls and re-publishes events every ~5 seconds. Wait for the next event processing cycle and check logs:
```bash
# Wait for a few more cycles
sleep 15
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-${ADAPTER_NAME} --tail=20 \
  | grep "Resource\[resource0\]"
```

**Expected Result:**
- Subsequent processing shows: `Resource[resource0] processed: operation=skip reason=generation 1 unchanged`

#### Step 3: Verify Maestro resource version does not change on Skip
**Action:**
```bash
# Capture resource bundle ID
RESOURCE_BUNDLE_ID=$(kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles \
  | jq -r --arg cid "${CLUSTER_ID}" \
    '.items[] | select(.metadata.labels["hyperfleet.io/cluster-id"] == $cid) | .id')
echo "RESOURCE_BUNDLE_ID=${RESOURCE_BUNDLE_ID}"

# Query the resource bundle version from Maestro - should stay at version 1
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/${RESOURCE_BUNDLE_ID} \
  | jq '{id: .id, version: .version}'
```

**Expected Result:**
- `version: 1` remains unchanged across multiple Skip operations

#### Step 4: Cleanup
**Action:**
```bash
kubectl delete ns ${CLUSTER_ID}-${ADAPTER_NAME}-namespace --ignore-not-found
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/${RESOURCE_BUNDLE_ID}
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}
> ```

---

## Test Title: Adapter can route ManifestWork to correct consumer based on targetCluster

### Description

This test validates that the adapter can route ManifestWorks to different Maestro consumers based on the `targetCluster` template value. The adapter task config uses `targetCluster: "{{ .placementClusterName }}"` where `placementClusterName` is captured from a precondition expression. By changing this expression to point to a different consumer, ManifestWorks are routed to the new consumer.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet environment deployed with Maestro transport adapter
2. Initial consumer `${MAESTRO_CONSUMER}` already registered
3. Adapter task config uses `targetCluster: "{{ .placementClusterName }}"` where `placementClusterName` is set via precondition capture expression

---

### Test Steps

#### Step 1: Register a second Maestro consumer
**Action:**
```bash
make create-maestro-consumer MAESTRO_CONSUMER=cluster2
```

**Expected Result:**
- Consumer `cluster2` created successfully

#### Step 2: Update adapter task config to use the new consumer
**Action:**
- Extract, modify, and re-apply the adapter task config:
```bash
# Extract current task config
kubectl get configmap hyperfleet-${ADAPTER_NAME}-task -n hyperfleet \
  -o jsonpath='{.data.task-config\.yaml}' > /tmp/adapter2-task-original.yaml

# Modify placementClusterName from "${MAESTRO_CONSUMER}" to "cluster2"
# In the task config, change:
#   expression: "\"${MAESTRO_CONSUMER}\""
# To:
#   expression: "\"cluster2\""

# Apply the modified config
kubectl create configmap hyperfleet-${ADAPTER_NAME}-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-cluster2.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart adapter to pick up new config
kubectl rollout restart deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet
kubectl rollout status deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet --timeout=60s
```

**Expected Result:**
- Adapter restarts with `placementClusterName` = `"cluster2"`

#### Step 3: Create a cluster and verify routing to cluster2
**Action:**
```bash
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "multi-consumer-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq -r '.id')
echo "CLUSTER_ID=${CLUSTER_ID}"
```

Wait ~15 seconds for the adapter to process.

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID

#### Step 4: Verify ManifestWork is on the correct consumer via Maestro API
**Action:**
```bash
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles \
  | jq '.items[] | {consumer_name: .consumer_name,
       cluster_id: .metadata.labels["hyperfleet.io/cluster-id"]}'
```

**Expected Result:**
- New cluster's resource bundle has `consumer_name: "cluster2"`
- Previously created clusters (before config change) remain on `consumer_name: "${MAESTRO_CONSUMER}"`

#### Step 5: Restore adapter config and cleanup
**Action:**
```bash
# Restore original config
kubectl create configmap hyperfleet-${ADAPTER_NAME}-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-original.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet
kubectl rollout status deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet --timeout=60s
```

---

## Test Title: Adapter can handle Maestro server unavailability gracefully

### Description

This test validates the adapter's behavior when the Maestro server is unreachable. The adapter should handle connection failures gracefully, report appropriate error status back to the HyperFleet API, and not crash. When Maestro recovers, the adapter should automatically retry and succeed on subsequent events.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet API, Sentinel, and Adapter are deployed and running
2. Adapter is deployed in Maestro transport mode and initially connected to Maestro
3. Ability to scale down the Maestro deployment

---

### Test Steps

#### Step 1: Verify adapter is running and Maestro is healthy
**Action:**
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-${ADAPTER_NAME} --no-headers
kubectl get pods -n maestro -l app=maestro --no-headers
```

**Expected Result:**
- Both ${ADAPTER_NAME} and maestro pods are `Running`

#### Step 2: Scale down Maestro to simulate unavailability
**Action:**
```bash
kubectl scale deployment maestro -n maestro --replicas=0
```

**Expected Result:**
- Maestro pod terminates, gRPC and HTTP endpoints become unreachable

#### Step 3: Create a cluster while Maestro is down
**Action:**
```bash
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "maestro-unavail-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq -r '.id')
echo "CLUSTER_ID=${CLUSTER_ID}"
```

**Expected Result:**
- Cluster creation succeeds (API is independent of Maestro)

#### Step 4: Verify adapter error handling (check logs after ~15 seconds)
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-${ADAPTER_NAME} --tail=30 \
  | grep -E "FAILED|error|connection refused" | head -5
```

**Expected Result:**
- Adapter logs show Maestro connection error
- Adapter does NOT crash (pod remains Running)

> **Note:** The error code `hyperfleet-adapter-16` is the adapter's internal MaestroError code (code 16 in the adapter's error enumeration, not a gRPC status code).

#### Step 5: Verify error status reported to HyperFleet API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq '.items[] | select(.adapter == "'"${ADAPTER_NAME}"'") | .conditions'
```

**Expected Result:**
- Health condition: `status: "False"`, error message should contain key points like `connection refused` or `hyperfleet-adapter-16`
- Applied condition: `status: "False"`

#### Step 6: Verify adapter pod is still running (no crash)
**Action:**
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-${ADAPTER_NAME} --no-headers
```

**Expected Result:**
- Pod is `Running` with 0 restarts

#### Step 7: Restore Maestro and verify recovery
**Action:**
```bash
kubectl scale deployment maestro -n maestro --replicas=1
kubectl rollout status deployment/maestro -n maestro --timeout=120s
```

**Expected Result:**
- Maestro pod becomes `Running`

> **Note:** After Maestro restores, the adapter's CloudEvents client (MQTT-based) takes a few seconds to re-establish the connection. During this window, events fail with "the cloudevents client is not ready". The adapter automatically recovers once the connection is restored.

#### Step 8: Verify recovery - resources created and status updated
**Action:**
```bash
# Verify resources now exist
kubectl get ns | grep ${CLUSTER_ID}-${ADAPTER_NAME}

# Verify status updated
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq '.items[] | select(.adapter == "'"${ADAPTER_NAME}"'") | .conditions[] | select(.type == "Health")'
```

**Expected Result:**
- Namespace created after recovery
- Health returns to `True`

#### Step 9: Cleanup
**Action:**
```bash
kubectl delete ns ${CLUSTER_ID}-${ADAPTER_NAME}-namespace --ignore-not-found

# Ensure Maestro is fully restored
kubectl get pods -n maestro --no-headers
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}
> ```

---

## Test Title: Adapter can handle invalid targetCluster (consumer not found) gracefully

### Description

This test validates the adapter's behavior when the configured `targetCluster` resolves to a Maestro consumer that does not exist. The adapter should detect the error, report it properly, and not crash.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet environment deployed with Maestro transport adapter
2. Maestro server is accessible
3. Adapter task config backup saved for restoration after test

---

### Test Steps

#### Step 1: Backup and modify adapter task config to target a non-existent consumer
**Action:**
```bash
# Backup original config
kubectl get configmap hyperfleet-${ADAPTER_NAME}-task -n hyperfleet \
  -o jsonpath='{.data.task-config\.yaml}' > /tmp/adapter2-task-original.yaml

# Modify: change placementClusterName from "${MAESTRO_CONSUMER}" to "non-existent-cluster"
# In the task config, change:
#   expression: "\"${MAESTRO_CONSUMER}\""
# To:
#   expression: "\"non-existent-cluster\""

# Apply modified config
kubectl create configmap hyperfleet-${ADAPTER_NAME}-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-modified.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart adapter
kubectl rollout restart deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet
kubectl rollout status deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet --timeout=60s
```

**Expected Result:**
- Adapter restarts with `placementClusterName` = `"non-existent-cluster"`

#### Step 2: Create a cluster to trigger adapter processing
**Action:**
```bash
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "invalid-consumer-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq -r '.id')
echo "CLUSTER_ID=${CLUSTER_ID}"
```

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID

#### Step 3: Verify error handling for invalid consumer (check logs after ~15 seconds)
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-${ADAPTER_NAME} --tail=30 \
  | grep -E "FAILED|error|non-existent" | head -5
```

**Expected Result:**
- Adapter logs show error related to consumer not found
- Error message includes the invalid consumer name
- Adapter does NOT crash

#### Step 4: Verify adapter pod is still running (no crash)
**Action:**
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-${ADAPTER_NAME} --no-headers
```

**Expected Result:**
- Pod is `Running` with 0 restarts

#### Step 5: Verify error status reported to API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq '.items[] | select(.adapter == "'"${ADAPTER_NAME}"'") | .conditions'
```

**Expected Result:**
- Health: `status: "False"`, error message should contain key points like `non-existent-cluster` or `consumer` not found
- Applied: `status: "False"`

#### Step 6: Restore and cleanup
**Action:**
```bash
# Restore original config
kubectl create configmap hyperfleet-${ADAPTER_NAME}-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-original.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet
kubectl rollout status deployment/hyperfleet-${ADAPTER_NAME} -n hyperfleet --timeout=60s
```

> **Important:** Always restore the adapter config after this test to avoid impacting other tests.

