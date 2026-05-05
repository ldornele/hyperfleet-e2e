package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// CreateCluster creates a new cluster and returns the created cluster object.
func (c *HyperFleetClient) CreateCluster(ctx context.Context, req openapi.ClusterCreateRequest) (*openapi.Cluster, error) {
	logger.Info("creating cluster", "name", req.Name)

	resp, err := c.PostCluster(ctx, req)
	if err != nil {
		logger.Error("failed to create cluster", "name", req.Name, "error", err)
		return nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	cluster, err := handleHTTPResponse[openapi.Cluster](resp, http.StatusCreated, "create cluster")
	if err != nil {
		return nil, err
	}

	logger.Info("cluster created", "cluster_id", *cluster.Id, "name", req.Name)
	return cluster, nil
}

// GetCluster retrieves a cluster by ID.
func (c *HyperFleetClient) GetCluster(ctx context.Context, clusterID string) (*openapi.Cluster, error) {
	resp, err := c.GetClusterById(ctx, clusterID, &openapi.GetClusterByIdParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}
	return handleHTTPResponse[openapi.Cluster](resp, http.StatusOK, "get cluster")
}

// ListClusters retrieves all clusters.
func (c *HyperFleetClient) ListClusters(ctx context.Context) (*openapi.ClusterList, error) {
	resp, err := c.GetClusters(ctx, &openapi.GetClustersParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	return handleHTTPResponse[openapi.ClusterList](resp, http.StatusOK, "list clusters")
}

// GetClusterStatuses retrieves all adapter statuses for a cluster.
func (c *HyperFleetClient) GetClusterStatuses(ctx context.Context, clusterID string) (*openapi.AdapterStatusList, error) {
	resp, err := c.Client.GetClusterStatuses(ctx, clusterID, &openapi.GetClusterStatusesParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster statuses: %w", err)
	}
	return handleHTTPResponse[openapi.AdapterStatusList](resp, http.StatusOK, "get cluster statuses")
}

// CreateClusterFromPayload creates a cluster from a JSON payload file.
// The payload file should contain a ClusterCreateRequest in JSON format.
func (c *HyperFleetClient) CreateClusterFromPayload(ctx context.Context, payloadPath string) (*openapi.Cluster, error) {
	logger.Debug("loading cluster payload", "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.ClusterCreateRequest](payloadPath)
	if err != nil {
		logger.Error("failed to load payload", "payload_path", payloadPath, "error", err)
		return nil, err
	}

	return c.CreateCluster(ctx, *req)
}

// DeleteCluster soft-deletes a cluster by ID (sets deleted_time, returns 202).
func (c *HyperFleetClient) DeleteCluster(ctx context.Context, clusterID string) (*openapi.Cluster, error) {
	logger.Info("deleting cluster", "cluster_id", clusterID)

	resp, err := c.DeleteClusterById(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete cluster: %w", err)
	}

	cluster, err := handleHTTPResponse[openapi.Cluster](resp, http.StatusAccepted, "delete cluster")
	if err != nil {
		return nil, err
	}

	logger.Info("cluster deleted", "cluster_id", clusterID)
	return cluster, nil
}

// PatchCluster updates a cluster via PATCH.
func (c *HyperFleetClient) PatchCluster(ctx context.Context, clusterID string, req openapi.ClusterPatchRequest) (*openapi.Cluster, error) {
	logger.Info("patching cluster", "cluster_id", clusterID)

	resp, err := c.PatchClusterById(ctx, clusterID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to patch cluster: %w", err)
	}

	cluster, err := handleHTTPResponse[openapi.Cluster](resp, http.StatusOK, "patch cluster")
	if err != nil {
		return nil, err
	}

	logger.Info("cluster patched", "cluster_id", clusterID, "generation", cluster.Generation)
	return cluster, nil
}

// PatchClusterFromPayload patches a cluster from a JSON payload file.
func (c *HyperFleetClient) PatchClusterFromPayload(ctx context.Context, clusterID, payloadPath string) (*openapi.Cluster, error) {
	logger.Debug("loading cluster patch payload", "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.ClusterPatchRequest](payloadPath)
	if err != nil {
		return nil, err
	}

	return c.PatchCluster(ctx, clusterID, *req)
}

// PatchClusterRaw sends a PATCH request and returns the raw HTTP response for status inspection.
func (c *HyperFleetClient) PatchClusterRaw(ctx context.Context, clusterID string, req openapi.ClusterPatchRequest) (*http.Response, error) {
	return c.PatchClusterById(ctx, clusterID, req)
}
