package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

// Sign issues a presigned upload URL for a public asset.
// @Summary Sign upload URL
// @Description Returns a presigned URL the client can PUT to for uploading public assets (avatars, org logos, etc).
// @Tags uploads
// @Accept json
// @Produce json
// @Param body body signUploadRequest true "Upload metadata"
// @Success 200 {object} signUploadResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Security BearerAuth
// @Router /v1/uploads/sign [post]
func (h *UploadsHandler) Sign(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}
	org, _ := middleware.OrgFromContext(r.Context())

	var req signUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	assetType := storage.AssetType(req.AssetType)
	policy, ok := h.presigner.Policy(assetType)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown asset_type"})
		return
	}
	if req.ContentType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content_type is required"})
		return
	}
	if _, ok := policy.AllowedTypes[req.ContentType]; !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "content_type not allowed for asset_type"})
		return
	}
	if req.SizeBytes <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "size_bytes must be positive"})
		return
	}
	if req.SizeBytes > policy.MaxBytes {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "size_bytes exceeds limit for asset_type"})
		return
	}

	signReq := storage.SignRequest{
		AssetType:   assetType,
		UserID:      user.ID,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		Filename:    req.Filename,
	}

	if assetType == storage.AssetTypeOrgLogo {
		orgID, err := resolveOrgLogoOrg(req.OrgID, org)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.requireOrgMembership(r, user.ID, orgID); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		signReq.OrgID = &orgID
	}

	out, err := h.presigner.Sign(r.Context(), signReq)
	if err != nil {
		slog.Warn("upload sign rejected", "user_id", user.ID, "asset_type", assetType, "error", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, signUploadResponse{
		UploadURL:       out.UploadURL,
		UploadMethod:    out.UploadMethod,
		RequiredHeaders: out.RequiredHeaders,
		Key:             out.Key,
		PublicURL:       out.PublicURL,
		ExpiresAt:       out.ExpiresAt.Format(time.RFC3339),
		MaxSizeBytes:    out.MaxSizeBytes,
	})
}

func resolveOrgLogoOrg(reqOrgID *string, ctxOrg *model.Org) (uuid.UUID, error) {
	if reqOrgID != nil && *reqOrgID != "" {
		parsed, err := uuid.Parse(*reqOrgID)
		if err != nil {
			return uuid.Nil, errors.New("invalid org_id")
		}
		return parsed, nil
	}
	if ctxOrg != nil {
		return ctxOrg.ID, nil
	}
	return uuid.Nil, errors.New("org_id is required for org_logo")
}

// requireOrgMembership asserts the user belongs to the target org. Any
// authenticated member may upload org-scoped public assets — we don't
// gate on owner/admin role. Cross-org probes are still rejected.
func (h *UploadsHandler) requireOrgMembership(r *http.Request, userID, orgID uuid.UUID) error {
	db := dbFromHandler(h)
	if db == nil {
		return errors.New("not a member of the requested organization")
	}
	var m model.OrgMembership
	if err := db.Where("user_id = ? AND org_id = ?", userID, orgID).First(&m).Error; err != nil {
		return errors.New("not a member of the requested organization")
	}
	return nil
}

func dbFromHandler(h *UploadsHandler) *gorm.DB {
	if h.db != nil {
		return h.db
	}
	return nil
}
