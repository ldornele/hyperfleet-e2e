package version

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: version][filtering] Version Spec Filtering",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var channelID string
		var versionDefault *client.Resource
		var versionEnabled *client.Resource
		var versionDisabled *client.Resource

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating parent channel")
			ch, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create channel")
			Expect(ch.Id).NotTo(BeNil(), "channel ID should not be nil")
			channelID = *ch.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestChannel(ctx, channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})

			ginkgo.By("creating version with is_default=true, enabled=true")
			versionDefault, err = h.Client.CreateVersion(ctx, channelID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: "ver-default-true",
				Spec: map[string]any{
					"is_default":    true,
					"enabled":       true,
					"raw_version":   "4.17.0",
					"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.0",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("creating version with is_default=false, enabled=true")
			versionEnabled, err = h.Client.CreateVersion(ctx, channelID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: "ver-enabled-only",
				Spec: map[string]any{
					"is_default":    false,
					"enabled":       true,
					"raw_version":   "4.17.1",
					"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.1",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("creating version with is_default=false, enabled=false")
			versionDisabled, err = h.Client.CreateVersion(ctx, channelID, client.ResourceCreateRequest{
				Kind: "Version",
				Name: "ver-disabled",
				Spec: map[string]any{
					"is_default":    false,
					"enabled":       false,
					"raw_version":   "4.16.0",
					"release_image": "quay.io/openshift-release-dev/ocp-release:4.16.0",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		ginkgo.It("should filter versions by spec.is_default='true'", func(ctx context.Context) {
			ginkgo.By("listing versions with spec.is_default filter")
			list, err := h.Client.ListVersions(ctx, channelID, "spec.is_default='true'")
			Expect(err).NotTo(HaveOccurred(), "failed to list filtered versions")

			ids := extractResourceIDs(list.Items)
			Expect(ids).To(ContainElement(*versionDefault.Id), "is_default=true version should be in results")
			Expect(ids).NotTo(ContainElement(*versionEnabled.Id), "is_default=false version should not be in results")
			Expect(ids).NotTo(ContainElement(*versionDisabled.Id), "disabled version should not be in results")
		})

		ginkgo.It("should filter versions by spec.enabled='true'", func(ctx context.Context) {
			ginkgo.By("listing versions with spec.enabled filter")
			list, err := h.Client.ListVersions(ctx, channelID, "spec.enabled='true'")
			Expect(err).NotTo(HaveOccurred(), "failed to list filtered versions")

			ids := extractResourceIDs(list.Items)
			Expect(ids).To(ContainElement(*versionDefault.Id), "default version should be in results")
			Expect(ids).To(ContainElement(*versionEnabled.Id), "enabled version should be in results")
			Expect(ids).NotTo(ContainElement(*versionDisabled.Id), "disabled version should not be in results")
		})
	},
)

func extractResourceIDs(resources []client.Resource) []string {
	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		if r.Id != nil {
			ids = append(ids, *r.Id)
		}
	}
	return ids
}
