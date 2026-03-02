# Feature: Adapter Framework - Maestro Transportation Layer

## Table of Contents

1. [Adapter can create ManifestWork via Maestro transport](#test-title-adapter-can-create-manifestwork-via-maestro-transport)
2. [K8s resources can be created and status reported via ManifestWork](#test-title-k8s-resources-can-be-created-and-status-reported-via-manifestwork)
3. [Adapter can skip ManifestWork creation when generation is unchanged](#test-title-adapter-can-skip-manifestwork-creation-when-generation-is-unchanged)
4. [Both K8s and Maestro transport adapters can process the same event independently](#test-title-both-k8s-and-maestro-transport-adapters-can-process-the-same-event-independently)
5. [Adapter can bridge sub-resource status via NestedDiscovery in ManifestWork](#test-title-adapter-can-bridge-sub-resource-status-via-nesteddiscovery-in-manifestwork)
6. [Adapter can route ManifestWork to correct consumer based on targetCluster](#test-title-adapter-can-route-manifestwork-to-correct-consumer-based-on-targetcluster)
7. [Adapter can handle Maestro server unavailability gracefully](#test-title-adapter-can-handle-maestro-server-unavailability-gracefully)
8. [Adapter can handle invalid targetCluster (consumer not found) gracefully](#test-title-adapter-can-handle-invalid-targetcluster-consumer-not-found-gracefully)
9. [Adapter can reject invalid Maestro TLS configuration at startup](#test-title-adapter-can-validate-maestro-tls-authentication-configuration)

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
make install-adapter2   # Maestro transport adapter (<ADAPTER_NAME> in this document refers to this adapter instance name, default: adapter2)

# 9. Port-forward HyperFleet API for local access
kubectl port-forward -n hyperfleet svc/hyperfleet-api 8000:8000 &
```

---

## Test Title: Adapter can create ManifestWork via Maestro transport

### Description

This test validates the ManifestWork creation workflow: creating a cluster via the HyperFleet API triggers the adapter to create a ManifestWork (resource bundle) on the Maestro server targeting the correct consumer. The ManifestWork must contain the expected inline manifests and correct metadata (labels, annotations).

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

1. HyperFleet API and Sentinel services are deployed and running successfully
2. Maestro is deployed and running successfully
3. At least one Maestro consumer is registered (e.g., `cluster1`)
4. Adapter is deployed in Maestro transport mode (`transport.client: "maestro"`)

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

#### Step 2: Verify ManifestWork was created on Maestro (via Maestro HTTP API)
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

Example output:
```json
{
  "id": "auto-generated unique ID by Maestro",
  "consumer_name": "cluster1, the target consumer this ManifestWork is routed to",
  "version": 1, // initial version on creation, increments on each update
  "manifest_names": [
    "<CLUSTER_ID>-<ADAPTER_NAME>-namespace, the Namespace manifest name",
    "<CLUSTER_ID>-<ADAPTER_NAME>-configmap, the ConfigMap manifest name"
  ]
}
```

#### Step 3: Verify ManifestWork metadata (labels and annotations)
**Action:**
```bash
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID> \
  | jq '.metadata | {labels, annotations}'
```

**Expected Result:**

1. **Code logic additions** (dynamically set by adapter code):
   - `consumer_name`: set to the resolved `targetCluster` value (e.g., `cluster1`)
   - `hyperfleet.io/generation` (label + annotation): set from the cluster's current generation value

2. **Manifest template configuration** (from adapter task config template):
   - Labels: `hyperfleet.io/cluster-id`, `hyperfleet.io/adapter`
   - Annotations: `hyperfleet.io/managed-by`

Example output:
```json
{
  "labels": {
    "hyperfleet.io/cluster-id": "<CLUSTER_ID>",
    "hyperfleet.io/generation": "1, code logic: set from cluster generation",
    "hyperfleet.io/adapter": "<ADAPTER_NAME>, template config: identifies the adapter"
  },
  "annotations": {
    "hyperfleet.io/generation": "1, code logic: used for idempotency check",
    "hyperfleet.io/managed-by": "<ADAPTER_NAME>, template config: indicates managing adapter"
  }
}
```

#### Step 4: Cleanup
**Action:**
```bash
# Delete the resource bundle on Maestro (via Maestro API)
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID>
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>
> ```

---

## Test Title: K8s resources can be created and status reported via ManifestWork

### Description

This test validates that the Maestro agent correctly applies the ManifestWork content to the target cluster, creating the expected K8s resources (Namespace, ConfigMap). It also verifies that the adapter reports status back to the HyperFleet API with Applied/Available/Health conditions.

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

1. HyperFleet API and Sentinel services are deployed and running successfully
2. Maestro is deployed and running successfully with an active agent
3. At least one Maestro consumer is registered (e.g., `cluster1`)
4. Adapter is deployed in Maestro transport mode (`transport.client: "maestro"`)

---

### Test Steps

#### Step 1: Create a cluster via HyperFleet API
**Action:**
```bash
curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "k8s-resources-test-'$(date +%Y%m%d-%H%M%S)'",
    "spec": {
      "platform": {
        "type": "gcp",
        "gcp": {"projectID": "test-project", "region": "us-central1"}
      },
      "release": {"version": "4.14.0"}
    }
  }' | jq '{id: .id, name: .name, generation: .generation}'
```

Wait ~15 seconds for the adapter to process and Maestro agent to apply.

**Expected Result:**
- API returns HTTP 201 with a valid cluster ID and `generation: 1`

#### Step 2: Verify K8s resources created by Maestro agent on target cluster
**Action:**
```bash
# Check namespace created
kubectl get ns | grep <CLUSTER_ID>

# Check ConfigMap created in the namespace
kubectl get configmap -n <CLUSTER_ID>-<ADAPTER_NAME>-namespace
```

**Expected Result:**
- Namespace `<CLUSTER_ID>-<ADAPTER_NAME>-namespace` exists and is `Active`
- ConfigMap `<CLUSTER_ID>-<ADAPTER_NAME>-configmap` exists in that namespace

#### Step 3: Verify adapter status report to HyperFleet API
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>/statuses \
  | jq '.items[] | select(.adapter == "adapter2")'
```

**Expected Result:**
- Status entry with `adapter: "<ADAPTER_NAME>"`
- Three conditions: Applied, Available, Health
- `observed_generation: 1`
- `observed_time` is present and is a valid timestamp
- Applied=True (AppliedManifestWorkComplete), Available=True (ResourcesAvailable), Health=True (Healthy)

Example output:
```json
{
  "adapter": "<ADAPTER_NAME>, the adapter name that reported this status",
  "observed_generation": 1,
  "observed_time": "2026-01-01T00:00:00Z, timestamp of when this status was observed",
  "conditions": [
    {
      "type": "Applied",
      "status": "True",
      "reason": "AppliedManifestWorkComplete, all manifests in ManifestWork were applied"
    },
    {
      "type": "Available",
      "status": "True",
      "reason": "ResourcesAvailable, all resources are available on the target cluster"
    },
    {
      "type": "Health",
      "status": "True",
      "reason": "Healthy, adapter health check passed"
    }
  ]
}
```

#### Step 4: Cleanup
**Action:**
```bash
# Delete the namespace created by Maestro agent
kubectl delete ns <CLUSTER_ID>-<ADAPTER_NAME>-namespace --ignore-not-found

# Delete the resource bundle on Maestro (via Maestro API)
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID>
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>
> ```

---

## Test Title: Adapter can skip ManifestWork creation when generation is unchanged

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

#### Step 2: Verify "Skip" operation on subsequent processing (same generation)
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

#### Step 3: Verify Maestro resource version does not change on Skip
**Action:**
```bash
# Query the resource bundle version from Maestro - should stay at version 1
kubectl exec -n maestro deployment/maestro -- \
  curl -s http://localhost:8000/api/maestro/v1/resource-bundles \
  | jq '.items[] | {id: .id, version: .version}'
```

**Expected Result:**
- `version: 1` remains unchanged across multiple Skip operations

#### Step 4: (Post-MVP) Update operation when generation changes

> **Note:** The "Update" operation cannot currently be tested end-to-end because the HyperFleet API does not support cluster PATCH/UPDATE to increment the generation. This is a post-MVP feature. The adapter code supports generation-based update via JSON merge patch when `generation` changes, which can be observed in unit tests.

#### Step 5: Cleanup
**Action:**
```bash
kubectl delete ns <CLUSTER_ID>-<ADAPTER_NAME>-namespace --ignore-not-found
kubectl exec -n maestro deployment/maestro -- \
  curl -s -X DELETE http://localhost:8000/api/maestro/v1/resource-bundles/<RESOURCE_BUNDLE_ID>
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>
> ```

---

## Test Title: Both K8s and Maestro transport adapters can process the same event independently

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

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>
> ```

---

## Test Title: Adapter can bridge sub-resource status via NestedDiscovery in ManifestWork

### Description

This test validates the nestedDiscovery mechanism that allows the adapter to discover and access sub-resources within a ManifestWork for status bridging in post-processing CEL expressions.

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

1. HyperFleet API, Sentinel, and Adapter (Maestro mode) are deployed
2. Maestro server is deployed with a registered consumer that has an active agent
3. Adapter task config defines nestedDiscoveries (e.g., `namespace0` and `configmap0`) that reference sub-resources by name within the ManifestWork's status feedback
4. Adapter task config defines post-processing CEL expressions that reference discovered resources (e.g., `resources.namespace0`, `resources.configmap0`)

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
kubectl get ns | grep <CLUSTER_ID>-<ADAPTER_NAME>
kubectl get configmap -n <CLUSTER_ID>-<ADAPTER_NAME>-namespace
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
- `data.namespace.name` = `"<CLUSTER_ID>-<ADAPTER_NAME>-namespace"`
- `data.configmap.clusterId` = `"<CLUSTER_ID>"`
- `data.configmap.name` = `"<CLUSTER_ID>-<ADAPTER_NAME>-configmap"`
- Conditions: Applied=True (AppliedManifestWorkComplete), Available=True (ResourcesAvailable), Health=True (Healthy)

Example output:
```json
{
  "namespace": {
    "phase": "Active, the namespace status phase from statusFeedback",
    "name": "<CLUSTER_ID>-<ADAPTER_NAME>-namespace, discovered via nestedDiscovery namespace0"
  },
  "configmap": {
    "clusterId": "<CLUSTER_ID>, extracted from ConfigMap data via statusFeedback",
    "name": "<CLUSTER_ID>-<ADAPTER_NAME>-configmap, discovered via nestedDiscovery configmap0"
  }
}
```

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

Example output:
```json
[
  {
    "resourceIdentifier": {
      "name": "<CLUSTER_ID>-<ADAPTER_NAME>-namespace",
      "group": "",
      "resource": "namespaces"
    },
    "feedbackRules": [
      {"type": "JSONPaths", "jsonPaths": [{"name": "phase", "path": ".status.phase"}]}
    ]
  },
  {
    "resourceIdentifier": {
      "name": "<CLUSTER_ID>-<ADAPTER_NAME>-configmap",
      "group": "",
      "resource": "configmaps",
      "namespace": "<CLUSTER_ID>-<ADAPTER_NAME>-namespace"
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

#### Step 6: Cleanup
**Action:**
```bash
kubectl delete ns <CLUSTER_ID>-<ADAPTER_NAME>-namespace --ignore-not-found
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>
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
- Health condition: `status: "False"`, error message should contain key points like `connection refused` or `hyperfleet-adapter-16`
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

**Expected Result:**
- Maestro pod becomes `Running`

> **Note:** After Maestro restores, the adapter's CloudEvents client (MQTT-based) takes a few seconds to re-establish the connection. During this window, events fail with "the cloudevents client is not ready". The adapter automatically recovers once the connection is restored.

#### Step 8: Verify recovery - resources created and status updated
**Action:**
```bash
# Verify resources now exist
kubectl get ns | grep <CLUSTER_ID>-<ADAPTER_NAME>

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
kubectl delete ns <CLUSTER_ID>-<ADAPTER_NAME>-namespace --ignore-not-found
```

> **Note:** This is a workaround cleanup method. Once the HyperFleet API supports DELETE operations for clusters, this step should be replaced with:
> ```bash
> curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/<CLUSTER_ID>
> ```

```bash
# Ensure Maestro is fully restored
kubectl get pods -n maestro --no-headers
```

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
- Health: `status: "False"`, error message should contain key points like `non-existent-cluster` or `consumer` not found
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

## Test Title: Adapter can reject invalid Maestro TLS configuration at startup

### Description

This test validates that the adapter fails at startup with a clear error message when `insecure: false` is configured without providing certificates or HTTPS URL. Full mTLS testing requires a Maestro deployment with TLS enabled, which is not available in the test environment.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-12 |
| **Updated** | 2026-02-26 |

---

### Preconditions

1. Maestro server is deployed (currently without TLS)
2. The adapter Helm chart is available for deployment

---

### Test Steps

#### Step 1: Deploy a new adapter with `insecure: false` configuration
**Action:**
- Deploy a new adapter instance configured with `insecure: false` while keeping the HTTP URL (no certs provided):
```bash
make install-adapter2 ADAPTER_INSECURE=false
```

> **Note:** `ADAPTER_INSECURE` is not yet supported as a Makefile parameter. As a workaround, manually set `insecure: false` in `helm/adapter2/adapter-config.yaml` and run `make install-adapter2`.

**Expected Result:**
- Adapter pod enters `CrashLoopBackOff` because HTTP URL is used with `insecure: false`

#### Step 2: Verify startup error message
**Action:**
```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/instance=hyperfleet-adapter2 --tail=20 \
  | grep -E "Error|error|failed"
```

**Expected Result:**
- Adapter fails at startup (fail-fast), not at runtime
- Error code `hyperfleet-adapter-17` with clear message about TLS/scheme mismatch
- Error message suggests corrective action: use `https://` URL or set `Insecure=true`

#### Step 3: Cleanup
**Action:**
- Redeploy adapter with the correct configuration:
```bash
make install-adapter2
```

**Expected Result:**
- Adapter starts successfully with default `insecure: true` configuration

