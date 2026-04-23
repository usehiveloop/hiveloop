package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *SubagentHandler) loadVisibleSubagent(id string, orgID uuid.UUID) (*model.Agent, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var agent model.Agent
	err = h.db.
		Where("id = ? AND agent_type = ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
			parsed, model.AgentTypeSubagent, orgID, "active").
		First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (h *SubagentHandler) loadOwnSubagent(id string, orgID uuid.UUID) (*model.Agent, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var agent model.Agent
	err = h.db.
		Where("id = ? AND agent_type = ? AND org_id = ?", parsed, model.AgentTypeSubagent, orgID).
		First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func writeSubagentLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
}
