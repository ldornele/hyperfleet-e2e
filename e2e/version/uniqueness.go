package version

import (
	"context"
	"errors"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: version][uniqueness] Version Name Uniqueness Per Channel",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper
		var channelAID string
		var channelBID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating channel A")
			chA, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create channel A")
			Expect(chA.Id).NotTo(BeNil(), "channel A ID should not be nil")
			channelAID = *chA.Id

			ginkgo.By("creating channel B")
			chB, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create channel B")
			Expect(chB.Id).NotTo(BeNil(), "channel B ID should not be nil")
			channelBID = *chB.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				for _, id := range []string{channelAID, channelBID} {
					if err := h.CleanupTestChannel(ctx, id); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", id, err)
					}
				}
			})
		})

		ginkgo.It("should allow same version name in different channels", func(ctx context.Context) {
			sharedName := "shared-version-name"
			spec := map[string]any{
				"enabled":       true,
				"raw_version":   "4.17.0",
				"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.0",
			}

			ginkgo.By("creating version with shared name in channel A")
			_, err := h.Client.CreateVersion(ctx, channelAID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: sharedName,
				Spec: spec,
			})
			Expect(err).NotTo(HaveOccurred(), "version in channel A should succeed")

			ginkgo.By("creating version with same name in channel B")
			_, err = h.Client.CreateVersion(ctx, channelBID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: sharedName,
				Spec: spec,
			})
			Expect(err).NotTo(HaveOccurred(), "version with same name in different channel should succeed")
		})

		ginkgo.It("should reject duplicate version name in same channel", func(ctx context.Context) {
			duplicateName := "duplicate-version"
			spec := map[string]any{
				"enabled":       true,
				"raw_version":   "4.17.0",
				"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.0",
			}

			ginkgo.By("creating first version")
			_, err := h.Client.CreateVersion(ctx, channelAID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: duplicateName,
				Spec: spec,
			})
			Expect(err).NotTo(HaveOccurred(), "first version should succeed")

			ginkgo.By("attempting to create second version with same name in same channel")
			_, err = h.Client.CreateVersion(ctx, channelAID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: duplicateName,
				Spec: spec,
			})
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusConflict),
				"duplicate version name in same channel should return 409")
		})
	},
)
