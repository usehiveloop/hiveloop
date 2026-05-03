package daytona

import (
	"context"
	"fmt"
)

// GetEndpoint returns a TTL-signed preview URL for the given sandbox port.
// Goes through api-client-go's GetSignedPortPreviewUrl since pkg/daytona's
// Sandbox.GetPreviewLink only exposes the un-signed (URL + token) variant.
func (d *Driver) GetEndpoint(ctx context.Context, externalID string, port int) (string, error) {
	resp, _, err := d.apiClient.SandboxAPI.
		GetSignedPortPreviewUrl(d.authCtx(ctx), externalID, int32(port)).
		ExpiresInSeconds(signedURLTTLSeconds).
		Execute()
	if err != nil {
		return "", fmt.Errorf("getting signed preview URL for sandbox %s port %d: %w", externalID, port, err)
	}
	if resp == nil {
		return "", fmt.Errorf("daytona returned no signed preview URL for sandbox %s port %d", externalID, port)
	}
	return resp.GetUrl(), nil
}
