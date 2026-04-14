package config

import (
	"fmt"
	"os"
)

type Config struct {
	AgentID           string
	SandboxID         string
	OrgID             string
	DriveEndpoint     string // ZIRALOOP_DRIVE_ENDPOINT — full URL to sandbox drive endpoint (uses Bridge API key for auth)
	EmbeddingEndpoint string // ZIRALOOP_EMBEDDING_ENDPOINT — proxy URL for embedding calls
	EmbeddingModel    string
	EmbeddingDims     int
	DBPath            string
}

func Load() (*Config, error) {
	cfg := &Config{
		AgentID:           os.Getenv("ZIRALOOP_AGENT_ID"),
		SandboxID:         os.Getenv("ZIRALOOP_SANDBOX_ID"),
		OrgID:             os.Getenv("ZIRALOOP_ORG_ID"),
		DriveEndpoint:     os.Getenv("ZIRALOOP_DRIVE_ENDPOINT"),
		EmbeddingEndpoint: os.Getenv("ZIRALOOP_EMBEDDING_ENDPOINT"),
		EmbeddingModel:    os.Getenv("ZIRALOOP_EMBEDDING_MODEL"),
		DBPath:            os.Getenv("ZIRALOOP_EMBEDDINGS_DB"),
	}

	if cfg.EmbeddingModel == "" {
		cfg.EmbeddingModel = "text-embedding-3-large"
	}

	if cfg.DBPath == "" {
		cfg.DBPath = "/tmp/ziraloop-vectors.db"
	}

	switch cfg.EmbeddingModel {
	case "text-embedding-3-large":
		cfg.EmbeddingDims = 3072
	case "text-embedding-3-small":
		cfg.EmbeddingDims = 1536
	default:
		cfg.EmbeddingDims = 3072
	}

	if cfg.EmbeddingEndpoint == "" {
		return nil, fmt.Errorf("ZIRALOOP_EMBEDDING_ENDPOINT is required")
	}

	return cfg, nil
}
