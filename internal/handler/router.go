package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// RouterHandler handles CRUD for Router, RouterTrigger, and RoutingRule.
type RouterHandler struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

func NewRouterHandler(db *gorm.DB, actionsCatalog *catalog.Catalog) *RouterHandler {
	return &RouterHandler{db: db, catalog: actionsCatalog}
}

// GetOrCreateRouter returns the org's router, creating one if it doesn't exist.
func (handler *RouterHandler) GetOrCreateRouter(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var router model.Router
	err := handler.db.Where("org_id = ?", org.ID).First(&router).Error
	if err == gorm.ErrRecordNotFound {
		router = model.Router{
			OrgID: org.ID,
			Name:  "Zira",
		}
		if createErr := handler.db.Create(&router).Error; createErr != nil {
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create router"})
			return
		}
	} else if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to load router"})
		return
	}

	writeJSON(writer, http.StatusOK, router)
}

// UpdateRouter updates the org's router (persona, default_agent, memory_team).
func (handler *RouterHandler) UpdateRouter(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body struct {
		Persona        *string `json:"persona"`
		DefaultAgentID *string `json:"default_agent_id"`
		MemoryTeam     *string `json:"memory_team"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var router model.Router
	if err := handler.db.Where("org_id = ?", org.ID).First(&router).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "router not found"})
		return
	}

	updates := map[string]any{}
	if body.Persona != nil {
		updates["persona"] = *body.Persona
	}
	if body.DefaultAgentID != nil {
		if *body.DefaultAgentID == "" {
			updates["default_agent_id"] = nil
		} else {
			parsed, err := uuid.Parse(*body.DefaultAgentID)
			if err != nil {
				writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid default_agent_id"})
				return
			}
			updates["default_agent_id"] = parsed
		}
	}
	if body.MemoryTeam != nil {
		updates["memory_team"] = *body.MemoryTeam
	}

	if len(updates) > 0 {
		handler.db.Model(&router).Updates(updates)
	}

	handler.db.Where("id = ?", router.ID).First(&router)
	writeJSON(writer, http.StatusOK, router)
}
