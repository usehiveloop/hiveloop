package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// DeleteConversationAsset removes both the S3 object and the DB row for
// "<folder>/<filename>" inside the authenticated conversation's drive.
//
// Auth: bearer token must equal the conversation's sandbox runtime secret.
//
//	DELETE /internal/conversations/{conversationID}/assets/*
func (h *UploadsHandler) DeleteConversationAsset(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset endpoints not configured"})
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

	key := buildConversationAssetKey(conv.ID, folder, filename)

	var asset model.ConversationAsset
	if err := h.db.Where("conversation_id = ? AND key = ?", conv.ID, key).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load asset"})
		return
	}

	if err := h.streamer.Delete(r.Context(), asset.Key); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete s3 object", "conversation_id", conv.ID, "key", asset.Key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete object"})
		return
	}
	if err := h.db.Delete(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete asset row", "conversation_id", conv.ID, "key", asset.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete asset"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type moveAssetRequest struct {
	// Asset is either the asset_url returned at upload time or a relative
	// "<folder>/<filename>" path inside the conversation drive.
	Asset string `json:"asset"`
	// NewPath is the destination folder. "" / "/" means the drive root.
	NewPath string `json:"new_path"`
}

type moveAssetResponse struct {
	ID        uuid.UUID `json:"id"`
	PublicURL string    `json:"asset_url"`
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Filename  string    `json:"filename"`
	UpdatedAt string    `json:"updated_at"`
}

// MoveConversationAsset relabels an asset's folder. Only the database
// `path` column is touched — the S3 key (and therefore the asset URL)
// stays put. Use this for organising the frontend listing without
// re-uploading multi-GB objects.
//
//	POST /internal/conversations/{conversationID}/assets/move
//	body: {"asset":"<asset_url|folder/filename>","new_path":"archive"}
func (h *UploadsHandler) MoveConversationAsset(w http.ResponseWriter, r *http.Request) {
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset endpoints not configured"})
		return
	}

	conv, ok := h.authConversation(w, r)
	if !ok {
		return
	}

	var req moveAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	folder, filename, err := resolveAssetReference(conv.ID, req.Asset)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	newFolder, err := validateFolder(req.NewPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	key := buildConversationAssetKey(conv.ID, folder, filename)
	var asset model.ConversationAsset
	if err := h.db.Where("conversation_id = ? AND key = ?", conv.ID, key).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load asset"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&asset).Updates(map[string]any{
		"path":       newFolder,
		"updated_at": now,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update asset"})
		return
	}

	writeJSON(w, http.StatusOK, moveAssetResponse{
		ID:        asset.ID,
		PublicURL: asset.PublicURL,
		Key:       asset.Key,
		Path:      newFolder,
		Filename:  asset.Filename,
		UpdatedAt: now.UTC().Format(time.RFC3339),
	})
}

// resolveAssetReference accepts either:
//   - an absolute asset URL (must contain "/pub/c/{convID}/<rest>")
//   - a relative "<folder>/<filename>" path inside the drive
//
// and returns the (folder, filename) pair so callers can rebuild the S3 key.
func resolveAssetReference(convID uuid.UUID, raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("asset is required")
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", errors.New("invalid asset URL")
		}
		marker := fmt.Sprintf("/pub/c/%s/", convID)
		idx := strings.Index(u.Path, marker)
		if idx < 0 {
			return "", "", errors.New("asset URL does not belong to this conversation")
		}
		raw = u.Path[idx+len(marker):]
	}

	return splitAssetPath(raw)
}
