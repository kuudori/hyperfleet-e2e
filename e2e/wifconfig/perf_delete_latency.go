package wifconfig

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][perf] Delete latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper
		var wifConfigID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			wifConfig, err := h.Client.CreateWifConfigFromPayload(ctx, h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(wifConfig.Id).NotTo(BeNil(), "wifconfig ID should be set")
			wifConfigID = *wifConfig.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestWifConfig(ctx, wifConfigID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup wifconfig %s: %v\n", wifConfigID, err)
				}
			})
		})

		ginkgo.It("should delete a wifconfig within acceptable latency", func(ctx context.Context) {
			ginkgo.By("deleting wifconfig and timing the response")
			start := time.Now()

			deleted, err := h.Client.DeleteWifConfig(ctx, wifConfigID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted wifconfig should have deleted_time set")
			elapsed := time.Since(start)

			ginkgo.GinkgoWriter.Printf("[PERF] DELETE /wifconfigs/%s latency: %v\n", wifConfigID, elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPIDelete),
				"wifconfig delete exceeded threshold")
		})
	},
)
