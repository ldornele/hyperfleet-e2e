package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

const stuckAdapterName = "cl-stuck"

var _ = ginkgo.Describe("[Suite: cluster][delete] Force-Delete Cluster Stuck in Finalizing",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2),
	func() {
		var (
			h                *helper.Helper
			adapterChartPath string
			apiChartPath     string
			baseDeployOpts   helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("Clone adapter Helm chart repository")
			var cleanupAdapterChart func() error
			var err error
			adapterChartPath, cleanupAdapterChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := cleanupAdapterChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup adapter chart: %v\n", err)
				}
			})

			ginkgo.By("Clone API Helm chart repository")
			var cleanupAPIChart func() error
			apiChartPath, cleanupAPIChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "api",
				RepoURL:   h.Cfg.APIDeployment.ChartRepo,
				Ref:       h.Cfg.APIDeployment.ChartRef,
				ChartPath: h.Cfg.APIDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone API Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := cleanupAPIChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup API chart: %v\n", err)
				}
			})

			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace: h.Cfg.Namespace,
				ChartPath: adapterChartPath,
			}
		})

		ginkgo.It("should force-delete a cluster stuck in Finalizing and remove all its nodepools",
			func(ctx context.Context) {
				// --- Deploy a stuck-adapter that will block normal deletion ---

				adapterName := stuckAdapterName

				ginkgo.GinkgoT().Setenv("ADAPTER_NAME", adapterName)

				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				ginkgo.By("Deploy stuck-adapter")
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err := h.DeployAdapter(ctx, deployOpts)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					}
					subscriptionID := h.Cfg.Namespace + "-" + helper.ResourceTypeClusters + "-" + adapterName
					if err := h.DeletePubSubSubscription(ctx, subscriptionID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to delete Pub/Sub subscription %s: %v\n", subscriptionID, err)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy stuck-adapter")

				// Tell the API that cl-stuck is required; without it, deletion can't complete
				ginkgo.By("Upgrade API to include stuck-adapter in required adapters")
				originalAdapters := h.GetAPIRequiredClusterAdapters()
				updatedAdapters := append(slices.Clone(originalAdapters), adapterName)

				err = h.UpgradeAPIRequiredAdapters(ctx, apiChartPath, h.Cfg.Namespace, updatedAdapters)
				Expect(err).NotTo(HaveOccurred(), "failed to upgrade API with stuck-adapter")

				deploymentName, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, releaseName)
				Expect(err).NotTo(HaveOccurred(), "failed to find stuck-adapter deployment name")

				// --- Create resources in a healthy state ---

				ginkgo.By("Create cluster and wait for Reconciled")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)
				h.DeferClusterCleanup(clusterID)

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.RestoreAPIRequiredAdaptersWithRetry(ctx, apiChartPath, h.Cfg.Namespace, originalAdapters, 3); err != nil {
						ginkgo.GinkgoWriter.Printf("CRITICAL: %v\n", err)
					}
				})

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("Create a nodepool and wait for Reconciled")
				np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
				Expect(np.Id).NotTo(BeNil())
				nodepoolID := *np.Id

				Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				// --- Simulate stuck deletion ---

				// Scaling to 0 makes the adapter unable to process delete events
				ginkgo.By("Scale down stuck-adapter to simulate unavailability")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale down stuck-adapter")

				ginkgo.By("Soft-delete the cluster")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
				Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "cluster should have deleted_time set")

				// Consistently proves the cluster remains stuck over time (not just a race)
				ginkgo.By("Verify cluster is stuck in Finalizing (not hard-deleted)")
				Consistently(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "cluster should still be accessible")
					g.Expect(cl.DeletedTime).NotTo(BeNil(), "cluster should still be soft-deleted")
					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
						"Reconciled should be False while stuck")
				}, h.Cfg.Timeouts.Adapter.Processing/4, h.Cfg.Polling.Interval).Should(Succeed())

				// --- Force-delete and verify cascade removal ---

				ginkgo.By("Force-delete the cluster")
				err = h.Client.ForceDeleteCluster(ctx, clusterID, "E2E test: stuck-adapter unable to finalize")
				Expect(err).NotTo(HaveOccurred(), "force-delete should succeed with 204")

				ginkgo.By("Verify cluster is hard-deleted (404)")
				Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Cluster.Deleted, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				// Force-delete cascades: child nodepools are removed in the same transaction
				ginkgo.By("Verify child nodepool is also removed (404)")
				Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				ginkgo.GinkgoWriter.Printf("Verified: force-delete removed cluster %s and nodepool %s\n", clusterID, nodepoolID)
			})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Adapter Handles 404 Gracefully After Force-Delete",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2),
	func() {
		var (
			h                *helper.Helper
			adapterChartPath string
			baseDeployOpts   helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("Clone adapter Helm chart repository")
			var cleanupAdapterChart func() error
			var err error
			adapterChartPath, cleanupAdapterChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := cleanupAdapterChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup adapter chart: %v\n", err)
				}
			})

			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace: h.Cfg.Namespace,
				ChartPath: adapterChartPath,
			}
		})

		ginkgo.It("should keep adapter pods healthy after force-deleting a resource they were processing",
			func(ctx context.Context) {
				// --- Deploy a dedicated adapter that will receive the delete event ---

				adapterName := stuckAdapterName

				ginkgo.GinkgoT().Setenv("ADAPTER_NAME", adapterName)

				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				ginkgo.By("Deploy adapter that will receive events for the cluster")
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err := h.DeployAdapter(ctx, deployOpts)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					}
					subscriptionID := h.Cfg.Namespace + "-" + helper.ResourceTypeClusters + "-" + adapterName
					if err := h.DeletePubSubSubscription(ctx, subscriptionID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to delete Pub/Sub subscription %s: %v\n", subscriptionID, err)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy adapter")

				// --- Create a cluster so the adapter has something to process ---

				ginkgo.By("Create cluster and wait for Reconciled")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id
				h.DeferClusterCleanup(clusterID)

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				// --- Capture baseline before triggering any deletion ---

				ginkgo.By("Capture initial restart counts before triggering deletion")
				initialPods, err := h.K8sClient.CoreV1().Pods(h.Cfg.Namespace).List(ctx, metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
				})
				Expect(err).NotTo(HaveOccurred(), "failed to list adapter pods for baseline")
				initialPodNames := make(map[string]bool)
				initialRestarts := make(map[string]int32)
				for _, p := range initialPods.Items {
					initialPodNames[p.Name] = true
					for _, cs := range p.Status.ContainerStatuses {
						initialRestarts[p.Name+"/"+cs.Name] = cs.RestartCount
					}
				}

				// --- Soft-delete then immediately force-delete while adapters are still running ---

				ginkgo.By("Soft-delete the cluster to trigger adapter delete events")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
				Expect(deletedCluster.DeletedTime).NotTo(BeNil())

				ginkgo.By("Force-delete the cluster before adapters can finalize")
				err = h.Client.ForceDeleteCluster(ctx, clusterID, "E2E test: verify adapter handles 404 gracefully")
				Expect(err).NotTo(HaveOccurred(), "force-delete should succeed with 204")

				// --- Verify the adapter handles the 404 gracefully ---

				ginkgo.By("Verify cluster is gone (404)")
				Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Cluster.Deleted, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				// The adapter will try to GET the resource or POST status and receive 404.
				// A well-behaved adapter should not crash or restart.
				ginkgo.By("Verify adapter pods remain Running and Ready (no crash loop)")
				Consistently(func(g Gomega) {
					pods, err := h.K8sClient.CoreV1().Pods(h.Cfg.Namespace).List(ctx, metav1.ListOptions{
						LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
					})
					g.Expect(err).NotTo(HaveOccurred(), "failed to list adapter pods")
					g.Expect(pods.Items).NotTo(BeEmpty(), "adapter should have at least one pod")
					g.Expect(pods.Items).To(HaveLen(len(initialPodNames)),
						"expected %d adapter pods but found %d", len(initialPodNames), len(pods.Items))

					for _, pod := range pods.Items {
						g.Expect(initialPodNames).To(HaveKey(pod.Name),
							"unexpected pod %s appeared (possible pod replacement after crash)", pod.Name)
						g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning),
							"adapter pod %s should be Running, got %s", pod.Name, pod.Status.Phase)
						for _, cs := range pod.Status.ContainerStatuses {
							g.Expect(cs.Ready).To(BeTrue(),
								"container %s in pod %s should be Ready", cs.Name, pod.Name)
							g.Expect(cs.RestartCount).To(BeNumerically("==", initialRestarts[pod.Name+"/"+cs.Name]),
								"container %s in pod %s restart count changed (initial: %d, now: %d)",
								cs.Name, pod.Name, initialRestarts[pod.Name+"/"+cs.Name], cs.RestartCount)
						}
					}
				}, h.Cfg.Timeouts.Adapter.Processing/2, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Verified: adapter %s handled 404 gracefully, no crashes or restarts\n", adapterName)
			})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete][negative] Force-Delete Rejected for Invalid Preconditions",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("Create cluster (no reconciliation needed for negative tests)")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil())
			clusterID = *cluster.Id
			h.DeferClusterCleanup(clusterID)
		})

		ginkgo.It("should return 409 Conflict when force-deleting a cluster not in Finalizing", func(ctx context.Context) {
			// The cluster exists but is not in Finalizing state, so force-delete must be rejected
			err := h.Client.ForceDeleteCluster(ctx, clusterID, "should be rejected")
			Expect(err).To(HaveOccurred(), "force-delete should fail on non-Finalizing resource")

			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be an HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusConflict),
				"expected 409 Conflict, got %d", httpErr.StatusCode)

			// Verify the cluster is unchanged
			cluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.DeletedTime).To(BeNil(), "cluster should NOT have been deleted")
		})

		ginkgo.It("should return 400 Bad Request when force-deleting with empty reason", func(ctx context.Context) {
			// Soft-delete first so the cluster is in Finalizing (bypasses the 409 guard)
			ginkgo.By("Soft-delete the cluster to put it in Finalizing")
			_, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			// Empty reason violates the API contract (reason is required, 1-1024 chars)
			ginkgo.By("Attempt force-delete with empty reason")
			err = h.Client.ForceDeleteCluster(ctx, clusterID, "")
			Expect(err).To(HaveOccurred(), "force-delete with empty reason should fail")

			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be an HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusBadRequest),
				"expected 400 Bad Request, got %d", httpErr.StatusCode)
		})

		ginkgo.It("should return 404 Not Found when force-deleting a nonexistent resource", func(ctx context.Context) {
			// Use a UUID that doesn't map to any resource
			fakeID := "00000000-0000-0000-0000-000000000000"
			err := h.Client.ForceDeleteCluster(ctx, fakeID, "testing nonexistent resource")
			Expect(err).To(HaveOccurred(), "force-delete on nonexistent resource should fail")

			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be an HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
				"expected 404 Not Found, got %d", httpErr.StatusCode)
		})
	},
)
