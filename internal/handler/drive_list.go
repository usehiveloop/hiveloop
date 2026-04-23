package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// List handles GET /v1/drive/assets.
func (handler *DriveHandler) List(writer http.ResponseWriter, request *http.Request) {
	orgID, agent, ok := handler.resolveAgentFromToken(writer, request)
	if !ok {
		return
	}

	limit, cursor, err := parsePagination(request)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	query := handler.db.Where("org_id = ? AND agent_id = ?", orgID, agent.ID)

	if contentTypeFilter := request.URL.Query().Get("content_type"); contentTypeFilter != "" {
		query = query.Where("content_type LIKE ?", contentTypeFilter+"%")
	}

	query = applyPagination(query, cursor, limit)

	var assets []model.DriveAsset
	if err := query.Find(&assets).Error; err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to list assets"})
		return
	}

	hasMore := len(assets) > limit
	if hasMore {
		assets = assets[:limit]
	}

	response := paginatedResponse[driveAssetResponse]{
		Data:    make([]driveAssetResponse, 0, len(assets)),
		HasMore: hasMore,
	}
	for _, asset := range assets {
		response.Data = append(response.Data, toDriveAssetResponse(asset))
	}
	if hasMore {
		last := assets[len(assets)-1]
		cursorStr := encodeCursor(last.CreatedAt, last.ID)
		response.NextCursor = &cursorStr
	}

	writeJSON(writer, http.StatusOK, response)
}

// Get handles GET /v1/drive/assets/{assetID}.
func (handler *DriveHandler) Get(writer http.ResponseWriter, request *http.Request) {
	orgID, agent, ok := handler.resolveAgentFromToken(writer, request)
	if !ok {
		return
	}

	assetID := chi.URLParam(request, "assetID")

	var asset model.DriveAsset
	if err := handler.db.Where("id = ? AND org_id = ? AND agent_id = ?", assetID, orgID, agent.ID).First(&asset).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to get asset"})
		return
	}

	downloadURL, err := handler.storage.PresignedURL(request.Context(), asset.S3Key, 15*time.Minute)
	if err != nil {
		slog.Error("drive presign failed", "asset_id", asset.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to generate download URL"})
		return
	}

	response := toDriveAssetResponse(asset)
	response.DownloadURL = &downloadURL

	writeJSON(writer, http.StatusOK, response)
}

// Delete handles DELETE /v1/drive/assets/{assetID}.
func (handler *DriveHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	orgID, agent, ok := handler.resolveAgentFromToken(writer, request)
	if !ok {
		return
	}

	assetID := chi.URLParam(request, "assetID")

	var asset model.DriveAsset
	if err := handler.db.Where("id = ? AND org_id = ? AND agent_id = ?", assetID, orgID, agent.ID).First(&asset).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to get asset"})
		return
	}

	if err := handler.storage.Delete(request.Context(), asset.S3Key); err != nil {
		slog.Error("drive s3 delete failed", "asset_id", asset.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to delete file from storage"})
		return
	}

	if err := handler.db.Delete(&asset).Error; err != nil {
		slog.Error("drive asset db delete failed", "asset_id", asset.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to delete asset record"})
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}
