package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/rag/ragclient"
	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

type RAGSearchHandler struct {
	client      *ragclient.Client
	datasetName string
}

func NewRAGSearchHandler(client *ragclient.Client, datasetName string) *RAGSearchHandler {
	return &RAGSearchHandler{client: client, datasetName: datasetName}
}

type ragSearchRequest struct {
	Query     string `json:"query"`
	Mode      string `json:"mode,omitempty"` // "hybrid" | "vector" | "bm25" (default hybrid)
	Rerank    bool   `json:"rerank,omitempty"`
	Limit     uint32 `json:"limit,omitempty"`
	BypassACL bool   `json:"bypass_acl,omitempty"`
}

func resolveSearchMode(s string) ragpb.SearchMode {
	switch s {
	case "vector":
		return ragpb.SearchMode_SEARCH_MODE_VECTOR_ONLY
	case "bm25":
		return ragpb.SearchMode_SEARCH_MODE_BM25_ONLY
	default:
		return ragpb.SearchMode_SEARCH_MODE_HYBRID
	}
}

type ragSearchHit struct {
	ChunkID      string  `json:"chunk_id"`
	DocID        string  `json:"doc_id"`
	ChunkIndex   uint32  `json:"chunk_index"`
	Score        float64 `json:"score"`
	Bm25Score    float64 `json:"bm25_score"`
	VectorScore  float64 `json:"vector_score"`
	RerankScore  float64 `json:"rerank_score"`
	Blurb        string  `json:"blurb"`
	Content      string  `json:"content"`
	DocUpdatedAt *string `json:"doc_updated_at,omitempty"`
}

type ragSearchResponse struct {
	Hits        []ragSearchHit `json:"hits"`
	AfterRerank uint32         `json:"after_rerank"`
}

// @Summary Search the knowledge base
// @Description Run a BM25 search against the org's RAG dataset, optionally reranked.
// @Tags rag
// @Accept json
// @Produce json
// @Param body body ragSearchRequest true "Search query"
// @Success 200 {object} ragSearchResponse
// @Security BearerAuth
// @Router /v1/rag/search [post]
func (h *RAGSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "rag engine not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}

	var req ragSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query is required"})
		return
	}
	limit := req.Limit
	if limit == 0 || limit > 50 {
		limit = 10
	}

	var acl []string
	if req.BypassACL {
		acl = []string{"*"}
	} else if user.Email != "" {
		acl = []string{user.Email}
	}

	resp, err := h.client.Search(r.Context(), &ragpb.SearchRequest{
		DatasetName:   h.datasetName,
		OrgId:         org.ID.String(),
		QueryText:     req.Query,
		Mode:          resolveSearchMode(req.Mode),
		AclAnyOf:      acl,
		IncludePublic: true,
		Limit:         limit,
		HybridAlpha:   0.7,
		Rerank:        req.Rerank,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
		return
	}

	out := ragSearchResponse{
		Hits:        make([]ragSearchHit, 0, len(resp.GetHits())),
		AfterRerank: resp.GetAfterRerank(),
	}
	for _, h := range resp.GetHits() {
		hit := ragSearchHit{
			ChunkID:     h.GetChunkId(),
			DocID:       h.GetDocId(),
			ChunkIndex:  h.GetChunkIndex(),
			Score:       h.GetScore(),
			Bm25Score:   h.GetBm25Score(),
			VectorScore: h.GetVectorScore(),
			RerankScore: h.GetRerankScore(),
			Blurb:       h.GetBlurb(),
			Content:     h.GetContent(),
		}
		if t := h.GetDocUpdatedAt(); t != nil {
			s := t.AsTime().Format(time.RFC3339)
			hit.DocUpdatedAt = &s
		}
		out.Hits = append(out.Hits, hit)
	}
	writeJSON(w, http.StatusOK, out)
}
