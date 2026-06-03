package client

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateChannel(ctx context.Context, req ResourceCreateRequest) (*Resource, error) {
	logger.Info("creating channel", "name", req.Name)
	channel, err := c.CreateResource(ctx, "channels", req)
	if err != nil {
		return nil, fmt.Errorf("create channel %q: %w", req.Name, err)
	}
	logger.Info("channel created", "channel_id", lo.FromPtr(channel.Id), "name", req.Name)
	return channel, nil
}

func (c *HyperFleetClient) GetChannel(ctx context.Context, channelID string) (*Resource, error) {
	return c.GetResource(ctx, fmt.Sprintf("channels/%s", channelID))
}

func (c *HyperFleetClient) ListChannels(ctx context.Context, search string) (*ResourceList, error) {
	return c.ListResources(ctx, "channels", search)
}

func (c *HyperFleetClient) DeleteChannel(ctx context.Context, channelID string) (*Resource, error) {
	logger.Info("deleting channel", "channel_id", channelID)
	channel, err := c.DeleteResource(ctx, fmt.Sprintf("channels/%s", channelID))
	if err != nil {
		return nil, err
	}
	logger.Info("channel deleted", "channel_id", channelID)
	return channel, nil
}

func (c *HyperFleetClient) PatchChannel(ctx context.Context, channelID string, req ResourcePatchRequest) (*Resource, error) {
	logger.Info("patching channel", "channel_id", channelID)
	channel, err := c.PatchResource(ctx, fmt.Sprintf("channels/%s", channelID), req)
	if err != nil {
		return nil, err
	}
	logger.Info("channel patched", "channel_id", channelID, "generation", channel.Generation)
	return channel, nil
}

func (c *HyperFleetClient) CreateChannelFromPayload(ctx context.Context, payloadPath string) (*Resource, error) {
	return c.CreateResourceFromPayload(ctx, "channels", payloadPath)
}
