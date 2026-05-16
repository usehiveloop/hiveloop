package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	githubprofile "github.com/usehiveloop/hiveloop/internal/profiles/github"
)

const githubProfileProvider = githubprofile.Provider

type createGitHubProfileRequest struct {
	ConnectionID string `json:"connection_id"`
	Label        string `json:"label,omitempty"`
}

type gitHubRepository struct {
	ID          string     `json:"id"`
	NodeID      string     `json:"node_id,omitempty"`
	Name        string     `json:"name"`
	FullName    string     `json:"full_name"`
	Private     bool       `json:"private"`
	HTMLURL     string     `json:"html_url,omitempty"`
	Description string     `json:"description,omitempty"`
	Owner       string     `json:"owner,omitempty"`
	Permissions model.JSON `json:"permissions,omitempty"`
}

type gitHubProfileRepositoriesResponse struct {
	Profile              agentProfileResponse `json:"profile"`
	Repositories         []gitHubRepository   `json:"repositories"`
	SelectedRepositories []gitHubRepository   `json:"selected_repositories"`
}

type createGitHubProfileResponse struct {
	Profile agentProfileResponse `json:"profile"`
}

type updateGitHubRepositoriesRequest struct {
	Repositories []gitHubRepository `json:"repositories"`
}

type gitHubRepositoryPermissionCheck struct {
	Repository        string `json:"repository"`
	CanRead           bool   `json:"can_read"`
	CanWrite          bool   `json:"can_write"`
	CanManageWebhooks bool   `json:"can_manage_webhooks"`
	Message           string `json:"message,omitempty"`
}

type gitHubRepositoryPermissionErrorResponse struct {
	Error    string                            `json:"error"`
	Code     string                            `json:"code"`
	Checks   []gitHubRepositoryPermissionCheck `json:"checks"`
	Required []string                          `json:"required_permissions"`
}

// @Summary Attach a GitHub profile to an AI employee
// @Description Verifies an org GitHub connection through Nango and stores it as the employee's single GitHub profile.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param body body createGitHubProfileRequest true "GitHub connection"
// @Success 201 {object} createGitHubProfileResponse
// @Success 200 {object} createGitHubProfileResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
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

	conn, err := h.loadGitHubConnection(orgID, connectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "github connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load github connection"})
		return
	}

	resp, status, ok := h.createGitHubProfileFromConnection(w, r, agent, orgID, conn, req.Label)
	if !ok {
		return
	}
	writeJSON(w, status, createGitHubProfileResponse{Profile: resp})
}

func (h *AgentProfileHandler) createGitHubProfileFromConnection(w http.ResponseWriter, r *http.Request, agent model.Agent, orgID uuid.UUID, conn model.InConnection, requestedLabel string) (agentProfileResponse, int, bool) {
	providerConfigKey := inNangoKey(conn.InIntegration.UniqueKey)
	identity, err := h.fetchGitHubIdentity(r.Context(), conn, providerConfigKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github profile identity fetch failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "connection_id", conn.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not verify GitHub profile"})
		return agentProfileResponse{}, 0, false
	}
	encryptedIdentity, err := githubprofile.EncryptIdentity(h.encKey, identity)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github profile identity encryption failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "connection_id", conn.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not encrypt GitHub profile identity"})
		return agentProfileResponse{}, 0, false
	}

	now := time.Now().UTC()
	login := stringFromJSON(identity, "login")
	externalID := login
	if externalID == "" {
		externalID = conn.NangoConnectionID
	}
	label := requestedLabel
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
	if conn.InIntegration.CustomApp {
		config["custom_app_integration_id"] = conn.InIntegration.ID.String()
	}

	var profile model.AgentProfile
	err = h.db.Where(
		"agent_id = ? AND provider = ? AND deleted_at IS NULL AND revoked_at IS NULL",
		agent.ID, githubProfileProvider,
	).First(&profile).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load github profile"})
		return agentProfileResponse{}, 0, false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		profile = model.AgentProfile{
			ID:                uuid.New(),
			OrgID:             orgID,
			AgentID:           agent.ID,
			Provider:          githubProfileProvider,
			ExternalID:        externalID,
			Label:             label,
			Identity:          model.JSON{},
			EncryptedIdentity: encryptedIdentity,
			Config:            config,
			Status:            "active",
			LastVerifiedAt:    &now,
		}
		if err := h.db.Create(&profile).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create github profile"})
			return agentProfileResponse{}, 0, false
		}
		resp, err := h.toAgentProfileResponse(profile)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read github profile identity"})
			return agentProfileResponse{}, 0, false
		}
		return resp, http.StatusCreated, true
	}

	profile.ExternalID = externalID
	profile.Label = label
	profile.Identity = model.JSON{}
	profile.EncryptedIdentity = encryptedIdentity
	profile.Config = mergeGitHubProfileConfig(profile.Config, config)
	profile.Status = "active"
	profile.StatusReason = ""
	profile.LastVerifiedAt = &now
	if err := h.db.Save(&profile).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update github profile"})
		return agentProfileResponse{}, 0, false
	}
	resp, err := h.toAgentProfileResponse(profile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read github profile identity"})
		return agentProfileResponse{}, 0, false
	}
	return resp, http.StatusOK, true
}

