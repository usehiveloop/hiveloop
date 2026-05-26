package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

// @Summary Create an connection
// @Description Stores a connection after the OAuth flow completes via Nango.
// @Tags connections
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body createConnectionRequest true "Connection details"
// @Success 201 {object} connectionResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id}/connections [post]
func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integUUID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req createConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.NangoConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}
	meta := req.Meta
	if meta == nil {
		meta = model.JSON{}
	}
	if integ.Provider == "bugsink" {
		nangoResp, err := h.nango.GetConnection(r.Context(), req.NangoConnectionID, nangoProviderConfigKey(integ.UniqueKey))
		if err != nil {
			logging.FromContext(r.Context()).WarnContext(r.Context(), "nango: get bugsink connection failed while enriching metadata",
				"error", err, "nango_connection_id", req.NangoConnectionID)
		} else {
			for key, value := range buildConnectionProviderConfig(nangoResp) {
				meta[key] = value
			}
		}
	}

	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		Meta:              meta,
		WebhookConfigured: boolPtr(!providerRequiresWebhookConfig(integ.Provider)),
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conn).Error; err != nil {
			return err
		}
		employee, err := ensureHivyEmployee(r.Context(), tx, org.ID)
		if err != nil {
			return err
		}
		if err := attachEmployeeRequiredSkillsForAgent(r.Context(), tx, org.ID, employee); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create connection", "error", err, "org_id", org.ID, "user_id", user.ID, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	conn.Integration = integ
	logging.FromContext(r.Context()).InfoContext(r.Context(), "connection created", "connection_id", conn.ID, "org_id", org.ID, "user_id", user.ID, "provider", integ.Provider)

	h.autoCreateRAGSourceForConnection(r.Context(), &conn, user.ID, org.ID)

	writeJSON(w, http.StatusCreated, h.toConnectionResponse(conn))
}

// autoCreateRAGSourceForConnection creates a RAG source when a
// connection's integration supports it. Failure is logged but never
// fails the connection creation — the user can always create one
// manually later.
func (h *ConnectionHandler) autoCreateRAGSourceForConnection(
	ctx context.Context,
	conn *model.Connection,
	userID uuid.UUID,
	orgID uuid.UUID,
) {
	if !conn.Integration.SupportsRAGSource {
		return
	}
	// Slack requires the user to configure which channels to index
	// before ingesting — skip auto-creation.
	if conn.Integration.Provider == "slack" {
		return
	}
	if h.enq == nil {
		return
	}

	src := &ragmodel.RAGSource{
		OrgIDValue: orgID,
		KindValue:  ragmodel.RAGSourceKindIntegration,
		Name:       conn.Integration.DisplayName,
		Status:     ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:    true,
		AccessType: ragmodel.AccessTypeSync,
		RefreshFreqSeconds:  intPtr(3600),
	}
	src.ConnectionID = &conn.ID

	if err := h.db.Create(src).Error; err != nil {
		if isDuplicateKeyError(err) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create rag source for connection %s: %w", conn.ID, err))
		return
	}

	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: src.ID})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("auto-create rag source: build ingest task for %s: %w", src.ID, err))
		return
	}
	if _, err := h.enq.Enqueue(task, asynq.Unique(60*time.Second)); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("auto-create rag source: enqueue ingest for %s: %w", src.ID, err))
	}
}
