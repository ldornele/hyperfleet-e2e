package nodepool

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][delete] Force-Delete Single NodePool Stuck in Finalizing",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should force-delete a single stuck nodepool without affecting the parent cluster",
			func(ctx context.Context) {
				// --- Create resources in a healthy state ---

				ginkgo.By("Create cluster and wait for Reconciled")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id
				h.DeferClusterCleanup(clusterID)

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("Create nodepool and wait for Reconciled")
				np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
				Expect(np.Id).NotTo(BeNil())
				nodepoolID := *np.Id

				Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				// --- Simulate stuck nodepool deletion by scaling down the existing nodepool adapter ---

				// Find the pre-deployed np-configmap adapter and scale it to zero.
				// This simulates a real production scenario where an adapter goes down.
				Expect(h.Cfg.Adapters.NodePool).NotTo(BeEmpty(), "nodepool adapter config is required for this test")
				npAdapterName := h.Cfg.Adapters.NodePool[0]
				npReleaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeNodepools, npAdapterName)

				deploymentName, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, npReleaseName)
				Expect(err).NotTo(HaveOccurred(), "failed to find nodepool adapter deployment")

				ginkgo.By("Scale down nodepool adapter to simulate unavailability")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale down nodepool adapter")

				// Restore the adapter after the test regardless of outcome
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Restore nodepool adapter to 1 replica")
					if err := h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 1); err != nil {
						ginkgo.GinkgoWriter.Printf("CRITICAL: failed to restore nodepool adapter %s: %v\n", npAdapterName, err)
					}
				})

				ginkgo.By("Soft-delete only the nodepool (not the cluster)")
				deletedNP, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred(), "DELETE nodepool should succeed with 202")
				Expect(deletedNP.DeletedTime).NotTo(BeNil(), "nodepool should have deleted_time set")

				// Prove the nodepool is genuinely stuck (adapter is down, can't finalize)
				ginkgo.By("Verify nodepool is stuck in Finalizing")
				Consistently(func(g Gomega) {
					npCheck, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
					g.Expect(err).NotTo(HaveOccurred(), "nodepool should still be accessible")
					g.Expect(npCheck.DeletedTime).NotTo(BeNil(), "nodepool should still be soft-deleted")
					g.Expect(h.HasResourceCondition(npCheck.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
						"Reconciled should be False while stuck")
				}, h.Cfg.Timeouts.Adapter.Processing/4, h.Cfg.Polling.Interval).Should(Succeed())

				// --- Force-delete the nodepool and verify parent cluster is unaffected ---

				ginkgo.By("Force-delete the nodepool")
				err = h.Client.ForceDeleteNodePool(ctx, clusterID, nodepoolID, "E2E test: nodepool adapter unavailable")
				Expect(err).NotTo(HaveOccurred(), "force-delete nodepool should succeed with 204")

				ginkgo.By("Verify nodepool is hard-deleted (404)")
				Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				// The parent cluster must remain accessible and healthy
				ginkgo.By("Verify parent cluster is unaffected and still Reconciled")
				parentCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "parent cluster should still be accessible")
				Expect(parentCluster.DeletedTime).To(BeNil(), "parent cluster should NOT be deleted")
				Expect(h.HasResourceCondition(parentCluster.Status.Conditions,
					client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue(),
					"parent cluster should still be Reconciled=True")

				ginkgo.GinkgoWriter.Printf("Verified: force-deleted nodepool %s, parent cluster %s unaffected\n", nodepoolID, clusterID)
			})
	},
)
