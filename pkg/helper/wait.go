package helper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// WaitForClusterCondition waits for a cluster to have a specific condition with the expected status
func (h *Helper) WaitForClusterCondition(ctx context.Context, clusterID string, conditionType string, expectedStatus openapi.ResourceConditionStatus, timeout time.Duration) error {
	logger.Debug("waiting for cluster condition", "cluster_id", clusterID, "condition_type", conditionType, "expected_status", expectedStatus, "timeout", timeout)

	Eventually(func(g Gomega) {
		cluster, err := h.Client.GetCluster(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
		g.Expect(cluster).NotTo(BeNil(), "cluster is nil")
		g.Expect(cluster.Status).NotTo(BeNil(), "cluster.Status is nil")

		// Check if the condition exists with the expected status
		found := false
		for _, cond := range cluster.Status.Conditions {
			if cond.Type == conditionType && cond.Status == expectedStatus {
				found = true
				break
			}
		}
		g.Expect(found).To(BeTrue(),
			fmt.Sprintf("cluster does not have condition %s=%s", conditionType, expectedStatus))
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("cluster reached target condition", "cluster_id", clusterID, "condition_type", conditionType, "status", expectedStatus)
	return nil
}

// WaitForAdapterCondition waits for a specific adapter condition to be in the expected status
func (h *Helper) WaitForAdapterCondition(ctx context.Context, clusterID, adapterName, condType string, expectedStatus openapi.AdapterConditionStatus, timeout time.Duration) error {
	Eventually(func(g Gomega) {
		statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

		// Find the specific adapter
		var found bool
		for _, status := range statuses.Items {
			if status.Adapter == adapterName {
				found = true
				hasCondition := h.HasAdapterCondition(status.Conditions, condType, expectedStatus)
				g.Expect(hasCondition).To(BeTrue(),
					fmt.Sprintf("adapter %s does not have condition %s=%s", adapterName, condType, expectedStatus))
				break
			}
		}
		g.Expect(found).To(BeTrue(), fmt.Sprintf("adapter %s not found", adapterName))
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	return nil
}

// WaitForAllAdapterConditions waits for all adapters to have the specified condition
func (h *Helper) WaitForAllAdapterConditions(ctx context.Context, clusterID, condType string, expectedStatus openapi.AdapterConditionStatus, timeout time.Duration) error {
	Eventually(func(g Gomega) {
		statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

		for _, adapterStatus := range statuses.Items {
			hasCondition := h.HasAdapterCondition(adapterStatus.Conditions, condType, expectedStatus)
			g.Expect(hasCondition).To(BeTrue(),
				fmt.Sprintf("adapter %s does not have condition %s=%s",
					adapterStatus.Adapter, condType, expectedStatus))
		}
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	return nil
}

// WaitForAllClusterAdaptersAtGeneration waits for all required cluster adapters to reconcile to the given generation
// with Applied, Available, and Health all True.
func (h *Helper) WaitForAllClusterAdaptersAtGeneration(ctx context.Context, clusterID string, generation int32, timeout time.Duration) error {
	logger.Debug("waiting for all cluster adapters at generation", "cluster_id", clusterID, "generation", generation, "timeout", timeout)

	Eventually(func(g Gomega) {
		statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

		adapterMap := make(map[string]openapi.AdapterStatus, len(statuses.Items))
		for _, s := range statuses.Items {
			adapterMap[s.Adapter] = s
		}

		for _, requiredAdapter := range h.Cfg.Adapters.Cluster {
			adapter, exists := adapterMap[requiredAdapter]
			g.Expect(exists).To(BeTrue(), "required adapter %s not found in statuses", requiredAdapter)
			g.Expect(adapter.ObservedGeneration).To(Equal(generation),
				"adapter %s observed_generation should be %d", requiredAdapter, generation)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Applied=True", requiredAdapter)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Available=True", requiredAdapter)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Health=True", requiredAdapter)
		}
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("all cluster adapters at generation", "cluster_id", clusterID, "generation", generation)
	return nil
}

// WaitForAllNodePoolAdaptersAtGeneration waits for all required nodepool adapters to reconcile to the given generation
// with Applied, Available, and Health all True.
func (h *Helper) WaitForAllNodePoolAdaptersAtGeneration(ctx context.Context, clusterID, nodepoolID string, generation int32, timeout time.Duration) error {
	logger.Debug("waiting for all nodepool adapters at generation", "cluster_id", clusterID, "nodepool_id", nodepoolID, "generation", generation, "timeout", timeout)

	Eventually(func(g Gomega) {
		statuses, err := h.Client.GetNodePoolStatuses(ctx, clusterID, nodepoolID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get nodepool statuses")

		adapterMap := make(map[string]openapi.AdapterStatus, len(statuses.Items))
		for _, s := range statuses.Items {
			adapterMap[s.Adapter] = s
		}

		for _, requiredAdapter := range h.Cfg.Adapters.NodePool {
			adapter, exists := adapterMap[requiredAdapter]
			g.Expect(exists).To(BeTrue(), "required adapter %s not found in statuses", requiredAdapter)
			g.Expect(adapter.ObservedGeneration).To(Equal(generation),
				"adapter %s observed_generation should be %d", requiredAdapter, generation)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Applied=True", requiredAdapter)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Available=True", requiredAdapter)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Health=True", requiredAdapter)
		}
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("all nodepool adapters at generation", "cluster_id", clusterID, "nodepool_id", nodepoolID, "generation", generation)
	return nil
}

// WaitForNodePoolCondition waits for a nodepool to have a specific condition with the expected status
func (h *Helper) WaitForNodePoolCondition(ctx context.Context, clusterID, nodepoolID string, conditionType string, expectedStatus openapi.ResourceConditionStatus, timeout time.Duration) error {
	logger.Debug("waiting for nodepool condition", "cluster_id", clusterID, "nodepool_id", nodepoolID, "condition_type", conditionType, "expected_status", expectedStatus, "timeout", timeout)

	Eventually(func(g Gomega) {
		nodepool, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get nodepool")
		g.Expect(nodepool).NotTo(BeNil(), "nodepool is nil")
		g.Expect(nodepool.Status).NotTo(BeNil(), "nodepool.Status is nil")

		// Check if the condition exists with the expected status
		found := false
		for _, cond := range nodepool.Status.Conditions {
			if cond.Type == conditionType && cond.Status == expectedStatus {
				found = true
				break
			}
		}
		g.Expect(found).To(BeTrue(),
			fmt.Sprintf("nodepool does not have condition %s=%s", conditionType, expectedStatus))
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("nodepool reached target condition", "cluster_id", clusterID, "nodepool_id", nodepoolID, "condition_type", conditionType, "status", expectedStatus)
	return nil
}

// WaitForAllClusterAdaptersFinalized waits for all required cluster adapters to report Finalized=True.
func (h *Helper) WaitForAllClusterAdaptersFinalized(ctx context.Context, clusterID string, timeout time.Duration) error {
	logger.Debug("waiting for all cluster adapters to finalize", "cluster_id", clusterID, "timeout", timeout)

	Eventually(func(g Gomega) {
		statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

		adapterMap := make(map[string]openapi.AdapterStatus, len(statuses.Items))
		for _, s := range statuses.Items {
			adapterMap[s.Adapter] = s
		}

		for _, requiredAdapter := range h.Cfg.Adapters.Cluster {
			adapter, exists := adapterMap[requiredAdapter]
			g.Expect(exists).To(BeTrue(), "required adapter %s not found in statuses", requiredAdapter)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeFinalized, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Finalized=True", requiredAdapter)
		}
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("all cluster adapters finalized", "cluster_id", clusterID)
	return nil
}

// WaitForAllNodePoolAdaptersFinalized waits for all required nodepool adapters to report Finalized=True.
func (h *Helper) WaitForAllNodePoolAdaptersFinalized(ctx context.Context, clusterID, nodepoolID string, timeout time.Duration) error {
	logger.Debug("waiting for all nodepool adapters to finalize", "cluster_id", clusterID, "nodepool_id", nodepoolID, "timeout", timeout)

	Eventually(func(g Gomega) {
		statuses, err := h.Client.GetNodePoolStatuses(ctx, clusterID, nodepoolID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get nodepool statuses")

		adapterMap := make(map[string]openapi.AdapterStatus, len(statuses.Items))
		for _, s := range statuses.Items {
			adapterMap[s.Adapter] = s
		}

		for _, requiredAdapter := range h.Cfg.Adapters.NodePool {
			adapter, exists := adapterMap[requiredAdapter]
			g.Expect(exists).To(BeTrue(), "required adapter %s not found in statuses", requiredAdapter)
			g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeFinalized, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
				"adapter %s should have Finalized=True", requiredAdapter)
		}
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("all nodepool adapters finalized", "cluster_id", clusterID, "nodepool_id", nodepoolID)
	return nil
}

// WaitForClusterHardDelete waits until GET cluster returns an error, confirming the resource
// has been permanently removed from the database by the inline hard-delete mechanism.
func (h *Helper) WaitForClusterHardDelete(ctx context.Context, clusterID string, timeout time.Duration) error {
	logger.Debug("waiting for cluster hard-delete", "cluster_id", clusterID, "timeout", timeout)

	Eventually(func(g Gomega) {
		_, err := h.Client.GetCluster(ctx, clusterID)
		g.Expect(err).To(HaveOccurred(), "cluster should return error after hard-delete")
		var httpErr *client.HTTPError
		g.Expect(errors.As(err, &httpErr)).To(BeTrue(), "expected HTTP error, got: %v", err)
		g.Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
			"cluster should return 404 after hard-delete, got %d", httpErr.StatusCode)
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("cluster hard-deleted", "cluster_id", clusterID)
	return nil
}

// WaitForNodePoolHardDelete waits until GET nodepool returns an error, confirming the resource
// has been permanently removed from the database by the inline hard-delete mechanism.
func (h *Helper) WaitForNodePoolHardDelete(ctx context.Context, clusterID, nodepoolID string, timeout time.Duration) error {
	logger.Debug("waiting for nodepool hard-delete", "cluster_id", clusterID, "nodepool_id", nodepoolID, "timeout", timeout)

	Eventually(func(g Gomega) {
		_, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
		g.Expect(err).To(HaveOccurred(), "nodepool should return error after hard-delete")
		var httpErr *client.HTTPError
		g.Expect(errors.As(err, &httpErr)).To(BeTrue(), "expected HTTP error, got: %v", err)
		g.Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
			"nodepool should return 404 after hard-delete, got %d", httpErr.StatusCode)
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("nodepool hard-deleted", "cluster_id", clusterID, "nodepool_id", nodepoolID)
	return nil
}

// WaitForNamespaceAbsent waits until no K8s namespaces exist with the given prefix.
func (h *Helper) WaitForNamespaceAbsent(ctx context.Context, namePrefix string, timeout time.Duration) error {
	logger.Debug("waiting for namespace absence", "prefix", namePrefix, "timeout", timeout)

	Eventually(func(g Gomega) {
		namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, namePrefix)
		g.Expect(err).NotTo(HaveOccurred(), "failed to check namespaces")
		g.Expect(namespaces).To(BeEmpty(), "namespaces with prefix %s should not exist", namePrefix)
	}, timeout, h.Cfg.Polling.Interval).Should(Succeed())

	logger.Info("namespace absent", "prefix", namePrefix)
	return nil
}