// @Summary List repositories for an employee GitHub profile
// @Description Lists repositories visible to the employee's attached GitHub profile and returns any selected repositories.
// @Tags agent-profiles
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Success 200 {object} gitHubProfileRepositoriesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
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

	profileResp, err := h.toAgentProfileResponse(profile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read github profile identity"})
		return
	}
	writeJSON(w, http.StatusOK, gitHubProfileRepositoriesResponse{
		Profile:              profileResp,
		Repositories:         repos,
		SelectedRepositories: selectedGitHubRepositories(profile.Config),
	})
}

// @Summary Update selected repositories for an employee GitHub profile
// @Description Stores the repositories this employee may access from its attached GitHub profile.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param body body updateGitHubRepositoriesRequest true "Selected repositories"
// @Success 200 {object} gitHubProfileRepositoriesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/github/repositories [patch]
func (h *AgentProfileHandler) UpdateGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	agent, _, profile, conn, providerConfigKey, ok := h.resolveGitHubProfileContext(w, r)
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
		if _, _, err := splitGitHubRepoFullName(repo.FullName); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	permissionChecks, err := h.checkGitHubRepositoryPermissions(r.Context(), conn, providerConfigKey, req.Repositories)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github repository permission check failed",
			"error", err, "profile_id", profile.ID, "agent_id", agent.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not verify GitHub repository permissions"})
		return
	}
	if failed := failedGitHubRepositoryPermissionChecks(permissionChecks); len(failed) > 0 {
		writeJSON(w, http.StatusBadRequest, gitHubRepositoryPermissionErrorResponse{
			Error:    "connected GitHub account does not have the required repository permissions",
			Code:     "github_repository_permissions_missing",
			Checks:   failed,
			Required: []string{"repository read access", "repository write access", "repository webhook/admin access"},
		})
		return
	}

	cfg := model.JSON{}
	for k, v := range profile.Config {
		cfg[k] = v
	}
	webhookURL, err := h.githubEmployeeWebhookURL(agent.ID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github webhook url setup failed",
			"error", err, "agent_id", agent.ID, "profile_id", profile.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "github webhook setup is not configured"})
		return
	}
	webhookSecret, err := h.ensureGitHubWebhookSecret(r.Context(), &profile)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "github webhook secret setup failed",
			"error", err, "profile_id", profile.ID, "agent_id", agent.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare GitHub webhook secret"})
		return
	}

	webhooks := githubWebhookMetadataByRepo(cfg)
	for _, repo := range req.Repositories {
		if githubWebhookMetadataMatches(webhooks[repo.FullName], webhookURL) {
			continue
		}
		meta, err := h.ensureGitHubRepositoryWebhook(r.Context(), conn, providerConfigKey, repo, webhookURL, webhookSecret)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "github repository webhook setup failed",
				"error", err, "profile_id", profile.ID, "agent_id", agent.ID, "repository", repo.FullName)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("failed to create GitHub webhook for %s after permission checks passed", repo.FullName),
			})
			return
		}
		webhooks[repo.FullName] = meta
		cfg[githubWebhookConfigKey] = githubWebhookMetadataToJSON(webhooks)
		profile.Config = cfg
		if err := h.db.Model(&profile).Update("config", profile.Config).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save GitHub webhook metadata"})
			return
		}
	}
	cfg["selected_repositories"] = reposToJSONList(req.Repositories)
	profile.Config = cfg
	if err := h.db.Save(&profile).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save repository selection"})
		return
	}

	profileResp, err := h.toAgentProfileResponse(profile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read github profile identity"})
		return
	}
	writeJSON(w, http.StatusOK, gitHubProfileRepositoriesResponse{
		Profile:              profileResp,
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
	conn, err := h.loadGitHubConnection(orgID, connectionID)
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

func (h *AgentProfileHandler) loadGitHubConnection(orgID uuid.UUID, connectionID uuid.UUID) (model.InConnection, error) {
	var conn model.InConnection
	err := h.db.Preload("InIntegration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, orgID).
		First(&conn).Error
	if err != nil {
		return model.InConnection{}, err
	}
	if conn.InIntegration.Provider != githubProfileProvider {
		return model.InConnection{}, gorm.ErrRecordNotFound
	}
	return conn, nil
}

func (h *AgentProfileHandler) fetchGitHubIdentity(ctx context.Context, conn model.InConnection, providerConfigKey string) (model.JSON, error) {
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodGet, providerConfigKey, conn.NangoConnectionID, "/user", "", nil, "")
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
	email, err := h.fetchGitHubVerifiedEmail(ctx, conn, providerConfigKey)
	if err != nil {
		return nil, err
	}
	identity["email"] = email
	identity["email_verified"] = true
	return identity, nil
}

type gitHubEmail struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

func (h *AgentProfileHandler) fetchGitHubVerifiedEmail(ctx context.Context, conn model.InConnection, providerConfigKey string) (string, error) {
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodGet, providerConfigKey, conn.NangoConnectionID, "/user/emails", "", nil, "")
	if err != nil {
		return "", err
	}
	if raw.StatusCode >= 400 {
		return "", fmt.Errorf("github user emails request failed: %d: %s", raw.StatusCode, string(raw.Body))
	}
	var emails []gitHubEmail
	if err := json.Unmarshal(raw.Body, &emails); err != nil {
		return "", err
	}
	for _, email := range emails {
		if email.Verified && strings.TrimSpace(email.Email) != "" {
			return strings.TrimSpace(email.Email), nil
		}
	}
	return "", fmt.Errorf("no verified email found on github account")
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
		if permissions, ok := jsonObjectFromAny(item["permissions"]); ok {
			repo.Permissions = permissions
		}
		if repo.ID != "" && repo.FullName != "" {
			repos = append(repos, repo)
		}
	}
	return repos, nil
}

