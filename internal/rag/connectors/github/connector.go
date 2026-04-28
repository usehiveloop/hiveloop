package github

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// pollOverlap absorbs clock skew + GitHub indexing latency between
// successive fetches. Mirrors backend/onyx/connectors/github/connector.py:802.
const pollOverlap = 3 * time.Hour

const Kind = "github"

var (
	_ interfaces.CheckpointedConnector[GithubCheckpoint] = (*GithubConnector)(nil)
	_ interfaces.PermSyncConnector                       = (*GithubConnector)(nil)
	_ interfaces.SlimConnector                           = (*GithubConnector)(nil)
	_ interfaces.EstimatingConnector                     = (*GithubConnector)(nil)
)

type GithubConnector struct {
	cfg        GithubConfig
	client     *Client
	channelBuf int

	finalCp atomic.Pointer[GithubCheckpoint]
}

func NewConnector(cfg GithubConfig, p proxyClient) *GithubConnector {
	return &GithubConnector{
		cfg:        cfg,
		client:     newClient(p),
		channelBuf: pageSize * 2,
	}
}

func (c *GithubConnector) Kind() string { return Kind }

func (c *GithubConnector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

func (c *GithubConnector) DummyCheckpoint() GithubCheckpoint { return dummyCheckpoint() }

func (c *GithubConnector) UnmarshalCheckpoint(raw json.RawMessage) (GithubCheckpoint, error) {
	return unmarshalCheckpoint(raw)
}

// LoadFromCheckpoint applies pollOverlap to start internally, so the
// effective window may be wider than the window passed in.
func (c *GithubConnector) LoadFromCheckpoint(
	ctx context.Context,
	src interfaces.Source,
	cp GithubCheckpoint,
	start, end time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	if !cp.Stage.IsValid() {
		cp = dummyCheckpoint()
	}
	if cp.Stage == StageStart {
		if len(c.cfg.Repositories) == 0 {
			return nil, errors.New("github: at least one repository must be configured")
		}
		cp.RepoIDsRemaining = make([]string, 0, len(c.cfg.Repositories))
		for _, repo := range c.cfg.Repositories {
			cp.RepoIDsRemaining = append(cp.RepoIDsRemaining, c.cfg.RepoOwner+"/"+repo)
		}
		cp.Stage = c.firstEnabledStage()
		cp.CurrPage = 1
	}

	effectiveStart := start
	if !start.IsZero() {
		effectiveStart = start.Add(-pollOverlap)
	}

	out := make(chan interfaces.DocumentOrFailure, c.channelBuf)
	go c.run(ctx, src, cp, effectiveStart, end, out)
	return out, nil
}

func (c *GithubConnector) firstEnabledStage() Stage {
	if c.cfg.IncludePRs {
		return StagePRs
	}
	if c.cfg.IncludeIssues {
		return StageIssues
	}
	return StageDone
}

func (c *GithubConnector) nextEnabledStage(s Stage) Stage {
	switch s {
	case StagePRs:
		if c.cfg.IncludeIssues {
			return StageIssues
		}
		return StageDone
	case StageIssues:
		return StageDone
	default:
		return StageDone
	}
}

func (c *GithubConnector) run(
	ctx context.Context,
	_ interfaces.Source,
	cp GithubCheckpoint,
	start, end time.Time,
	out chan<- interfaces.DocumentOrFailure,
) {
	defer close(out)
	defer func() {
		final := cp
		c.finalCp.Store(&final)
	}()

	slog.Info("github run: start",
		"repos", c.cfg.Repositories, "owner", c.cfg.RepoOwner,
		"include_prs", c.cfg.IncludePRs, "include_issues", c.cfg.IncludeIssues,
		"state", c.cfg.StateFilter, "stage", cp.Stage,
		"window_start", start, "window_end", end,
		"repos_remaining", len(cp.RepoIDsRemaining))
	defer func() {
		slog.Info("github run: end", "final_stage", cp.Stage)
	}()

	access := map[string]*interfaces.ExternalAccess{}

	for cp.Stage != StageDone {
		if cp.CurrentRepoFullName == nil {
			if len(cp.RepoIDsRemaining) == 0 {
				cp.Stage = c.nextEnabledStage(cp.Stage)
				cp.CurrPage = 1
				cp.RepoIDsRemaining = c.repoFullNames()
				continue
			}
			full := cp.RepoIDsRemaining[0]
			cp.RepoIDsRemaining = cp.RepoIDsRemaining[1:]
			cp.CurrentRepoFullName = &full
			cp.CurrPage = 1
		}

		full := *cp.CurrentRepoFullName
		// On visibility-fetch failure we skip the repo rather than
		// emit PRs without ACL data.
		acc, ok := access[full]
		if !ok {
			ext, err := c.computeRepoAccess(ctx, full)
			if err != nil {
				slog.Warn("github run: visibility fetch failed",
					"repo", full, "err", err)
				out <- interfaces.NewDocFailure(entityFailure(full, "github: resolve repo visibility", err))
				cp.CurrentRepoFullName = nil
				continue
			}
			slog.Info("github run: visibility resolved",
				"repo", full, "stage", cp.Stage, "page", cp.CurrPage)
			access[full] = ext
			acc = ext
		}

		var done bool
		switch cp.Stage {
		case StagePRs:
			done = fetchPRsPage(ctx, c.client, full, c.cfg.StateFilter, &cp, start, end, acc, out)
		case StageIssues:
			done = fetchIssuesPage(ctx, c.client, full, c.cfg.StateFilter, &cp, start, end, acc, out)
		default:
			done = true
		}
		slog.Info("github run: page fetched",
			"repo", full, "stage", cp.Stage, "page", cp.CurrPage, "done", done)
		if !done {
			continue
		}
		cp.CurrentRepoFullName = nil
		cp.CurrentRepoID = nil
		cp.CurrPage = 1
	}
}

func (c *GithubConnector) repoFullNames() []string {
	out := make([]string, 0, len(c.cfg.Repositories))
	for _, r := range c.cfg.Repositories {
		out = append(out, c.cfg.RepoOwner+"/"+r)
	}
	return out
}

func (c *GithubConnector) computeRepoAccess(ctx context.Context, fullName string) (*interfaces.ExternalAccess, error) {
	repo, err := c.client.getRepo(ctx, fullName)
	if err != nil {
		return nil, err
	}
	return mapVisibility(repo), nil
}

func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	connectionID, providerKey := connectionFromSource(src)
	return NewConnector(cfg, newNangoProxy(deps.Nango, providerKey, connectionID)), nil
}

// connectionFromSource returns ("", "") when the Source doesn't expose
// the Nango fields — the connector then fails fast on the first proxy
// call rather than silently no-op'ing.
func connectionFromSource(src interfaces.Source) (string, string) {
	type connectionSource interface {
		NangoConnectionID() string
		NangoProviderConfigKey() string
	}
	if cs, ok := src.(connectionSource); ok {
		return cs.NangoConnectionID(), cs.NangoProviderConfigKey()
	}
	return "", Kind
}
