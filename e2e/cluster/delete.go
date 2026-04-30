package cluster

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Cluster Deletion Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
			clusterID = *cluster.Id

			err = h.WaitForClusterCondition(ctx, clusterID, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.Cluster.Ready)
			Expect(err).NotTo(HaveOccurred(), "cluster should reach Reconciled=True before deletion")
		})

		ginkgo.It("should complete full deletion lifecycle from soft-delete through hard-delete", func(ctx context.Context) {
			clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")
			Expect(deletedCluster.Generation).To(Equal(clusterBefore.Generation+1), "generation should increment after soft-delete")

			ginkgo.By("waiting for all adapters to report Finalized=True")
			err = h.WaitForAllClusterAdaptersFinalized(ctx, clusterID, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "all adapters should report Finalized=True")

			ginkgo.By("verifying adapter conditions after finalization")
			statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			for _, adapter := range statuses.Items {
				Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeApplied, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
					"adapter %s should have Applied=False after finalization", adapter.Adapter)
				Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeAvailable, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
					"adapter %s should have Available=False after finalization", adapter.Adapter)
				Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
					"adapter %s should have Health=True after finalization", adapter.Adapter)
			}

			ginkgo.By("waiting for cluster to be hard-deleted")
			err = h.WaitForClusterHardDelete(ctx, clusterID, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "cluster should be permanently removed after hard-delete")

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			err = h.WaitForNamespaceAbsent(ctx, clusterID, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "K8s namespace should be absent after deletion")
		})

		ginkgo.It("should return 409 Conflict when PATCHing a soft-deleted cluster", ginkgo.Label(labels.Negative), func(ctx context.Context) {
			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")
			deletedGeneration := deletedCluster.Generation

			ginkgo.By("attempting PATCH on the soft-deleted cluster")
			patchReq := openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"updated-key": "should-not-work"},
			}
			resp, err := h.Client.PatchClusterRaw(ctx, clusterID, patchReq)
			Expect(err).NotTo(HaveOccurred(), "raw PATCH request should not fail at transport level")
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusConflict),
				"PATCH on soft-deleted cluster should return 409 Conflict")

			ginkgo.By("verifying cluster state is unchanged after rejected PATCH")
			cluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Generation).To(Equal(deletedGeneration), "generation should not change after rejected PATCH")
			Expect(cluster.DeletedTime).NotTo(BeNil(), "cluster should still be marked as deleted")
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Cluster Cascade Deletion",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID1 string
		var nodepoolID2 string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
			clusterID = *cluster.Id

			err = h.WaitForClusterCondition(ctx, clusterID, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.Cluster.Ready)
			Expect(err).NotTo(HaveOccurred(), "cluster should reach Reconciled=True")

			ginkgo.By("creating first nodepool and waiting for Reconciled")
			np1, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create first nodepool")
			Expect(np1.Id).NotTo(BeNil())
			nodepoolID1 = *np1.Id

			err = h.WaitForNodePoolCondition(ctx, clusterID, nodepoolID1, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.NodePool.Ready)
			Expect(err).NotTo(HaveOccurred(), "first nodepool should reach Reconciled=True")

			ginkgo.By("creating second nodepool and waiting for Reconciled")
			np2, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create second nodepool")
			Expect(np2.Id).NotTo(BeNil())
			nodepoolID2 = *np2.Id

			err = h.WaitForNodePoolCondition(ctx, clusterID, nodepoolID2, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.NodePool.Ready)
			Expect(err).NotTo(HaveOccurred(), "second nodepool should reach Reconciled=True")
		})

		ginkgo.It("should cascade deletion to child nodepools and hard-delete all resources", func(ctx context.Context) {
			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "cluster should have deleted_time set")

			ginkgo.By("verifying cascade: both child nodepools have deleted_time set")
			np1, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID1)
			Expect(err).NotTo(HaveOccurred(), "first nodepool should still be accessible")
			Expect(np1.DeletedTime).NotTo(BeNil(), "first nodepool should have deleted_time set via cascade")

			np2, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID2)
			Expect(err).NotTo(HaveOccurred(), "second nodepool should still be accessible")
			Expect(np2.DeletedTime).NotTo(BeNil(), "second nodepool should have deleted_time set via cascade")

			ginkgo.By("waiting for both nodepools to be hard-deleted")
			err = h.WaitForNodePoolHardDelete(ctx, clusterID, nodepoolID1, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "first nodepool should be hard-deleted")

			err = h.WaitForNodePoolHardDelete(ctx, clusterID, nodepoolID2, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "second nodepool should be hard-deleted")

			ginkgo.By("waiting for cluster to be hard-deleted after all nodepools removed")
			err = h.WaitForClusterHardDelete(ctx, clusterID, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "cluster should be hard-deleted after all child nodepools removed")

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			err = h.WaitForNamespaceAbsent(ctx, clusterID, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "K8s namespace should be absent after cascade deletion")
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)
