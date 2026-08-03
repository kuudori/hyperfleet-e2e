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

var _ = ginkgo.Describe("[Suite: wifconfig][perf] Create latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should create a wifconfig within acceptable latency", func(ctx context.Context) {
			ginkgo.By("creating a wifconfig and timing the response")
			start := time.Now()

			wifConfig, err := h.Client.CreateWifConfigFromPayload(ctx, h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"))
			if wifConfig != nil && wifConfig.Id != nil {
				id := *wifConfig.Id
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestWifConfig(ctx, id); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup wifconfig %s: %v\n", id, err)
					}
				})
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(wifConfig.Id).NotTo(BeNil(), "wifconfig ID should be set")
			elapsed := time.Since(start)

			ginkgo.GinkgoWriter.Printf("[PERF] POST /wifconfigs latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPICreate),
				"wifconfig create exceeded threshold")
		})
	},
)
