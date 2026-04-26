// Top-level orchestrator. Drives the linear walk over the configured
// repos, dispatches to fetch_prs.go / fetch_issues.go per stage, advances
// the checkpoint, and threads per-doc ExternalAccess through every
// emitted Document.
//
// Onyx analog: GithubConnector at backend/onyx/connectors/github/connector.py:437
// (`_fetch_from_github`). The high-level shape — stage-machine, repo
// queue, page cursor — matches Onyx exactly. The 3-hour overlap on the
// poll-window start (connector.py:802) is applied here as pollOverlap.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// pollOverlap is the connector-internal back-step on the window-start.
// Matches connector.py:802 — catches PRs/Issues that updated just before
// the last successful run finished, accounting for clock skew + GitHub
// indexing latency.
const pollOverlap = 3 * time.Hour

// Kind is the registry key. Exposed because init.go references it; keep
// in sync with the providerConfigKey ("github") used by the Nango
// integration row.
const Kind = "github"

// Compile-time interface conformance assertions. If the connector
// stops satisfying any of these the build breaks at this point rather
// than at the registry consumer in cmd/server.
var (
	_ interfaces.CheckpointedConnector[GithubCheckpoint] = (*GithubConnector)(nil)
	_ interfaces.PermSyncConnector                       = (*GithubConnector)(nil)
	_ interfaces.SlimConnector                           = (*GithubConnector)(nil)
)

// GithubConnector is the registered connector. Owns the resolved config
// + the typed client over Nango's proxy. One instance per RAGSource per
// run; the factory builds a fresh one each time.
type GithubConnector struct {
	cfg    GithubConfig
	client *Client
	// channelBuf is the size of the DocumentOrFailure channel. Plenty
	// large to absorb a full page of fetches before the consumer drains.
	channelBuf int
}

// NewConnector is the public constructor for tests. Production goes
// through init.go's factory.
func NewConnector(cfg GithubConfig, p proxyClient) *GithubConnector {
	return &GithubConnector{
		cfg:        cfg,
		client:     newClient(p),
		channelBuf: pageSize * 2,
	}
}

// Kind satisfies interfaces.Connector.
func (c *GithubConnector) Kind() string { return Kind }

// ValidateConfig parses src.Config and returns an error if the JSON is
// malformed or fails LoadConfig's invariants. No network calls — that
// belongs in a separate validate step (left for a future tranche; Onyx
// does it via validate_connector_settings, gated on credentials we don't
// hold here).
func (c *GithubConnector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

// DummyCheckpoint satisfies interfaces.CheckpointedConnector.
func (c *GithubConnector) DummyCheckpoint() GithubCheckpoint { return dummyCheckpoint() }

// UnmarshalCheckpoint satisfies interfaces.CheckpointedConnector.
func (c *GithubConnector) UnmarshalCheckpoint(raw json.RawMessage) (GithubCheckpoint, error) {
	return unmarshalCheckpoint(raw)
}

// LoadFromCheckpoint runs the ingest. Returns immediately with the
// channel; the worker goroutine drains repos + pages and closes the
// channel on completion. Errors from the orchestrator (config parse,
// repo resolution) come back via the error return; per-doc + per-page
// failures travel as ConnectorFailure on the channel.
//
// The connector applies pollOverlap to start internally (connector.py:802),
// so the window the caller sees may be wider than the window passed in.
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
		// Initial entry: resolve repos. The config either pins them
		// (Repositories non-empty) or asks for everything under RepoOwner.
		// The "everything" branch hits a list endpoint we don't fetch
		// in this tranche — config.Repositories is required for now;
		// Onyx parity follows when org-wide enumeration is wired.
		if len(c.cfg.Repositories) == 0 {
			return nil, errors.New("github: at least one repository must be configured")
		}
		cp.RepoIDsRemaining = make([]string, 0, len(c.cfg.Repositories))
		for _, repo := range c.cfg.Repositories {
			cp.RepoIDsRemaining = append(cp.RepoIDsRemaining, c.cfg.RepoOwner+"/"+repo)
		}
		// Skip directly to whichever stage is enabled.
		cp.Stage = c.firstEnabledStage()
		cp.CurrPage = 1
	}

	// Apply pollOverlap to start (only meaningful when start is non-zero;
	// a fresh-from-beginning run has start == epoch and the subtraction
	// is harmless).
	effectiveStart := start
	if !start.IsZero() {
		effectiveStart = start.Add(-pollOverlap)
	}

	out := make(chan interfaces.DocumentOrFailure, c.channelBuf)
	go c.run(ctx, src, cp, effectiveStart, end, out)
	return out, nil
}

