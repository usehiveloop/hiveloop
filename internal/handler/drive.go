package handler

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

// DriveHandler handles agent drive asset CRUD.
type DriveHandler struct {
	db      *gorm.DB
	storage *storage.S3Client
}

func NewDriveHandler(db *gorm.DB, storage *storage.S3Client) *DriveHandler {
	return &DriveHandler{db: db, storage: storage}
}

const maxDriveUploadBytes = 50 << 20 // 50 MiB

const maxDriveFileBytes = 50 << 20 // 50 MiB

// "text/" is intentionally NOT a prefix — only specific safe text subtypes
// are allowed via allowedExactContentTypes, so text/html (and similar
// browser-executable types) cannot be stored and served back inline via
// presigned URLs.
var allowedContentTypePrefixes = []string{
	"image/",
	"video/",
	"audio/",
	"application/vnd.openxmlformats-officedocument.",
	"application/vnd.oasis.opendocument.",
	"application/vnd.ms-",
}

var allowedExactContentTypes = map[string]struct{}{
	"application/pdf":  {},
	"application/json": {},
	"application/msword": {},
	"application/zip":  {},
	"text/plain":       {},
	"text/csv":         {},
	"text/markdown":    {},
	"text/x-markdown":  {},
}

// disallowedImageSubtypes are image/* types that can execute script in
// browsers (SVG) and so must be rejected even though they match image/*.
var disallowedImageSubtypes = map[string]struct{}{
	"image/svg+xml": {},
	"image/svg":     {},
}

// dangerousExtensions are filename suffixes we refuse regardless of the
// client-declared content type, to block executable uploads that sneak in
// under generic types like application/octet-stream.
var dangerousExtensions = []string{
	".html", ".htm", ".xhtml", ".svg",
	".exe", ".dll", ".bat", ".cmd", ".com", ".msi",
	".sh", ".bash", ".zsh", ".ps1",
	".js", ".mjs", ".jsp", ".php", ".phtml",
}

func isAllowedDriveContentType(contentType, filename string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if semi := strings.Index(ct, ";"); semi >= 0 {
		ct = strings.TrimSpace(ct[:semi])
	}
	if ct == "" {
		return false
	}

	lowerName := strings.ToLower(filename)
	for _, ext := range dangerousExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return false
		}
	}

	if _, bad := disallowedImageSubtypes[ct]; bad {
		return false
	}

	for _, prefix := range allowedContentTypePrefixes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	if _, ok := allowedExactContentTypes[ct]; ok {
		return true
	}
	return false
}

// isAllowedContentType is kept for backward compatibility.
func isAllowedContentType(contentType string) bool {
	return isAllowedDriveContentType(contentType, "")
}

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

func (handler *DriveHandler) resolveAgentFromToken(writer http.ResponseWriter, request *http.Request) (uuid.UUID, *model.Agent, bool) {
	claims, ok := middleware.ClaimsFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing token claims"})
		return uuid.Nil, nil, false
	}

	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "invalid org in token"})
		return uuid.Nil, nil, false
	}

	var tokenRecord model.Token
	if err := handler.db.Select("meta").Where("jti = ?", claims.JTI).First(&tokenRecord).Error; err != nil {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "token not found"})
		return uuid.Nil, nil, false
	}

	agentIDStr, ok := tokenRecord.Meta["agent_id"].(string)
	if !ok || agentIDStr == "" {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "token is not scoped to an agent"})
		return uuid.Nil, nil, false
	}

	var agent model.Agent
	if err := handler.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", agentIDStr, orgID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return uuid.Nil, nil, false
		}
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to find agent"})
		return uuid.Nil, nil, false
	}

	return orgID, &agent, true
}

// Upload handles POST /v1/drive/assets.
func (handler *DriveHandler) Upload(writer http.ResponseWriter, request *http.Request) {
	orgID, agent, ok := handler.resolveAgentFromToken(writer, request)
	if !ok {
		return
	}

	// Cap the entire request body before parsing so oversized uploads are
	// rejected instead of spilled to a temp file on disk.
	request.Body = http.MaxBytesReader(writer, request.Body, maxDriveUploadBytes)

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
		if fileHeader.Size > maxDriveFileBytes {
			writeJSON(writer, http.StatusRequestEntityTooLarge, map[string]string{
				"error": fmt.Sprintf("file %q exceeds the %d MiB per-file limit", fileHeader.Filename, maxDriveFileBytes>>20),
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
			slog.Error("drive upload failed", "agent_id", agent.ID, "filename", fileHeader.Filename, "error", err)
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
			slog.Error("drive asset db insert failed", "agent_id", agent.ID, "error", err)
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to save asset record"})
			return
		}

		assets = append(assets, toDriveAssetResponse(asset))
	}

	writeJSON(writer, http.StatusCreated, map[string]any{"data": assets})
}