func (h *AgentProfileHandler) checkGitHubRepositoryPermissions(ctx context.Context, conn model.InConnection, providerConfigKey string, repos []gitHubRepository) ([]gitHubRepositoryPermissionCheck, error) {
	checks := make([]gitHubRepositoryPermissionCheck, 0, len(repos))
	for _, repo := range repos {
		check, err := h.checkGitHubRepositoryPermission(ctx, conn, providerConfigKey, repo)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, nil
}

func (h *AgentProfileHandler) checkGitHubRepositoryPermission(ctx context.Context, conn model.InConnection, providerConfigKey string, repo gitHubRepository) (gitHubRepositoryPermissionCheck, error) {
	check := gitHubRepositoryPermissionCheck{Repository: repo.FullName}
	owner, name, err := splitGitHubRepoFullName(repo.FullName)
	if err != nil {
		check.Message = err.Error()
		return check, nil
	}
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodGet, providerConfigKey, conn.NangoConnectionID, githubRepoPath(owner, name), "", nil, "")
	if err != nil {
		return check, err
	}
	if raw.StatusCode == http.StatusNotFound || raw.StatusCode == http.StatusForbidden || raw.StatusCode == http.StatusUnauthorized {
		check.Message = "The connected GitHub account cannot access this repository."
		return check, nil
	}
	if raw.StatusCode >= 400 {
		return check, fmt.Errorf("github repository permission request failed: %d: %s", raw.StatusCode, string(raw.Body))
	}

	var payload map[string]any
	if err := json.Unmarshal(raw.Body, &payload); err != nil {
		return check, fmt.Errorf("decode github repository permission response: %w", err)
	}
	check.CanRead = true
	permissions, ok := jsonObjectFromAny(payload["permissions"])
	if !ok {
		check.Message = "GitHub did not return repository permissions for the connected account."
		return check, nil
	}
	admin := boolFromAny(permissions["admin"])
	maintain := boolFromAny(permissions["maintain"])
	push := boolFromAny(permissions["push"])
	check.CanWrite = admin || maintain || push
	if admin {
		canManageWebhooks, message, err := h.checkGitHubRepositoryWebhookPermission(ctx, conn, providerConfigKey, owner, name)
		if err != nil {
			return check, err
		}
		check.CanManageWebhooks = canManageWebhooks
		check.Message = message
	}
	if !check.CanWrite && !check.CanManageWebhooks {
		check.Message = "The connected GitHub account needs write access and admin access to manage webhooks."
	} else if !check.CanWrite {
		check.Message = "The connected GitHub account needs write access to this repository."
	} else if !check.CanManageWebhooks {
		if check.Message == "" {
			check.Message = "The connected GitHub account needs admin access to create and update repository webhooks."
		}
	}
	return check, nil
}