// firstEnabledStage picks PRS, falling back to ISSUES if PRs are
// disabled, or DONE if neither is.
func (c *GithubConnector) firstEnabledStage() Stage {
	if c.cfg.IncludePRs {
		return StagePRs
	}
	if c.cfg.IncludeIssues {
		return StageIssues
	}
	return StageDone
}

// nextEnabledStage advances from the current stage to the next enabled
// one. Pure function over cfg.
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

// run is the goroutine body. Walks the queue of repos, dispatches per
// stage, closes the channel on exit.
func (c *GithubConnector) run(
	ctx context.Context,
	_ interfaces.Source,
	cp GithubCheckpoint,
	start, end time.Time,
	out chan<- interfaces.DocumentOrFailure,
) {
	defer close(out)

	// Per-repo ExternalAccess cache: we resolve visibility once when we
	// first touch a repo, then thread the same access into every PR/Issue
	// from that repo so the rest of the loop is allocation-free.
	access := map[string]*interfaces.ExternalAccess{}

	for cp.Stage != StageDone {
		// Pop a repo if we don't have one in flight.
		if cp.CurrentRepoFullName == nil {
			if len(cp.RepoIDsRemaining) == 0 {
				// Stage exhausted; advance.
				cp.Stage = c.nextEnabledStage(cp.Stage)
				cp.CurrPage = 1
				// Re-prime the queue for the new stage.
				cp.RepoIDsRemaining = c.repoFullNames()
				continue
			}
			full := cp.RepoIDsRemaining[0]
			cp.RepoIDsRemaining = cp.RepoIDsRemaining[1:]
			cp.CurrentRepoFullName = &full
			cp.CurrPage = 1
		}

		full := *cp.CurrentRepoFullName
		// Lazily resolve this repo's ExternalAccess. If the visibility
		// fetch fails we emit an entity failure for the repo and skip it
		// — better than throwing PRs into the index without ACL data.
		acc, ok := access[full]
		if !ok {
			ext, err := c.computeRepoAccess(ctx, full)
			if err != nil {
				out <- interfaces.NewDocFailure(entityFailure(full, "github: resolve repo visibility", err))
				cp.CurrentRepoFullName = nil
				continue
			}
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
		if !done {
			// More pages in this repo for this stage — loop continues.
			continue
		}
		// Repo done at this stage; clear the cursor.
		cp.CurrentRepoFullName = nil
		cp.CurrentRepoID = nil
		cp.CurrPage = 1
	}
}

// repoFullNames is a small helper that rebuilds the repo queue from
// config when transitioning between stages. Each stage walks every
// configured repo independently.
func (c *GithubConnector) repoFullNames() []string {
	out := make([]string, 0, len(c.cfg.Repositories))
	for _, r := range c.cfg.Repositories {
		out = append(out, c.cfg.RepoOwner+"/"+r)
	}
	return out
}

// computeRepoAccess derives ExternalAccess from a repo's visibility +
// org metadata. Lives here (not in perm_sync.go) because the ingest
// path needs it on every Document.
func (c *GithubConnector) computeRepoAccess(ctx context.Context, fullName string) (*interfaces.ExternalAccess, error) {
	repo, err := c.client.getRepo(ctx, fullName)
	if err != nil {
		return nil, err
	}
	return mapVisibility(repo), nil
}

// Build is the registered factory. Constructs a connector from a
// RAGSource + a *nango.Client. The connection ID is read off the
// RAGSource's config (jsonb) — a real production wiring will pull it
// off the InConnection row, but until 3E ships the API surface that
// stamps the row, the test path takes a static value through Config.
//
// Production calls this from interfaces.Lookup("github") inside the
// scheduler.
func Build(src interfaces.Source, client *nango.Client) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	// The connection ID is an attribute of the RAGSource→InConnection
	// link, not the Config blob, so the Source interface alone can't
	// surface it. Reading it via a typed assertion keeps this layer
	// independent of the concrete RAGSource type.
	connectionID, providerKey := connectionFromSource(src)
	return NewConnector(cfg, newNangoProxy(client, providerKey, connectionID)), nil
}

// connectionFromSource pulls the Nango connection ID + provider key off
// a Source. Returns ("", "") when the Source doesn't expose them — the
// connector then fails fast on the first proxy call rather than
// silently no-op'ing.
//
// Production's RAGSource will satisfy a richer interface (TBD in 3E);
// for now we look for an embedded type assertion. The factory composes
// with the real *RAGSource via a small wrapper at the call site.
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
