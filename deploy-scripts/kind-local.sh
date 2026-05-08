#!/usr/bin/env bash
# kind-local.sh - Local kind cluster lifecycle for E2E testing
#
# Usage:
#   ./deploy-scripts/kind-local.sh up        # Create cluster, deploy components, port-forward services
#   ./deploy-scripts/kind-local.sh setup     # Create cluster, install RabbitMQ, load images
#   ./deploy-scripts/kind-local.sh deploy    # Deploy API, sentinels, adapters
#   ./deploy-scripts/kind-local.sh undeploy  # Remove all components
#   ./deploy-scripts/kind-local.sh port-forward # Forward API and Maestro locally
#
# Env vars:
#   NAMESPACE          (hyperfleet)
#   PROJECTS_DIR       (~/projects)
#   INFRA_DIR          (~/projects/hyperfleet-infra)
#   CLUSTER_ADAPTERS   (cl-namespace,cl-job,cl-deployment,cl-maestro)
#   NODEPOOL_ADAPTERS  (np-configmap)
#   RABBITMQ_URL       (amqp://guest:guest@rabbitmq:5672)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

NAMESPACE="${NAMESPACE:-hyperfleet}"
PROJECTS_DIR="${PROJECTS_DIR:-${HOME}/projects}"
INFRA_DIR="${INFRA_DIR:-${PROJECTS_DIR}/hyperfleet-infra}"
KIND_CLUSTER="${KIND_CLUSTER:-kind}"
KIND_CONTEXT="kind-${KIND_CLUSTER}"
CLUSTER_ADAPTERS="${CLUSTER_ADAPTERS:-cl-namespace,cl-job,cl-deployment,cl-maestro}"
MAESTRO_NS="${MAESTRO_NS:-maestro}"
MAESTRO_CONSUMER="${MAESTRO_CONSUMER:-cluster1}"
NODEPOOL_ADAPTERS="${NODEPOOL_ADAPTERS:-np-configmap}"
RABBITMQ_URL="${RABBITMQ_URL:-amqp://guest:guest@rabbitmq:5672}"
MAESTRO_LOCAL_PORT="${MAESTRO_LOCAL_PORT:-8100}"
PF_PID_FILE="/tmp/hyperfleet-kind-pf-${KIND_CLUSTER}.pids"

cleanup_port_forwards() {
  if [[ -f "${PF_PID_FILE}" ]]; then
    while read -r pid; do
      kill "${pid}" 2>/dev/null || true
    done < "${PF_PID_FILE}"
    rm -f "${PF_PID_FILE}"
  fi
  # Kill any stale kubectl port-forward processes for our ports
  pgrep -f "kubectl.*port-forward.*8000" | xargs kill 2>/dev/null || true
  pgrep -f "kubectl.*port-forward.*${MAESTRO_LOCAL_PORT}" | xargs kill 2>/dev/null || true
}

require_kind_context() {
  local current
  current="$(kubectl config current-context 2>/dev/null || echo "")"
  if [[ "${current}" != "${KIND_CONTEXT}" ]]; then
    echo "ERROR: current kubectl context is '${current}', expected '${KIND_CONTEXT}'"
    echo "Run: kubectl config use-context ${KIND_CONTEXT}"
    exit 1
  fi
}

cmd_setup() {
  echo "=== Creating kind cluster ==="
  kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$" || kind create cluster --name "${KIND_CLUSTER}"
  require_kind_context
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

  if [[ ! -d "${INFRA_DIR}" ]]; then
    echo "ERROR: hyperfleet-infra not found at ${INFRA_DIR}"
    echo "Clone it: git clone https://github.com/openshift-hyperfleet/hyperfleet-infra.git ${INFRA_DIR}"
    exit 1
  fi

  echo "=== Installing RabbitMQ ==="
  if [[ ! -f "${INFRA_DIR}/manifests/rabbitmq.yaml" ]]; then
    echo "ERROR: rabbitmq.yaml not found at ${INFRA_DIR}/manifests/"
    echo "Check that hyperfleet-infra is up to date: cd ${INFRA_DIR} && git pull"
    exit 1
  fi
  kubectl apply -f "${INFRA_DIR}/manifests/rabbitmq.yaml" --namespace "${NAMESPACE}"
  echo "Waiting for RabbitMQ..."
  local retries=60
  until kubectl get pod -l app=rabbitmq -n "${NAMESPACE}" --no-headers 2>/dev/null | grep -q .; do
    ((retries--)) || { echo "ERROR: Timed out waiting for RabbitMQ pod (120s)"; exit 1; }
    sleep 2
  done
  kubectl wait --for=condition=ready pod -l app=rabbitmq --namespace "${NAMESPACE}" --timeout=120s

  echo "=== Installing Maestro ==="
  make -C "${INFRA_DIR}" install-maestro NAMESPACE="${MAESTRO_NS}"
  make -C "${INFRA_DIR}" create-maestro-consumer MAESTRO_CONSUMER="${MAESTRO_CONSUMER}" NAMESPACE="${MAESTRO_NS}"

  echo "=== Loading images ==="
  "${SCRIPT_DIR}/kind-build-images.sh" "$@"
}

