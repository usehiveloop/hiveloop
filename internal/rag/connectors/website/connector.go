// Package website is a RAG connector that crawls a public website via
// the Spider.cloud client and emits one Document per page in markdown.
//
// Crawl strategy: BFS from the seed URL, with sitemap-aware discovery
// turned on so the site's own curated URL list fills the budget first.
// MaxPages caps the run (default 500). Subdomains and link-budget
// tunables stay zero-valued for v1; revisit when callers need them.
package website

import (
	"context"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	"github.com/usehiveloop/hiveloop/internal/spider"
)

const Kind = "website"

var _ interfaces.Connector = (*WebsiteConnector)(nil)

type WebsiteConnector struct {
	cfg    WebsiteConfig
	spider *spider.Client
}

func NewConnector(cfg WebsiteConfig, sp *spider.Client) *WebsiteConnector {
	return &WebsiteConnector{cfg: cfg, spider: sp}
}

func (c *WebsiteConnector) Kind() string { return Kind }

func (c *WebsiteConnector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	if deps.Spider == nil {
		return nil, fmt.Errorf("website: spider client not configured")
	}
	return NewConnector(cfg, deps.Spider), nil
}
