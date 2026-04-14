package config

import (
	"fmt"
	"os"
)

type Config struct {
	AgentID           string
	SandboxID         string
	OrgID             string
	DriveEndpoint     string // ZIRALOOP_DRIVE_ENDPOINT — sandbox drive endpoint (uses Bridge API key for auth)
	EmbeddingEndpoint string // ZIRALOOP_EMBEDDING_ENDPOINT — proxy URL for embedding calls
	EmbeddingAPIKey   string // ZIRALOOP_EMBEDDING_API_KEY — proxy token (ptok_) for embedding auth
	EmbeddingModel    string // ZIRALOOP_EMBEDDING_MODEL — model name
	EmbeddingDims     int
	DBPath            string // ZIRALOOP_EMBEDDINGS_DB — local DB path
}

func Load() (*Config, error) {
	cfg := &Config{
		AgentID:           os.Getenv("ZIRALOOP_AGENT_ID"),
		SandboxID:         os.Getenv("ZIRALOOP_SANDBOX_ID"),
		OrgID:             os.Getenv("ZIRALOOP_ORG_ID"),
		DriveEndpoint:     os.Getenv("ZIRALOOP_DRIVE_ENDPOINT"),
		EmbeddingEndpoint: os.Getenv("ZIRALOOP_EMBEDDING_ENDPOINT"),
		EmbeddingAPIKey:   os.Getenv("ZIRALOOP_EMBEDDING_API_KEY"),
		EmbeddingModel:    os.Getenv("ZIRALOOP_EMBEDDING_MODEL"),
		DBPath:            os.Getenv("ZIRALOOP_EMBEDDINGS_DB"),
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

	var missing []string
	if cfg.EmbeddingEndpoint == "" {
		missing = append(missing, "ZIRALOOP_EMBEDDING_ENDPOINT")
	}
	if cfg.EmbeddingAPIKey == "" {
		missing = append(missing, "ZIRALOOP_EMBEDDING_API_KEY")
	}
	if cfg.EmbeddingModel == "" {
		missing = append(missing, "ZIRALOOP_EMBEDDING_MODEL")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"ziraloop-embeddings is disabled: missing required environment variables: %s. "+
				"This usually means no OpenAI credential was found in the org. "+
				"Add an OpenAI API key to your org credentials to enable code embeddings",
			fmt.Sprintf("%v", missing),
		)
	}

	return cfg, nil
}
