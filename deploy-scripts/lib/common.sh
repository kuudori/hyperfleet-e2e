#!/usr/bin/env bash

# common.sh - Common utilities for CLM deployment scripts
#
# This module provides shared functionality used across all deployment scripts:
# - Logging functions
# - Dependency checking
# - Kubernetes context validation

# ============================================================================
# Logging Functions
# ============================================================================

log_info() {
    echo "[INFO] $*"
}

log_success() {
    echo "[SUCCESS] $*"
}

log_warning() {
    echo "[WARNING] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

log_verbose() {
    if [[ "${VERBOSE}" == "true" ]]; then
        echo "[VERBOSE] $*"
    fi
}

log_section() {
    echo
    echo "==================================================================="
    echo "$*"
    echo "==================================================================="
}

# ============================================================================
# Dependency Checking
# ============================================================================

check_dependencies() {
    log_section "Checking Dependencies"

    local missing_deps=()

    local deps=("kubectl" "helm" "git")
    for dep in "${deps[@]}"; do
        if ! command -v "${dep}" &> /dev/null; then
            missing_deps+=("${dep}")
            log_error "Required dependency '${dep}' not found"
        else
            local version
            case "${dep}" in
                kubectl)
                    version=$(kubectl version --client --short 2>/dev/null | head -n1 || echo "unknown")
                    ;;
                helm)
                    version=$(helm version --short 2>/dev/null || echo "unknown")
                    ;;
                git)
                    version=$(git --version || echo "unknown")
                    ;;
            esac
            log_verbose "Found ${dep}: ${version}"
        fi
    done

    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_error "Please install the missing dependencies and try again"
        return 1
    fi

    log_success "All dependencies are available"
    return 0
}

# ============================================================================
# Kubernetes Context Validation
# ============================================================================

validate_kubectl_context() {
    log_section "Validating Kubernetes Context"

    if ! kubectl cluster-info &> /dev/null; then
        log_error "Unable to connect to Kubernetes cluster"
        log_error "Please ensure your kubeconfig is properly configured"
        return 1
    fi

    local context
    context=$(kubectl config current-context)
    log_info "Current kubectl context: ${context}"

    local cluster_info
    cluster_info=$(kubectl cluster-info 2>&1 | head -n1 || echo "unknown")
    log_verbose "Cluster info: ${cluster_info}"

    log_success "Kubectl context validated"
    return 0
}

# ============================================================================
# Pod Health Verification
# ============================================================================

