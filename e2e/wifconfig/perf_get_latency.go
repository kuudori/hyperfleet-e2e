package wifconfig

import (
	"context"
	"slices"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][perf] API read latency",
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

		ginkgo.It("should read a wifconfig within acceptable latency", func(ctx context.Context) {
			ginkgo.By("warming up with untimed read")
			_, err := h.Client.GetWifConfig(ctx, wifConfigID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("measuring GET /wifconfigs/{id} response time")
			const samples = 5
			durations := make([]time.Duration, samples)
			for i := range samples {
				start := time.Now()
				_, err = h.Client.GetWifConfig(ctx, wifConfigID)
				Expect(err).NotTo(HaveOccurred())
				durations[i] = time.Since(start)
			}
			slices.Sort(durations)
			median := durations[samples/2]
			ginkgo.GinkgoWriter.Printf("[PERF] GET /wifconfigs/%s latency: %v (median of %d samples)\n", wifConfigID, median, samples)
			Expect(median).To(BeNumerically("<", config.ThresholdAPIRead),
				"GET /wifconfigs/{id} exceeded threshold")
		})
	},
)
