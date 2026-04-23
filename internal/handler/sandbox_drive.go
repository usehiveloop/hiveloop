package handler

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

const maxSandboxDriveUploadBytes = 50 << 20 // 50 MiB

const maxSandboxDriveFileBytes = 50 << 20 // 50 MiB

// SandboxDriveHandler provides drive access from within sandboxes,
// authenticated with the Bridge control plane API key.
//
// Routes:
//
//	POST   /internal/sandbox-drive/{sandboxID}/assets
//	GET    /internal/sandbox-drive/{sandboxID}/assets
//	GET    /internal/sandbox-drive/{sandboxID}/assets/{assetID}
//	DELETE /internal/sandbox-drive/{sandboxID}/assets/{assetID}
type SandboxDriveHandler struct {
	db      *gorm.DB
	storage *storage.S3Client
	encKey  *crypto.SymmetricKey
}

func NewSandboxDriveHandler(db *gorm.DB, store *storage.S3Client, encKey *crypto.SymmetricKey) *SandboxDriveHandler {
	return &SandboxDriveHandler{db: db, storage: store, encKey: encKey}
}

// resolveSandboxAgent validates the Bridge API key and resolves the agent
// assigned to the sandbox.
func (handler *SandboxDriveHandler) resolveSandboxAgent(writer http.ResponseWriter, request *http.Request) (uuid.UUID, *model.Agent, bool) {
	sandboxID := chi.URLParam(request, "sandboxID")
	if sandboxID == "" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "sandbox ID is required"})
		return uuid.Nil, nil, false
	}

	apiKey := ""
	if auth := request.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			apiKey = parts[1]
		}
	}
	if apiKey == "" {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return uuid.Nil, nil, false
	}

	var sandbox model.Sandbox
	if err := handler.db.Where("id = ? AND status IN ('running','starting')", sandboxID).First(&sandbox).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return uuid.Nil, nil, false
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return uuid.Nil, nil, false
	}

	decryptedKey, err := handler.encKey.Decrypt(sandbox.EncryptedBridgeAPIKey)
	if err != nil {
		slog.Error("failed to decrypt bridge API key", "sandbox_id", sandboxID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return uuid.Nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(decryptedKey), []byte(apiKey)) != 1 {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "invalid API key"})
		return uuid.Nil, nil, false
	}

	var agent model.Agent
	if err := handler.db.Where("sandbox_id = ? AND deleted_at IS NULL", sandbox.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "no agent assigned to this sandbox"})
			return uuid.Nil, nil, false
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to find agent"})
		return uuid.Nil, nil, false
	}

	if agent.OrgID == nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "agent has no org"})
		return uuid.Nil, nil, false
	}

	return *agent.OrgID, &agent, true
}

// Upload handles POST /internal/sandbox-drive/{sandboxID}/assets.
func (handler *SandboxDriveHandler) Upload(writer http.ResponseWriter, request *http.Request) {
	orgID, agent, ok := handler.resolveSandboxAgent(writer, request)
	if !ok {
		return
	}

	// Cap the entire request body before parsing so oversized uploads are
	// rejected instead of spilled to a temp file on disk.
	request.Body = http.MaxBytesReader(writer, request.Body, maxSandboxDriveUploadBytes)

	if err := request.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid multipart form or payload too large"})
		return
	}

	files := request.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "no files provided (use form field 'files')"})
		return
	}

	var assets []driveAssetResponse

	for _, fileHeader := range files {
		if fileHeader.Size > maxSandboxDriveFileBytes {
			writeJSON(writer, http.StatusRequestEntityTooLarge, map[string]string{
				"error": fmt.Sprintf("file %q exceeds the %d MiB per-file limit", fileHeader.Filename, maxSandboxDriveFileBytes>>20),
			})
			return
		}

		contentType := fileHeader.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = mime.TypeByExtension(fileHeader.Filename)
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}

		if !isAllowedDriveContentType(contentType, fileHeader.Filename) {
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
			slog.Error("sandbox drive upload failed", "agent_id", agent.ID, "filename", fileHeader.Filename, "error", err)
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to upload file to storage"})
			return
		}
		file.Close()

		asset := model.DriveAsset{
			ID:          assetID,
			OrgID:       orgID,
			AgentID:     agent.ID,
			Filename:    fileHeader.Filename,
			ContentType: contentType,
			Size:        fileHeader.Size,
			S3Key:       s3Key,
		}
		if err := handler.db.Create(&asset).Error; err != nil {
			_ = handler.storage.Delete(request.Context(), s3Key)
			slog.Error("sandbox drive asset db insert failed", "agent_id", agent.ID, "error", err)
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to save asset record"})
			return
		}

		assets = append(assets, toDriveAssetResponse(asset))
	}

	writeJSON(writer, http.StatusCreated, map[string]any{"data": assets})
}

// List handles GET /internal/sandbox-drive/{sandboxID}/assets.
func (handler *SandboxDriveHandler) List(writer http.ResponseWriter, request *http.Request) {
	_, agent, ok := handler.resolveSandboxAgent(writer, request)
	if !ok {
		return
	}

	var assets []model.DriveAsset
	if err := handler.db.Where("agent_id = ?", agent.ID).Order("created_at DESC").Limit(100).Find(&assets).Error; err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to list assets"})
		return
	}

	response := make([]driveAssetResponse, 0, len(assets))
	for _, asset := range assets {
		response = append(response, toDriveAssetResponse(asset))
	}

	writeJSON(writer, http.StatusOK, map[string]any{"assets": response})
}

// Get handles GET /internal/sandbox-drive/{sandboxID}/assets/{assetID}.
func (handler *SandboxDriveHandler) Get(writer http.ResponseWriter, request *http.Request) {
	_, agent, ok := handler.resolveSandboxAgent(writer, request)
	if !ok {
		return
	}

	assetID := chi.URLParam(request, "assetID")

	var asset model.DriveAsset
	if err := handler.db.Where("id = ? AND agent_id = ?", assetID, agent.ID).First(&asset).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to get asset"})
		return
	}

	downloadURL, err := handler.storage.PresignedURL(request.Context(), asset.S3Key, 15*time.Minute)
	if err != nil {
		slog.Error("sandbox drive presign failed", "asset_id", asset.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to generate download URL"})
		return
	}

	response := toDriveAssetResponse(asset)
	response.DownloadURL = &downloadURL

	writeJSON(writer, http.StatusOK, response)
}

// Delete handles DELETE /internal/sandbox-drive/{sandboxID}/assets/{assetID}.
func (handler *SandboxDriveHandler) Delete(writer http.ResponseWriter, request *http.Request) {
	_, agent, ok := handler.resolveSandboxAgent(writer, request)
	if !ok {
		return
	}

	assetID := chi.URLParam(request, "assetID")

	var asset model.DriveAsset
	if err := handler.db.Where("id = ? AND agent_id = ?", assetID, agent.ID).First(&asset).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to get asset"})
		return
	}

	if err := handler.storage.Delete(request.Context(), asset.S3Key); err != nil {
		slog.Error("sandbox drive s3 delete failed", "asset_id", asset.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to delete file from storage"})
		return
	}

	if err := handler.db.Delete(&asset).Error; err != nil {
		slog.Error("sandbox drive asset db delete failed", "asset_id", asset.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to delete asset record"})
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}
