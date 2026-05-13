package handler

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type streamAssetResponse struct {
	ID          uuid.UUID `json:"id"`
	PublicURL   string    `json:"asset_url"`
	Key         string    `json:"key"`
	Path        string    `json:"path"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Bytes       int64     `json:"bytes"`
}

// StreamConversationAsset accepts a streamed PUT body (any size, any type)
// and stores it under the conversation's drive. The wildcard URL segment is
// "<folder>/<filename>" — trailing path components after /assets/.
//
// Auth: bearer token must equal the conversation's sandbox bridge API key.
//
//	PUT /internal/conversations/{conversationID}/assets/*
func (h *UploadsHandler) StreamConversationAsset(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "streaming uploads not configured"})
		return
	}

	folder, filename, perr := splitAssetPath(chi.URLParam(r, "*"))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}

	conv, ok := h.authConversation(w, r)
	if !ok {
		return
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		if guessed := mime.TypeByExtension(filepath.Ext(filename)); guessed != "" {
			contentType = guessed
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	key := buildConversationAssetKey(conv.ID, folder, filename)

	stored, err := h.streamer.Stream(r.Context(), key, contentType, r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "conversation asset stream failed", "conversation_id", conv.ID, "key", key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}

	asset := model.ConversationAsset{
		ConversationID: conv.ID,
		OrgID:          conv.OrgID,
		SandboxID:      conv.SandboxID,
		Path:           folder,
		Filename:       filename,
		Key:            stored.Key,
		PublicURL:      stored.PublicURL,
		ContentType:    contentType,
		Bytes:          stored.Bytes,
	}
	if err := h.db.Where("key = ?", stored.Key).Assign(map[string]any{
		"conversation_id":     conv.ID,
		"org_id":              conv.OrgID,
		"sandbox_id":          conv.SandboxID,
		"path":                folder,
		"filename":            filename,
		assetURLStorageColumn: stored.PublicURL,
		"content_type":        contentType,
		"bytes":               stored.Bytes,
		"updated_at":          time.Now(),
	}).FirstOrCreate(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "persist conversation asset", "conversation_id", conv.ID, "key", stored.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save asset"})
		return
	}

	writeJSON(w, http.StatusCreated, streamAssetResponse{
		ID:          asset.ID,
		PublicURL:   stored.PublicURL,
		Key:         stored.Key,
		Path:        folder,
		Filename:    filename,
		ContentType: contentType,
		Bytes:       stored.Bytes,
	})
}
