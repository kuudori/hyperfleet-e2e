package helper

import (
    "context"
    "fmt"
    "sort"
    "strings"

    k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
    appsv1 "k8s.io/api/apps/v1"
    batchv1 "k8s.io/api/batch/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/labels"
)

// VerifyNamespaceActive verifies a namespace exists and is in Active phase
func (h *Helper) VerifyNamespaceActive(ctx context.Context, name string, expectedLabels, expectedAnnotations map[string]string) error {
    logger.Info("verifying namespace status", "namespace", name)

    // Fetch namespace
    ns, err := h.K8sClient.FetchNamespace(ctx, name)
    if err != nil {
        return err
    }

    // Check phase
    if !k8sclient.HasNamespacePhase(ns, corev1.NamespaceActive) {
        return fmt.Errorf("namespace %s phase is %v, expected Active", ns.Name, ns.Status.Phase)
    }

    // Verify labels
    if err := verifyMapContains(ns.Labels, expectedLabels, "label"); err != nil {
        return fmt.Errorf("namespace %s: %w", name, err)
    }

    // Verify annotations
    if err := verifyMapContains(ns.Annotations, expectedAnnotations, "annotation"); err != nil {
        return fmt.Errorf("namespace %s: %w", name, err)
    }

    logger.Info("namespace verified successfully", "namespace", name, "phase", ns.Status.Phase)
    return nil
}

// VerifyJobComplete verifies a job exists and has completed successfully.
// Uses expectedLabels to find the job via label selector - if the list returns a job,
// it's guaranteed to have those labels (no need to verify them again).
func (h *Helper) VerifyJobComplete(ctx context.Context, namespace string, expectedLabels, expectedAnnotations map[string]string) error {
    labelSelector := labels.SelectorFromSet(expectedLabels).String()
    logger.Info("verifying job status", "namespace", namespace, "label_selector", labelSelector)

    // Get job (handles uniqueness validation internally)
    job, err := h.K8sClient.GetUniqueJobByLabels(ctx, namespace, expectedLabels)
    if err != nil {
        return err
    }

    // Check completion
    if !k8sclient.HasJobCondition(job, batchv1.JobComplete, corev1.ConditionTrue) {
        return fmt.Errorf("job %s in namespace %s has not completed successfully (conditions: %+v)",
            job.Name, namespace, job.Status.Conditions)
    }

    // Verify annotations
    if err := verifyMapContains(job.Annotations, expectedAnnotations, "annotation"); err != nil {
        return fmt.Errorf("job %s in namespace %s: %w", job.Name, namespace, err)
    }

    logger.Info("job verified successfully",
        "namespace", namespace,
        "job", job.Name,
        "succeeded", job.Status.Succeeded,
        "active", job.Status.Active,
        "failed", job.Status.Failed)
    return nil
}

// VerifyDeploymentAvailable verifies a deployment exists and is available.
// Uses expectedLabels to find the deployment via label selector - if the list returns a deployment,
// it's guaranteed to have those labels (no need to verify them again).
func (h *Helper) VerifyDeploymentAvailable(ctx context.Context, namespace string, expectedLabels, expectedAnnotations map[string]string) error {
    labelSelector := labels.SelectorFromSet(expectedLabels).String()
    logger.Info("verifying deployment status", "namespace", namespace, "label_selector", labelSelector)

    // Get deployment (handles uniqueness validation internally)
    deploy, err := h.K8sClient.GetUniqueDeploymentByLabels(ctx, namespace, expectedLabels)
    if err != nil {
        return err
    }

    // Check availability
    if !k8sclient.HasDeploymentCondition(deploy, appsv1.DeploymentAvailable, corev1.ConditionTrue) {
        return fmt.Errorf("deployment %s in namespace %s is not available (availableReplicas=%d, conditions: %+v)",
            deploy.Name, namespace, deploy.Status.AvailableReplicas, deploy.Status.Conditions)
    }

    // Verify annotations
    if err := verifyMapContains(deploy.Annotations, expectedAnnotations, "annotation"); err != nil {
        return fmt.Errorf("deployment %s in namespace %s: %w", deploy.Name, namespace, err)
    }

    logger.Info("deployment verified successfully",
        "namespace", namespace,
        "deployment", deploy.Name,
        "available_replicas", deploy.Status.AvailableReplicas)
    return nil
}

// VerifyConfigMap verifies a configmap exists with expected labels and annotations.
// Uses expectedLabels to find the configmap via label selector - if the list returns a configmap,
// it's guaranteed to have those labels (no need to verify them again).
func (h *Helper) VerifyConfigMap(ctx context.Context, namespace string, expectedLabels, expectedAnnotations map[string]string) error {
	labelSelector := labels.SelectorFromSet(expectedLabels).String()
	logger.Info("verifying configmap status", "namespace", namespace, "label_selector", labelSelector)

	// Get configmap (handles uniqueness validation internally)
	cm, err := h.K8sClient.GetUniqueConfigMapByLabels(ctx, namespace, expectedLabels)
	if err != nil {
		return err
	}

	// Verify annotations
	if err := verifyMapContains(cm.Annotations, expectedAnnotations, "annotation"); err != nil {
		return fmt.Errorf("configmap %s in namespace %s: %w", cm.Name, namespace, err)
	}

	logger.Info("configmap verified successfully",
		"namespace", namespace,
		"configmap", cm.Name)
	return nil
}

// verifyMapContains checks if actual map contains all expected key-value pairs
func verifyMapContains(actual, expected map[string]string, mapType string) error {
    missing := make([]string, 0, len(expected))
    mismatched := make([]string, 0, len(expected))

    for key, expectedValue := range expected {
        actualValue, exists := actual[key]
        if !exists {
            missing = append(missing, key)
            continue
        }

        if actualValue != expectedValue {
            mismatched = append(mismatched, fmt.Sprintf("%s (expected: %s, actual: %s)",
                key, expectedValue, actualValue))
        }
    }

    if len(missing) > 0 || len(mismatched) > 0 {
        var errParts []string
        if len(missing) > 0 {
            sort.Strings(missing)
            errParts = append(errParts, fmt.Sprintf("missing %ss: %s", mapType, strings.Join(missing, ", ")))
        }
        if len(mismatched) > 0 {
            sort.Strings(mismatched)
            errParts = append(errParts, fmt.Sprintf("mismatched %ss: %s", mapType, strings.Join(mismatched, "; ")))
        }
        return fmt.Errorf("%s", strings.Join(errParts, "; "))
    }

    return nil
}
