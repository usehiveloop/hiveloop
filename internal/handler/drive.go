package handler

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/storage"
)

// DriveHandler handles agent drive asset CRUD.
type DriveHandler struct {
	db      *gorm.DB
	storage *storage.S3Client
}

// NewDriveHandler creates a new DriveHandler.
func NewDriveHandler(db *gorm.DB, storage *storage.S3Client) *DriveHandler {
	return &DriveHandler{db: db, storage: storage}
}

// allowedContentTypePrefixes defines MIME type prefixes accepted for upload.
var allowedContentTypePrefixes = []string{
	"image/",
	"video/",
	"audio/",
	"text/",
	"application/pdf",
	"application/vnd.openxmlformats-officedocument.",
	"application/msword",
	"application/vnd.ms-",
}

func isAllowedContentType(contentType string) bool {
	for _, prefix := range allowedContentTypePrefixes {
		if strings.HasPrefix(contentType, prefix) {
			return true
		}
	}
	return false
}

// driveAssetResponse is the JSON response for a single drive asset.
type driveAssetResponse struct {
	ID          string  `json:"id"`
	AgentID     string  `json:"agent_id"`
	Filename    string  `json:"filename"`
	ContentType string  `json:"content_type"`
	Size        int64   `json:"size"`
	DownloadURL *string `json:"download_url,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func toDriveAssetResponse(asset model.DriveAsset) driveAssetResponse {
	return driveAssetResponse{
		ID:          asset.ID.String(),
		AgentID:     asset.AgentID.String(),
		Filename:    asset.Filename,
		ContentType: asset.ContentType,
		Size:        asset.Size,
		CreatedAt:   asset.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   asset.UpdatedAt.Format(time.RFC3339),
	}
}

// Upload handles POST /v1/agents/{id}/drive/assets.
func (handler *DriveHandler) Upload(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(request, "id")
	var agent model.Agent
	if err := handler.db.Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", agentID, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to find agent"})
		return
	}

	// 32 MB max memory for multipart parsing; larger files spill to temp disk.
	if err := request.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	files := request.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "no files provided (use form field 'files')"})
		return
	}

	var assets []driveAssetResponse

	for _, fileHeader := range files {
		// Determine content type from header or detect from extension.
		contentType := fileHeader.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = mime.TypeByExtension(fileHeader.Filename)
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}

		if !isAllowedContentType(contentType) {
			writeJSON(writer, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("file %q: content type %q is not allowed", fileHeader.Filename, contentType),
			})
			return
		}

		assetID := uuid.New()
		s3Key := fmt.Sprintf("drives/%s/%s/%s", agent.ID, assetID, fileHeader.Filename)

		file, err := fileHeader.Open()
		if err != nil {
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to read uploaded file"})
			return
		}

		if err := handler.storage.Upload(request.Context(), s3Key, file, contentType, fileHeader.Size); err != nil {
			file.Close()
			slog.Error("drive upload failed", "agent_id", agent.ID, "filename", fileHeader.Filename, "error", err)
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to upload file to storage"})
			return
		}
		file.Close()

		asset := model.DriveAsset{
			ID:          assetID,
			OrgID:       org.ID,
			AgentID:     agent.ID,
			Filename:    fileHeader.Filename,
			ContentType: contentType,
			Size:        fileHeader.Size,
			S3Key:       s3Key,
		}
		if err := handler.db.Create(&asset).Error; err != nil {
			// Best-effort cleanup of the S3 object if DB insert fails.
			_ = handler.storage.Delete(request.Context(), s3Key)
			slog.Error("drive asset db insert failed", "agent_id", agent.ID, "error", err)
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to save asset record"})
			return
		}

		assets = append(assets, toDriveAssetResponse(asset))
	}

	writeJSON(writer, http.StatusCreated, map[string]any{"data": assets})
}

// List handles GET /v1/agents/{id}/drive/assets.
func (handler *DriveHandler) List(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(request, "id")
	var agent model.Agent
	if err := handler.db.Select("id").Where("id = ? AND org_id = ? AND is_system = false AND deleted_at IS NULL", agentID, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to find agent"})
		return
	}

	limit, cursor, err := parsePagination(request)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	query := handler.db.Where("org_id = ? AND agent_id = ?", org.ID, agent.ID)

	// Optional content_type prefix filter (e.g. ?content_type=image).
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

// Get handles GET /v1/agents/{id}/drive/assets/{assetID}.
func (handler *DriveHandler) Get(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(request, "id")
	assetID := chi.URLParam(request, "assetID")

	var asset model.DriveAsset
	if err := handler.db.Where("id = ? AND org_id = ? AND agent_id = ?", assetID, org.ID, agentID).First(&asset).Error; err != nil {
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

// Delete handles DELETE /v1/agents/{id}/drive/assets/{assetID}.
func (handler *DriveHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(request, "id")
	assetID := chi.URLParam(request, "assetID")

	var asset model.DriveAsset
	if err := handler.db.Where("id = ? AND org_id = ? AND agent_id = ?", assetID, org.ID, agentID).First(&asset).Error; err != nil {
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
