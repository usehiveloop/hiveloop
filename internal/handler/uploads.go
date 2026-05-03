package handler

import (
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

type UploadsHandler struct {
	db        *gorm.DB
	presigner storage.Presigner
	streamer  storage.Streamer
	encKey    *crypto.SymmetricKey
}

func NewUploadsHandler(db *gorm.DB, presigner storage.Presigner) *UploadsHandler {
	return &UploadsHandler{db: db, presigner: presigner}
}

// WithStreamer enables the agent-facing streaming upload endpoint. The
// streamer is normally the same S3Presigner that satisfies both interfaces;
// the encryption key is needed to verify the bridge bearer token.
func (h *UploadsHandler) WithStreamer(s storage.Streamer, encKey *crypto.SymmetricKey) *UploadsHandler {
	h.streamer = s
	h.encKey = encKey
	return h
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
