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

var _ = ginkgo.Describe("[Suite: wifconfig][perf] API list latency",
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

		ginkgo.It("should list wifconfigs within acceptable latency", func(ctx context.Context) {
			ginkgo.By("measuring GET /wifconfigs response time")
			start := time.Now()
			_, err := h.Client.ListWifConfigs(ctx, "")
			Expect(err).NotTo(HaveOccurred())
			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] GET /wifconfigs latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPIList),
				"GET /wifconfigs exceeded threshold")
		})
	},
)
