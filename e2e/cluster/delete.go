package cluster

import (
	"context"
	"errors"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Cluster Deletion Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Ready, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should complete full deletion lifecycle from soft-delete through hard-delete", func(ctx context.Context) {
			clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")
			Expect(deletedCluster.Generation).To(Equal(clusterBefore.Generation+1), "generation should increment after soft-delete")

			ginkgo.By("waiting for cluster adapters to finalize and cluster to be hard-deleted")
			// Hard-delete executes atomically within the POST /adapter_statuses request that
			// computes Reconciled=True, so there is no observable window to see Finalized=True
			// on the statuses endpoint. Accept either Finalized=True OR 404 (already hard-deleted).
			Eventually(func(g Gomega) {
				httpStatus, err := h.PollClusterHTTPStatus(ctx, clusterID)()
				g.Expect(err).NotTo(HaveOccurred())
				if httpStatus == http.StatusNotFound {
					return
				}
				statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statuses.Items).NotTo(BeEmpty(), "adapter statuses should be present before hard-delete")
				for _, requiredAdapter := range h.Cfg.Adapters.Cluster {
					found := false
					for _, adapter := range statuses.Items {
						if adapter.Adapter == requiredAdapter {
							found = true
							g.Expect(h.HasAdapterCondition(adapter.Conditions, client.ConditionTypeFinalized, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"adapter %s should have Finalized=True", requiredAdapter)
						}
					}
					g.Expect(found).To(BeTrue(), "required adapter %s not found in statuses", requiredAdapter)
				}
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("confirming cluster is hard-deleted")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())
		})

		ginkgo.It("should return 409 Conflict when PATCHing a soft-deleted cluster", ginkgo.Label(labels.Negative), func(ctx context.Context) {
			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")
			deletedGeneration := deletedCluster.Generation

			ginkgo.By("attempting PATCH on the soft-deleted cluster")
			patchReq := openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"updated-key": "should-not-work"},
			}
			resp, err := h.Client.PatchClusterRaw(ctx, clusterID, patchReq)
			Expect(err).NotTo(HaveOccurred(), "raw PATCH request should not fail at transport level")
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusConflict),
				"PATCH on soft-deleted cluster should return 409 Conflict")

			ginkgo.By("verifying cluster state is unchanged after rejected PATCH")
			cluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Generation).To(Equal(deletedGeneration), "generation should not change after rejected PATCH")
			Expect(cluster.DeletedTime).NotTo(BeNil(), "cluster should still be marked as deleted")
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
				if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
				}
			}
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Cluster Cascade Deletion",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID1 string
		var nodepoolID2 string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Ready, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("creating two nodepools")
			np1, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create first nodepool")
			Expect(np1.Id).NotTo(BeNil())
			nodepoolID1 = *np1.Id

			np2, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create second nodepool")
			Expect(np2.Id).NotTo(BeNil())
			nodepoolID2 = *np2.Id

			ginkgo.By("waiting for both nodepools to reach Reconciled")
			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID1), h.Cfg.Timeouts.NodePool.Ready, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID2), h.Cfg.Timeouts.NodePool.Ready, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should cascade deletion to child nodepools and hard-delete all resources", func(ctx context.Context) {
			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "cluster should have deleted_time set")

			ginkgo.By("verifying cascade: both child nodepools are soft-deleted or already hard-deleted")
			Eventually(func(g Gomega) {
				np1, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID1)
				var httpErr *client.HTTPError
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred(), "first nodepool should be accessible or 404")
				g.Expect(np1.DeletedTime).NotTo(BeNil(), "first nodepool should have deleted_time set via cascade")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			Eventually(func(g Gomega) {
				np2, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID2)
				var httpErr *client.HTTPError
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred(), "second nodepool should be accessible or 404")
				g.Expect(np2.DeletedTime).NotTo(BeNil(), "second nodepool should have deleted_time set via cascade")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("waiting for both nodepools to be hard-deleted")
			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID1), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID2), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("waiting for cluster to be hard-deleted after all nodepools removed")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
				if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
				}
			}
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)
