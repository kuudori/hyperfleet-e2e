package client

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/util"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateVersion(ctx context.Context, channelID string, req ResourceCreateRequest) (*Resource, error) {
	logger.Info("creating version", "channel_id", channelID, "name", req.Name)
	version, err := c.CreateResource(ctx, ChannelsPath+"/"+channelID+"/"+VersionsPath, req)
	if err != nil {
		return nil, fmt.Errorf("create version %q in channel %s: %w", req.Name, channelID, err)
	}
	logger.Info("version created", "channel_id", channelID, "version_id", util.FromPtr(version.Id), "name", req.Name)
	return version, nil
}

func (c *HyperFleetClient) GetVersion(ctx context.Context, channelID, versionID string) (*Resource, error) {
	return c.GetResource(ctx, ChannelsPath+"/"+channelID+"/"+VersionsPath+"/"+versionID)
}

func (c *HyperFleetClient) ListVersions(ctx context.Context, channelID, search string) (*ResourceList, error) {
	return c.ListResources(ctx, ChannelsPath+"/"+channelID+"/"+VersionsPath, search)
}

func (c *HyperFleetClient) DeleteVersion(ctx context.Context, channelID, versionID string) (*Resource, error) {
	logger.Info("deleting version", "channel_id", channelID, "version_id", versionID)
	version, err := c.DeleteResource(ctx, ChannelsPath+"/"+channelID+"/"+VersionsPath+"/"+versionID)
	if err != nil {
		return nil, fmt.Errorf("delete version %s in channel %s: %w", versionID, channelID, err)
	}
	logger.Info("version deleted", "channel_id", channelID, "version_id", versionID)
	return version, nil
}

func (c *HyperFleetClient) PatchVersion(ctx context.Context, channelID, versionID string, req ResourcePatchRequest) (*Resource, error) {
	logger.Info("patching version", "channel_id", channelID, "version_id", versionID)
	version, err := c.PatchResource(ctx, ChannelsPath+"/"+channelID+"/"+VersionsPath+"/"+versionID, req)
	if err != nil {
		return nil, fmt.Errorf("patch version %s in channel %s: %w", versionID, channelID, err)
	}
	logger.Info("version patched", "channel_id", channelID, "version_id", versionID, "generation", version.Generation)
	return version, nil
}

func (c *HyperFleetClient) CreateVersionFromPayload(ctx context.Context, channelID, payloadPath string) (*Resource, error) {
	return c.CreateResourceFromPayload(ctx, ChannelsPath+"/"+channelID+"/"+VersionsPath, payloadPath)
}
