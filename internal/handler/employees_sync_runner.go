package handler

import (
	"context"
	"fmt"

	hsdk "github.com/usehiveloop/hermes/pkg/sdk"

	"github.com/usehiveloop/hiveloop/internal/hermes"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *EmployeeHandler) runEmployeeSync(ctx context.Context, agent *model.Agent, sb *model.Sandbox) (*hsdk.SyncResponse, error) {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt sidecar api key: %w", err)
	}
	syncReq, err := hermes.Compile(ctx, h.compileDeps, agent)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	client, err := hermes.New(sb.BridgeURL, apiKey)
	if err != nil {
		return nil, fmt.Errorf("init hermes client: %w", err)
	}
	return client.SyncConfig(ctx, *syncReq)
}

func toSyncResponseDTO(resp *hsdk.SyncResponse) syncEmployeeResponse {
	out := syncEmployeeResponse{}
	if resp == nil {
		return out
	}
	if resp.Applied != nil {
		out.Applied = *resp.Applied
	}
	if resp.Deleted != nil {
		out.Deleted = *resp.Deleted
	}
	if resp.ReposCloned != nil {
		out.ReposCloned = *resp.ReposCloned
	}
	if resp.RestartTriggered != nil {
		out.RestartTriggered = *resp.RestartTriggered
	}
	if resp.Errors != nil {
		out.Errors = *resp.Errors
	}
	return out
}
