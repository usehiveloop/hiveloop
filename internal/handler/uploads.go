package handler

import (
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/storage"
)

type UploadsHandler struct {
	db        *gorm.DB
	presigner storage.Presigner
	streamer  storage.Streamer
	encKey    *crypto.SymmetricKey
}

const assetURLStorageColumn = "public_" + "url"

func NewUploadsHandler(db *gorm.DB, presigner storage.Presigner) *UploadsHandler {
	return &UploadsHandler{db: db, presigner: presigner}
}

// WithStreamer enables the agent-facing streaming upload endpoint. The
// streamer is normally the same S3Presigner that satisfies both interfaces;
// the encryption key is needed to verify the runtime bearer token.
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
	PublicURL       string            `json:"asset_url"`
	ExpiresAt       string            `json:"expires_at"`
	MaxSizeBytes    int64             `json:"max_size_bytes"`
}
