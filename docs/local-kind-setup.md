# Local E2E Testing with kind

Run E2E tests locally using [kind](https://kind.sigs.k8s.io/) and RabbitMQ — no GCP dependencies.

## Prerequisites

- **Go** 1.25+ — [go.dev](https://go.dev/doc/install)
- **Docker** — [docker.com](https://www.docker.com/)
- **kind** — [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- **kubectl** 1.28+ — [kubernetes.io](https://kubernetes.io/docs/tasks/tools/)
- **helm** 3+ — [helm.sh](https://helm.sh/docs/intro/install/)
- **git**, **curl** — for cloning repos and downloading OpenAPI specs

## Repository Layout

This setup expects HyperFleet repos to be siblings under one parent directory.
The default is `~/projects/`, but you can use any layout via `PROJECTS_DIR`:

```bash
export PROJECTS_DIR=~/work/redhat
# or just for one command:
PROJECTS_DIR=~/code make local-up
```

## Clone Repositories

All component repos are required — images are built locally from source.

```bash
mkdir -p "${PROJECTS_DIR:-~/projects}"
cd "${PROJECTS_DIR:-~/projects}"
for repo in hyperfleet-e2e hyperfleet-infra hyperfleet-api hyperfleet-sentinel hyperfleet-adapter; do
  git clone https://github.com/openshift-hyperfleet/${repo}.git
done
```

> **Maestro** (`cl-maestro` adapter) uses the [OCM Maestro](https://github.com/openshift-online/maestro) resource management service to manage ManifestWorks. It is installed automatically by `make local-up` via the `hyperfleet-infra` repo. The `status-reporter` sidecar image is auto-cloned and built if not present locally.

## Quick Start

```bash
# One command: create cluster + deploy everything + port-forward
make local-up

# Run tests
make local-test
```

For individual steps, use the script directly:

```bash
./deploy-scripts/kind-local.sh setup        # Create cluster, RabbitMQ, Maestro, load images
./deploy-scripts/kind-local.sh deploy       # Deploy API, sentinels, adapters
./deploy-scripts/kind-local.sh port-forward # Forward API to localhost:8000, Maestro to localhost:8100
./deploy-scripts/kind-local.sh undeploy     # Remove all components + stop port-forwards
```

## What Each Command Does

| Command | What it does |
|---------|-------------|
| `make local-up` | Creates kind cluster, installs RabbitMQ + Maestro, builds and loads images, deploys all components (JWT disabled), port-forwards API (`localhost:8000`) and Maestro (`localhost:8100`) |
| `make local-test` | Builds E2E binary and runs tier0 tests against `localhost:8000` |
| `make local-undeploy` | Removes all deployed components (incl. Maestro namespace), stops port-forwards |
| `make local-rebuild` | Rebuilds all images (cached), restarts all deployments |
| `make local-rebuild COMPONENT=adapter` | Rebuilds only the adapter image, restarts only adapter pods |
| `make local-rebuild NO_CACHE=1` | Rebuilds all images without Docker cache (use after `git pull`) |

## Rebuilding After Code Changes

```bash
# Rebuild and restart a specific component (uses Docker cache)
make local-rebuild COMPONENT=adapter

# Force rebuild without cache (after git pull with upstream changes)
make local-rebuild COMPONENT=adapter NO_CACHE=1

# Rebuild everything
make local-rebuild
```

Valid component names: `api`, `sentinel`, `adapter`, `sr` (status-reporter).

> **Note on `COMPONENT=sr`:** `status-reporter` runs as a sidecar inside Job pods created by adapters at runtime. Rebuilding the image does not affect existing Jobs — the new image is picked up on the next adapter task execution. To force pickup, trigger a new cluster operation (create/update/delete a test cluster).

> **Note:** Port-forwards usually survive pod restarts but may break during rolling updates. If `localhost:8000` stops responding after a rebuild, re-run `./deploy-scripts/kind-local.sh port-forward`.

## Running Specific Tests

```bash
# By suite name (--focus uses regex on test description)
HYPERFLEET_API_URL=http://localhost:8000 \
MAESTRO_URL=http://localhost:8100 \
./bin/hyperfleet-e2e test \
  --focus="\[Suite: cluster\]" --log-level=info

# By label (same mechanism as make local-test)
HYPERFLEET_API_URL=http://localhost:8000 \
MAESTRO_URL=http://localhost:8100 \
./bin/hyperfleet-e2e test \
  --label-filter=tier0 --log-level=info
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_DIR` | `~/projects` | Parent directory containing component repos |
| `KIND_CLUSTER` | `kind` | Name of the kind cluster |
| `NAMESPACE` | `hyperfleet` | Kubernetes namespace for components |
| `INFRA_DIR` | `~/projects/hyperfleet-infra` | Path to hyperfleet-infra repo |
| `CLUSTER_ADAPTERS` | `cl-namespace,cl-job,cl-deployment,cl-maestro` | Cluster adapters to deploy |
| `NODEPOOL_ADAPTERS` | `np-configmap` | NodePool adapters to deploy |
| `RABBITMQ_URL` | `amqp://guest:guest@rabbitmq:5672` | RabbitMQ AMQP connection URL |
| `MAESTRO_NS` | `maestro` | Kubernetes namespace for Maestro |
| `MAESTRO_CONSUMER` | `cluster1` | Maestro consumer name |
| `MAESTRO_LOCAL_PORT` | `8100` | Local port for Maestro port-forward |

## Port-Forward Lifecycle

Port-forwards run as background processes. They are:
- **Started** by `make local-up` or `./deploy-scripts/kind-local.sh port-forward`
- **Stopped** by `make local-undeploy` or by re-running `port-forward` (kills previous ones first)
- **Lost** if the terminal closes, the laptop sleeps, or the process is otherwise interrupted

To re-establish port-forwards without redeploying:

```bash
./deploy-scripts/kind-local.sh port-forward
```

## Troubleshooting

**ImagePullBackOff** — Image not loaded into kind. Run `kind load docker-image <image>`. The deploy script patches `imagePullPolicy` automatically, but adapter-created resources (Jobs, Deployments) use the policy from adapter task configs.

**db-migrate crashing** — API binary doesn't match Helm chart. Rebuild with `make local-rebuild COMPONENT=api NO_CACHE=1`.

**Cluster not reaching Reconciled** — Check adapter logs: `kubectl logs -l app.kubernetes.io/instance=adapter-clusters-cl-job -n hyperfleet --tail=20`. Look for `ImagePullBackOff` on sidecar containers.

**Docker cache stale** — Use `make local-rebuild NO_CACHE=1` after `git pull`. Docker layer caching can silently use old source.

**Docker disk full** — Repeated builds consume disk. Prune old images: `docker image prune`.

**Connection refused on localhost:8000** — Port-forwards are dead. Re-run: `./deploy-scripts/kind-local.sh port-forward`.

**Wrong kubectl context** — Script requires `kind-kind` as current context. Switch with: `kubectl config use-context kind-kind`.

**`make local-undeploy`** removes deployed components and Maestro namespace but leaves the kind cluster running. To fully clean up: `kind delete cluster`.