verify_pod_health() {
    local namespace="$1"
    local selector="$2"
    local component_name="${3:-component}"
    local timeout="${4:-60}"
    local interval="${5:-5}"

    log_info "Verifying pod health for ${component_name}..."
    log_verbose "Namespace: ${namespace}, Selector: ${selector}"

    local elapsed=0
    while [[ ${elapsed} -lt ${timeout} ]]; do
        # Get pod status
        local pod_status
        pod_status=$(kubectl get pods -n "${namespace}" -l "${selector}" \
            -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\t"}{range .status.containerStatuses[*]}{.state.waiting.reason}{" "}{.state.terminated.reason}{end}{"\n"}{end}' 2>/dev/null)

        if [[ -z "${pod_status}" ]]; then
            log_warning "No pods found with selector ${selector} in namespace ${namespace}"
            sleep ${interval}
            ((elapsed += interval))
            continue
        fi

        # Check for failure states
        local has_failures=false
        local failure_details=""

        while IFS=$'\t' read -r pod_name phase reasons; do
            log_verbose "Pod ${pod_name}: phase=${phase}, reasons=${reasons}"

            # Check for problematic states
            if [[ "${phase}" == "Failed" ]] || \
               [[ "${reasons}" == *"CrashLoopBackOff"* ]] || \
               [[ "${reasons}" == *"Error"* ]] || \
               [[ "${reasons}" == *"ImagePullBackOff"* ]] || \
               [[ "${reasons}" == *"ErrImagePull"* ]]; then
                has_failures=true
                failure_details="${failure_details}\n  - ${pod_name}: ${phase} (${reasons})"
            fi
        done <<< "${pod_status}"

        if [[ "${has_failures}" == "true" ]]; then
            log_error "Pod health check failed for ${component_name}:"
            echo -e "${failure_details}"
            log_info "Pod details:"
            kubectl get pods -n "${namespace}" -l "${selector}"
            return 1
        fi

        # Check if all pods are running
        local running_count
        running_count=$(kubectl get pods -n "${namespace}" -l "${selector}" \
            -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' 2>/dev/null | grep -c "^Running$" || echo "0")

        local total_count
        total_count=$(kubectl get pods -n "${namespace}" -l "${selector}" --no-headers 2>/dev/null | wc -l | tr -d ' ')

        if [[ ${running_count} -gt 0 ]] && [[ ${running_count} -eq ${total_count} ]]; then
            log_success "All pods for ${component_name} are running (${running_count}/${total_count})"
            return 0
        fi

        log_verbose "Waiting for pods to be ready: ${running_count}/${total_count} running (${elapsed}s/${timeout}s)"
        sleep ${interval}
        ((elapsed += interval))
    done

    log_error "Timeout waiting for ${component_name} pods to become healthy"
    log_info "Current pod status:"
    kubectl get pods -n "${namespace}" -l "${selector}"
    return 1
}

# ============================================================================
# Debug Log Capture
# ============================================================================

capture_debug_logs() {
    local namespace="$1"
    local selector="$2"
    local component_name="$3"
    local output_dir="${4:-${WORK_DIR:-${PWD}}/debug-logs}"
    local capture_failed=false

    log_section "Capturing Debug Logs for ${component_name}"

    # Create output directory
    if ! mkdir -p "${output_dir}"; then
        log_error "Failed to create debug log directory: ${output_dir}"
        return 1
    fi

    local timestamp
    timestamp=$(date +"%Y%m%d-%H%M%S")
    local log_prefix="${output_dir}/${component_name}-${timestamp}-$$-${RANDOM}"

    log_info "Saving debug logs to: ${log_prefix}-*"

    # Capture pod logs
    log_info "Capturing pod logs..."
    kubectl logs -n "${namespace}" -l "${selector}" --all-containers=true --prefix=true > "${log_prefix}-pods.log" 2>&1 || { log_warning "Failed to capture current pod logs"; capture_failed=true; }

    # Capture previous pod logs (for crashed containers)
    log_info "Capturing previous pod logs..."
    kubectl logs -n "${namespace}" -l "${selector}" --all-containers=true --prefix=true --previous > "${log_prefix}-pods-previous.log" 2>&1 || true

    # Capture pod descriptions
    log_info "Capturing pod descriptions..."
    kubectl describe pods -n "${namespace}" -l "${selector}" > "${log_prefix}-pods-describe.txt" 2>&1 || { log_warning "Failed to capture pod descriptions"; capture_failed=true; }

    # Capture pod status
    log_info "Capturing pod status..."
    kubectl get pods -n "${namespace}" -l "${selector}" -o wide > "${log_prefix}-pods-status.txt" 2>&1 || { log_warning "Failed to capture pod status"; capture_failed=true; }
    kubectl get pods -n "${namespace}" -l "${selector}" -o yaml > "${log_prefix}-pods-yaml.yaml" 2>&1 || { log_warning "Failed to capture pod YAML"; capture_failed=true; }

    # Capture events
    log_info "Capturing namespace events..."
    kubectl get events -n "${namespace}" --sort-by='.lastTimestamp' > "${log_prefix}-events.txt" 2>&1 || { log_warning "Failed to capture namespace events"; capture_failed=true; }

    # Capture deployment/statefulset status if exists
    log_info "Capturing deployment/statefulset status..."
    kubectl get deployments,statefulsets -n "${namespace}" -l "${selector}" -o wide > "${log_prefix}-workloads-status.txt" 2>&1 || { log_warning "Failed to capture workload status"; capture_failed=true; }
    kubectl get deployments,statefulsets -n "${namespace}" -l "${selector}" -o yaml > "${log_prefix}-workloads-yaml.yaml" 2>&1 || { log_warning "Failed to capture workload YAML"; capture_failed=true; }

    # Capture services and endpoints
    log_info "Capturing services and endpoints..."
    kubectl get svc,endpoints -n "${namespace}" -l "${selector}" -o wide > "${log_prefix}-network.txt" 2>&1 || { log_warning "Failed to capture services and endpoints"; capture_failed=true; }

    # Create a summary file
    cat > "${log_prefix}-summary.txt" <<EOF
Debug Log Capture Summary
=========================
Component: ${component_name}
Namespace: ${namespace}
Selector: ${selector}
Timestamp: ${timestamp}

Files Generated:
- ${log_prefix}-pods.log (current pod logs)
- ${log_prefix}-pods-previous.log (previous pod logs for crashed containers)
- ${log_prefix}-pods-describe.txt (pod descriptions)
- ${log_prefix}-pods-status.txt (pod status)
- ${log_prefix}-pods-yaml.yaml (pod YAML manifests)
- ${log_prefix}-events.txt (namespace events)
- ${log_prefix}-workloads-status.txt (deployment/statefulset status)
- ${log_prefix}-workloads-yaml.yaml (deployment/statefulset YAML manifests)
- ${log_prefix}-network.txt (services and endpoints)
EOF

    if [[ "${capture_failed}" == "true" ]]; then
        log_warning "Debug logs captured with partial failures"
        return 1
    fi
    log_success "Debug logs captured successfully"
    log_info "Debug log location: ${output_dir}/"
    log_info "Log prefix: ${component_name}-${timestamp}-*"

    return 0
}

# ============================================================================
# Namespace Management
# ============================================================================

delete_namespace() {
    local namespace="$1"

    log_section "Deleting Namespace"

    # Check if namespace exists
    if ! kubectl get namespace "${namespace}" &> /dev/null; then
        log_warning "Namespace '${namespace}' does not exist"
        return 0
    fi

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would delete namespace: ${namespace}"
        return 0
    fi

    log_info "Deleting namespace: ${namespace}"
    log_warning "This will remove all resources in the namespace"

    if kubectl delete namespace "${namespace}" --wait --timeout=5m; then
        log_success "Namespace '${namespace}' deleted successfully"
        return 0
    else
        log_error "Failed to delete namespace '${namespace}'"
        log_info "You may need to manually remove finalizers or check for stuck resources"
        return 1
    fi
}
