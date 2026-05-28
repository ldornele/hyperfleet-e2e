package helper

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// Helper provides utility functions for e2e tests
type Helper struct {
	Cfg           *config.Config
	Client        *client.HyperFleetClient
	K8sClient     *k8sclient.Client
	MaestroClient *maestro.Client
}

// TestDataPath resolves a relative path within the testdata directory
// This ensures testdata paths work correctly whether invoked via go test or the e2e binary
func (h *Helper) TestDataPath(relativePath string) string {
	return filepath.Join(h.Cfg.TestDataDir, relativePath)
}

// GetTestCluster creates a new temporary test cluster
func (h *Helper) GetTestCluster(ctx context.Context, payloadPath string) (string, error) {
	cluster, err := h.Client.CreateClusterFromPayload(ctx, payloadPath)
	if err != nil {
		return "", err
	}
	if cluster == nil {
		return "", fmt.Errorf("CreateClusterFromPayload returned nil")
	}
	if cluster.Id == nil {
		return "", fmt.Errorf("created cluster has no ID")
	}
	return *cluster.Id, nil
}

// CleanupTestCluster deletes the test cluster via the HyperFleet API and waits for hard-delete (404).
// The API DELETE owns the full cleanup lifecycle: adapter finalization, Maestro teardown, namespace deletion.
func (h *Helper) CleanupTestCluster(ctx context.Context, clusterID string) error {
	logger.Info("deleting cluster via API", "cluster_id", clusterID)

	if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
		return fmt.Errorf("delete cluster %s: %w", clusterID, err)
	}

	pollFn := h.PollClusterHTTPStatus(ctx, clusterID)
	deadline := time.Now().Add(h.Cfg.Timeouts.Cluster.Deleted)
	for time.Now().Before(deadline) {
		status, err := pollFn()
		if err != nil {
			return fmt.Errorf("polling hard-delete for cluster %s: %w", clusterID, err)
		}
		if status == http.StatusNotFound {
			logger.Info("cluster hard-deleted", "cluster_id", clusterID)
			return nil
		}
		if status >= 400 {
			return fmt.Errorf("unexpected HTTP %d while waiting for cluster %s hard-delete", status, clusterID)
		}
		time.Sleep(h.Cfg.Polling.Interval)
	}

	return fmt.Errorf("cluster %s not hard-deleted within %s", clusterID, h.Cfg.Timeouts.Cluster.Deleted)
}

// CleanupTestNodePool deletes the test nodepool via the HyperFleet API and waits for hard-delete (404).
// The API DELETE owns the full cleanup lifecycle.
func (h *Helper) CleanupTestNodePool(ctx context.Context, clusterID, nodepoolID string) error {
	logger.Info("deleting nodepool via API", "cluster_id", clusterID, "nodepool_id", nodepoolID)

	if _, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID); err != nil {
		return fmt.Errorf("delete nodepool %s: %w", nodepoolID, err)
	}

	pollFn := h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID)
	deadline := time.Now().Add(h.Cfg.Timeouts.NodePool.Reconciled)
	for time.Now().Before(deadline) {
		status, err := pollFn()
		if err != nil {
			return fmt.Errorf("polling hard-delete for nodepool %s: %w", nodepoolID, err)
		}
		if status == http.StatusNotFound {
			logger.Info("nodepool hard-deleted", "cluster_id", clusterID, "nodepool_id", nodepoolID)
			return nil
		}
		if status >= 400 {
			return fmt.Errorf("unexpected HTTP %d while waiting for nodepool %s hard-delete", status, nodepoolID)
		}
		time.Sleep(h.Cfg.Polling.Interval)
	}

	return fmt.Errorf("nodepool %s not hard-deleted within %s", nodepoolID, h.Cfg.Timeouts.NodePool.Reconciled)
}

// GetMaestroClient returns the Maestro client, initializing it lazily on first access
// This avoids the overhead of K8s service discovery for test suites that don't use Maestro
func (h *Helper) GetMaestroClient() *maestro.Client {
	if h.MaestroClient == nil {
		h.MaestroClient = maestro.NewClient("")
	}
	return h.MaestroClient
}
