package version

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: version][perf] Create latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper
		var channelID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating parent channel")
			ch, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(ch.Id).NotTo(BeNil(), "channel ID should be set")
			channelID = *ch.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestChannel(ctx, channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})
		})

		ginkgo.It("should create a version within acceptable latency", func(ctx context.Context) {
			ginkgo.By("creating a version and timing the response")
			start := time.Now()

			version, err := h.Client.CreateVersionFromPayload(ctx, channelID, h.TestDataPath("payloads/versions/version-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(version.Id).NotTo(BeNil(), "version ID should be set")
			elapsed := time.Since(start)

			ginkgo.GinkgoWriter.Printf("[PERF] POST /channels/%s/versions latency: %v\n", channelID, elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPICreate),
				"version create exceeded threshold")
		})
	},
)
