#!/usr/bin/env bash
# kind-build-images.sh - Build and load HyperFleet images into kind
#
# Usage:
#   ./deploy-scripts/kind-build-images.sh                    # Build all
#   ./deploy-scripts/kind-build-images.sh adapter             # Build only adapter
#   ./deploy-scripts/kind-build-images.sh --no-cache adapter  # Force rebuild
#
# Components: api, sentinel, adapter, sr (status-reporter)
# Env vars: PROJECTS_DIR (~/projects), KIND_CLUSTER (kind)

set -euo pipefail

PROJECTS_DIR="${PROJECTS_DIR:-${HOME}/projects}"
CI_REGISTRY="registry.ci.openshift.org/ci"
NO_CACHE=""
KIND_CLUSTER="${KIND_CLUSTER:-kind}"
BUILD_LIST=()

COMPONENTS=(
  "hyperfleet-api:api"
  "hyperfleet-sentinel:sentinel"
  "hyperfleet-adapter:adapter"
  "status-reporter:sr"
)

valid_component_names() {
  local entry
  local names=()

  for entry in "${COMPONENTS[@]}"; do
    names+=("${entry##*:}")
  done

  printf '%s' "${names[*]}"
}

is_valid_component() {
  local wanted="$1"
  local entry

  for entry in "${COMPONENTS[@]}"; do
    [[ "${wanted}" == "${entry##*:}" ]] && return 0
  done

  return 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-cache) NO_CACHE="--no-cache"; shift ;;
    -h|--help)
      echo "Usage: $0 [--no-cache] [COMPONENT...]"
      echo ""
      echo "Builds and loads HyperFleet images into kind from local repos."
      echo "No args = build all. Named args = build only those."
      echo ""
      echo "Components: api, sentinel, adapter, sr (status-reporter)"
      echo ""
      echo "Env: PROJECTS_DIR=$PROJECTS_DIR  KIND_CLUSTER=$KIND_CLUSTER"
      exit 0
      ;;
    -*) echo "Unknown option: $1"; exit 1 ;;
    *) BUILD_LIST+=("$1"); shift ;;
  esac
done

# Validate component names
if [[ ${#BUILD_LIST[@]} -gt 0 ]]; then
  for s in "${BUILD_LIST[@]}"; do
    if ! is_valid_component "${s}"; then
      echo "ERROR: Unknown component '${s}'. Valid: $(valid_component_names)"
      exit 1
    fi
  done
fi

should_build() {
  [[ ${#BUILD_LIST[@]} -eq 0 ]] && return 0
  local short="$1"
  for s in "${BUILD_LIST[@]}"; do
    [[ "${s}" == "${short}" ]] && return 0
  done
  return 1
}

echo "=== Building HyperFleet images (cluster: ${KIND_CLUSTER}) ==="

for entry in "${COMPONENTS[@]}"; do
  IFS=: read -r name short <<< "${entry}"

  should_build "${short}" || continue

  dir="${PROJECTS_DIR}/${name}"
  if [[ ! -d "${dir}" ]]; then
    if [[ "${name}" == "status-reporter" ]]; then
      echo "[CLONE] ${name} (not found locally, cloning from GitHub)..."
      git clone --depth 1 https://github.com/openshift-hyperfleet/status-reporter.git "${dir}"
    else
      echo "[ERROR] ${name} not found at ${dir}"
      exit 1
    fi
  fi

  echo "[BUILD] ${name}..."
  docker build ${NO_CACHE} -t "${CI_REGISTRY}/${name}:latest" "${dir}"

  echo "[LOAD]  ${name} -> kind..."
  kind load docker-image "${CI_REGISTRY}/${name}:latest" --name "${KIND_CLUSTER}"
  echo ""
done

echo "=== Done ==="
