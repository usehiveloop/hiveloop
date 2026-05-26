package handler

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// authConversation parses the {conversationID} URL param, loads the
// conversation (with its sandbox), and verifies the Authorization bearer
// matches the sandbox's decrypted runtime secret. On failure it writes the
// JSON error response and returns nil — callers should return immediately.
func (h *UploadsHandler) authConversation(w http.ResponseWriter, r *http.Request) (*model.EmployeeSession, bool) {
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset endpoints not configured"})
		return nil, false
	}

	convIDStr := chi.URLParam(r, "conversationID")
	convID, err := uuid.Parse(convIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid conversation_id"})
		return nil, false
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil, false
	}

	var conv model.EmployeeSession
	if err := h.db.Preload("Sandbox").Where("id = ?", convID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return nil, false
	}

	wantKey, err := h.encKey.DecryptString(conv.Sandbox.EncryptedRuntimeSecret)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt runtime secret", "conversation_id", convID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid runtime secret"})
		return nil, false
	}

	return &conv, true
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

// splitAssetPath splits a "<folder>/<filename>" tail into a sanitized folder
// (without leading or trailing slash) and a sanitized filename. Empty path,
// missing filename, traversal segments and absolute paths are rejected.
func splitAssetPath(raw string) (folder, filename string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("filename is required")
	}
	if strings.HasPrefix(raw, "/") {
		return "", "", errors.New("path must be relative")
	}
	if strings.ContainsRune(raw, 0) {
		return "", "", errors.New("invalid path")
	}

	for seg := range strings.SplitSeq(raw, "/") {
		if seg == "." || seg == ".." {
			return "", "", fmt.Errorf("invalid path segment %q", seg)
		}
	}

	dir, file := path.Split(raw)
	dir = strings.Trim(dir, "/")
	if file == "" {
		return "", "", errors.New("filename is required")
	}

	return dir, file, nil
}

// validateFolder runs the same rules as splitAssetPath but for a bare folder
// (no filename component). Empty input means "drive root" and is allowed.
func validateFolder(raw string) (string, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", nil
	}
	if strings.ContainsRune(raw, 0) {
		return "", errors.New("invalid path")
	}
	for seg := range strings.SplitSeq(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("invalid path segment %q", seg)
		}
	}
	return raw, nil
}

func buildConversationAssetKey(convID uuid.UUID, folder, filename string) string {
	if folder == "" {
		return fmt.Sprintf("pub/c/%s/%s", convID, filename)
	}
	return fmt.Sprintf("pub/c/%s/%s/%s", convID, folder, filename)
}
