package daytona

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	apiclient "github.com/daytonaio/daytona/libs/api-client-go"
	daytonasdk "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	sdktypes "github.com/daytonaio/daytona/libs/sdk-go/pkg/types"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const signedURLTTLSeconds = 3600

type Config struct {
	APIURL              string
	APIKey              string
	Target              string
	BridgeBinaryVersion string
}

// Driver talks to Daytona exclusively through the official Go SDKs:
//   - sdk holds the high-level pkg/daytona client (sandbox/snapshot CRUD,
//     preview links, process execution, …).
//   - apiClient holds the lower-level generated api-client-go used for the
//     three endpoints the high-level SDK doesn't expose:
//     SetAutostopInterval, GetSignedPortPreviewUrl, GetSnapshotBuildLogs.
//
// All hand-rolled net/http calls were removed when this driver migrated, so
// any future endpoint should be added by extending this struct rather than
// reaching back to raw http.
type Driver struct {
	sdk                 *daytonasdk.Client
	apiClient           *apiclient.APIClient
	apiKey              string
	bridgeBinaryVersion string
}

func NewDriver(cfg Config) (*Driver, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	apiURL := strings.TrimSpace(cfg.APIURL)
	target := strings.TrimSpace(cfg.Target)

	if cfg.BridgeBinaryVersion == "" {
		return nil, fmt.Errorf("daytona: BridgeBinaryVersion is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("daytona: APIKey is required")
	}

	sdkClient, err := daytonasdk.NewClientWithConfig(&sdktypes.DaytonaConfig{
		APIKey: apiKey,
		APIUrl: apiURL,
		Target: target,
	})
	if err != nil {
		return nil, fmt.Errorf("creating daytona sdk client: %w", err)
	}

	apiClient, err := newAPIClient(apiURL)
	if err != nil {
		return nil, fmt.Errorf("creating daytona api client: %w", err)
	}

	return &Driver{
		sdk:                 sdkClient,
		apiClient:           apiClient,
		apiKey:              apiKey,
		bridgeBinaryVersion: cfg.BridgeBinaryVersion,
	}, nil
}

// newAPIClient mirrors what daytonasdk.NewClientWithConfig does internally
// for the api-client-go layer, so endpoints not surfaced by the high-level
// SDK still go through the same generated request pipeline.
func newAPIClient(apiURL string) (*apiclient.APIClient, error) {
	if apiURL == "" {
		// Mirror api-client-go's default base path when caller leaves it
		// blank — pkg/daytona does the same.
		return apiclient.NewAPIClient(apiclient.NewConfiguration()), nil
	}
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parsing API URL %q: %w", apiURL, err)
	}
	cfg := apiclient.NewConfiguration()
	cfg.Host = parsed.Host
	cfg.Scheme = parsed.Scheme
	cfg.Servers = apiclient.ServerConfigurations{
		{URL: fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)},
	}
	return apiclient.NewAPIClient(cfg), nil
}

// authCtx attaches the Daytona API key as the Bearer token for api-client-go
// calls — same shape pkg/daytona's getAuthContext uses internally.
func (d *Driver) authCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, apiclient.ContextAccessToken, d.apiKey)
}

func (d *Driver) CreateSandbox(ctx context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	base := sdktypes.SandboxBaseParams{
		EnvVars: opts.EnvVars,
		Labels:  opts.Labels,
		Public:  false,
	}

	// SDK's Client.Create switches on value types (types.SnapshotParams,
	// types.ImageParams) — passing pointers falls through to the default
	// branch which silently drops env vars + labels and uses the platform's
	// default image. Pass values.
	var params any
	if opts.SnapshotID != "" {
		params = sdktypes.SnapshotParams{
			SandboxBaseParams: base,
			Snapshot:          opts.SnapshotID,
		}
	} else {
		params = sdktypes.ImageParams{
			SandboxBaseParams: base,
			Image:             "hiveloop/bridge:latest",
		}
	}

	sb, err := d.sdk.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
	}

	if err := sb.WaitForStart(ctx, 3*time.Minute); err != nil {
		return nil, fmt.Errorf("waiting for sandbox %s: %w", sb.ID, err)
	}

	return &sandbox.SandboxInfo{
		ExternalID: sb.ID,
		Status:     sandbox.StatusRunning,
	}, nil
}

func (d *Driver) DeleteSandbox(ctx context.Context, externalID string) error {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	if err := sb.Delete(ctx); err != nil {
		return fmt.Errorf("deleting sandbox %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) GetStatus(ctx context.Context, externalID string) (sandbox.SandboxStatus, error) {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return sandbox.StatusError, sandbox.ErrSandboxNotFound
		}
		return sandbox.StatusError, fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	return mapState(string(sb.State)), nil
}

func (d *Driver) StartSandbox(ctx context.Context, externalID string) error {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	if err := sb.Start(ctx); err != nil {
		return fmt.Errorf("starting sandbox %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) StopSandbox(ctx context.Context, externalID string) error {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	if err := sb.Stop(ctx); err != nil {
		return fmt.Errorf("stopping sandbox %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) ArchiveSandbox(ctx context.Context, externalID string) error {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	if err := sb.Archive(ctx); err != nil {
		return fmt.Errorf("archiving sandbox %s: %w", externalID, err)
	}
	return nil
}

// isSDKNotFound returns true if the SDK error wraps a 404. The SDK doesn't
// expose its DaytonaError type with a stable code field, so we substring-match
// — coarse but practical until the SDK exposes a typed sentinel.
func isSDKNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}
