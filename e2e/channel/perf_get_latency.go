package channel

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

var _ = ginkgo.Describe("[Suite: channel][perf] API read latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper
		var channelID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			channel, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(channel.Id).NotTo(BeNil(), "channel ID should be set")
			channelID = *channel.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestChannel(ctx, channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})
		})

		ginkgo.It("should read a channel within acceptable latency", func(ctx context.Context) {
			ginkgo.By("warming up with untimed read")
			_, err := h.Client.GetChannel(ctx, channelID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("measuring GET /channels/{id} response time")
			const samples = 5
			durations := make([]time.Duration, samples)
			for i := range samples {
				start := time.Now()
				_, err = h.Client.GetChannel(ctx, channelID)
				Expect(err).NotTo(HaveOccurred())
				durations[i] = time.Since(start)
			}
			slices.Sort(durations)
			median := durations[samples/2]
			ginkgo.GinkgoWriter.Printf("[PERF] GET /channels/%s latency: %v (median of %d samples)\n", channelID, median, samples)
			Expect(median).To(BeNumerically("<", config.ThresholdAPIRead),
				"GET /channels/{id} exceeded threshold")
		})
	},
)
