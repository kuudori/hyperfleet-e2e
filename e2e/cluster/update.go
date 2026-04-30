package cluster

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][update] Cluster Update Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled at generation 1")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
			clusterID = *cluster.Id

			err = h.WaitForClusterCondition(ctx, clusterID, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.Cluster.Ready)
			Expect(err).NotTo(HaveOccurred(), "cluster should reach Reconciled=True at generation 1")
		})

		ginkgo.It("should update cluster via PATCH, trigger reconciliation, and reach Reconciled at new generation", func(ctx context.Context) {
			ginkgo.By("verifying cluster is at generation 1 before PATCH")
			clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterBefore.Generation).To(Equal(int32(1)), "cluster should be at generation 1 before update")

			ginkgo.By("sending PATCH to update cluster spec")
			patchedCluster, err := h.Client.PatchClusterFromPayload(ctx, clusterID, h.TestDataPath("payloads/clusters/cluster-patch.json"))
			Expect(err).NotTo(HaveOccurred(), "PATCH request should succeed")
			expectedGen := clusterBefore.Generation + 1
			Expect(patchedCluster.Generation).To(Equal(expectedGen), "generation should increment after PATCH")

			ginkgo.By("waiting for all adapters to reconcile at new generation")
			err = h.WaitForAllClusterAdaptersAtGeneration(ctx, clusterID, expectedGen, h.Cfg.Timeouts.Adapter.Processing)
			Expect(err).NotTo(HaveOccurred(), "all adapters should reconcile to generation %d", expectedGen)

			ginkgo.By("verifying cluster reaches Reconciled=True at new generation")
			err = h.WaitForClusterCondition(ctx, clusterID, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.Cluster.Ready)
			Expect(err).NotTo(HaveOccurred(), "cluster should reach Reconciled=True at generation %d", expectedGen)

			finalCluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalCluster.Generation).To(Equal(expectedGen), "final cluster generation should match expected")

			hasReconciled := h.HasResourceCondition(finalCluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
			Expect(hasReconciled).To(BeTrue(), "cluster should have Reconciled=True")

			for _, cond := range finalCluster.Status.Conditions {
				if cond.Type == client.ConditionTypeReconciled {
					Expect(cond.ObservedGeneration).To(Equal(expectedGen), "Reconciled condition observed_generation should match expected")
				}
			}
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
