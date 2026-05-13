package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	hivecrypto "github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type GitHubEmployeeWebhookHandler struct {
	db  *gorm.DB
	kms *hivecrypto.KeyWrapper
}

func NewGitHubEmployeeWebhookHandler(db *gorm.DB, kms *hivecrypto.KeyWrapper) *GitHubEmployeeWebhookHandler {
	return &GitHubEmployeeWebhookHandler{db: db, kms: kms}
}

// Handle processes POST /internal/webhooks/github/employees/{agentID}.
func (h *GitHubEmployeeWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty body"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND is_employee = ?", agentID, true).First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		return
	}
	var profile model.AgentProfile
	if err := h.db.Where(
		"agent_id = ? AND provider = ? AND status = ? AND deleted_at IS NULL AND revoked_at IS NULL",
		agentID, githubProfileProvider, "active",
	).First(&profile).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "github profile not found"})
		return
	}

	secrets, err := decryptGitHubProfileSecrets(r.Context(), h.kms, profile)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github employee webhook: failed to decrypt profile secret",
			"error", err, "agent_id", agentID, "profile_id", profile.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load webhook secret"})
		return
	}
	secret := stringFromAny(secrets[githubWebhookSecretKey])
	if secret == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "github webhook secret not found"})
		return
	}
	if !verifyGitHubWebhookSignature(body, secret, r.Header.Get("X-Hub-Signature-256")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"})
		return
	}

	var probe struct {
		Action     string `json:"action"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}

	orgID := ""
	if agent.OrgID != nil {
		orgID = agent.OrgID.String()
	}
	logging.FromContext(r.Context()).InfoContext(r.Context(), "github employee webhook received",
		"org_id", orgID,
		"agent_id", agentID,
		"profile_id", profile.ID,
		"github_event", r.Header.Get("X-GitHub-Event"),
		"github_delivery", r.Header.Get("X-GitHub-Delivery"),
		"action", probe.Action,
		"repository", probe.Repository.FullName,
		"payload_bytes", len(body),
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func verifyGitHubWebhookSignature(body []byte, secret string, signature string) bool {
	signature = strings.TrimSpace(signature)
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
