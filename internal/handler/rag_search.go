package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

type RAGSearchHandler struct {
	qd         *qdrant.Client
	embedder   *embedclient.Embedder
	reranker   *embedclient.Reranker
	collection string
}

func NewRAGSearchHandler(qd *qdrant.Client, embedder *embedclient.Embedder,
	reranker *embedclient.Reranker, collection string) *RAGSearchHandler {
	return &RAGSearchHandler{qd: qd, embedder: embedder, reranker: reranker, collection: collection}
}

type ragSearchRequest struct {
	Query     string `json:"query"`
	Rerank    bool   `json:"rerank,omitempty"`
	Limit     uint32 `json:"limit,omitempty"`
	BypassACL bool   `json:"bypass_acl,omitempty"`
}

type ragSearchHit struct {
	ID          string  `json:"id"`
	DocID       string  `json:"doc_id"`
	Score       float64 `json:"score"`
	RerankScore float64 `json:"rerank_score,omitempty"`
	Title       string  `json:"title,omitempty"`
	Link        string  `json:"link,omitempty"`
	Blurb       string  `json:"blurb,omitempty"`
	Content     string  `json:"content,omitempty"`
}

type ragSearchResponse struct {
	Hits []ragSearchHit `json:"hits"`
}

const candidatePool uint32 = 50

// @Summary Search the knowledge base
// @Description Hybrid retrieval against the org's RAG dataset, optionally reranked.
// @Tags rag
// @Accept json
// @Produce json
// @Param body body ragSearchRequest true "Search query"
// @Success 200 {object} ragSearchResponse
// @Security BearerAuth
// @Router /v1/rag/search [post]
func (h *RAGSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	if h.qd == nil || h.embedder == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "rag search not configured"})
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
	if strings.TrimSpace(req.Query) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query is required"})
		return
	}
	limit := req.Limit
	if limit == 0 || limit > 50 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	vectors, err := h.embedder.Embed(ctx, []string{req.Query})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "embed query: " + err.Error()})
		return
	}

	acl := []string{}
	if user.Email != "" {
		acl = append(acl, user.Email)
	}
	filter := qdrant.BuildACLFilter(org.ID.String(), acl, req.BypassACL)

	topK := limit
	if req.Rerank && h.reranker != nil {
		topK = candidatePool
	}
	hits, err := h.qd.Search(ctx, qdrant.SearchRequest{
		Collection:  h.collection,
		Vector:      vectors[0],
		Filter:      filter,
		Limit:       topK,
		WithPayload: true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "qdrant search: " + err.Error()})
		return
	}

	out := ragSearchResponse{Hits: hitsToResponse(hits)}
	if req.Rerank && h.reranker != nil && len(hits) > 0 {
		reranked, err := rerankHits(ctx, h.reranker, req.Query, out.Hits, int(limit))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "rerank: " + err.Error()})
			return
		}
		out.Hits = reranked
	} else if uint32(len(out.Hits)) > limit {
		out.Hits = out.Hits[:limit]
	}
	writeJSON(w, http.StatusOK, out)
}

func hitsToResponse(hits []qdrant.Hit) []ragSearchHit {
	out := make([]ragSearchHit, 0, len(hits))
	for _, h := range hits {
		hit := ragSearchHit{Score: h.Score}
		if id, ok := h.ID.(string); ok {
			hit.ID = id
		}
		if h.Payload != nil {
			if v, ok := h.Payload["doc_id"].(string); ok {
				hit.DocID = v
			}
			if v, ok := h.Payload["semantic_id"].(string); ok {
				hit.Title = v
			}
			if v, ok := h.Payload["link"].(string); ok {
				hit.Link = v
			}
			if v, ok := h.Payload["content"].(string); ok {
				hit.Content = v
				if len(v) > 200 {
					hit.Blurb = v[:200]
				} else {
					hit.Blurb = v
				}
			}
		}
		out = append(out, hit)
	}
	return out
}

func rerankHits(ctx context.Context, rer *embedclient.Reranker, query string,
	hits []ragSearchHit, topN int) ([]ragSearchHit, error) {
	docs := make([]string, len(hits))
	for i := range hits {
		c := hits[i].Content
		if len(c) > 1500 {
			c = c[:1500]
		}
		docs[i] = c
	}
	results, err := rer.Rerank(ctx, query, docs, topN)
	if err != nil {
		return nil, err
	}
	out := make([]ragSearchHit, 0, len(results))
	for _, r := range results {
		if r.Index < 0 || r.Index >= len(hits) {
			continue
		}
		hit := hits[r.Index]
		hit.RerankScore = r.Score
		out = append(out, hit)
	}
	return out, nil
}