cmd_deploy() {
  require_kind_context

  # Keep the API install inline until deploy-clm.sh can pass local-only Helm overrides.
  local api_chart="${PROJECTS_DIR}/hyperfleet-api/charts"
  if [[ ! -d "${api_chart}" ]]; then
    echo "ERROR: API chart not found at ${api_chart}"
    echo "Clone hyperfleet-api into ${PROJECTS_DIR} or set PROJECTS_DIR"
    exit 1
  fi

  echo "=== Deploying API (JWT disabled, chart: ${api_chart}) ==="
  helm upgrade --install api "${api_chart}" \
    --namespace "${NAMESPACE}" --create-namespace \
    --set image.registry=registry.ci.openshift.org \
    --set image.repository=ci/hyperfleet-api \
    --set image.tag=latest \
    --set image.pullPolicy=IfNotPresent \
    --set service.type=ClusterIP \
    --set "config.server.jwt.enabled=false" \
    --set "config.adapters.required.cluster={${CLUSTER_ADAPTERS}}" \
    --set "config.adapters.required.nodepool={${NODEPOOL_ADAPTERS}}" \
    --wait --timeout 3m

  echo "=== Deploying Sentinels and Adapters ==="
  SENTINEL_BROKER_RABBITMQ_URL="${RABBITMQ_URL}" \
  ADAPTER_BROKER_RABBITMQ_URL="${RABBITMQ_URL}" \
  ADAPTER_BROKER_TYPE=rabbitmq \
  SENTINEL_BROKER_TYPE=rabbitmq \
  IMAGE_PULL_POLICY=IfNotPresent \
  NAMESPACE="${NAMESPACE}" \
  API_SERVICE_TYPE=ClusterIP \
  INSTALL_API=false \
  CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="${CLUSTER_ADAPTERS}" \
  NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="${NODEPOOL_ADAPTERS}" \
  "${SCRIPT_DIR}/deploy-clm.sh" --action install
}

cmd_undeploy() {
  require_kind_context

  cleanup_port_forwards

  helm uninstall api --namespace "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

  NAMESPACE="${NAMESPACE}" \
  CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="${CLUSTER_ADAPTERS}" \
  NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="${NODEPOOL_ADAPTERS}" \
  "${SCRIPT_DIR}/deploy-clm.sh" --action uninstall --delete-k8s-resources

  kubectl delete namespace "${MAESTRO_NS}" --ignore-not-found 2>/dev/null || true
}

cmd_port_forward() {
  require_kind_context
  cleanup_port_forwards

  kubectl --context "${KIND_CONTEXT}" port-forward -n "${NAMESPACE}" svc/hyperfleet-api 8000:8000 &
  echo $! > "${PF_PID_FILE}"
  kubectl --context "${KIND_CONTEXT}" port-forward -n "${MAESTRO_NS}" svc/maestro "${MAESTRO_LOCAL_PORT}":8000 &
  echo $! >> "${PF_PID_FILE}"

  local api_endpoint="http://localhost:8000/api/hyperfleet/v1/clusters"
  local api_ready=false
  for _ in {1..10}; do
    if curl -sf "${api_endpoint}" > /dev/null 2>&1; then
      api_ready=true
      break
    fi
    sleep 2
  done
  if [[ "${api_ready}" == "true" ]]; then
    echo "API ready at http://localhost:8000"
  else
    echo "ERROR: API not reachable at localhost:8000 (port already in use?)"
    exit 1
  fi
  if curl -sf http://localhost:"${MAESTRO_LOCAL_PORT}"/api/maestro/v1/consumers > /dev/null 2>&1; then
    echo "Maestro ready at http://localhost:${MAESTRO_LOCAL_PORT}"
  else
    echo "WARNING: Maestro not reachable at localhost:${MAESTRO_LOCAL_PORT} (maestro tests may fail)"
  fi
}

cmd_up() {
  cmd_setup "$@"
  cmd_deploy
  cmd_port_forward
}

case "${1:-}" in
  up)           shift; cmd_up "$@" ;;
  setup)        shift; cmd_setup "$@" ;;
  deploy)       cmd_deploy ;;
  undeploy)     cmd_undeploy ;;
  port-forward) cmd_port_forward ;;
  *)
    echo "Usage: $0 {up|setup|deploy|undeploy|port-forward}"
    echo ""
    echo "  up [COMPONENTS...]      Full setup: create cluster + deploy + port-forward"
    echo "  setup [COMPONENTS...]   Create kind cluster, install RabbitMQ, load images"
    echo "  deploy                  Deploy API, sentinels, and adapters"
    echo "  undeploy                Remove all components"
    echo "  port-forward            Forward API and Maestro to localhost"
    echo ""
    echo "  COMPONENTS args passed to kind-build-images.sh (e.g. 'adapter')"
    exit 1
    ;;
esac
