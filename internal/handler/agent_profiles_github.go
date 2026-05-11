package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const githubProfileProvider = "github"

type createGitHubProfileRequest struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label,omitempty"`
}

type gitHubRepository struct {
	ID          string `json:"id"`
	NodeID      string `json:"node_id,omitempty"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url,omitempty"`
	Description string `json:"description,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

type gitHubProfileRepositoriesResponse struct {
	Profile              agentProfileResponse `json:"profile"`
	Repositories         []gitHubRepository   `json:"repositories"`
	SelectedRepositories []gitHubRepository   `json:"selected_repositories"`
}

type updateGitHubRepositoriesRequest struct {
	Repositories []gitHubRepository `json:"repositories"`
}

// @Router /v1/agents/{agentID}/profiles/github [post]
func (h *AgentProfileHandler) CreateGitHub(w http.ResponseWriter, r *http.Request) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		writeAgentProfileResolveError(w, err)
		return
	}
	if h.nango == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nango is not configured"})
		return
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	var req createGitHubProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	connectionID, err := uuid.Parse(req.ConnectionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection_id must be a valid UUID"})
		return
	}

	conn, err := h.loadGitHubConnection(orgID, user.ID, connectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "github connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load github connection"})
		return
	}

	providerConfigKey := inNangoKey(conn.InIntegration.UniqueKey)
	identity, err := h.fetchGitHubIdentity(r, conn, providerConfigKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github profile identity fetch failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "connection_id", conn.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not verify GitHub profile"})
		return
	}

	now := time.Now().UTC()
	login := stringFromJSON(identity, "login")
	externalID := login
	if externalID == "" {
		externalID = conn.NangoConnectionID
	}
	label := req.Label
	if label == "" {
		label = login
	}
	if label == "" {
		label = "GitHub Profile"
	}
	config := model.JSON{
		"in_connection_id":      conn.ID.String(),
		"nango_connection_id":   conn.NangoConnectionID,
		"provider_config_key":   providerConfigKey,
		"selected_repositories": []any{},
	}

	var profile model.AgentProfile
	err = h.db.Where(
		"agent_id = ? AND provider = ? AND deleted_at IS NULL AND revoked_at IS NULL",
		agent.ID, githubProfileProvider,
	).First(&profile).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load github profile"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		profile = model.AgentProfile{
			ID:             uuid.New(),
			OrgID:          orgID,
			AgentID:        agent.ID,
			Provider:       githubProfileProvider,
			ExternalID:     externalID,
			Label:          label,
			Identity:       identity,
			Config:         config,
			Status:         "active",
			LastVerifiedAt: &now,
		}
		if err := h.db.Create(&profile).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create github profile"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"profile": toAgentProfileResponse(profile)})
		return
	}

	profile.ExternalID = externalID
	profile.Label = label
	profile.Identity = identity
	profile.Config = mergeGitHubProfileConfig(profile.Config, config)
	profile.Status = "active"
	profile.StatusReason = ""
	profile.LastVerifiedAt = &now
	if err := h.db.Save(&profile).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update github profile"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toAgentProfileResponse(profile)})
}

// @Router /v1/agents/{agentID}/profiles/github/repositories [get]
func (h *AgentProfileHandler) ListGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	_, _, profile, conn, providerConfigKey, ok := h.resolveGitHubProfileContext(w, r)
	if !ok {
		return
	}

	repos, err := h.fetchGitHubRepositories(r, conn, providerConfigKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github repository listing failed",
			"error", err, "profile_id", profile.ID, "connection_id", conn.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not list GitHub repositories"})
		return
	}

	writeJSON(w, http.StatusOK, gitHubProfileRepositoriesResponse{
		Profile:              toAgentProfileResponse(profile),
		Repositories:         repos,
		SelectedRepositories: selectedGitHubRepositories(profile.Config),
	})
}

// @Router /v1/agents/{agentID}/profiles/github/repositories [patch]
func (h *AgentProfileHandler) UpdateGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	_, _, profile, _, _, ok := h.resolveGitHubProfileContext(w, r)
	if !ok {
		return
	}

	var req updateGitHubRepositoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.Repositories) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "select at least one repository"})
		return
	}
	for _, repo := range req.Repositories {
		if repo.ID == "" || repo.FullName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repositories must include id and full_name"})
			return
		}
	}

	cfg := model.JSON{}
	for k, v := range profile.Config {
		cfg[k] = v
	}
	cfg["selected_repositories"] = reposToJSONList(req.Repositories)
	profile.Config = cfg
	if err := h.db.Save(&profile).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save repository selection"})
		return
	}

	writeJSON(w, http.StatusOK, gitHubProfileRepositoriesResponse{
		Profile:              toAgentProfileResponse(profile),
		Repositories:         nil,
		SelectedRepositories: selectedGitHubRepositories(profile.Config),
	})
}

func (h *AgentProfileHandler) resolveGitHubProfileContext(w http.ResponseWriter, r *http.Request) (model.Agent, uuid.UUID, model.AgentProfile, model.InConnection, string, bool) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		writeAgentProfileResolveError(w, err)
		return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
	}
	if h.nango == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nango is not configured"})
		return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
	}
	var profile model.AgentProfile
	err = h.db.Where(
		"agent_id = ? AND provider = ? AND deleted_at IS NULL AND revoked_at IS NULL",
		agent.ID, githubProfileProvider,
	).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "github profile not connected"})
			return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load github profile"})
		return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
	}

	connectionID, err := uuid.Parse(stringFromJSON(profile.Config, "in_connection_id"))
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "github profile is missing its connection"})
		return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
	}
	conn, err := h.loadGitHubConnection(orgID, user.ID, connectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "github connection not found"})
			return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load github connection"})
		return model.Agent{}, uuid.Nil, model.AgentProfile{}, model.InConnection{}, "", false
	}
	return agent, orgID, profile, conn, inNangoKey(conn.InIntegration.UniqueKey), true
}

func (h *AgentProfileHandler) loadGitHubConnection(orgID uuid.UUID, userID uuid.UUID, connectionID uuid.UUID) (model.InConnection, error) {
	var conn model.InConnection
	err := h.db.Preload("InIntegration").
		Where("id = ? AND org_id = ? AND user_id = ? AND revoked_at IS NULL", connectionID, orgID, userID).
		First(&conn).Error
	if err != nil {
		return model.InConnection{}, err
	}
	if conn.InIntegration.Provider != githubProfileProvider {
		return model.InConnection{}, gorm.ErrRecordNotFound
	}
	return conn, nil
}

func (h *AgentProfileHandler) fetchGitHubIdentity(r *http.Request, conn model.InConnection, providerConfigKey string) (model.JSON, error) {
	raw, err := h.nango.RawProxyRequest(r.Context(), http.MethodGet, providerConfigKey, conn.NangoConnectionID, "/user", "", nil, "")
	if err != nil {
		return nil, err
	}
	if raw.StatusCode >= 400 {
		return nil, fmt.Errorf("github user request failed: %d: %s", raw.StatusCode, string(raw.Body))
	}
	var identity model.JSON
	if err := json.Unmarshal(raw.Body, &identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (h *AgentProfileHandler) fetchGitHubRepositories(r *http.Request, conn model.InConnection, providerConfigKey string) ([]gitHubRepository, error) {
	q := url.Values{}
	q.Set("per_page", "100")
	q.Set("sort", "updated")
	q.Set("affiliation", "owner,collaborator,organization_member")
	raw, err := h.nango.RawProxyRequest(r.Context(), http.MethodGet, providerConfigKey, conn.NangoConnectionID, "/user/repos", q.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	if raw.StatusCode >= 400 {
		return nil, fmt.Errorf("github repositories request failed: %d: %s", raw.StatusCode, string(raw.Body))
	}

	var payload []map[string]any
	if err := json.Unmarshal(raw.Body, &payload); err != nil {
		return nil, err
	}
	repos := make([]gitHubRepository, 0, len(payload))
	for _, item := range payload {
		repo := gitHubRepository{
			ID:          fmt.Sprint(item["id"]),
			NodeID:      stringFromAny(item["node_id"]),
			Name:        stringFromAny(item["name"]),
			FullName:    stringFromAny(item["full_name"]),
			Private:     boolFromAny(item["private"]),
			HTMLURL:     stringFromAny(item["html_url"]),
			Description: stringFromAny(item["description"]),
		}
		if owner, ok := item["owner"].(map[string]any); ok {
			repo.Owner = stringFromAny(owner["login"])
		}
		if repo.ID != "" && repo.FullName != "" {
			repos = append(repos, repo)
		}
	}
	return repos, nil
}

func writeAgentProfileResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAgentNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
	case err.Error() == "missing org context":
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case err.Error() == "invalid agent id" || err.Error() == "profiles can only be attached to AI employees":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
	}
}

func mergeGitHubProfileConfig(existing model.JSON, next model.JSON) model.JSON {
	out := model.JSON{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range next {
		out[k] = v
	}
	if selected := selectedGitHubRepositories(existing); len(selected) > 0 {
		out["selected_repositories"] = reposToJSONList(selected)
	}
	return out
}

func selectedGitHubRepositories(cfg model.JSON) []gitHubRepository {
	raw, ok := cfg["selected_repositories"]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var repos []gitHubRepository
	if err := json.Unmarshal(b, &repos); err != nil {
		return nil
	}
	return repos
}

func reposToJSONList(repos []gitHubRepository) []any {
	out := make([]any, 0, len(repos))
	for _, repo := range repos {
		out = append(out, map[string]any{
			"id":          repo.ID,
			"node_id":     repo.NodeID,
			"name":        repo.Name,
			"full_name":   repo.FullName,
			"private":     repo.Private,
			"html_url":    repo.HTMLURL,
			"description": repo.Description,
			"owner":       repo.Owner,
		})
	}
	return out
}

func stringFromJSON(v model.JSON, key string) string {
	if v == nil {
		return ""
	}
	return stringFromAny(v[key])
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}
