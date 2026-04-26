package handler

import (
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/storage"
)

type UploadsHandler struct {
	db        *gorm.DB
	presigner storage.Presigner
}

func NewUploadsHandler(db *gorm.DB, presigner storage.Presigner) *UploadsHandler {
	return &UploadsHandler{db: db, presigner: presigner}
}

type signUploadRequest struct {
	AssetType   string  `json:"asset_type"`
	ContentType string  `json:"content_type"`
	SizeBytes   int64   `json:"size_bytes"`
	Filename    string  `json:"filename,omitempty"`
	OrgID       *string `json:"org_id,omitempty"`
}

type signUploadResponse struct {
	UploadURL       string            `json:"upload_url"`
	UploadMethod    string            `json:"upload_method"`
	RequiredHeaders map[string]string `json:"required_headers"`
	Key             string            `json:"key"`
	PublicURL       string            `json:"public_url"`
	ExpiresAt       string            `json:"expires_at"`
	MaxSizeBytes    int64             `json:"max_size_bytes"`
}
