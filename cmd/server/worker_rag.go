package main

import (
	"context"
	"log/slog"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
	"github.com/usehiveloop/hiveloop/internal/spider"
)

func buildRagDeps(
	ctx context.Context,
	cfg *config.Config,
	db *gorm.DB,
	nangoClient *nango.Client,
	spiderClient *spider.Client,
) *ragtasks.Deps {
	if cfg.QdrantHost == "" {
		slog.Warn("rag worker: QDRANT_HOST not set — rag:* handlers disabled")
		return nil
	}
	if cfg.LLMAPIURL == "" || cfg.LLMAPIKey == "" || cfg.LLMModel == "" {
		slog.Warn("rag worker: LLM_API_URL/LLM_API_KEY/LLM_MODEL not set — rag:* handlers disabled")
		return nil
	}
	qd, err := qdrant.New(qdrant.Config{
		Host:   cfg.QdrantHost,
		Port:   cfg.QdrantPort,
		UseTLS: cfg.QdrantUseTLS,
		APIKey: cfg.QdrantAPIKey,
	})
	if err != nil {
		slog.Error("rag worker: dial qdrant failed — rag:* handlers disabled",
			"host", cfg.QdrantHost, "port", cfg.QdrantPort, "err", err)
		return nil
	}
	if err := qd.EnsureCollection(ctx, qdrant.CollectionConfig{
		Name:      cfg.QdrantCollection,
		VectorDim: cfg.LLMEmbeddingDim,
		OnDisk:    true,
	}); err != nil {
		slog.Error("rag worker: ensure qdrant collection failed — rag:* handlers disabled",
			"collection", cfg.QdrantCollection, "err", err)
		return nil
	}
	embedder := embedclient.NewEmbedder(embedclient.EmbedderConfig{
		BaseURL: cfg.LLMAPIURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
		Dim:     cfg.LLMEmbeddingDim,
	})
	slog.Info("rag worker: qdrant + embedder ready",
		"host", cfg.QdrantHost, "port", cfg.QdrantPort,
		"collection", cfg.QdrantCollection,
		"vector_dim", cfg.LLMEmbeddingDim)
	return &ragtasks.Deps{
		DB:         db,
		Qdrant:     qd,
		Embedder:   embedder,
		Nango:      nangoClient,
		Spider:     spiderClient,
		Collection: cfg.QdrantCollection,
		BatchSize:  cfg.RagBatchSize,
	}
}
