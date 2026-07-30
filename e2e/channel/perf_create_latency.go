package channel

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: channel][perf] Create latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should create a channel within acceptable latency", func(ctx context.Context) {
			ginkgo.By("creating a channel and timing the response")
			start := time.Now()

			channel, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			if channel != nil && channel.Id != nil {
				id := *channel.Id
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestChannel(ctx, id); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", id, err)
					}
				})
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(channel.Id).NotTo(BeNil(), "channel ID should be set")
			elapsed := time.Since(start)

			ginkgo.GinkgoWriter.Printf("[PERF] POST /channels latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPICreate),
				"channel create exceeded threshold")
		})
	},
)
