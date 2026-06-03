package client

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateVersion(ctx context.Context, channelID string, req ResourceCreateRequest) (*Resource, error) {
	logger.Info("creating version", "channel_id", channelID, "name", req.Name)
	version, err := c.CreateResource(ctx, fmt.Sprintf("channels/%s/versions", channelID), req)
	if err != nil {
		return nil, fmt.Errorf("create version %q in channel %s: %w", req.Name, channelID, err)
	}
	logger.Info("version created", "channel_id", channelID, "version_id", lo.FromPtr(version.Id), "name", req.Name)
	return version, nil
}

func (c *HyperFleetClient) GetVersion(ctx context.Context, channelID, versionID string) (*Resource, error) {
	return c.GetResource(ctx, fmt.Sprintf("channels/%s/versions/%s", channelID, versionID))
}

func (c *HyperFleetClient) ListVersions(ctx context.Context, channelID, search string) (*ResourceList, error) {
	return c.ListResources(ctx, fmt.Sprintf("channels/%s/versions", channelID), search)
}

func (c *HyperFleetClient) DeleteVersion(ctx context.Context, channelID, versionID string) (*Resource, error) {
	logger.Info("deleting version", "channel_id", channelID, "version_id", versionID)
	version, err := c.DeleteResource(ctx, fmt.Sprintf("channels/%s/versions/%s", channelID, versionID))
	if err != nil {
		return nil, err
	}
	logger.Info("version deleted", "channel_id", channelID, "version_id", versionID)
	return version, nil
}

func (c *HyperFleetClient) PatchVersion(ctx context.Context, channelID, versionID string, req ResourcePatchRequest) (*Resource, error) {
	logger.Info("patching version", "channel_id", channelID, "version_id", versionID)
	version, err := c.PatchResource(ctx, fmt.Sprintf("channels/%s/versions/%s", channelID, versionID), req)
	if err != nil {
		return nil, err
	}
	logger.Info("version patched", "channel_id", channelID, "version_id", versionID, "generation", version.Generation)
	return version, nil
}

func (c *HyperFleetClient) CreateVersionFromPayload(ctx context.Context, channelID, payloadPath string) (*Resource, error) {
	return c.CreateResourceFromPayload(ctx, fmt.Sprintf("channels/%s/versions", channelID), payloadPath)
}