func (h *AgentProfileHandler) checkGitHubRepositoryWebhookPermission(ctx context.Context, conn model.InConnection, providerConfigKey string, owner string, repo string) (bool, string, error) {
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodGet, providerConfigKey, conn.NangoConnectionID, githubRepoHooksPath(owner, repo), "", nil, "")
	if err != nil {
		return false, "", err
	}
	if raw.StatusCode == http.StatusUnauthorized || raw.StatusCode == http.StatusForbidden || raw.StatusCode == http.StatusNotFound {
		return false, "The connected GitHub account or OAuth grant cannot manage repository webhooks. Reconnect GitHub with an admin account and make sure webhook permissions are granted.", nil
	}
	if raw.StatusCode >= 400 {
		return false, "", fmt.Errorf("github repository webhook permission request failed: %d: %s", raw.StatusCode, string(raw.Body))
	}
	return true, "", nil
}

func failedGitHubRepositoryPermissionChecks(checks []gitHubRepositoryPermissionCheck) []gitHubRepositoryPermissionCheck {
	failed := make([]gitHubRepositoryPermissionCheck, 0)
	for _, check := range checks {
		if !check.CanRead || !check.CanWrite || !check.CanManageWebhooks {
			failed = append(failed, check)
		}
	}
	return failed
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
	if stringFromJSON(existing, "in_connection_id") == stringFromJSON(next, "in_connection_id") {
		if selected := selectedGitHubRepositories(existing); len(selected) > 0 {
			out["selected_repositories"] = reposToJSONList(selected)
		}
	}
	if _, ok := out["selected_repositories"]; !ok {
		out["selected_repositories"] = []any{}
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
		item := map[string]any{
			"id":          repo.ID,
			"node_id":     repo.NodeID,
			"name":        repo.Name,
			"full_name":   repo.FullName,
			"private":     repo.Private,
			"html_url":    repo.HTMLURL,
			"description": repo.Description,
			"owner":       repo.Owner,
		}
		if len(repo.Permissions) > 0 {
			item["permissions"] = repo.Permissions
		}
		out = append(out, item)
	}
	return out
}

func jsonObjectFromAny(v any) (model.JSON, bool) {
	if v == nil {
		return nil, false
	}
	if obj, ok := v.(map[string]any); ok {
		return model.JSON(obj), true
	}
	if obj, ok := v.(model.JSON); ok {
		return obj, true
	}
	return nil, false
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
