package handler

import (
	"crypto/subtle"
	"fmt"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type streamAssetResponse struct {
	ID          uuid.UUID `json:"id"`
	PublicURL   string    `json:"public_url"`
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
	if h.streamer == nil || h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "streaming uploads not configured"})
		return
	}

	convIDStr := chi.URLParam(r, "conversationID")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid conversation_id"})
		return
	}

	rest := chi.URLParam(r, "*")
	folder, filename, perr := splitAssetPath(rest)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	var conv model.AgentConversation
	if err := h.db.Preload("Sandbox").Where("id = ?", convID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return
	}

	wantKey, err := h.encKey.DecryptString(conv.Sandbox.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt bridge api key", "conversation_id", convID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge api key"})
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

	key := buildConversationAssetKey(convID, folder, filename)

	stored, err := h.streamer.Stream(r.Context(), key, contentType, r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "conversation asset stream failed", "conversation_id", convID, "key", key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}

	asset := model.ConversationAsset{
		ConversationID: convID,
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
		"conversation_id": convID,
		"org_id":          conv.OrgID,
		"sandbox_id":      conv.SandboxID,
		"path":            folder,
		"filename":        filename,
		"public_url":      stored.PublicURL,
		"content_type":    contentType,
		"bytes":           stored.Bytes,
		"updated_at":      time.Now(),
	}).FirstOrCreate(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "persist conversation asset", "conversation_id", convID, "key", stored.Key, "error", err)
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

func bearerFromHeader(h string) string {
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// splitAssetPath splits the wildcard URL tail into a sanitized folder
// (without leading or trailing slash) and a sanitized filename. Empty path,
// missing filename, traversal segments and absolute paths are rejected.
func splitAssetPath(raw string) (folder, filename string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("filename is required")
	}
	if strings.HasPrefix(raw, "/") {
		return "", "", fmt.Errorf("path must be relative")
	}
	if strings.ContainsRune(raw, 0) {
		return "", "", fmt.Errorf("invalid path")
	}

	// Reject raw `.` / `..` segments before any normalisation so traversal
	// attempts fail loudly instead of silently collapsing.
	for _, seg := range strings.Split(raw, "/") {
		if seg == "." || seg == ".." {
			return "", "", fmt.Errorf("invalid path segment %q", seg)
		}
	}

	dir, file := path.Split(raw)
	dir = strings.Trim(dir, "/")
	if file == "" {
		return "", "", fmt.Errorf("filename is required")
	}

	return dir, file, nil
}

func buildConversationAssetKey(convID uuid.UUID, folder, filename string) string {
	if folder == "" {
		return fmt.Sprintf("pub/c/%s/%s", convID, filename)
	}
	return fmt.Sprintf("pub/c/%s/%s/%s", convID, folder, filename)
}
