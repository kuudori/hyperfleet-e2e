# Feature: Adapter Framework - Maestro Transportation Layer

## Table of Contents

1. [Maestro transport E2E: ManifestWork creation and status feedback](#test-title-maestro-transport-e2e-manifestwork-creation-and-status-feedback)
2. [Resource generation tracking with ManifestWork (Create/Skip)](#test-title-resource-generation-tracking-with-manifestwork-createskip)
3. [Transport mode comparison (Kubernetes vs Maestro)](#test-title-transport-mode-comparison-kubernetes-vs-maestro)
4. [NestedDiscovery within ManifestWork for status bridging](#test-title-nesteddiscovery-within-manifestwork-for-status-bridging)
5. [Multiple targetCluster values for ManifestWork routing](#test-title-multiple-targetcluster-values-for-manifestwork-routing)
6. [Maestro server unavailability error handling](#test-title-maestro-server-unavailability-error-handling)
7. [Invalid targetCluster (consumer not found) error handling](#test-title-invalid-targetcluster-consumer-not-found-error-handling)
8. [Maestro TLS authentication validation](#test-title-maestro-tls-authentication-validation)

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

# 8. Deploy adapters
make install-adapter1   # K8s transport adapter
make install-adapter2   # Maestro transport adapter

# 9. Port-forward HyperFleet API for local access
kubectl port-forward -n hyperfleet svc/hyperfleet-api 8000:8000 &
```

---

## Test Title: Maestro transport E2E: ManifestWork creation and status feedback

### Description

This test validates the complete end-to-end flow of the Maestro transportation layer: creating a cluster via the HyperFleet API triggers the adapter to create a ManifestWork on the Maestro server targeting the correct consumer (target cluster). The ManifestWork must contain the expected inline manifests (Namespace and ConfigMap), and the adapter must report status back to the HyperFleet API with Applied/Available/Health conditions.

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

1. HyperFleet API and Sentinel services are deployed and running
2. Maestro server is deployed and accessible (gRPC on port 8090, HTTP on port 8000)
3. At least one Maestro consumer is registered (e.g., `cluster1`)
4. Adapter is deployed in Maestro transport mode (`transport.client: "maestro"`)
5. HyperFleet API is accessible (e.g., via port-forward or Ingress)
6. All pods in `hyperfleet` namespace are `Running` (API, 2 sentinels, 2 adapters)
7. All pods in `maestro` namespace are `Running` (maestro, maestro-agent, maestro-db, maestro-mqtt)

---

### Test Steps

#### Step 1: Create a cluster via HyperFleet API
**Action:**
```bash
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "maestro-e2e-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {
        "type": "gcp",
        "gcp": {"projectID": "test-project", "region": "us-central1"}
      },
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name, generation: .generation}'
```

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID and `generation: 1`

#### Step 2: Verify adapter processes the event (check adapter logs)
**Action:**
- Wait ~10 seconds for Sentinel to poll the cluster and publish an event to the broker, then check adapter2 logs:
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=50 \
  | grep -E "Event received|Precondition|Applying ManifestWork|Resource\[resource0\]|Event execution"
```

**Expected Result:**
- Event received from Sentinel via broker
- Preconditions evaluated as MET
- ManifestWork applied to target consumer
- Event execution status: success

#### Step 3: Verify ManifestWork was created on Maestro (via Maestro HTTP API)
**Action:**
- Query the Maestro resource-bundles API from inside the maestro pod:
```bash
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles \
  | jq '.items[] | {id: .id, consumer_name: .consumer_name, version: .version,
       manifest_names: [.manifests[].metadata.name]}'
```

**Expected Result:**
- A resource bundle exists targeting `cluster1`
- Contains 2 manifests (Namespace and ConfigMap)

#### Step 4: Verify ManifestWork metadata (labels and annotations)
**Action:**
```bash
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID> \
  | jq '.metadata | {labels, annotations}'
```

**Expected Result:**
- Labels include `hyperfleet.io/cluster-id`, `hyperfleet.io/generation`, `hyperfleet.io/adapter`
- Annotations include `hyperfleet.io/generation`, `hyperfleet.io/managed-by`

#### Step 5: Verify K8s resources created by Maestro agent on target cluster
**Action:**
```bash
# Check namespace created
kubectl get ns | grep <CLUSTER_ID>

# Check ConfigMap created in the namespace
kubectl get configmap -n <CLUSTER_ID>-adapter2-namespace
```

**Expected Result:**
- Namespace `<CLUSTER_ID>-adapter2-namespace` exists and is `Active`
- ConfigMap `<CLUSTER_ID>-adapter2-configmap` exists in that namespace

#### Step 6: Verify adapter status report to HyperFleet API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | select(.adapter == "adapter2")'
```

**Expected Result:**
- Status entry with `adapter: "adapter2"`
- Three conditions: Applied, Available, Health
- `observed_generation: 1`
- Applied=True (AppliedManifestWorkComplete), Available=True (ResourcesAvailable), Health=True (Healthy)

#### Step 7: Cleanup
**Action:**
```bash
# Delete the namespace created by Maestro agent
kubectl delete ns <CLUSTER_ID>-adapter2-namespace --ignore-not-found

# Delete the resource bundle on Maestro (via Maestro API)
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID>
```

---

## Test Title: Resource generation tracking with ManifestWork (Create/Skip)

### Description

This test validates the generation-based idempotency mechanism for ManifestWork operations via Maestro transport. When a ManifestWork does not exist, it should be created. When the same event is reprocessed with the same generation, the update should be skipped. The "Update" operation (when generation changes) is post-MVP because the HyperFleet API does not currently support cluster PATCH/UPDATE to increment the generation.

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
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "gen-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name, generation: .generation}'
```

**Expected Result:**
- Cluster created with `generation: 1`
- Adapter logs show `operation=create reason=resource not found`

#### Step 2: Verify "Create" operation in adapter logs
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=50 \
  | grep "Resource\[resource0\]" | tail -5
```

**Expected Result:**
- First processing shows: `Resource[resource0] processed: operation=create reason=resource not found`

#### Step 3: Verify "Skip" operation on subsequent processing (same generation)
**Action:**
- The Sentinel continuously polls and re-publishes events every ~5 seconds. Wait for the next event processing cycle and check logs:
```bash
# Wait for a few more cycles
sleep 15
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=20 \
  | grep "Resource\[resource0\]"
```

**Expected Result:**
- Subsequent processing shows: `Resource[resource0] processed: operation=skip reason=generation 1 unchanged`

#### Step 4: Verify Maestro resource version does not change on Skip
**Action:**
```bash
# Query the resource bundle version from Maestro - should stay at version 1
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles \
  | jq '.items[] | {id: .id, version: .version}'
```

**Expected Result:**
- `version: 1` remains unchanged across multiple Skip operations

#### Step 5: (Post-MVP) Update operation when generation changes

> **Note:** The "Update" operation cannot currently be tested end-to-end because the HyperFleet API does not support cluster PATCH/UPDATE to increment the generation. This is a post-MVP feature. The adapter code supports generation-based update via JSON merge patch when `generation` changes, which can be observed in unit tests.

#### Step 6: Cleanup
**Action:**
```bash
kubectl delete ns <CLUSTER_ID>-adapter2-namespace --ignore-not-found
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID>
```

---

## Test Title: Transport mode comparison (Kubernetes vs Maestro)

### Description

This test compares the behavior of two adapters processing the same cluster event: adapter1 (K8s transport) creates resources directly on the hub cluster, while adapter2 (Maestro transport) creates a ManifestWork via Maestro which the agent applies to a target cluster. Both adapters report status independently to the HyperFleet API. This validates that transport selection is a deployment concern and does not affect the adapter framework's status reporting pattern.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet API and Sentinel are deployed
2. Both adapter1 (K8s transport) and adapter2 (Maestro transport) are deployed simultaneously
3. Both adapters subscribe to the same broker topic and process the same events
4. Each adapter has a unique `metadata.name` in its task config (e.g., `adapter1`, `adapter2`)

---

### Test Steps

#### Step 1: Verify both adapters are running
**Action:**
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/name=hyperfleet-adapter --no-headers
```

**Expected Result:**
- Two adapter pods running: `hyperfleet-adapter1-*` and `hyperfleet-adapter2-*`

#### Step 2: Create a cluster (both adapters will process the event)
**Action:**
```bash
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "transport-compare-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name}'
```

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID

#### Step 3: Verify K8s transport adapter (adapter1) created resources directly
**Action:**
```bash
# adapter1 creates a ConfigMap directly in the hyperfleet namespace
kubectl get configmap -n hyperfleet | grep <CLUSTER_ID>-adapter1
```

**Expected Result:**
- ConfigMap `<CLUSTER_ID>-adapter1-configmap` exists in the `hyperfleet` namespace (directly created by adapter1)

#### Step 4: Verify Maestro transport adapter (adapter2) created ManifestWork
**Action:**
```bash
# adapter2 creates a ManifestWork via Maestro, which creates resources in a separate namespace
kubectl get ns | grep <CLUSTER_ID>-adapter2
kubectl get configmap -n <CLUSTER_ID>-adapter2-namespace
```

**Expected Result:**
- Namespace `<CLUSTER_ID>-adapter2-namespace` created by Maestro agent
- ConfigMap `<CLUSTER_ID>-adapter2-configmap` inside that namespace

#### Step 5: Compare status reports from both adapters
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | {adapter: .adapter, conditions: [.conditions[] | {type, status, reason}]}'
```

**Expected Result:**
- Two separate status entries (one per adapter), no overwriting
- Both report the same condition types: Applied, Available, Health
- adapter1 (K8s) shows Applied=True because it directly creates the ConfigMap
- adapter2 (Maestro) shows Applied=True (AppliedManifestWorkComplete)
- Each adapter's status is independently maintained

#### Step 6: Cleanup
**Action:**
```bash
kubectl delete configmap <CLUSTER_ID>-adapter1-configmap -n hyperfleet --ignore-not-found
kubectl delete ns <CLUSTER_ID>-adapter2-namespace --ignore-not-found
```

---

## Test Title: NestedDiscovery within ManifestWork for status bridging

### Description

This test validates the nestedDiscovery mechanism that allows the adapter to discover and access sub-resources within a ManifestWork for status bridging in post-processing CEL expressions. The current adapter2 task config defines nestedDiscoveries for `namespace0` and `configmap0`, which reference sub-resources by name within the ManifestWork's status feedback.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet API, Sentinel, and Adapter (Maestro mode) are deployed
2. Maestro server is deployed with a registered consumer that has an active agent
3. Adapter task config defines nestedDiscoveries and post-processing CEL expressions that reference discovered resources

---

### Test Steps

#### Step 1: Create a cluster and wait for adapter processing
**Action:**
```bash
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "nested-discovery-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name}'
```

Wait ~15 seconds for the adapter to process.

#### Step 2: Verify ManifestWork created and resources deployed
**Action:**
```bash
# Confirm K8s resources were created by Maestro agent
kubectl get ns | grep <CLUSTER_ID>-adapter2
kubectl get configmap -n <CLUSTER_ID>-adapter2-namespace
```

**Expected Result:**
- Namespace and ConfigMap created successfully - ManifestWork content was applied correctly

#### Step 3: Check adapter logs for nestedDiscovery CEL evaluation
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=50 \
  | grep -E "CEL evaluation|namespace0|configmap0|nestedDiscovery"
```

**Expected Result:**
- No CEL evaluation errors in adapter logs
- CEL expressions referencing `resources.namespace0` and `resources.configmap0` evaluate successfully
- Status data (namespace phase, configmap data) is populated from Maestro statusFeedback

#### Step 4: Verify impact on status report
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | select(.adapter == "adapter2") | .data'
```

**Expected Result:**
- `data.namespace.phase` = `"Active"`
- `data.namespace.name` = `"<CLUSTER_ID>-adapter2-namespace"`
- `data.configmap.clusterId` = `"<CLUSTER_ID>"`
- `data.configmap.name` = `"<CLUSTER_ID>-adapter2-configmap"`
- Conditions: Applied=True (AppliedManifestWorkComplete), Available=True (ResourcesAvailable), Health=True (Healthy)

#### Step 5: Verify feedbackRules configuration in Maestro resource bundle
**Action:**
```bash
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID> \
  | jq '.manifest_configs'
```

**Expected Result:**
- `manifestConfigs` contains feedbackRules with JSONPaths for status collection:
  - Namespace: `.status.phase`
  - ConfigMap: `.data`, `.metadata.resourceVersion`

#### Step 6: Cleanup
**Action:**
```bash
kubectl delete ns <CLUSTER_ID>-adapter2-namespace --ignore-not-found
```

---

## Test Title: Multiple targetCluster values for ManifestWork routing

### Description

This test validates that the adapter can route ManifestWorks to different Maestro consumers based on the `targetCluster` template value. The adapter task config uses `targetCluster: "{{ .placementClusterName }}"` where `placementClusterName` is captured from a precondition expression. By changing this expression to point to a different consumer, ManifestWorks are routed to the new consumer.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. HyperFleet environment deployed with Maestro transport adapter
2. Initial consumer `cluster1` already registered
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
kubectl get configmap hyperfleet-adapter2-task -n hyperfleet \
  -o jsonpath='{.data.task-config\.yaml}' > /tmp/adapter2-task-original.yaml

# Modify placementClusterName from "cluster1" to "cluster2"
# In the task config, change:
#   expression: "\"cluster1\""
# To:
#   expression: "\"cluster2\""

# Apply the modified config
kubectl create configmap hyperfleet-adapter2-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-cluster2.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart adapter to pick up new config
kubectl rollout restart deployment/hyperfleet-adapter2 -n hyperfleet
kubectl rollout status deployment/hyperfleet-adapter2 -n hyperfleet --timeout=60s
```

**Expected Result:**
- Adapter restarts with `placementClusterName` = `"cluster2"`

#### Step 3: Create a cluster and verify routing to cluster2
**Action:**
```bash
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "multi-consumer-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name}'
```

Wait ~15 seconds, then check adapter logs:
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=30 \
  | grep "Applying ManifestWork" | head -3
```

**Expected Result:**
- Log shows `Applying ManifestWork cluster2/<CLUSTER_ID>-adapter2` (targeting `cluster2` instead of `cluster1`)

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
- Previously created clusters (before config change) remain on `consumer_name: "cluster1"`

#### Step 5: Restore adapter config and cleanup
**Action:**
```bash
# Restore original config
kubectl create configmap hyperfleet-adapter2-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-original.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-adapter2 -n hyperfleet
kubectl rollout status deployment/hyperfleet-adapter2 -n hyperfleet --timeout=60s
```

---

## Test Title: Maestro server unavailability error handling

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
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --no-headers
kubectl get pods -n maestro -l app=maestro --no-headers
```

**Expected Result:**
- Both adapter2 and maestro pods are `Running`

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
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "maestro-unavail-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name}'
```

**Expected Result:**
- Cluster creation succeeds (API is independent of Maestro)

#### Step 4: Verify adapter error handling (check logs after ~15 seconds)
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=30 \
  | grep -E "FAILED|error|connection refused" | head -5
```

**Expected Result:**
- Adapter logs show Maestro connection error
- Adapter does NOT crash (pod remains Running)

> **Note:** The error code `hyperfleet-adapter-16` is the adapter's internal MaestroError code (code 16 in the adapter's error enumeration, not a gRPC status code).

#### Step 5: Verify error status reported to HyperFleet API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | select(.adapter == "adapter2") | .conditions'
```

**Expected Result:**
- Health condition: `status: "False"` with detailed error message
- Applied condition: `status: "False"`

#### Step 6: Verify adapter pod is still running (no crash)
**Action:**
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --no-headers
```

**Expected Result:**
- Pod is `Running` with 0 restarts

#### Step 7: Restore Maestro and verify recovery
**Action:**
```bash
kubectl scale deployment maestro -n maestro --replicas=1
kubectl rollout status deployment/maestro -n maestro --timeout=120s
```

Then wait ~30 seconds for the Sentinel to re-publish the event and adapter to retry.

```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=50 \
  | grep "<CLUSTER_ID>" | grep -E "operation=|Event execution" | tail -5
```

**Expected Result:**
- Maestro recovers
- Adapter successfully creates ManifestWork on next retry
- Health condition returns to `True`

> **Note:** After Maestro restores, the adapter's CloudEvents client (MQTT-based) takes a few seconds to re-establish the connection. During this window, events fail with "the cloudevents client is not ready". The adapter automatically recovers once the connection is restored.

#### Step 8: Verify recovery - resources created and status updated
**Action:**
```bash
# Verify resources now exist
kubectl get ns | grep <CLUSTER_ID>-adapter2

# Verify status updated
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | select(.adapter == "adapter2") | .conditions[] | select(.type == "Health")'
```

**Expected Result:**
- Namespace created after recovery
- Health returns to `True`

#### Step 9: Cleanup
**Action:**
```bash
kubectl delete ns <CLUSTER_ID>-adapter2-namespace --ignore-not-found
# Ensure Maestro is fully restored
kubectl get pods -n maestro --no-headers
```

---

## Test Title: Invalid targetCluster (consumer not found) error handling

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
kubectl get configmap hyperfleet-adapter2-task -n hyperfleet \
  -o jsonpath='{.data.task-config\.yaml}' > /tmp/adapter2-task-original.yaml

# Modify: change placementClusterName from "cluster1" to "non-existent-cluster"
# In the task config, change:
#   expression: "\"cluster1\""
# To:
#   expression: "\"non-existent-cluster\""

# Apply modified config
kubectl create configmap hyperfleet-adapter2-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-modified.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart adapter
kubectl rollout restart deployment/hyperfleet-adapter2 -n hyperfleet
kubectl rollout status deployment/hyperfleet-adapter2 -n hyperfleet --timeout=60s
```

**Expected Result:**
- Adapter restarts with `placementClusterName` = `"non-existent-cluster"`

#### Step 2: Create a cluster to trigger adapter processing
**Action:**
```bash
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "invalid-consumer-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {"type": "gcp", "gcp": {"projectID": "test", "region": "us-central1"}},
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name}'
```

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID

#### Step 3: Verify error handling for invalid consumer (check logs after ~15 seconds)
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=30 \
  | grep -E "FAILED|error|non-existent" | head -5
```

**Expected Result:**
- Adapter logs show error related to consumer not found
- Error message includes the invalid consumer name
- Adapter does NOT crash

#### Step 4: Verify adapter pod is still running (no crash)
**Action:**
```bash
kubectl get pods -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --no-headers
```

**Expected Result:**
- Pod is `Running` with 0 restarts

#### Step 5: Verify error status reported to API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | select(.adapter == "adapter2") | .conditions'
```

**Expected Result:**
- Health: `status: "False"`, with error details
- Applied: `status: "False"`

#### Step 6: Restore and cleanup
**Action:**
```bash
# Restore original config
kubectl create configmap hyperfleet-adapter2-task -n hyperfleet \
  --from-file=task-config.yaml=/tmp/adapter2-task-original.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-adapter2 -n hyperfleet
kubectl rollout status deployment/hyperfleet-adapter2 -n hyperfleet --timeout=60s
```

> **Important:** Always restore the adapter config after this test to avoid impacting other tests.

---

## Test Title: Maestro TLS authentication validation

### Description

This test validates the Maestro TLS/insecure configuration for the adapter. It covers: (1) insecure mode (`insecure: true`) allows successful connection over plain HTTP, (2) setting `insecure: false` without providing certificates or HTTPS URL causes the adapter to fail at startup with a clear error message. Full mTLS testing requires a Maestro deployment with TLS enabled, which is not available in the test environment.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive/Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. Maestro server is deployed (currently without TLS)
2. The adapter deployment config supports TLS configuration via `spec.clients.maestro`
3. Adapter config backup saved for restoration after test

---

### Test Steps

#### Step 1: Verify current adapter config (insecure mode)
**Action:**
```bash
kubectl get configmap hyperfleet-adapter2-config -n hyperfleet \
  -o jsonpath='{.data.adapter-config\.yaml}' | grep -A2 "insecure"
```

**Expected Result:**
- Config shows `insecure: true`

#### Step 2: Verify insecure mode works correctly
**Action:**
- Create a cluster and confirm ManifestWork creation succeeds (see Test 1)

**Expected Result:**
- Adapter connects to Maestro without TLS verification
- ManifestWork operations succeed
- This is verified by successful execution of Test 1

#### Step 3: Set `insecure: false` without providing certificates
**Action:**
```bash
# Backup original config
kubectl get configmap hyperfleet-adapter2-config -n hyperfleet \
  -o jsonpath='{.data.adapter-config\.yaml}' > /tmp/adapter2-config-original.yaml

# Modify insecure: true → insecure: false
# (keep httpServerAddress as http:// - no certs provided)

kubectl create configmap hyperfleet-adapter2-config -n hyperfleet \
  --from-file=adapter-config.yaml=/tmp/adapter2-config-tls.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-adapter2 -n hyperfleet
```

**Expected Result:**
- Adapter fails to start because HTTP URL is used with `insecure: false`

#### Step 4: Verify startup error message
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=20 \
  | grep -E "Error|error|failed"
```

**Expected Result:**
- Adapter fails at startup (fail-fast), not at runtime
- Error code `hyperfleet-adapter-17` with clear message about TLS/scheme mismatch
- Error message suggests corrective action: use `https://` URL or set `Insecure=true`

#### Step 5: Restore adapter config
**Action:**
```bash
kubectl create configmap hyperfleet-adapter2-config -n hyperfleet \
  --from-file=adapter-config.yaml=/tmp/adapter2-config-original.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-adapter2 -n hyperfleet
kubectl rollout status deployment/hyperfleet-adapter2 -n hyperfleet --timeout=120s
```

**Expected Result:**
- Adapter restores to `insecure: true` and starts successfully

