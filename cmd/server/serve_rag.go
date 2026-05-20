package main

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	ragtools "github.com/usehivy/hivy/internal/rag"
	"github.com/usehivy/hivy/internal/rag/embedclient"
	"github.com/usehivy/hivy/internal/rag/qdrant"
	ragscheduler "github.com/usehivy/hivy/internal/rag/scheduler"
)

func setupRAGRuntime(
	cfg *config.Config,
	db *gorm.DB,
	enqueuer enqueue.TaskEnqueuer,
	mcpHandler *handler.MCPHandler,
) (*handler.RAGSourceHandler, *handler.RAGSearchHandler, error) {
	sourceHandler := handler.NewRAGSourceHandler(db, enqueuer, ragscheduler.HasPermSyncCapability, billing.NewCreditsService(db))
	searchHandler, qd, embedder, reranker, err := buildRAGSearch(cfg)
	if err != nil {
		return nil, nil, err
	}
	if qd != nil && embedder != nil && mcpHandler != nil {
		mcpHandler.SetKnowledgeTools(ragtools.NewKnowledgeToolsFunc(qd, embedder, reranker, cfg.QdrantCollection))
	}
	return sourceHandler, searchHandler, nil
}

func buildRAGSearch(cfg *config.Config) (*handler.RAGSearchHandler, *qdrant.Client, *embedclient.Embedder, *embedclient.Reranker, error) {
	if cfg.QdrantHost == "" || cfg.LLMAPIURL == "" || cfg.LLMAPIKey == "" || cfg.LLMModel == "" {
		slog.Warn("rag search: qdrant or LLM not configured — /v1/rag/search disabled")
		return nil, nil, nil, nil, nil
	}

	qd, err := qdrant.New(qdrant.Config{
		Host: cfg.QdrantHost, Port: cfg.QdrantPort,
		UseTLS: cfg.QdrantUseTLS, APIKey: cfg.QdrantAPIKey,
	})
	if err != nil {
		slog.Error("rag search: dial qdrant failed — /v1/rag/search disabled", "error", err)
		return nil, nil, nil, nil, err
	}

	embedder := embedclient.NewEmbedder(embedclient.EmbedderConfig{
		BaseURL: cfg.LLMAPIURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
		Dim:     cfg.LLMEmbeddingDim,
	})

	var reranker *embedclient.Reranker
	if cfg.RerankerBaseURL != "" && cfg.RerankerAPIKey != "" && cfg.RerankerModel != "" {
		reranker = embedclient.NewReranker(embedclient.RerankerConfig{
			BaseURL: cfg.RerankerBaseURL,
			APIKey:  cfg.RerankerAPIKey,
			Model:   cfg.RerankerModel,
		})
	}

	return handler.NewRAGSearchHandler(qd, embedder, reranker, cfg.QdrantCollection), qd, embedder, reranker, nil
}
